package html

import (
	"fmt"
	"time"

	. "maragu.dev/gomponents"
	data "maragu.dev/gomponents-datastar"
	. "maragu.dev/gomponents/html"

	"app/model"
)

// TimelineID of the element that holds the days of the timeline.
const TimelineID = "timeline"

// DayPostsID of the element that holds the posts of the given day.
func DayPostsID(d model.Date) string {
	return "day-" + d.String() + "-posts"
}

type GroupPageProps struct {
	PageProps

	// GroupName on the sign.
	GroupName string

	// Members is how many people are in the group.
	Members int

	// UserName of the person the page is for.
	UserName string

	// Now in the group's time zone.
	Now time.Time

	// Timeline is the first page of the timeline, with its times in the group's time zone.
	Timeline model.Timeline

	Composer ComposerProps

	// Extra is put beside the person's name on the sign, if not nil.
	Extra Node
}

// GroupPage is the page a member opens: the sign with the group's name, the composer, and the timeline.
func GroupPage(props GroupPageProps) Node {
	props.Title = props.GroupName
	props.Composer.Newest = newestDay(props.Timeline.Days)

	members := fmt.Sprint(props.Members, " members")
	if props.Members == 1 {
		members = "1 member"
	}

	return document(props.PageProps,
		Header(Class("bg-primary-500 text-stone-900 [&_:focus-visible]:outline-stone-900"),
			Div(Class(wrap+" flex min-h-[104px] flex-col justify-between gap-3 pt-3 pb-5 lg:min-h-[152px] lg:gap-2 lg:pt-[18px] lg:pb-[30px]"),
				Div(Class("flex flex-wrap items-baseline gap-x-5 text-[15px]/6"),
					Span(Text("regroup")),
					Span(Text(members)),
					Div(Class("ml-auto flex items-baseline gap-x-5"),
						Span(Class("font-serif text-lg/6 font-[640]"), Text(props.UserName)),
						Iff(props.Extra != nil, func() Node {
							return Span(Class("[&_a]:underline [&_a]:decoration-stone-900/50 [&_a]:decoration-1 [&_a]:underline-offset-4 [&_a]:hover:decoration-stone-900"), props.Extra)
						}),
					),
				),
				H1(Class(signName), Text(props.GroupName)),
			),
		),
		Main(Class(wrap+" pt-6 pb-24 lg:pt-8"),
			Composer(props.Composer),
			Div(ID(TimelineID),
				If(len(props.Timeline.Days) == 0,
					P(Class(postColumn+" mt-10 text-stone-600"), Text("Nothing has been posted yet. The first post goes in the field above.")),
				),
				Map(props.Timeline.Days, func(day model.TimelineDay) Node {
					return TimelineDay(day, props.Now)
				}),
			),
			EarlierPosts(props.Timeline.Next),
			NoScript(P(Class(postColumn+" mt-10 text-stone-600"), Text("Opening comments and loading earlier posts need JavaScript."))),
		),
		P(ID("request-failed"), Role("alert"),
			Class("fixed inset-x-0 bottom-0 hidden bg-stone-900 px-5 py-3 text-center text-white"),
			data.Signals(map[string]any{"_failed": false}),
			data.On("datastar-fetch", "$_failed = evt.detail.type === 'error' || evt.detail.type === 'retries-failed' ? true : evt.detail.type === 'started' ? false : $_failed"),
			data.Class("hidden", "!$_failed"),
			Text("That did not go through. Check your connection and try again."),
		),
	)
}

// signName is the lettering on the sign.
const signName = "font-serif text-[44px]/[0.9] font-[620] tracking-[-0.02em] text-balance wrap-anywhere lg:text-[96px]/[0.8]"

// postColumn is where the text of posts starts: beside the names in their margin column on a wide
// screen, and at the left edge on a narrow one.
const postColumn = "max-w-[608px] lg:ml-[232px]"

type PrivatePageProps struct {
	PageProps

	// Extra is put under the sign, if not nil.
	Extra Node
}

// PrivatePage is all that someone who is not signed in sees. It names neither the group nor anyone in it.
func PrivatePage(props PrivatePageProps) Node {
	props.Title = "regroup"

	return document(props.PageProps,
		Header(Class("bg-primary-500 text-stone-900 [&_:focus-visible]:outline-stone-900"),
			Div(Class(wrap+" flex min-h-[104px] flex-col justify-between gap-3 pt-3 pb-5 lg:min-h-[152px] lg:gap-2 lg:pt-[18px] lg:pb-[30px]"),
				Div(Class("text-[15px]/6"), Text("regroup")),
				H1(Class(signName), Text("This group is private.")),
			),
		),
		Iff(props.Extra != nil, func() Node {
			return Main(Class(wrap+" pt-8 [&_a]:underline [&_a]:decoration-stone-400 [&_a]:decoration-1 [&_a]:underline-offset-4 [&_a]:hover:decoration-stone-900"), P(props.Extra))
		}),
	)
}

// newestDay of the given days, as the composer reports it, or empty if there are none.
func newestDay(days []model.TimelineDay) string {
	if len(days) == 0 {
		return ""
	}
	return days[0].Date.String()
}

type ComposerProps struct {
	// Body is what the person typed, kept when it was refused.
	Body string

	// Message says why the body was refused, if it was.
	Message string

	// Newest is the date of the newest day on the person's page, or empty if their timeline is empty.
	// The composer sends it along with a post, so that the answer can put the post under the right
	// day heading.
	Newest string

	// Fresh, if not empty, gives the field a new identity, so that a field patched into a page replaces
	// the one with the text just posted in it instead of keeping that text. The composer sends it along
	// with a post, so that a refusal can keep the field's identity, and so what is typed in it.
	Fresh string
}

// Composer is the form a post is written in.
func Composer(props ComposerProps) Node {
	fieldID := "composer-body"
	if props.Fresh != "" {
		fieldID += "-" + props.Fresh
	}

	return Form(ID("composer"), Method("post"), Action("/posts"), Class(postColumn),
		data.On("submit", "@post('/posts', {contentType: 'form', retryMaxCount: 0})"),
		data.Indicator("_sending"),
		Input(Type("hidden"), Name("newest"), Value(props.Newest)),
		Input(Type("hidden"), Name("field"), Value(props.Fresh)),
		Div(Class("flex items-end gap-2"),
			Label(For(fieldID), Class("sr-only"), Text("Write to the group")),
			textarea(fieldID, "Write to the group", "3", props.Body, props.Message != "", "composer-message", "min-h-[52px] px-[22px] py-3"),
			Button(Type("submit"), data.Attr("disabled", "$_sending"),
				Class("inline-flex h-[52px] shrink-0 cursor-pointer items-center rounded-full bg-primary-500 px-[22px] text-stone-900 hover:bg-primary-400 active:bg-primary-600 disabled:cursor-default disabled:opacity-60"),
				Text("Post"),
			),
		),
		message("composer-message", props.Message),
	)
}

// textarea that grows with what is written in it, where the browser can do that, and is as many rows
// tall as given where it cannot.
func textarea(id, placeholder, rows, value string, invalid bool, messageID, class string) Node {
	return Textarea(ID(id), Name("body"), Rows(rows), Placeholder(placeholder),
		If(invalid, Aria("invalid", "true")),
		If(invalid, Aria("describedby", messageID)),
		Class("field-sizing-content max-h-[50vh] w-full min-w-0 resize-none rounded-[26px] border border-stone-500 bg-white leading-[26px] text-stone-900 caret-primary-700 placeholder:text-stone-500 hover:border-stone-900 focus-visible:border-primary-700 focus-visible:-outline-offset-1 "+class),
		// The browser drops a line break right after the opening tag, so one goes first.
		Text("\n"+value),
	)
}

// message beside a field, saying why what was written in it was refused. Nothing if the text is empty.
func message(id, text string) Node {
	if text == "" {
		return nil
	}

	return P(ID(id), Role("alert"), Class("mt-2 flex items-start gap-2 text-stone-900"),
		Raw(`<svg class="mt-[5px] shrink-0" width="18" height="18" viewBox="0 0 18 18" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" aria-hidden="true"><circle cx="9" cy="9" r="7.25"/><path d="M9 5.25v4.5M9 12.6v.15"/></svg>`),
		Span(Text(text)),
	)
}

// TimelineDay is one day of the timeline: its heading and its posts. Now, in the group's time zone, decides
// whether the heading names the year.
func TimelineDay(day model.TimelineDay, now time.Time) Node {
	date := day.Date.Time().Format("Monday 2 January")
	if day.Date.Year != now.Year() {
		date = day.Date.Time().Format("Monday 2 January 2006")
	}

	var heading Node
	switch {
	case day.Today:
		heading = Group{Text("Today "), dayDate(date)}
	case day.Yesterday:
		heading = Group{Text("Yesterday "), dayDate(date)}
	default:
		heading = Text(date)
	}

	return Section(Data("day", day.Date.String()), Class("mt-12 lg:mt-14"),
		H2(Class("mb-6 flex flex-wrap items-baseline gap-x-5 gap-y-2 font-serif text-[40px]/none font-[620] tracking-[-0.015em] lg:mb-7 lg:text-[64px]/none"), heading),
		Div(ID(DayPostsID(day.Date)),
			TimelinePosts(day.Posts),
		),
	)
}

// dayDate written out beside "Today" or "Yesterday".
func dayDate(date string) Node {
	return Span(Class("font-sans text-base/none font-normal tracking-normal whitespace-nowrap text-stone-600"), Text(date))
}

// TimelinePosts as they are shown under their day's heading.
func TimelinePosts(posts []model.TimelinePost) Node {
	return Map(posts, func(p model.TimelinePost) Node {
		id := p.ID.String()

		return Article(ID("post-"+id), Data("post", id), Class("mb-9 grid grid-cols-1 gap-y-2 lg:grid-cols-[200px_608px] lg:gap-x-8"),
			Div(Class("flex flex-wrap items-baseline gap-x-3 lg:block"),
				H3(Class("font-serif text-[28px]/none font-[640] tracking-[-0.005em] wrap-anywhere text-primary-700 lg:text-[36px]/none"), Text(p.AuthorName)),
				Div(Class("flex items-baseline gap-x-3 text-[15px]/6 text-stone-600 lg:mt-2"),
					Time(DateTime(p.Created.T.Format(time.RFC3339)), Class("tabular-nums"), Text(p.Created.T.Format("15:04"))),
					If(p.New, newMark("new")),
				),
			),
			Div(Class("min-w-0"),
				P(Data("body", ""), Class("wrap-anywhere whitespace-pre-wrap"), Text(p.Body)),
				// Opening the comments posts the form below, so that the post's ID is in the body of the
				// request and not in its URL: which comments someone opened is nobody else's business.
				Details(Class("mt-2"),
					data.On("toggle", "evt.target.open && @post('/comments/open', {contentType: 'form', selector: '#post-"+id+"-open', retryMaxCount: 0})"),
					Summary(Class("inline-flex cursor-pointer list-none rounded-sm text-[15px]/6 text-stone-600 hover:text-stone-900 [&::-webkit-details-marker]:hidden"),
						CommentsSummary(p.ID, p.Comments, p.NewComments),
					),
					Form(ID("post-"+id+"-open"), Method("post"), Action("/comments/open"), Hidden(""),
						Input(Type("hidden"), Name("post"), Value(id)),
					),
					Div(ID("post-"+id+"-comments")),
				),
			),
		)
	})
}

// newMark with the given text, for what is new to the reader. It is a dot and a word, so that it does
// not depend on its colour.
func newMark(text string) Node {
	return Span(Data("mark", "new"), Class("inline-flex items-center gap-[7px] text-primary-700"),
		Span(Class("size-[9px] rounded-full bg-primary-500"), Aria("hidden", "true")),
		Text(text),
	)
}

// CommentsSummary of the given post: how many comments it has, and how many of them are new to the
// reader, if any are. It is what opens the comments.
func CommentsSummary(postID model.PostID, comments, newComments int) Node {
	label := fmt.Sprint(comments, " comments")
	switch comments {
	case 0:
		label = "Comment"
	case 1:
		label = "1 comment"
	}

	return Span(ID("post-"+postID.String()+"-comments-summary"), Class("inline-flex items-baseline gap-x-5"),
		Span(Class("underline decoration-stone-400 decoration-1 underline-offset-4"), Text(label)),
		If(newComments > 0, newMark(fmt.Sprint(newComments, " new"))),
	)
}

type CommentsProps struct {
	PostID model.PostID

	// Comments of the post, oldest first, with their times in the group's time zone.
	Comments []model.TimelineComment

	// Now in the group's time zone.
	Now time.Time

	Form CommentFormProps
}

// Comments of a post, opened in place under it, with the form to add one.
func Comments(props CommentsProps) Node {
	props.Form.PostID = props.PostID

	return Div(ID("post-"+props.PostID.String()+"-comments"),
		Div(Class("mt-4 border-l border-stone-200 pl-5 motion-safe:animate-fade-in lg:pl-6"),
			If(len(props.Comments) > 0,
				Ol(Class("mb-5 grid gap-4"),
					Map(props.Comments, func(c model.TimelineComment) Node {
						id := c.ID.String()

						return Li(ID("comment-"+id), Data("comment", id),
							P(Class("flex flex-wrap items-baseline gap-x-2.5"),
								Span(Class("font-serif text-xl/7 font-[680] wrap-anywhere text-primary-700"), Text(c.AuthorName)),
								Span(Class("flex items-baseline gap-x-3 text-[15px]/6 text-stone-600"),
									Time(DateTime(c.Created.T.Format(time.RFC3339)), Class("tabular-nums"), Text(commentTime(c.Created.T, props.Now))),
									If(c.New, newMark("new")),
								),
							),
							P(Data("body", ""), Class("wrap-anywhere whitespace-pre-wrap"), Text(c.Body)),
						)
					}),
				),
			),
			CommentForm(props.Form),
		),
	)
}

// commentTime as it is shown beside a comment: the time alone if it was written today, and with the day
// if it was not, since a comment can come days after its post.
func commentTime(t, now time.Time) string {
	today := model.DateOf(now)
	switch date := model.DateOf(t); {
	case date == today:
		return t.Format("15:04")
	case date == today.AddDays(-1):
		return "Yesterday " + t.Format("15:04")
	case date.Year == today.Year:
		return t.Format("2 January 15:04")
	default:
		return t.Format("2 January 2006 15:04")
	}
}

type CommentFormProps struct {
	PostID model.PostID

	// Body is what the person typed, kept when it was refused.
	Body string

	// Message says why the body was refused, if it was.
	Message string

	// Fresh, if not empty, gives the field a new identity. See [ComposerProps.Fresh].
	Fresh string
}

// CommentForm is the form a comment on a post is written in.
func CommentForm(props CommentFormProps) Node {
	id := props.PostID.String()
	fieldID := "post-" + id + "-comment-body"
	if props.Fresh != "" {
		fieldID += "-" + props.Fresh
	}
	messageID := "post-" + id + "-comment-message"

	return Form(ID("post-"+id+"-comment-form"), Method("post"), Action("/comments"),
		data.On("submit", "@post('/comments', {contentType: 'form', retryMaxCount: 0})"),
		data.Indicator("_sending"),
		Input(Type("hidden"), Name("post"), Value(id)),
		Input(Type("hidden"), Name("field"), Value(props.Fresh)),
		Div(Class("flex items-end gap-2"),
			Label(For(fieldID), Class("sr-only"), Text("Write a comment")),
			textarea(fieldID, "Write a comment", "2", props.Body, props.Message != "", messageID, "min-h-11 px-[18px] py-2"),
			Button(Type("submit"), data.Attr("disabled", "$_sending"), Class(secondaryButton+" h-11"), Text("Comment")),
		),
		message(messageID, props.Message),
	)
}

const secondaryButton = "inline-flex shrink-0 cursor-pointer items-center rounded-full border border-stone-400 px-[18px] text-stone-900 hover:border-stone-900 disabled:cursor-default disabled:opacity-60"

// EarlierPosts is the control at the end of the timeline that loads the page after the given cursor.
// It is empty when the cursor is, which is at the end of the timeline.
//
// The cursor is posted, so that how far back someone has read is not in a URL.
func EarlierPosts(next string) Node {
	return Form(ID("earlier"), Method("post"), Action("/posts/earlier"), Class(postColumn+" mt-12"),
		data.On("submit", "@post('/posts/earlier', {contentType: 'form', retryMaxCount: 0})"),
		data.Indicator("_sending"),
		If(next != "", Group{
			Input(Type("hidden"), Name("after"), Value(next)),
			Button(Type("submit"), data.Attr("disabled", "$_sending"), Class(secondaryButton+" h-11"), Text("Earlier posts")),
		}),
	)
}

type MessagePageProps struct {
	PageProps

	// Heading of the page.
	Heading string

	// Message under the heading.
	Message string
}

// MessagePage says one thing in plain words, such as why something could not be done.
func MessagePage(props MessagePageProps) Node {
	props.Title = props.Heading

	return Page(props.PageProps,
		H1(Text(props.Heading)),
		P(Text(props.Message)),
	)
}
