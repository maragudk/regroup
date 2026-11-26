package http

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	. "maragu.dev/gomponents"
	"maragu.dev/httph"

	"app/html"
	"app/model"
)

// timelineService is what the timeline's routes need from the business logic.
type timelineService interface {
	CreateComment(ctx context.Context, actor model.UserID, postID model.PostID, body string) (model.Comment, error)
	CreatePost(ctx context.Context, actor model.UserID, body string) (model.Post, error)
	GetComments(ctx context.Context, actor model.UserID, postID model.PostID) ([]model.TimelineComment, error)
	GetMembers(ctx context.Context, actor model.UserID) (int, error)
	GetTimeline(ctx context.Context, actor model.UserID, cursor string, limit int) (model.Timeline, error)
}

// Timeline registers the group page and what happens on it:
//
//   - GET / is the group page, and counts as the person's look at the timeline. Someone who is not signed
//     in gets a page that says only that the group is private.
//   - POST /posts creates a post.
//   - POST /posts/earlier gets the posts after a cursor.
//   - POST /comments/open gets the comments of a post, and records that the person opened them.
//   - POST /comments creates a comment on a post.
//
// All but the first answer Datastar with patches to the page the request came from. Reading goes by
// POST too, with the post or the cursor in the body, so that what someone read is not in a URL, and so
// in no trace or log. A request without Datastar, from a browser that does not run the page's script,
// is answered with a redirect to the group page or a page with a message.
//
// Someone who is not signed in, or not active, gets nothing of the group from any of them.
func Timeline(r *Router, log *slog.Logger, svc timelineService, opts InjectHTTPRouterOptions) {
	group := opts.Group

	extra := func(signedIn bool) Node {
		if opts.PreviewLink == nil {
			return nil
		}
		return opts.PreviewLink(signedIn)
	}

	// private is the answer to someone who is not signed in.
	private := func(props html.PageProps, code int) (Node, error) {
		page := html.PrivatePage(html.PrivatePageProps{PageProps: props, Extra: extra(false)})
		if code == http.StatusOK {
			return page, nil
		}
		return page, httph.HTTPError{Code: code}
	}

	// failed is the answer when the business logic returned an error that is not about what was written.
	failed := func(props html.PageProps, err error) (Node, error) {
		switch {
		case errors.Is(err, model.ErrorUserInactive), errors.Is(err, model.ErrorUserNotFound):
			return private(props, http.StatusUnauthorized)
		case errors.Is(err, model.ErrorPostNotFound):
			return html.NotFoundPage(props), httph.HTTPError{Code: http.StatusNotFound}
		case errors.Is(err, model.ErrorCursorInvalid):
			return badRequest(props)
		default:
			log.ErrorContext(props.Ctx, "Error on the timeline", "error", err)
			return html.ErrorPage(props), err
		}
	}

	r.Get("/", func(props html.PageProps) (Node, error) {
		user := getUserFromContext(props.Ctx)
		if user == nil {
			return private(props, http.StatusOK)
		}

		members, err := svc.GetMembers(props.Ctx, user.ID)
		if err != nil {
			if errors.Is(err, model.ErrorUserInactive) || errors.Is(err, model.ErrorUserNotFound) {
				return private(props, http.StatusOK)
			}
			return failed(props, err)
		}

		// The look at the timeline comes last, so that nothing after it can fail and leave the person
		// with the marks used up and no page to see them on.
		timeline, err := svc.GetTimeline(props.Ctx, user.ID, "", 0)
		if err != nil {
			if errors.Is(err, model.ErrorUserInactive) || errors.Is(err, model.ErrorUserNotFound) {
				return private(props, http.StatusOK)
			}
			return failed(props, err)
		}

		return html.GroupPage(html.GroupPageProps{
			PageProps: props,
			GroupName: group.Name,
			Members:   members,
			UserName:  user.Name,
			Now:       opts.Now().In(group.TimeZone),
			Timeline:  timeline,
			Extra:     extra(true),
		}), nil
	})

	r.Post("/posts", func(props html.PageProps) (Node, error) {
		user := getUserFromContext(props.Ctx)
		if user == nil {
			return private(props, http.StatusUnauthorized)
		}

		tooLarge, err := parseForm(props)
		if err != nil {
			return badRequest(props)
		}
		body := props.R.PostFormValue("body")
		newest := props.R.PostFormValue("newest")

		var post model.Post
		err = model.ErrorBodyTooLong
		if !tooLarge {
			post, err = svc.CreatePost(props.Ctx, user.ID, body)
		}
		if message := refusal(err, "post", model.PostBodyMaxLength); message != "" {
			if !isDatastar(props.R) {
				return refused(props, "That was not posted", message)
			}
			return nil, writePatches(props.W, patch{elements: html.Composer(html.ComposerProps{
				Body:    body,
				Message: message,
				Newest:  newest,
				Fresh:   props.R.PostFormValue("field"),
			})})
		}
		if err != nil {
			return failed(props, err)
		}

		if !isDatastar(props.R) {
			http.Redirect(props.W, props.R, "/", http.StatusSeeOther)
			return nil, nil
		}

		now := opts.Now().In(group.TimeZone)
		day := model.GroupByDay([]model.TimelinePost{{Post: post, AuthorName: user.Name}}, group.TimeZone, now)[0]

		// The post goes under the heading of its day if the person's page has one, which their page told
		// us with the post. Otherwise it brings its day's heading along.
		timeline := patch{selector: "#" + html.TimelineID, mode: "prepend", elements: html.TimelineDay(day, now)}
		switch newest {
		case day.Date.String():
			timeline = patch{selector: "#" + html.DayPostsID(day.Date), mode: "prepend", elements: html.TimelinePosts(day.Posts)}
		case "":
			// Their timeline was empty, and said so. The post takes the place of that.
			timeline.mode = "inner"
		}

		return nil, writePatches(props.W,
			patch{elements: html.Composer(html.ComposerProps{Newest: day.Date.String(), Fresh: post.ID.String()})},
			timeline,
		)
	})

	r.Post("/posts/earlier", func(props html.PageProps) (Node, error) {
		user := getUserFromContext(props.Ctx)
		if user == nil {
			return private(props, http.StatusUnauthorized)
		}

		if tooLarge, err := parseForm(props); err != nil || tooLarge {
			return badRequest(props)
		}

		// An empty cursor would be the first page, and getting that counts as a look at the timeline.
		after, err := model.ParseTimelineCursor(props.R.PostFormValue("after"))
		if err != nil {
			return badRequest(props)
		}

		if !isDatastar(props.R) {
			http.Redirect(props.W, props.R, "/", http.StatusSeeOther)
			return nil, nil
		}

		timeline, err := svc.GetTimeline(props.Ctx, user.ID, after.String(), 0)
		if err != nil {
			return failed(props, err)
		}

		now := opts.Now().In(group.TimeZone)
		var patches []patch

		// The page before ended on the day of its last post, which is where the cursor points. If this
		// page goes on with the same day, its posts go under the heading that is already there.
		last := model.DateOf(after.Created.T.In(group.TimeZone))
		var days []model.TimelineDay
		for _, day := range timeline.Days {
			if day.Date == last {
				patches = append(patches, patch{selector: "#" + html.DayPostsID(day.Date), mode: "append", elements: html.TimelinePosts(day.Posts)})
				continue
			}
			days = append(days, day)
		}
		if len(days) > 0 {
			patches = append(patches, patch{selector: "#" + html.TimelineID, mode: "append", elements: Map(days, func(day model.TimelineDay) Node {
				return html.TimelineDay(day, now)
			})})
		}
		patches = append(patches, patch{elements: html.EarlierPosts(timeline.Next)})

		return nil, writePatches(props.W, patches...)
	})

	// comments of the given post as patches: the list with its form, and the post's count of comments,
	// which no longer has anything new in it for the person.
	comments := func(props html.PageProps, user *model.User, postID model.PostID, form html.CommentFormProps) (Node, error) {
		comments, err := svc.GetComments(props.Ctx, user.ID, postID)
		if err != nil {
			return failed(props, err)
		}
		for i := range comments {
			comments[i].Created.T = comments[i].Created.T.In(group.TimeZone)
		}

		return nil, writePatches(props.W, patch{elements: Group{
			html.CommentsSummary(postID, len(comments), 0),
			html.Comments(html.CommentsProps{
				PostID:   postID,
				Comments: comments,
				Now:      opts.Now().In(group.TimeZone),
				Form:     form,
			}),
		}})
	}

	r.Post("/comments/open", func(props html.PageProps) (Node, error) {
		user := getUserFromContext(props.Ctx)
		if user == nil {
			return private(props, http.StatusUnauthorized)
		}

		if tooLarge, err := parseForm(props); err != nil || tooLarge {
			return badRequest(props)
		}

		if !isDatastar(props.R) {
			http.Redirect(props.W, props.R, "/", http.StatusSeeOther)
			return nil, nil
		}

		return comments(props, user, model.PostID(props.R.PostFormValue("post")), html.CommentFormProps{})
	})

	r.Post("/comments", func(props html.PageProps) (Node, error) {
		user := getUserFromContext(props.Ctx)
		if user == nil {
			return private(props, http.StatusUnauthorized)
		}

		tooLarge, err := parseForm(props)
		if err != nil {
			return badRequest(props)
		}
		postID := model.PostID(props.R.PostFormValue("post"))
		body := props.R.PostFormValue("body")

		var comment model.Comment
		err = model.ErrorBodyTooLong
		if !tooLarge {
			comment, err = svc.CreateComment(props.Ctx, user.ID, postID, body)
		}
		if message := refusal(err, "comment", model.CommentBodyMaxLength); message != "" {
			if !isDatastar(props.R) {
				return refused(props, "That comment was not added", message)
			}
			return nil, writePatches(props.W, patch{elements: html.CommentForm(html.CommentFormProps{
				PostID:  postID,
				Body:    body,
				Message: message,
				Fresh:   props.R.PostFormValue("field"),
			})})
		}
		if err != nil {
			return failed(props, err)
		}

		if !isDatastar(props.R) {
			http.Redirect(props.W, props.R, "/", http.StatusSeeOther)
			return nil, nil
		}

		return comments(props, user, postID, html.CommentFormProps{Fresh: comment.ID.String()})
	})
}

// parseForm of the request, reading no more of it than the longest post could take up. It reports
// whether the request was larger than that.
func parseForm(props html.PageProps) (tooLarge bool, err error) {
	// A character is at most four bytes, and each byte at most three characters in a form.
	props.R.Body = http.MaxBytesReader(props.W, props.R.Body, 12*model.PostBodyMaxLength+4096)
	if err := props.R.ParseForm(); err != nil {
		var tooLargeErr *http.MaxBytesError
		if errors.As(err, &tooLargeErr) {
			return true, nil
		}
		return false, err
	}
	return false, nil
}

// refusal in plain words, for a body that was refused as the given kind of thing with the given
// maximum length. It is empty if the error is not a refusal of the body, or is nil.
func refusal(err error, kind string, maxLength int) string {
	switch {
	case errors.Is(err, model.ErrorBodyEmpty):
		return "Write something first."
	case errors.Is(err, model.ErrorBodyTooLong):
		return fmt.Sprintf("That is too long. A %v can be up to %d,%03d characters.", kind, maxLength/1000, maxLength%1000)
	case errors.Is(err, model.ErrorBodyInvalid):
		return "That text has characters that cannot be used here. Remove them and try again."
	default:
		return ""
	}
}

// refused is the answer to a browser that does not run the page's script, when what it sent was refused.
func refused(props html.PageProps, heading, message string) (Node, error) {
	page := html.MessagePage(html.MessagePageProps{PageProps: props, Heading: heading, Message: message + " Go back to try again."})
	return page, httph.HTTPError{Code: http.StatusUnprocessableEntity}
}

// badRequest is the answer to a request that the page could not have sent.
func badRequest(props html.PageProps) (Node, error) {
	page := html.MessagePage(html.MessagePageProps{PageProps: props, Heading: "That did not work", Message: "The request was not one this page understands."})
	return page, httph.HTTPError{Code: http.StatusBadRequest}
}
