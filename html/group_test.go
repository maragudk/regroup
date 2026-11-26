package html_test

import (
	"regexp"
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

	t.Run("says that a request failed in a place that is hidden until one does", func(t *testing.T) {
		page := render(t, html.GroupPage(html.GroupPageProps{GroupName: "The Loft", Now: now}))

		is.True(t, regexp.MustCompile(`<p id="request-failed" role="alert" class="[^"]*\bhidden\b[^"]*"[^>]*data-class="\{hidden: !\$_failed\}"[^>]*>That did not go through\. Check your connection and try again\.</p>`).MatchString(page), page)
	})

	t.Run("shows any body that can be posted as exactly that text, in a post, in a comment, and in a field, and never as an element", func(t *testing.T) {
		elements := func(body string) (int, *nethtml.Node) {
			post := model.TimelinePost{Post: model.Post{ID: "p_1", Created: model.Time{T: now}, Body: body}, AuthorName: "Signe"}
			comment := model.TimelineComment{Comment: model.Comment{ID: "c_1", Created: model.Time{T: now}, PostID: "p_1", Body: body}, AuthorName: "Thomas"}

			page := render(t, Group{
				html.GroupPage(html.GroupPageProps{
					GroupName: "The Loft",
					Now:       now,
					Timeline:  model.Timeline{Days: []model.TimelineDay{{Date: model.DateOf(now), Today: true, Posts: []model.TimelinePost{post}}}},
					Composer:  html.ComposerProps{Body: body, Message: "Refused."},
				}),
				html.Comments(html.CommentsProps{PostID: "p_1", Comments: []model.TimelineComment{comment}, Now: now, Form: html.CommentFormProps{Body: body, Message: "Refused."}}),
			})

			doc, err := nethtml.Parse(strings.NewReader(page))
			is.NotError(t, err)

			var count int
			for n := range doc.Descendants() {
				if n.Type == nethtml.ElementNode {
					count++
				}
			}
			return count, doc
		}

		plain, _ := elements("Cake")

		hegel.Test(t, func(ht *hegel.T) {
			body, err := model.CleanPostBody(hegel.Draw(ht, markup()))
			ht.Assume(err == nil)

			count, doc := elements(body)
			if count != plain {
				ht.Fatalf("the body %q changed the number of elements on the page from %v to %v", body, plain, count)
			}

			var bodies, fields int
			for n := range doc.Descendants() {
				if n.Type != nethtml.ElementNode {
					continue
				}

				var isBody bool
				for _, a := range n.Attr {
					isBody = isBody || a.Key == "data-body"
				}
				if !isBody && n.Data != "textarea" {
					continue
				}

				if n.FirstChild == nil || n.FirstChild != n.LastChild || n.FirstChild.Type != nethtml.TextNode {
					ht.Fatalf("the body %q did not come out as one piece of text in %v", body, n.Data)
				}
				if n.FirstChild.Data != body {
					ht.Fatalf("the body %q came out as %q in %v", body, n.FirstChild.Data, n.Data)
				}

				if isBody {
					bodies++
				} else {
					fields++
				}
			}
			if bodies != 2 || fields != 2 {
				ht.Fatalf("expected the body %q in a post, a comment, and two fields, but found it in %v and %v", body, bodies, fields)
			}
		})
	})
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
		}),
		hegel.Text().MaxSize(20),
	)

	return hegel.Composite(func(tc hegel.TestCase) string {
		return strings.Join(hegel.Draw(tc, hegel.Lists(piece).MinSize(1).MaxSize(8)), "")
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
}

func TestPrivatePage(t *testing.T) {
	t.Run("says only that the group is private", func(t *testing.T) {
		page := render(t, html.PrivatePage(html.PrivatePageProps{}))

		is.True(t, strings.Contains(page, "<title>regroup</title>"), page)
		is.True(t, regexp.MustCompile(`<h1[^>]*>This group is private\.</h1>`).MatchString(page), page)
		is.True(t, !strings.Contains(page, "<main"), page)
		is.True(t, !strings.Contains(page, "<a "), page)
		is.True(t, !strings.Contains(page, "<form"), page)
	})

	t.Run("puts what it is given under the sign", func(t *testing.T) {
		page := render(t, html.PrivatePage(html.PrivatePageProps{Extra: A(Href("/somewhere"), Text("Somewhere"))}))

		is.True(t, strings.Contains(page, `<p><a href="/somewhere">Somewhere</a></p>`), page)
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
			section := render(t, html.TimelineDay(test.day, now))

			heading := regexp.MustCompile(`<h2[^>]*>(.*)</h2>`).FindStringSubmatch(section)
			is.True(t, heading != nil, section)
			is.Equal(t, test.expected, regexp.MustCompile(`<[^>]*>`).ReplaceAllString(heading[1], ""))
		})
	}
}

func TestComments(t *testing.T) {
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
	t.Run("sends the cursor in the body of a post, and has no button at the end of the timeline", func(t *testing.T) {
		form := render(t, html.EarlierPosts("abc"))
		is.True(t, strings.Contains(form, `<form id="earlier" method="post" action="/posts/earlier"`), form)
		is.True(t, strings.Contains(form, `<input type="hidden" name="after" value="abc">`), form)
		is.True(t, regexp.MustCompile(`<button type="submit"[^>]*>Earlier posts</button>`).MatchString(form), form)

		form = render(t, html.EarlierPosts(""))
		is.True(t, strings.Contains(form, `<form id="earlier"`), form)
		is.True(t, !strings.Contains(form, "<button") && !strings.Contains(form, "<input"), form)
	})
}
