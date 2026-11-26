package service

import (
	"context"
	"errors"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"app/model"
)

// postCreator is the store a post is created in.
type postCreator interface {
	GetUser(ctx context.Context, id model.UserID) (model.User, error)
	CreatePost(ctx context.Context, p model.Post) (model.Post, error)
}

// CreatePost wires [Fat.CreatePost] to the given store and clock.
//
// Panics if the operation is already wired.
func CreatePost(f *Fat, db postCreator, now func() time.Time) {
	if f.createPost != nil {
		panic("service: CreatePost already wired")
	}

	f.createPost = func(ctx context.Context, actor model.UserID, body string) (model.Post, error) {
		if err := checkActive(ctx, db, actor); err != nil {
			return model.Post{}, err
		}

		body, err := model.CleanPostBody(body)
		if err != nil {
			return model.Post{}, err
		}

		p, err := db.CreatePost(ctx, model.Post{Created: model.Time{T: now()}, AuthorID: actor, Body: body})
		if err != nil {
			return model.Post{}, err
		}

		trace.SpanFromContext(ctx).SetAttributes(attribute.String("post.id", p.ID.String()))
		f.log.InfoContext(ctx, "Created post", "postID", p.ID, "authorID", actor)

		return p, nil
	}
}

// CreatePost by the acting user, who must be active, with the given body, cleaned by [model.CleanPostBody].
// It is posted now, by the clock the operation was wired with.
//
// Panics unless the operation was wired, by [Setup] or by the function of the same name.
func (f *Fat) CreatePost(ctx context.Context, actor model.UserID, body string) (model.Post, error) {
	if f.createPost == nil {
		panic("service: CreatePost not wired; call service.CreatePost or service.Setup")
	}

	ctx, span := f.tracer.Start(ctx, "service.CreatePost")
	defer span.End()

	p, err := f.createPost(ctx, actor, body)
	recordError(span, err)
	return p, err
}

// commentCreator is the store a comment is created in.
type commentCreator interface {
	GetUser(ctx context.Context, id model.UserID) (model.User, error)
	CreateComment(ctx context.Context, c model.Comment) (model.Comment, error)
}

// CreateComment wires [Fat.CreateComment] to the given store and clock.
//
// Panics if the operation is already wired.
func CreateComment(f *Fat, db commentCreator, now func() time.Time) {
	if f.createComment != nil {
		panic("service: CreateComment already wired")
	}

	f.createComment = func(ctx context.Context, actor model.UserID, postID model.PostID, body string) (model.Comment, error) {
		if err := checkActive(ctx, db, actor); err != nil {
			return model.Comment{}, err
		}

		body, err := model.CleanCommentBody(body)
		if err != nil {
			return model.Comment{}, err
		}

		c, err := db.CreateComment(ctx, model.Comment{Created: model.Time{T: now()}, PostID: postID, AuthorID: actor, Body: body})
		if err != nil {
			return model.Comment{}, err
		}

		trace.SpanFromContext(ctx).SetAttributes(attribute.String("post.id", postID.String()), attribute.String("comment.id", c.ID.String()))
		f.log.InfoContext(ctx, "Created comment", "commentID", c.ID, "postID", postID, "authorID", actor)

		return c, nil
	}
}

// CreateComment by the acting user, who must be active, on the given post, with the given body, cleaned by
// [model.CleanCommentBody]. It is written now, by the clock the operation was wired with.
// Returns [model.ErrorPostNotFound] if there is no such post.
//
// Panics unless the operation was wired, by [Setup] or by the function of the same name.
func (f *Fat) CreateComment(ctx context.Context, actor model.UserID, postID model.PostID, body string) (model.Comment, error) {
	if f.createComment == nil {
		panic("service: CreateComment not wired; call service.CreateComment or service.Setup")
	}

	ctx, span := f.tracer.Start(ctx, "service.CreateComment")
	defer span.End()

	c, err := f.createComment(ctx, actor, postID, body)
	recordError(span, err)
	return c, err
}

// timelineGetter is the store the timeline is read from.
type timelineGetter interface {
	GetUser(ctx context.Context, id model.UserID) (model.User, error)
	GetTimeline(ctx context.Context, reader model.UserID, after *model.TimelineCursor, limit int) ([]model.TimelinePost, *model.TimelineCursor, error)
}

// GetTimeline wires [Fat.GetTimeline] to the given store and clock, grouping into days in the given
// group's time zone.
//
// Panics if the operation is already wired.
func GetTimeline(f *Fat, db timelineGetter, now func() time.Time, group model.Group) {
	if f.getTimeline != nil {
		panic("service: GetTimeline already wired")
	}

	f.getTimeline = func(ctx context.Context, actor model.UserID, cursor string, limit int) (model.Timeline, error) {
		if err := checkActive(ctx, db, actor); err != nil {
			return model.Timeline{}, err
		}

		var after *model.TimelineCursor
		if cursor != "" {
			c, err := model.ParseTimelineCursor(cursor)
			if err != nil {
				return model.Timeline{}, err
			}
			after = &c
		}

		if limit < 1 {
			limit = 20
		}
		limit = min(limit, 100)

		posts, next, err := db.GetTimeline(ctx, actor, after, limit)
		if err != nil {
			return model.Timeline{}, err
		}

		timeline := model.Timeline{Days: model.GroupByDay(posts, group.TimeZone, now())}
		if next != nil {
			timeline.Next = next.String()
		}
		return timeline, nil
	}
}

// GetTimeline page for the acting user, who must be active, grouped into days in the group's time zone,
// and labelled today and yesterday by the clock the operation was wired with.
//
// An empty cursor is the first page, and anything else is the [model.Timeline.Next] of a page before.
// A cursor that is not is [model.ErrorCursorInvalid]. The limit of posts on the page is 20 if it is below
// 1, and at most 100.
//
// Each post is marked with what is new to the acting user, and to nobody else. Getting the first page is a
// look at the timeline: its marks are computed against the user's previous look, and then this look is
// recorded. The pages after it keep the marks of the first page. See [model.TimelinePost] for the marks.
//
// Panics unless the operation was wired, by [Setup] or by the function of the same name.
func (f *Fat) GetTimeline(ctx context.Context, actor model.UserID, cursor string, limit int) (model.Timeline, error) {
	if f.getTimeline == nil {
		panic("service: GetTimeline not wired; call service.GetTimeline or service.Setup")
	}

	ctx, span := f.tracer.Start(ctx, "service.GetTimeline")
	defer span.End()

	timeline, err := f.getTimeline(ctx, actor, cursor, limit)
	recordError(span, err)
	return timeline, err
}

// commentsGetter is the store comments are read from.
type commentsGetter interface {
	GetUser(ctx context.Context, id model.UserID) (model.User, error)
	GetComments(ctx context.Context, reader model.UserID, postID model.PostID) ([]model.TimelineComment, error)
}

// GetComments wires [Fat.GetComments] to the given store.
//
// Panics if the operation is already wired.
func GetComments(f *Fat, db commentsGetter) {
	if f.getComments != nil {
		panic("service: GetComments already wired")
	}

	f.getComments = func(ctx context.Context, actor model.UserID, postID model.PostID) ([]model.TimelineComment, error) {
		if err := checkActive(ctx, db, actor); err != nil {
			return nil, err
		}

		// The post is not recorded on the span, since who opened which comments is read state.
		return db.GetComments(ctx, actor, postID)
	}
}

// GetComments of the given post for the acting user, who must be active, oldest first, and record that
// the user opened them. Each comment is marked with whether it was new to the acting user, and to nobody
// else, until this opening. Returns [model.ErrorPostNotFound] if there is no such post.
//
// Panics unless the operation was wired, by [Setup] or by the function of the same name.
func (f *Fat) GetComments(ctx context.Context, actor model.UserID, postID model.PostID) ([]model.TimelineComment, error) {
	if f.getComments == nil {
		panic("service: GetComments not wired; call service.GetComments or service.Setup")
	}

	ctx, span := f.tracer.Start(ctx, "service.GetComments")
	defer span.End()

	comments, err := f.getComments(ctx, actor, postID)
	recordError(span, err)
	return comments, err
}

// checkActive that the acting user exists and is active, or return [model.ErrorUserNotFound] or
// [model.ErrorUserInactive].
func checkActive(ctx context.Context, db userGetter, actor model.UserID) error {
	u, err := db.GetUser(ctx, actor)
	if err != nil {
		return err
	}
	if !u.Active {
		return model.ErrorUserInactive
	}
	return nil
}

// recordError on the span, if there is one and it is not a refusal from the business domain, such as an
// empty body or an inactive user.
func recordError(span trace.Span, err error) {
	var domainErr model.Error
	if err == nil || errors.As(err, &domainErr) {
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}
