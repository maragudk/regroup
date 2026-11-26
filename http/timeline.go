package http

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	. "maragu.dev/gomponents"
	"maragu.dev/httph"

	"app/html"
	"app/model"
)

// timelineService is what the timeline's routes need from the business logic.
type timelineService interface {
	CreateComment(ctx context.Context, actor model.UserID, postID model.PostID, token, body string) (model.Comment, error)
	CreatePost(ctx context.Context, actor model.UserID, token, body string) (model.Post, error)
	GetComments(ctx context.Context, actor model.UserID, postID model.PostID) ([]model.TimelineComment, error)
	GetMembers(ctx context.Context, actor model.UserID) (int, error)
	GetTimeline(ctx context.Context, actor model.UserID, cursor string, limit int) (model.Timeline, error)
}

// timeline registers the group page and what happens on it:
//
//   - GET / is the group page, and counts as the person's look at the timeline. Someone who is not signed
//     in gets a page that says only that the group is private.
//   - POST /posts creates a post.
//   - POST /posts/earlier gets the posts after a cursor.
//   - POST /comments/open gets the comments of a post, and records that the person opened them.
//   - POST /comments creates a comment on a post.
//
// Each form that makes a post or a comment carries a token of its own, and every form shown gets a new
// one. A form sent again with the same token makes nothing, and is answered as the first time. If it
// was sent with other text this time, the form comes back with that text in it, and a message that it
// was not posted, or with 409 without Datastar. A form without a token, or with one that is not one, is
// answered with 400.
//
// All but the first answer Datastar with patches to the page the request came from, and none of them
// counts as a look. Reading goes by POST too, with the post or the cursor in the body, so that what
// someone read is not in a URL.
//
// A request without Datastar comes from a browser that does not run the page's script. For that, a post
// or a comment is still made, and answered with a redirect to the group page, or with a page that says
// why not. Nothing is read for it, since there is no page to show it in place on.
//
// Someone who is not signed in, or not active, gets nothing of the group from any of them.
func timeline(r *Router, log *slog.Logger, svc timelineService, opts InjectHTTPRouterOptions) {
	group := opts.Group

	if opts.Random == nil {
		panic("http: no randomness for form tokens in InjectHTTPRouterOptions.Random")
	}

	// token for a form that makes something. Every form shown gets a new one.
	var randomLock sync.Mutex
	token := func() string {
		randomLock.Lock()
		defer randomLock.Unlock()

		t, err := model.NewFormToken(opts.Random)
		if err != nil {
			panic("error reading randomness for a form token: " + err.Error())
		}
		return t.String()
	}
	posts := html.PostsProps{Token: token}

	now := func() time.Time {
		return opts.Now().In(group.TimeZone)
	}

	extra := func(signedIn bool) Node {
		if opts.Extra == nil {
			return nil
		}
		return opts.Extra(signedIn)
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
	// Someone who stopped being active since the request began is not signed in, and gets the given code.
	failed := func(props html.PageProps, err error, signedOutCode int) (Node, error) {
		switch {
		case errors.Is(err, model.ErrorUserInactive), errors.Is(err, model.ErrorUserNotFound):
			return private(props, signedOutCode)
		case errors.Is(err, model.ErrorPostNotFound):
			return html.NotFoundPage(props), httph.HTTPError{Code: http.StatusNotFound}
		case errors.Is(err, model.ErrorFormTokenInvalid):
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
			return failed(props, err, http.StatusOK)
		}

		// The look at the timeline comes last, so that nothing after it can fail and leave the person
		// with the marks used up and no page to see them on.
		timeline, err := svc.GetTimeline(props.Ctx, user.ID, "", opts.TimelinePageSize)
		if err != nil {
			return failed(props, err, http.StatusOK)
		}

		var days []model.Date
		for _, day := range timeline.Days {
			days = append(days, day.Date)
		}

		return html.GroupPage(html.GroupPageProps{
			PageProps: props,
			GroupName: group.Name,
			Members:   members,
			UserName:  user.Name,
			Now:       now(),
			Timeline:  timeline,
			Composer:  html.ComposerProps{Days: firstDays(days), Token: token()},
			Posts:     posts,
			Earlier:   html.EarlierPostsProps{Next: timeline.Next, Days: lastDays(days), Posts: len(days) > 0},
			Extra:     extra(true),
		}), nil
	})

	r.Post("/posts", func(props html.PageProps) (Node, error) {
		user := getUserFromContext(props.Ctx)
		if user == nil {
			return private(props, http.StatusUnauthorized)
		}

		if page, err := parseForm(props); err != nil {
			return page, err
		}
		body := props.R.PostFormValue("body")
		days := parseDays(props.R.PostFormValue("days"))

		post, err := svc.CreatePost(props.Ctx, user.ID, props.R.PostFormValue("token"), body)
		if message := refusal(err, "post", model.PostBodyMaxLength); message != "" {
			if !isDatastar(props.R) {
				return refused(props, "That was not posted", message)
			}
			return nil, writePatches(props.W, patch{elements: html.Composer(html.ComposerProps{
				Body:    echo(body),
				Message: message,
				Days:    days,
				Fresh:   echo(props.R.PostFormValue("field")),
				Token:   token(),
				Focus:   true,
			})})
		}

		// The same form sent again with other text made nothing. The person gets what the first sending
		// made, as the first time, and keeps the text they sent now, with a message.
		changed := errors.Is(err, model.ErrorFormResentChanged)
		if err != nil && !changed {
			return failed(props, err, http.StatusUnauthorized)
		}

		if !isDatastar(props.R) {
			if changed {
				return resent(props, "That was not posted", resentPost)
			}
			http.Redirect(props.W, props.R, "/", http.StatusSeeOther)
			return nil, nil
		}

		// composer for the page to have at its top, which names the given days.
		composer := func(days []model.Date) Node {
			if changed {
				return html.Composer(html.ComposerProps{
					Body:   echo(body),
					Notice: resentPost,
					Days:   days,
					Fresh:  echo(props.R.PostFormValue("field")),
					Token:  token(),
					Focus:  true,
				})
			}
			return Group{html.Composer(html.ComposerProps{Days: days, Fresh: post.ID.String(), Token: token(), Focus: true}), posted(post)}
		}

		now := now()
		day := model.GroupByDay([]model.TimelinePost{{Post: post, AuthorName: user.Name}}, group.TimeZone, now)[0]

		// The same form can be sent again after the post is on the page, so the post is added only to a
		// timeline that does not have it yet, which makes adding it idempotent.
		absent := "#" + html.TimelineID + ":not(:has(#post-" + post.ID.String() + "))"

		// The page said which days it has a heading for at its top. The post goes under the heading of
		// its day if that is one of them. A post from a day older than the newest the page has brings no
		// heading of its own, which would be above newer days: that is a form sent again some time after
		// it first was, or a clock that went back. It goes under the heading of its day if the page has
		// that further down, and is not added otherwise.
		if slices.ContainsFunc(days, func(d model.Date) bool { return d.Compare(day.Date) >= 0 }) {
			return nil, writePatches(props.W,
				patch{elements: composer(days)},
				patch{selector: absent + " #" + html.DayPostsID(day.Date), mode: "prepend", elements: html.TimelinePosts(day.Posts, posts)},
			)
		}

		// Otherwise it brings its day's heading along, to the top of a timeline that does not have the
		// day yet. If the timeline was empty, and said so, the day takes the place of that, on a timeline
		// that has no day.
		patches := []patch{
			{elements: composer(firstDays(append([]model.Date{day.Date}, days...)))},
			{selector: absent + ":not(:has(#" + html.DayPostsID(day.Date) + "))", mode: "prepend", elements: html.TimelineDay(day, now, posts)},
		}
		if len(days) == 0 {
			patches[1] = patch{selector: absent + ":not(:has([data-day]))", mode: "inner", elements: html.TimelineDay(day, now, posts)}
			return nil, writePatches(props.W, patches...)
		}

		// A new day at the top means the page was made on an earlier day, so "Today" and "Yesterday" over
		// the days below it are no longer true. Their headings are set right.
		today := model.DateOf(now)
		patches = append(patches, patch{elements: Map(days, func(d model.Date) Node {
			return html.DayHeading(model.TimelineDay{Date: d, Today: d == today, Yesterday: d == today.AddDays(-1)}, now)
		})})

		return nil, writePatches(props.W, patches...)
	})

	r.Post("/posts/earlier", func(props html.PageProps) (Node, error) {
		user := getUserFromContext(props.Ctx)
		if user == nil {
			return private(props, http.StatusUnauthorized)
		}

		if page, err := parseForm(props); err != nil {
			return page, err
		}

		// An empty cursor would be the first page, and getting that counts as a look at the timeline.
		after, err := model.ParseTimelineCursor(props.R.PostFormValue("after"))
		if err != nil {
			return badRequest(props)
		}

		if !isDatastar(props.R) {
			return needsScript(props)
		}

		timeline, err := svc.GetTimeline(props.Ctx, user.ID, after.String(), opts.TimelinePageSize)
		if err != nil {
			return failed(props, err, http.StatusUnauthorized)
		}

		// The page has a heading for the day its last post is from, which is where the cursor points,
		// and for the days it said it has at its end. Posts from those days go under the headings that
		// are there, and the other days bring their own.
		known := parseDays(props.R.PostFormValue("days"))
		if last := model.DateOf(after.Created.T.In(group.TimeZone)); !slices.Contains(known, last) {
			known = append(known, last)
		}

		// The first post loaded takes the keyboard focus, which would otherwise fall back to the page when
		// the button that asked for it goes away.
		now := now()
		var patches []patch
		var days []model.TimelineDay
		focus := posts
		focus.Focus = true
		for _, day := range timeline.Days {
			// A post can be on the page already under a day that goes on, put there by its form sent
			// again, so each post goes only to a timeline that does not have it yet.
			if slices.Contains(known, day.Date) {
				for _, p := range day.Posts {
					absent := "#" + html.TimelineID + ":not(:has(#post-" + p.ID.String() + ")) #" + html.DayPostsID(day.Date)
					patches = append(patches, patch{selector: absent, mode: "append", elements: html.TimelinePosts([]model.TimelinePost{p}, focus)})
					focus = posts
				}
				continue
			}
			days = append(days, day)
			known = append(known, day.Date)
		}
		if len(days) > 0 {
			patches = append(patches, patch{selector: "#" + html.TimelineID, mode: "append", elements: Map(days, func(day model.TimelineDay) Node {
				n := html.TimelineDay(day, now, focus)
				focus = posts
				return n
			})})
		}
		patches = append(patches, patch{elements: Group{
			html.EarlierPosts(html.EarlierPostsProps{Next: timeline.Next, Days: lastDays(known), Posts: true}),
			loaded(timeline),
		}})

		return nil, writePatches(props.W, patches...)
	})

	// comments of the given post as patches: the list, the post's count of comments, which no longer has
	// anything new in it for the person, and the given other elements.
	comments := func(props html.PageProps, user *model.User, postID model.PostID, others ...Node) (Node, error) {
		list, err := svc.GetComments(props.Ctx, user.ID, postID)
		if err != nil {
			return failed(props, err, http.StatusUnauthorized)
		}
		for i := range list {
			list[i].Created.T = list[i].Created.T.In(group.TimeZone)
		}

		return nil, writePatches(props.W, patch{elements: Group{
			html.CommentsSummary(postID, len(list), 0),
			html.Comments(html.CommentsProps{PostID: postID, Comments: list, Now: now()}),
			Group(others),
		}})
	}

	r.Post("/comments/open", func(props html.PageProps) (Node, error) {
		user := getUserFromContext(props.Ctx)
		if user == nil {
			return private(props, http.StatusUnauthorized)
		}

		if page, err := parseForm(props); err != nil {
			return page, err
		}

		if !isDatastar(props.R) {
			return needsScript(props)
		}

		// The form to add a comment is not part of the answer, so that opening the comments again
		// leaves what someone has begun to write in it alone.
		return comments(props, user, model.PostID(props.R.PostFormValue("post")))
	})

	r.Post("/comments", func(props html.PageProps) (Node, error) {
		user := getUserFromContext(props.Ctx)
		if user == nil {
			return private(props, http.StatusUnauthorized)
		}

		if page, err := parseForm(props); err != nil {
			return page, err
		}
		postID := model.PostID(props.R.PostFormValue("post"))
		body := props.R.PostFormValue("body")

		comment, err := svc.CreateComment(props.Ctx, user.ID, postID, props.R.PostFormValue("token"), body)
		if message := refusal(err, "comment", model.CommentBodyMaxLength); message != "" {
			if !isDatastar(props.R) {
				return refused(props, "That comment was not added", message)
			}
			return nil, writePatches(props.W, patch{elements: html.CommentForm(html.CommentFormProps{
				PostID:  model.PostID(echo(postID.String())),
				Body:    echo(body),
				Message: message,
				Fresh:   echo(props.R.PostFormValue("field")),
				Token:   token(),
				Focus:   true,
			})})
		}

		// The same form sent again with other text made nothing. See the same in POST /posts.
		changed := errors.Is(err, model.ErrorFormResentChanged)
		if err != nil && !changed {
			return failed(props, err, http.StatusUnauthorized)
		}

		if !isDatastar(props.R) {
			if changed {
				return resent(props, "That comment was not added", resentComment)
			}
			http.Redirect(props.W, props.R, "/", http.StatusSeeOther)
			return nil, nil
		}

		// The form of a comment belongs to its post, so the form that comes back is that post's, even
		// for a request that names another post, which the page could not have sent.
		if changed {
			return comments(props, user, comment.PostID, html.CommentForm(html.CommentFormProps{
				PostID: comment.PostID,
				Body:   echo(body),
				Notice: resentComment,
				Fresh:  echo(props.R.PostFormValue("field")),
				Token:  token(),
				Focus:  true,
			}))
		}

		// A comment sent again gets what it got the first time, which is on the post it was made on.
		return comments(props, user, comment.PostID,
			html.CommentForm(html.CommentFormProps{PostID: comment.PostID, Fresh: comment.ID.String(), Token: token(), Focus: true}),
			html.Announcement(comment.ID.String(), "Your comment was added."),
		)
	})
}

// posted says to screen readers that the given post is on the timeline.
func posted(p model.Post) Node {
	return html.Announcement(p.ID.String(), "Your post is on the timeline.")
}

// loaded says to screen readers how many earlier posts the given page loaded, and if they are the last.
func loaded(page model.Timeline) Node {
	var first model.PostID
	var count int
	for _, day := range page.Days {
		for _, p := range day.Posts {
			if first == "" {
				first = p.ID
			}
			count++
		}
	}

	switch {
	case count == 0:
		return html.Announcement("none", "There are no earlier posts.")
	case count == 1 && page.Next == "":
		return html.Announcement(first.String(), "Loaded 1 earlier post. There are no posts before it.")
	case count == 1:
		return html.Announcement(first.String(), "Loaded 1 earlier post.")
	case page.Next == "":
		return html.Announcement(first.String(), fmt.Sprint("Loaded ", count, " earlier posts. There are no posts before these."))
	default:
		return html.Announcement(first.String(), fmt.Sprint("Loaded ", count, " earlier posts."))
	}
}

// parseForm of the request, which may be no larger than a megabyte. That is many times the longest
// post, so that a post that is too long can still be read, and refused as that. If the form cannot be
// read, parseForm returns the answer to give.
func parseForm(props html.PageProps) (Node, error) {
	props.R.Body = http.MaxBytesReader(props.W, props.R.Body, 1<<20)

	err := props.R.ParseForm()
	if err == nil {
		return nil, nil
	}

	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		page := html.MessagePage(html.MessagePageProps{PageProps: props, Heading: "That is too long", Message: "What was sent is far longer than a post can be."})
		return page, httph.HTTPError{Code: http.StatusRequestEntityTooLarge}
	}
	return badRequest(props)
}

// parseDays that a page said it has headings for, in the form [html.ComposerProps.Days] are sent in.
// What is not a date is left out, and so is anything after the first few, since a page only names a few.
func parseDays(s string) []model.Date {
	var days []model.Date
	for part := range strings.SplitSeq(s, ",") {
		if len(days) == daysSent {
			break
		}
		t, err := time.Parse(time.DateOnly, part)
		if err != nil {
			continue
		}
		if d := model.DateOf(t); !slices.Contains(days, d) {
			days = append(days, d)
		}
	}
	return days
}

// daysSent is how many days a form names. Three is enough for the days whose headings can read "Today"
// or "Yesterday", and for the days around a change of the clocks that a later post can still belong to.
const daysSent = 3

// firstDays of the given days, for a form to name.
func firstDays(days []model.Date) []model.Date {
	return days[:min(len(days), daysSent)]
}

// lastDays of the given days, for a form to name.
func lastDays(days []model.Date) []model.Date {
	return days[max(len(days)-daysSent, 0):]
}

// echo of what a request sent, for putting it back on the page: the same text, with whatever in it is
// not text replaced, so that the answer is.
func echo(s string) string {
	return strings.ToValidUTF8(s, "\ufffd")
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

// resentPost and resentComment say why the text in a form that was sent again was not posted or added,
// when the same form was sent before with other text.
const (
	resentPost    = "Your earlier try did go through, and is on the timeline as it was sent. This text has not been posted."
	resentComment = "Your earlier try did go through, and is with the comments as it was sent. This text has not been added."
)

// resent is the answer to a browser that does not run the page's script, when it sent the same form again
// with other text.
func resent(props html.PageProps, heading, message string) (Node, error) {
	page := html.MessagePage(html.MessagePageProps{PageProps: props, Heading: heading, Message: message})
	return page, httph.HTTPError{Code: http.StatusConflict}
}

// refused is the answer to a browser that does not run the page's script, when what it sent was refused.
func refused(props html.PageProps, heading, message string) (Node, error) {
	page := html.MessagePage(html.MessagePageProps{PageProps: props, Heading: heading, Message: message})
	return page, httph.HTTPError{Code: http.StatusUnprocessableEntity}
}

// needsScript is the answer to a browser that does not run the page's script, when it asks for something
// that can only be shown in place.
func needsScript(props html.PageProps) (Node, error) {
	page := html.MessagePage(html.MessagePageProps{PageProps: props, Heading: "That needs JavaScript", Message: "Comments and earlier posts open in place on the group page, which takes JavaScript."})
	return page, httph.HTTPError{Code: http.StatusBadRequest}
}

// badRequest is the answer to a request that the page could not have sent.
func badRequest(props html.PageProps) (Node, error) {
	page := html.MessagePage(html.MessagePageProps{PageProps: props, Heading: "That did not work", Message: "The request was not one this page understands."})
	return page, httph.HTTPError{Code: http.StatusBadRequest}
}
