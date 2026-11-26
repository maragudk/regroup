package html_test

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	nethtml "golang.org/x/net/html"
	"hegel.dev/go/hegel"
	. "maragu.dev/gomponents"
	. "maragu.dev/gomponents/html"
	"maragu.dev/is"

	"app/html"
	"app/model"
)

// now is not today, so that a test can tell the time it gives from the real one.
var now = time.Date(2031, 3, 14, 10, 0, 0, 0, time.UTC)

func TestGroupPage(t *testing.T) {
	t.Run("has the group's name as its title and on the sign", func(t *testing.T) {
		page := render(t, html.GroupPage(html.GroupPageProps{GroupName: "The Loft", Members: 14, UserName: "Signe", Now: now}))

		is.True(t, strings.Contains(page, "<title>The Loft</title>"), page)
		is.True(t, regexp.MustCompile(`<h1[^>]*>The Loft</h1>`).MatchString(page), page)
		is.True(t, strings.Contains(page, "<span>14 members</span>"), page)
	})

	t.Run("counts one member as one member", func(t *testing.T) {
		page := render(t, html.GroupPage(html.GroupPageProps{GroupName: "The Loft", Members: 1, Now: now}))

		is.True(t, strings.Contains(page, "<span>1 member</span>"), page)
	})

	t.Run("puts what it is given beside the person's name on the sign, and nothing when it is given nothing", func(t *testing.T) {
		page := render(t, html.GroupPage(html.GroupPageProps{GroupName: "The Loft", Now: now, Extra: A(Href("/somewhere"), Text("Somewhere"))}))
		is.True(t, strings.Contains(page, `<a href="/somewhere">Somewhere</a>`), page)

		page = render(t, html.GroupPage(html.GroupPageProps{GroupName: "The Loft", Now: now}))
		is.True(t, !strings.Contains(page, "<a "), page)
	})

	t.Run("says that a request failed in a place that is hidden until one does, and turns the buttons of what is being sent off until its own request has been answered", func(t *testing.T) {
		doc, err := nethtml.Parse(strings.NewReader(render(t, html.GroupPage(html.GroupPageProps{GroupName: "The Loft", Now: now}))))
		is.NotError(t, err)

		var found int
		for n := range doc.Descendants() {
			if n.Type != nethtml.ElementNode || attribute(n, "id") != "request-failed" {
				continue
			}
			found++

			is.Equal(t, "p", n.Data)
			is.Equal(t, "alert", attribute(n, "role"))
			// The words sit in the page's column, at its left edge.
			is.Equal(t, "span", n.FirstChild.Data)
			is.True(t, strings.Contains(attribute(n.FirstChild, "class"), "max-w-[1440px]"), attribute(n.FirstChild, "class"))
			is.True(t, !strings.Contains(attribute(n, "class"), "text-center"), attribute(n, "class"))
			is.Equal(t, "That did not go through. Try again, and reload the page if it keeps happening.", n.FirstChild.FirstChild.Data)

			// Hidden from the start, and shown by the signal that the listener sets.
			is.True(t, slices.Contains(strings.Fields(attribute(n, "class")), "hidden"), attribute(n, "class"))
			is.Equal(t, `{"_failed":false}`, attribute(n, "data-signals"))
			is.Equal(t, "{hidden: !$_failed}", attribute(n, "data-class"))

			listener := attribute(n, "data-on:datastar-fetch")
			for _, part := range []string{
				// A button counts the requests it is off for, such as a comment on its way and the opening
				// of its post's comments, and comes on again only when none of them is on its way.
				"if (type === 'started') { $_failed = false; buttons.forEach(b => { b.sending = (b.sending || 0) + 1; b.disabled = true }) }",
				"else if (type === 'finished') { buttons.forEach(b => { b.sending = Math.max((b.sending || 0) - 1, 0); b.disabled = b.sending > 0 }) }",
				"else if (type === 'error' || type === 'retries-failed') { $_failed = true }",
				"buttons = evt.detail.el.querySelectorAll('button')",
			} {
				is.True(t, strings.Contains(listener, part), listener)
			}
		}
		is.Equal(t, 1, found)
	})

	t.Run("has one polite live region, for what is said to screen readers, with nothing in it at first", func(t *testing.T) {
		doc, err := nethtml.Parse(strings.NewReader(render(t, html.GroupPage(html.GroupPageProps{GroupName: "The Loft", Now: now}))))
		is.NotError(t, err)

		var regions []*nethtml.Node
		for n := range doc.Descendants() {
			if n.Type == nethtml.ElementNode && (attribute(n, "aria-live") != "" || attribute(n, "role") == "status") {
				regions = append(regions, n)
			}
		}
		is.Equal(t, 1, len(regions))
		is.Equal(t, "announcement", attribute(regions[0], "id"))
		is.Equal(t, "status", attribute(regions[0], "role"))
		is.Equal(t, "polite", attribute(regions[0], "aria-live"))
		is.True(t, slices.Contains(strings.Fields(attribute(regions[0], "class")), "sr-only"), attribute(regions[0], "class"))
		is.True(t, regions[0].FirstChild == nil, "the region is not empty")
	})

	t.Run("sends every form through Datastar as a form, once, to the path the form itself posts to, and never as a GET", func(t *testing.T) {
		post := model.TimelinePost{Post: model.Post{ID: "p_1", Created: model.Time{T: now}, Body: "Cake"}, AuthorName: "Signe"}
		page := render(t, html.GroupPage(html.GroupPageProps{
			GroupName: "The Loft",
			Now:       now,
			Timeline:  model.Timeline{Days: []model.TimelineDay{{Date: model.DateOf(now), Today: true, Posts: []model.TimelinePost{post}}}},
			Earlier:   html.EarlierPostsProps{Next: "abc"},
		}))
		doc, err := nethtml.Parse(strings.NewReader(page))
		is.NotError(t, err)

		var forms []string
		for n := range doc.Descendants() {
			if n.Type != nethtml.ElementNode {
				continue
			}
			for _, a := range n.Attr {
				is.True(t, !strings.Contains(a.Val, "@get("), a.Val)
			}
			if n.Data != "form" {
				continue
			}
			forms = append(forms, attribute(n, "action"))
			is.Equal(t, "post", attribute(n, "method"))

			// The form that opens the comments is sent by the element around it, when that opens.
			if attribute(n, "action") == "/comments/open" {
				is.Equal(t, "", attribute(n, "data-on:submit"))
				is.Equal(t, "details", n.Parent.Data)
				is.Equal(t, n.Parent.Attr[0].Val+"-open", attribute(n, "id"))
				// Only one opening is on its way at a time, so that a second cannot use up the marks the
				// first brings before the reader has seen them.
				is.Equal(t, "el.open && !el.opening && (el.opening = @post('/comments/open', {contentType: 'form', selector: '#' + el.id + '-open', retryMaxCount: 0, requestCancellation: 'disabled'}).catch(() => $_failed = true).finally(() => el.opening = null))", attribute(n.Parent, "data-on:toggle"))
				continue
			}
			// A request that cannot be sent at all fails with an error that would go unhandled. It is
			// caught, and shown in the failure bar.
			is.Equal(t, "@post('"+attribute(n, "action")+"', {contentType: 'form', retryMaxCount: 0, requestCancellation: 'disabled'}).catch(() => $_failed = true)", attribute(n, "data-on:submit"))
		}
		is.EqualSlice(t, []string{"/posts", "/comments/open", "/comments", "/posts/earlier"}, forms)
	})

	t.Run("marks the name of the person the page is for, and the name of the author of every post and comment, for checks to find", func(t *testing.T) {
		post := model.TimelinePost{Post: model.Post{ID: "p_1", Created: model.Time{T: now}, Body: "Cake"}, AuthorName: "Thomas"}
		comment := model.TimelineComment{Comment: model.Comment{ID: "c_1", Created: model.Time{T: now}, PostID: "p_1", Body: "Yes"}, AuthorName: "Ada"}
		doc, err := nethtml.Parse(strings.NewReader(render(t, Group{
			html.GroupPage(html.GroupPageProps{
				GroupName: "The Loft",
				UserName:  "Signe",
				Now:       now,
				Timeline:  model.Timeline{Days: []model.TimelineDay{{Date: model.DateOf(now), Today: true, Posts: []model.TimelinePost{post}}}},
			}),
			html.Comments(html.CommentsProps{PostID: "p_1", Comments: []model.TimelineComment{comment}, Now: now}),
		})))
		is.NotError(t, err)

		var people, authors []string
		for n := range doc.Descendants() {
			if n.Type != nethtml.ElementNode {
				continue
			}
			for _, a := range n.Attr {
				switch a.Key {
				case "data-person":
					people = append(people, n.FirstChild.Data)
				case "data-author":
					authors = append(authors, n.FirstChild.Data)
				}
			}
		}
		is.EqualSlice(t, []string{"Signe"}, people)
		is.EqualSlice(t, []string{"Thomas", "Ada"}, authors)
	})

	t.Run("shows any body that can be posted, and any name, as exactly that text, wherever it comes out, and never as an element or an attribute", func(t *testing.T) {
		// page with the given body as a post, a comment, and what was typed in both fields, and the given
		// name as the group, the person, both authors, and the part of both fields' IDs that is sent in.
		// It returns how many elements and attributes the page has, and the page.
		page := func(body, name string) (int, int, *nethtml.Node) {
			post := model.TimelinePost{Post: model.Post{ID: "p_1", Created: model.Time{T: now}, Body: body}, AuthorName: name}
			comment := model.TimelineComment{Comment: model.Comment{ID: "c_1", Created: model.Time{T: now}, PostID: "p_1", Body: body}, AuthorName: name}

			doc, err := nethtml.Parse(strings.NewReader(render(t, Group{
				html.GroupPage(html.GroupPageProps{
					GroupName: name,
					UserName:  name,
					Now:       now,
					Timeline:  model.Timeline{Days: []model.TimelineDay{{Date: model.DateOf(now), Today: true, Posts: []model.TimelinePost{post}}}},
					Composer:  html.ComposerProps{Body: body, Message: "Refused.", Fresh: name},
				}),
				html.Comments(html.CommentsProps{PostID: "p_1", Comments: []model.TimelineComment{comment}, Now: now}),
				html.CommentForm(html.CommentFormProps{PostID: "p_1", Body: body, Message: "Refused.", Fresh: name}),
			})))
			is.NotError(t, err)

			var elements, attributes int
			for n := range doc.Descendants() {
				if n.Type == nethtml.ElementNode {
					elements++
					attributes += len(n.Attr)
				}
			}
			return elements, attributes, doc
		}

		plainElements, plainAttributes, _ := page("Cake", "Signe")

		hegel.Test(t, func(ht *hegel.T) {
			body, err := model.CleanPostBody(hegel.Draw(ht, markup()))
			ht.Assume(err == nil)
			name := hegel.Draw(ht, markup())

			// A browser reading HTML makes a line feed of every carriage return, and shows the replacement
			// character for a NUL. A body has neither, but nothing says that of a name.
			shown := strings.NewReplacer("\r\n", "\n", "\r", "\n", "\x00", "\ufffd").Replace(name)

			elements, attributes, doc := page(body, name)
			if elements != plainElements || attributes != plainAttributes {
				ht.Fatalf("the body %q and the name %q changed the page from %v elements and %v attributes to %v and %v", body, name, plainElements, plainAttributes, elements, attributes)
			}

			var bodies, fields, names, ids int
			for n := range doc.Descendants() {
				if n.Type != nethtml.ElementNode {
					continue
				}

				var expected string
				switch {
				case n.Data == "textarea" && n.FirstChild == nil:
					// The empty field for a comment under the post.
					continue
				case n.Data == "textarea":
					fields++
					expected = body
					// The name is part of the field's ID, unless it is empty.
					suffix := ""
					if shown != "" {
						suffix = "-" + shown
					}
					if id := attribute(n, "id"); id == "composer-body"+suffix || id == "post-p_1-comment-body"+suffix {
						ids++
					}
				case attribute(n, "data-body") != "" || len(n.Attr) > 0 && n.Attr[0].Key == "data-body":
					bodies++
					expected = body
				case n.Data == "h1" || n.Data == "h3" || n.Data == "title":
					names++
					expected = shown
				default:
					continue
				}

				var got string
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					if c.Type != nethtml.TextNode {
						ht.Fatalf("%q or %q put something other than text into %v", body, name, n.Data)
					}
					got += c.Data
				}
				if got != expected {
					ht.Fatalf("expected %q in %v, got %q", expected, n.Data, got)
				}
			}
			if bodies != 2 || fields != 2 || names != 3 || ids != 2 {
				ht.Fatalf("expected %q in a post, a comment, and two fields, and %q in three places and two IDs, but found %v, %v, %v, and %v", body, name, bodies, fields, names, ids)
			}
		})
	})
}

// attributeOK of the given element with the given name, and whether it has one.
func attributeOK(n *nethtml.Node, key string) (string, bool) {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val, true
		}
	}
	return "", false
}

// attribute of the given element with the given name, or empty if it has none.
func attribute(n *nethtml.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// markup is text made of pieces of HTML that try to become elements, attributes, or comments, mixed
// with line breaks and any other text.
func markup() hegel.Generator[string] {
	piece := hegel.OneOf(
		hegel.SampledFrom([]string{
			"<script>alert(1)</script>", `<img src="x" onerror="alert(1)">`, `<a href="https://example.com/">link</a>`,
			"</p>", "</article>", "</textarea>", "</section></main><h1>", "<textarea>", "<!--", "-->", "<![CDATA[", "]]>",
			"&amp;", "&lt;", "&#39;", "&", "<", ">", `"`, "'", "`", "=", "/", "\n", "\n\n", "\t", " ", "æøå", "😀",
			`" onmouseover="alert(1)`, "<svg/onload=alert(1)>", "<style>*{display:none}</style>", "<base href=//example.com>",
			"\r", "\r\n", "\x00",
		}),
		hegel.Text(),
	)

	return hegel.Composite(func(tc hegel.TestCase) string {
		return strings.Join(hegel.Draw(tc, hegel.Lists(piece)), "")
	})
}

func TestComposer(t *testing.T) {
	t.Run("keeps what was typed as it was, with a line break at its start too", func(t *testing.T) {
		for _, typed := range []string{"\nCake", "\n\n  Cake\n", "Cake", ""} {
			doc, err := nethtml.Parse(strings.NewReader(render(t, html.Composer(html.ComposerProps{Body: typed, Message: "Refused."}))))
			is.NotError(t, err)

			var got string
			var fields int
			for n := range doc.Descendants() {
				if n.Type == nethtml.ElementNode && n.Data == "textarea" {
					fields++
					for c := n.FirstChild; c != nil; c = c.NextSibling {
						got += c.Data
					}
				}
			}
			is.Equal(t, 1, fields)
			is.Equal(t, typed, got)
		}
	})

	t.Run("says why what was typed was refused, and ties the message to the field", func(t *testing.T) {
		form := render(t, html.Composer(html.ComposerProps{Body: "Cake", Message: "Refused."}))
		is.True(t, strings.Contains(form, `aria-invalid="true" aria-describedby="composer-message"`), form)
		is.True(t, regexp.MustCompile(`<p id="composer-message" role="alert"[^>]*>.*<span>Refused\.</span></p>`).MatchString(form), form)

		form = render(t, html.Composer(html.ComposerProps{}))
		is.True(t, !strings.Contains(form, "aria-invalid") && !strings.Contains(form, "role=\"alert\""), form)
	})

	t.Run("says what it has to say about what was typed beside the field, without calling it wrong", func(t *testing.T) {
		for _, form := range []string{
			render(t, html.Composer(html.ComposerProps{Body: "Cake", Notice: "Not posted."})),
			render(t, html.CommentForm(html.CommentFormProps{PostID: "p_1", Body: "Cake", Notice: "Not posted."})),
		} {
			is.True(t, !strings.Contains(form, "aria-invalid"), form)
			is.True(t, regexp.MustCompile(`aria-describedby="(composer|post-p_1-comment)-message"`).MatchString(form), form)
			is.True(t, regexp.MustCompile(`<p id="(composer|post-p_1-comment)-message" role="alert"[^>]*>.*<span>Not posted\.</span></p>`).MatchString(form), form)
		}
	})

	t.Run("has the field take the keyboard focus when it is put on the page, if asked to, unless the focus is elsewhere", func(t *testing.T) {
		field := textareaOf(t, html.Composer(html.ComposerProps{Focus: true, Token: "q6urq6urq6urq6urq6urqw"}))
		// The token is in the expression so that it is a new one each time, which Datastar runs again.
		is.Equal(t, "'q6urq6urq6urq6urq6urqw' && (document.activeElement === document.body || el.form.contains(document.activeElement)) && el.focus()", attribute(field, "data-init"))

		other := textareaOf(t, html.Composer(html.ComposerProps{Focus: true, Token: "AAAAAAAAAAAAAAAAAAAAAA", Message: "Refused."}))
		is.True(t, attribute(field, "data-init") != attribute(other, "data-init"))
	})

	t.Run("has the field take no focus otherwise", func(t *testing.T) {
		form := render(t, html.Composer(html.ComposerProps{Token: "q6urq6urq6urq6urq6urqw", Message: "Refused."}))
		is.True(t, !strings.Contains(form, "data-init"), form)
	})
}

func TestCommentForm(t *testing.T) {
	t.Run("has the field take the keyboard focus when it is put on the page, if asked to, unless the focus is elsewhere", func(t *testing.T) {
		field := textareaOf(t, html.CommentForm(html.CommentFormProps{PostID: "p_1", Focus: true, Token: "q6urq6urq6urq6urq6urqw"}))
		is.Equal(t, "'q6urq6urq6urq6urq6urqw' && (document.activeElement === document.body || el.form.contains(document.activeElement)) && el.focus()", attribute(field, "data-init"))

		other := textareaOf(t, html.CommentForm(html.CommentFormProps{PostID: "p_1", Focus: true, Token: "AAAAAAAAAAAAAAAAAAAAAA"}))
		is.True(t, attribute(field, "data-init") != attribute(other, "data-init"))
	})

	t.Run("has the field take no focus otherwise", func(t *testing.T) {
		form := render(t, html.CommentForm(html.CommentFormProps{PostID: "p_1", Token: "q6urq6urq6urq6urq6urqw", Message: "Refused."}))
		is.True(t, !strings.Contains(form, "data-init"), form)
	})
}

// textareaOf the given form, which has one.
func textareaOf(t *testing.T, form Node) *nethtml.Node {
	t.Helper()

	doc, err := nethtml.Parse(strings.NewReader(render(t, form)))
	is.NotError(t, err)
	var fields []*nethtml.Node
	for n := range doc.Descendants() {
		if n.Type == nethtml.ElementNode && n.Data == "textarea" {
			fields = append(fields, n)
		}
	}
	is.Equal(t, 1, len(fields))
	return fields[0]
}

func TestPrivatePage(t *testing.T) {
	t.Run("says only that the group is private", func(t *testing.T) {
		page := render(t, html.PrivatePage(html.PrivatePageProps{}))

		is.True(t, strings.Contains(page, "<title>regroup</title>"), page)
		is.True(t, regexp.MustCompile(`<main><div[^>]*><div[^>]*><div[^>]*>regroup</div><h1[^>]*>This group is private\.</h1></div></div></main>`).MatchString(page), page)
		is.True(t, !strings.Contains(page, "<a "), page)
		is.True(t, !strings.Contains(page, "<form"), page)
	})

	t.Run("puts what it is given under the sign", func(t *testing.T) {
		page := render(t, html.PrivatePage(html.PrivatePageProps{Extra: A(Href("/somewhere"), Text("Somewhere"))}))

		is.True(t, regexp.MustCompile(`<p[^>]*><a href="/somewhere">Somewhere</a></p></main>`).MatchString(page), page)
	})
}

func TestAnnouncement(t *testing.T) {
	t.Run("is the live region with the given words in it, in an element named after what they are about", func(t *testing.T) {
		is.Equal(t, `<p id="announcement" role="status" aria-live="polite" class="sr-only"><span id="announcement-p_1">Your post is on the timeline.</span></p>`,
			render(t, html.Announcement("p_1", "Your post is on the timeline.")))
	})

	t.Run("is the empty live region when there is nothing to say", func(t *testing.T) {
		is.Equal(t, `<p id="announcement" role="status" aria-live="polite" class="sr-only"></p>`, render(t, html.Announcement("", "")))
	})
}

func TestTimelinePosts(t *testing.T) {
	posts := []model.TimelinePost{
		{Post: model.Post{ID: "p_1", Created: model.Time{T: now}, Body: "Cake"}, AuthorName: "Signe"},
		{Post: model.Post{ID: "p_2", Created: model.Time{T: now}, Body: "More cake"}, AuthorName: "Thomas"},
	}

	t.Run("has the first post take the keyboard focus when it is put on the page, if asked to, and no other", func(t *testing.T) {
		doc, err := nethtml.Parse(strings.NewReader(render(t, html.TimelinePosts(posts, html.PostsProps{Focus: true}))))
		is.NotError(t, err)

		var articles []*nethtml.Node
		for n := range doc.Descendants() {
			if n.Type == nethtml.ElementNode && n.Data == "article" {
				articles = append(articles, n)
			}
		}
		is.Equal(t, 2, len(articles))
		is.Equal(t, "-1", attribute(articles[0], "tabindex"))
		// The focus moves unless the person has put it somewhere while waiting, such as in a field.
		is.Equal(t, "(document.activeElement === document.body || !!document.activeElement?.closest('#earlier')) && el.focus()", attribute(articles[0], "data-init"))
		is.Equal(t, "", attribute(articles[1], "tabindex"))
		is.Equal(t, "", attribute(articles[1], "data-init"))

		// The post that takes the focus is not outlined as a control would be.
		is.True(t, slices.Contains(strings.Fields(attribute(articles[0], "class")), "outline-none"), attribute(articles[0], "class"))
		is.True(t, !slices.Contains(strings.Fields(attribute(articles[1], "class")), "outline-none"), attribute(articles[1], "class"))
	})

	t.Run("says the comments are loading until they come, and hides the form to add one until then, so that the two show together", func(t *testing.T) {
		doc, err := nethtml.Parse(strings.NewReader(render(t, html.TimelinePosts(posts[:1], html.PostsProps{}))))
		is.NotError(t, err)

		var comments, form *nethtml.Node
		for n := range doc.Descendants() {
			switch {
			case n.Type != nethtml.ElementNode:
			case attribute(n, "id") == "post-p_1-comments":
				comments = n
			case attribute(n, "id") == "post-p_1-comment-form":
				form = n
			}
		}
		is.True(t, comments != nil && form != nil)

		_, loading := attributeOK(comments, "data-loading")
		is.True(t, loading, "the place for the comments is not marked as loading")
		is.Equal(t, "Loading comments", comments.FirstChild.FirstChild.Data)
		is.True(t, slices.Contains(strings.Fields(attribute(comments, "class")), "peer"), attribute(comments, "class"))
		is.True(t, form.PrevSibling == comments, "the form does not come right after the comments")
		is.True(t, slices.Contains(strings.Fields(attribute(form, "class")), "peer-data-loading:hidden"), attribute(form, "class"))
	})

	t.Run("has no post take the focus otherwise", func(t *testing.T) {
		page := render(t, html.TimelinePosts(posts, html.PostsProps{}))
		is.True(t, !strings.Contains(page, "tabindex"), page)
		is.True(t, !strings.Contains(page, "data-init"), page)
	})
}

func TestTimelineDay(t *testing.T) {
	tests := []struct {
		name     string
		day      model.TimelineDay
		expected string
	}{
		{name: "today as Today, with the date beside it", day: model.TimelineDay{Date: model.Date{Year: 2031, Month: 3, Day: 14}, Today: true}, expected: "Today Friday 14 March"},
		{name: "yesterday as Yesterday, with the date beside it", day: model.TimelineDay{Date: model.Date{Year: 2031, Month: 3, Day: 13}, Yesterday: true}, expected: "Yesterday Thursday 13 March"},
		{name: "another day of this year with its weekday, day, and month", day: model.TimelineDay{Date: model.Date{Year: 2031, Month: 1, Day: 2}}, expected: "Thursday 2 January"},
		{name: "a day of another year with the year too", day: model.TimelineDay{Date: model.Date{Year: 2030, Month: 12, Day: 31}}, expected: "Tuesday 31 December 2030"},
		{name: "yesterday in another year with the year too", day: model.TimelineDay{Date: model.Date{Year: 2030, Month: 12, Day: 31}, Yesterday: true}, expected: "Yesterday Tuesday 31 December 2030"},
	}

	for _, test := range tests {
		t.Run("heads "+test.name, func(t *testing.T) {
			section := render(t, html.TimelineDay(test.day, now, html.PostsProps{}))

			heading := regexp.MustCompile(`<h2 id="day-` + test.day.Date.String() + `-heading"[^>]*>(.*)</h2>`).FindStringSubmatch(section)
			is.True(t, heading != nil, section)
			is.Equal(t, test.expected, regexp.MustCompile(`<[^>]*>`).ReplaceAllString(heading[1], ""))

			// The heading alone is the same heading.
			is.True(t, strings.Contains(section, render(t, html.DayHeading(test.day, now))), section)

			// The first word is in the serif, and the rest beside it in the sans on a phone, as the date
			// beside Today and Yesterday is everywhere. A date written out is one line of serif on a wider
			// screen.
			rest := regexp.MustCompile(`^[^<]* <span class="([^"]*)">([^<]*)</span>$`).FindStringSubmatch(heading[1])
			is.True(t, rest != nil, heading[1])
			is.Equal(t, test.expected[strings.Index(test.expected, " ")+1:], rest[2])
			class := strings.Fields(rest[1])
			if test.day.Today || test.day.Yesterday {
				is.True(t, slices.Contains(class, "font-sans"), rest[1])
				return
			}
			for _, c := range []string{"max-sm:font-sans", "max-sm:text-stone-600", "max-sm:whitespace-nowrap"} {
				is.True(t, slices.Contains(class, c), rest[1])
			}
			is.True(t, !slices.Contains(class, "font-sans"), rest[1])
		})
	}
}

func TestComments(t *testing.T) {
	t.Run("fades in when they are put on the page, and only for someone who has not asked for less motion", func(t *testing.T) {
		list := render(t, html.Comments(html.CommentsProps{PostID: "p_1", Now: now, Comments: []model.TimelineComment{
			{Comment: model.Comment{ID: "c_1", Created: model.Time{T: now}, Body: "Hello"}, AuthorName: "Signe"},
		}}))
		is.True(t, regexp.MustCompile(`<div id="post-p_1-comments" class="peer"><ol class="[^"]*\bmotion-safe:animate-fade-in\b[^"]*">`).MatchString(list), list)

		// Nothing else on any page moves.
		for _, path := range []string{"common.go", "group.go"} {
			source, err := os.ReadFile(path)
			is.NotError(t, err)
			for _, class := range regexp.MustCompile(`[\w:-]*(?:animate|transition|duration)-[\w-]*`).FindAllString(string(source), -1) {
				is.Equal(t, "motion-safe:animate-fade-in", class)
			}
		}
	})

	t.Run("is an empty place when there are no comments, which no longer says they are loading", func(t *testing.T) {
		is.Equal(t, `<div id="post-p_1-comments" class="peer"></div>`, render(t, html.Comments(html.CommentsProps{PostID: "p_1", Now: now})))
	})

	t.Run("shows the time of a comment alone when it is from today, and with its day when it is not", func(t *testing.T) {
		at := func(t time.Time) model.TimelineComment {
			return model.TimelineComment{Comment: model.Comment{ID: "c_1", Created: model.Time{T: t}, Body: "Hello"}, AuthorName: "Signe"}
		}

		page := render(t, html.Comments(html.CommentsProps{PostID: "p_1", Now: now, Comments: []model.TimelineComment{
			at(time.Date(2030, 12, 31, 23, 59, 0, 0, time.UTC)),
			at(time.Date(2031, 3, 12, 8, 5, 0, 0, time.UTC)),
			at(time.Date(2031, 3, 13, 23, 59, 0, 0, time.UTC)),
			at(time.Date(2031, 3, 14, 0, 0, 0, 0, time.UTC)),
		}}))

		var times []string
		for _, match := range regexp.MustCompile(`<time[^>]*>([^<]*)</time>`).FindAllStringSubmatch(page, -1) {
			times = append(times, match[1])
		}
		is.EqualSlice(t, []string{"31 December 2030 23:59", "12 March 08:05", "Yesterday 23:59", "00:00"}, times)
	})
}

func TestEarlierPosts(t *testing.T) {
	t.Run("sends the cursor and the days in the body of a post, and has no button at the end of the timeline", func(t *testing.T) {
		form := render(t, html.EarlierPosts(html.EarlierPostsProps{Next: "abc", Days: []model.Date{{Year: 2031, Month: 3, Day: 14}, {Year: 2031, Month: 3, Day: 13}}}))
		is.True(t, strings.Contains(form, `<form id="earlier" method="post" action="/posts/earlier"`), form)
		is.True(t, strings.Contains(form, `<input type="hidden" name="after" value="abc">`), form)
		is.True(t, strings.Contains(form, `<input type="hidden" name="days" value="2031-03-14,2031-03-13">`), form)
		is.True(t, regexp.MustCompile(`<button type="submit"[^>]*>Earlier posts</button>`).MatchString(form), form)

		form = render(t, html.EarlierPosts(html.EarlierPostsProps{Days: []model.Date{{Year: 2031, Month: 3, Day: 14}}}))
		is.True(t, strings.Contains(form, `<form id="earlier"`), form)
		is.True(t, !strings.Contains(form, "<button") && !strings.Contains(form, "<input"), form)
	})

	t.Run("says on its button that earlier posts are loading while they are on their way", func(t *testing.T) {
		form := render(t, html.EarlierPosts(html.EarlierPostsProps{Next: "abc", Posts: true}))
		is.True(t, strings.Contains(form, ` data-indicator="_loadingEarlier"`), form)
		is.True(t, regexp.MustCompile(`<button type="submit"[^>]* data-text="\$_loadingEarlier \? &#39;Loading earlier posts&#39; : &#39;Earlier posts&#39;"[^>]*>Earlier posts</button>`).MatchString(form), form)
		is.True(t, !strings.Contains(form, "Nothing was posted before this."), form)
	})

	t.Run("ends a timeline that has posts with a line that says nothing was posted before, and an empty one with nothing", func(t *testing.T) {
		form := render(t, html.EarlierPosts(html.EarlierPostsProps{Posts: true}))
		is.True(t, regexp.MustCompile(`<p class="text-stone-600">Nothing was posted before this\.</p></form>$`).MatchString(form), form)

		form = render(t, html.EarlierPosts(html.EarlierPostsProps{}))
		is.True(t, !strings.Contains(form, "Nothing was posted"), form)
	})
}
