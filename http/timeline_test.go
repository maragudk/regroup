package http_test

import (
	"bytes"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	nethtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	gluehttp "maragu.dev/glue/http"
	. "maragu.dev/gomponents"
	. "maragu.dev/gomponents/html"
	"maragu.dev/httph"
	"maragu.dev/is"

	"app/html"
	apphttp "app/http"
	"app/model"
	"app/service"
	"app/sqlite"
	"app/sqlitetest"
)

// base is a Friday morning that is not today, so that a test can tell the clock it gives from the real
// one. It is 10:00 in the group's time zone.
var base = time.Date(2031, 3, 14, 9, 0, 0, 0, time.UTC)

// requested matches the path in each request that a Datastar expression makes.
var requested = regexp.MustCompile(`@(?:get|post|put|patch|delete)\(([^,)]*)`)

func TestTimeline(t *testing.T) {
	t.Run("shows someone who is not signed in only that the group is private", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		post := a.createPost(t, signe, base.Add(-time.Hour), "Cake in the kitchen.")
		a.createComment(t, signe, post, base.Add(-time.Minute), "It is carrot.")

		// A member has just had the page, with everything on it.
		member := a.get(t, a.signIn(t, signe), "/").Body.String()
		is.True(t, strings.Contains(member, "Cake in the kitchen."), member)

		res := a.get(t, nil, "/")
		is.Equal(t, http.StatusOK, res.Code)

		page := res.Body.String()
		doc := parse(t, page)
		is.Equal(t, "This group is private.", text(one(t, doc, tag("h1"))))
		is.Equal(t, "regroup", text(one(t, doc, tag("title"))))
		for _, secret := range []string{"The Loft", "Cake", "carrot", "Signe", "member", post.ID.String()} {
			is.True(t, !strings.Contains(page, secret), "the page has "+secret+" in it:\n"+page)
		}
		is.Equal(t, 0, len(all(doc, tag("form"))))
		is.Equal(t, 0, len(all(doc, tag("a"))))
	})

	t.Run("gives someone who is not signed in nothing of the group from any route, and does nothing for them", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		post := a.createPost(t, signe, base.Add(-time.Hour), "Cake in the kitchen.")
		a.createComment(t, signe, post, base.Add(-time.Minute), "It is carrot.")

		requests := map[string]url.Values{
			"/posts":         {"body": {"Hello"}},
			"/posts/earlier": {"after": {model.TimelineCursor{Created: model.Time{T: base}, Seq: 5}.String()}},
			"/comments/open": {"post": {post.ID.String()}},
			"/comments":      {"post": {post.ID.String()}, "body": {"Hello"}},
		}
		for path, form := range requests {
			for _, datastar := range []bool{true, false} {
				res := a.post(t, nil, path, form, datastar)
				is.Equal(t, http.StatusUnauthorized, res.Code, path)

				body := res.Body.String()
				for _, secret := range []string{"The Loft", "Cake", "carrot", "Signe"} {
					is.True(t, !strings.Contains(body, secret), "the answer from "+path+" has "+secret+" in it:\n"+body)
				}
			}
		}

		is.Equal(t, 1, a.count(t, "posts"))
		is.Equal(t, 1, a.count(t, "comments"))
	})

	t.Run("treats someone who is no longer active as not signed in", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		amira := sqlitetest.CreateUser(t, a.db, "Amira")
		a.createPost(t, signe, base.Add(-time.Hour), "Cake in the kitchen.")
		cookie := a.signIn(t, amira)
		is.Equal(t, "The Loft", text(one(t, parse(t, a.get(t, cookie, "/").Body.String()), tag("h1"))))

		sqlitetest.DeactivateUser(t, a.db, amira.ID)

		res := a.get(t, cookie, "/")
		is.Equal(t, http.StatusOK, res.Code)
		page := res.Body.String()
		is.Equal(t, "This group is private.", text(one(t, parse(t, page), tag("h1"))))
		for _, secret := range []string{"The Loft", "Cake", "Signe", "Amira"} {
			is.True(t, !strings.Contains(page, secret), "the page has "+secret+" in it:\n"+page)
		}

		res = a.post(t, cookie, "/posts", url.Values{"body": {"Hello"}}, true)
		is.Equal(t, http.StatusUnauthorized, res.Code)
		is.Equal(t, 1, a.count(t, "posts"))
	})

	t.Run("shows the group's name as the title and on the sign, with how many members it has and who the page is for", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		sqlitetest.CreateUser(t, a.db, "Thomas")
		amira := sqlitetest.CreateUser(t, a.db, "Amira")
		sqlitetest.DeactivateUser(t, a.db, amira.ID)

		res := a.get(t, a.signIn(t, signe), "/")
		is.Equal(t, http.StatusOK, res.Code)

		doc := parse(t, res.Body.String())
		is.Equal(t, "The Loft", text(one(t, doc, tag("title"))))
		is.Equal(t, "The Loft", text(one(t, doc, tag("h1"))))
		sign := text(one(t, doc, tag("header")))
		is.True(t, strings.Contains(sign, "2 members"), sign)
		is.True(t, strings.Contains(sign, "Signe"), sign)
		is.True(t, !strings.Contains(sign, "Thomas"), sign)
	})

	t.Run("says so when nothing has been posted, and points at the composer", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")

		doc := parse(t, a.get(t, a.signIn(t, signe), "/").Body.String())

		timeline := one(t, doc, id("timeline"))
		is.Equal(t, "Nothing has been posted yet. The first post goes in the field above.", text(timeline))
		is.Equal(t, 0, len(all(doc, tag("h2"))))
		is.Equal(t, 0, len(all(doc, tag("article"))))
		is.Equal(t, "", attr(one(t, one(t, doc, id("composer")), named("newest")), "value"))
		is.Equal(t, 0, len(all(one(t, doc, id("earlier")), tag("button"))))
	})

	t.Run("shows posts newest first under the heading of their day, with their author and the time in the group's time zone", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		thomas := sqlitetest.CreateUser(t, a.db, "Thomas")
		a.createPost(t, thomas, time.Date(2030, 12, 31, 12, 0, 0, 0, time.UTC), "Happy new year, nearly.")
		a.createPost(t, signe, time.Date(2031, 3, 10, 7, 5, 0, 0, time.UTC), "Monday.")
		a.createPost(t, thomas, time.Date(2031, 3, 13, 15, 5, 0, 0, time.UTC), "Found an umbrella.")
		// 23:30 UTC on Thursday is 00:30 on Friday in the group's time zone.
		a.createPost(t, signe, time.Date(2031, 3, 13, 23, 30, 0, 0, time.UTC), "Still up.")
		a.createPost(t, thomas, time.Date(2031, 3, 14, 8, 40, 0, 0, time.UTC), "Cake in the kitchen.\n\nIt is carrot.")

		doc := parse(t, a.get(t, a.signIn(t, signe), "/").Body.String())

		is.EqualSlice(t, []string{
			"Today Friday 14 March",
			"Yesterday Thursday 13 March",
			"Monday 10 March",
			"Tuesday 31 December 2030",
		}, texts(all(doc, tag("h2"))))

		sections := all(doc, tag("section"))
		is.Equal(t, 4, len(sections))
		is.EqualSlice(t, []string{"Thomas", "Signe"}, texts(all(sections[0], tag("h3"))))
		is.EqualSlice(t, []string{"09:40", "00:30"}, texts(all(sections[0], tag("time"))))
		is.EqualSlice(t, []string{"Cake in the kitchen.\n\nIt is carrot.", "Still up."}, texts(all(sections[0], hasAttr("data-body"))))
		is.EqualSlice(t, []string{"Found an umbrella."}, texts(all(sections[1], hasAttr("data-body"))))
		is.EqualSlice(t, []string{"16:05"}, texts(all(sections[1], tag("time"))))
		is.EqualSlice(t, []string{"Monday."}, texts(all(sections[2], hasAttr("data-body"))))
		is.EqualSlice(t, []string{"Happy new year, nearly."}, texts(all(sections[3], hasAttr("data-body"))))

		is.Equal(t, "2031-03-14", attr(one(t, one(t, doc, id("composer")), named("newest")), "value"))
	})

	t.Run("has its headings in order: the group, then the days, then the authors", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		a.createPost(t, signe, base.Add(-25*time.Hour), "Yesterday's.")
		a.createPost(t, signe, base.Add(-time.Hour), "Today's.")

		doc := parse(t, a.get(t, a.signIn(t, signe), "/").Body.String())

		var headings []string
		for _, n := range all(doc, func(n *nethtml.Node) bool { return regexp.MustCompile(`^h[1-6]$`).MatchString(n.Data) }) {
			headings = append(headings, n.Data)
		}
		is.EqualSlice(t, []string{"h1", "h2", "h3", "h2", "h3"}, headings)
	})

	t.Run("labels every field", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		post := a.createPost(t, signe, base.Add(-time.Hour), "Cake.")
		cookie := a.signIn(t, signe)

		doc := parse(t, a.get(t, cookie, "/").Body.String())
		field := one(t, one(t, doc, id("composer")), tag("textarea"))
		is.Equal(t, "Write to the group", text(one(t, doc, func(n *nethtml.Node) bool { return n.Data == "label" && attr(n, "for") == attr(field, "id") })))

		opened := a.patches(t, a.post(t, cookie, "/comments/open", url.Values{"post": {post.ID.String()}}, true))
		field = one(t, opened[0], tag("textarea"))
		is.Equal(t, "Write a comment", text(one(t, opened[0], func(n *nethtml.Node) bool { return n.Data == "label" && attr(n, "for") == attr(field, "id") })))
	})

	t.Run("marks what is new to the reader, and a reload uses the marks on posts up", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		thomas := sqlitetest.CreateUser(t, a.db, "Thomas")
		old := a.createPost(t, thomas, base.Add(-48*time.Hour), "An old post.")
		cookie := a.signIn(t, signe)
		a.get(t, cookie, "/")
		a.get(t, cookie, "/")

		a.createPost(t, signe, base.Add(-3*time.Hour), "My own post.")
		fresh := a.createPost(t, thomas, base.Add(-2*time.Hour), "A new post.")
		a.createComment(t, thomas, old, base.Add(-time.Hour), "One.")
		a.createComment(t, thomas, old, base.Add(-time.Minute), "Two.")
		a.createComment(t, signe, old, base.Add(-time.Second), "Three, my own.")

		doc := parse(t, a.get(t, cookie, "/").Body.String())
		is.EqualSlice(t, []string{"new"}, texts(all(one(t, doc, id("post-"+fresh.ID.String())), mark)))
		is.Equal(t, "Comment", text(one(t, doc, id("post-"+fresh.ID.String()+"-comments-summary"))))
		is.EqualSlice(t, []string{"2 new"}, texts(all(one(t, doc, id("post-"+old.ID.String())), mark)))
		is.Equal(t, "3 comments2 new", text(one(t, doc, id("post-"+old.ID.String()+"-comments-summary"))))
		is.Equal(t, 2, len(all(doc, mark)))

		doc = parse(t, a.get(t, cookie, "/").Body.String())
		is.Equal(t, 0, len(all(one(t, doc, id("post-"+fresh.ID.String())), mark)))
		is.EqualSlice(t, []string{"2 new"}, texts(all(doc, mark)))
	})

	t.Run("keeps one reader's marks out of what another reader gets", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		thomas := sqlitetest.CreateUser(t, a.db, "Thomas")
		amira := sqlitetest.CreateUser(t, a.db, "Amira")
		post := a.createPost(t, amira, base.Add(-2*time.Hour), "The window will not close.")
		a.createComment(t, amira, post, base.Add(-time.Hour), "I have taped it.")
		signeCookie, thomasCookie, amiraCookie := a.signIn(t, signe), a.signIn(t, thomas), a.signIn(t, amira)

		// Signe looks twice and opens the comments, which uses up every mark of hers.
		a.get(t, signeCookie, "/")
		a.get(t, signeCookie, "/")
		opened := a.patches(t, a.post(t, signeCookie, "/comments/open", url.Values{"post": {post.ID.String()}}, true))
		is.EqualSlice(t, []string{"new"}, texts(all(opened[0], mark)))
		is.Equal(t, 0, len(all(parse(t, a.get(t, signeCookie, "/").Body.String()), mark)))

		// Thomas has not looked, so everything is still new to him, and to him alone.
		doc := parse(t, a.get(t, thomasCookie, "/").Body.String())
		is.EqualSlice(t, []string{"new", "1 new"}, texts(all(doc, mark)))
		opened = a.patches(t, a.post(t, thomasCookie, "/comments/open", url.Values{"post": {post.ID.String()}}, true))
		is.EqualSlice(t, []string{"new"}, texts(all(opened[0], mark)))

		// Nothing Amira wrote is ever new to her.
		is.Equal(t, 0, len(all(parse(t, a.get(t, amiraCookie, "/").Body.String()), mark)))
		opened = a.patches(t, a.post(t, amiraCookie, "/comments/open", url.Values{"post": {post.ID.String()}}, true))
		is.Equal(t, 0, len(all(opened[0], mark)))
	})

	t.Run("shows no control for anything that is not built", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		a.createPost(t, signe, base.Add(-time.Hour), "Cake.")

		doc := parse(t, a.get(t, a.signIn(t, signe), "/").Body.String())

		is.Equal(t, 0, len(all(doc, tag("a"))))
		is.EqualSlice(t, []string{"Post"}, texts(all(doc, tag("button"))))
		is.EqualSlice(t, []string{"/posts", "/comments/open", "/posts/earlier"}, attrs(all(doc, tag("form")), "action"))
	})

	t.Run("answers a path that is not there with a page that has nothing of the group on it", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		a.createPost(t, signe, base.Add(-time.Hour), "Cake.")

		res := a.get(t, nil, "/nope")
		is.Equal(t, http.StatusNotFound, res.Code)
		page := res.Body.String()
		is.Equal(t, "Not found", text(one(t, parse(t, page), tag("h1"))))
		for _, secret := range []string{"The Loft", "Cake", "Signe"} {
			is.True(t, !strings.Contains(page, secret), page)
		}
	})
}

func TestTimeline_post(t *testing.T) {
	t.Run("posts what was written, and answers with the post under the heading of today that the page has, and with an empty composer", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		cookie := a.signIn(t, signe)

		res := a.post(t, cookie, "/posts", url.Values{"body": {"  Cake in the kitchen.\r\n\r\nIt is carrot.  "}, "newest": {"2031-03-14"}}, true)
		is.Equal(t, http.StatusOK, res.Code)
		is.Equal(t, "text/event-stream", res.Header().Get("Content-Type"))

		decoded, patches := a.decode(t, res), a.patches(t, res)
		is.Equal(t, 2, len(decoded))

		is.Equal(t, "", decoded[0].Selector)
		is.Equal(t, "", decoded[0].Mode)
		composer := one(t, patches[0], id("composer"))
		is.Equal(t, "", text(one(t, composer, tag("textarea"))))
		is.Equal(t, "2031-03-14", attr(one(t, composer, named("newest")), "value"))
		is.Equal(t, 0, len(all(composer, role("alert"))))

		is.Equal(t, "#day-2031-03-14-posts", decoded[1].Selector)
		is.Equal(t, "prepend", decoded[1].Mode)
		is.Equal(t, 0, len(all(patches[1], tag("h2"))))
		article := one(t, patches[1], tag("article"))
		is.Equal(t, "Signe", text(one(t, article, tag("h3"))))
		is.Equal(t, "10:00", text(one(t, article, tag("time"))))
		is.Equal(t, "Cake in the kitchen.\n\nIt is carrot.", text(one(t, article, hasAttr("data-body"))))
		is.Equal(t, 0, len(all(article, mark)))

		doc := parse(t, a.get(t, cookie, "/").Body.String())
		is.EqualSlice(t, []string{"Cake in the kitchen.\n\nIt is carrot."}, texts(all(doc, hasAttr("data-body"))))
		is.EqualSlice(t, []string{"Today Friday 14 March"}, texts(all(doc, tag("h2"))))
	})

	t.Run("gives the composer a field of a new identity after a post, so that a page does not keep what was posted in it", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		cookie := a.signIn(t, signe)

		before := one(t, one(t, parse(t, a.get(t, cookie, "/").Body.String()), id("composer")), tag("textarea"))

		composer := one(t, a.patches(t, a.post(t, cookie, "/posts", url.Values{"body": {"Cake."}}, true))[0], id("composer"))
		after := one(t, composer, tag("textarea"))
		is.True(t, attr(before, "id") != attr(after, "id"), "the field kept its ID "+attr(after, "id"))
		is.Equal(t, attr(after, "id"), attr(one(t, composer, tag("label")), "for"))
		is.Equal(t, attr(after, "id"), "composer-body-"+attr(one(t, composer, named("field")), "value"))
	})

	t.Run("brings the heading of today along when the page has an older day at its top", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")

		res := a.post(t, a.signIn(t, signe), "/posts", url.Values{"body": {"Cake."}, "newest": {"2031-03-13"}}, true)

		decoded, patches := a.decode(t, res), a.patches(t, res)
		is.Equal(t, "#timeline", decoded[1].Selector)
		is.Equal(t, "prepend", decoded[1].Mode)
		section := one(t, patches[1], tag("section"))
		is.Equal(t, "Today Friday 14 March", text(one(t, section, tag("h2"))))
		is.Equal(t, "Cake.", text(one(t, one(t, section, id("day-2031-03-14-posts")), hasAttr("data-body"))))
		is.Equal(t, "2031-03-14", attr(one(t, patches[0], named("newest")), "value"))
	})

	t.Run("puts the first post in the place of the words that say the timeline is empty", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")

		res := a.post(t, a.signIn(t, signe), "/posts", url.Values{"body": {"Cake."}, "newest": {""}}, true)

		decoded, patches := a.decode(t, res), a.patches(t, res)
		is.Equal(t, "#timeline", decoded[1].Selector)
		is.Equal(t, "inner", decoded[1].Mode)
		is.Equal(t, "Today Friday 14 March", text(one(t, patches[1], tag("h2"))))
		is.Equal(t, "Cake.", text(one(t, patches[1], hasAttr("data-body"))))
	})

	t.Run("does not count as a look at the timeline", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		thomas := sqlitetest.CreateUser(t, a.db, "Thomas")
		a.createPost(t, thomas, base.Add(-time.Hour), "New to Signe.")
		cookie := a.signIn(t, signe)

		res := a.post(t, cookie, "/posts", url.Values{"body": {"Cake."}, "newest": {"2031-03-14"}}, true)
		is.Equal(t, http.StatusOK, res.Code)

		// Had posting been a look, this would be the second one, and the mark would be gone.
		doc := parse(t, a.get(t, cookie, "/").Body.String())
		is.EqualSlice(t, []string{"new"}, texts(all(doc, mark)))
	})

	for _, test := range []struct {
		name    string
		body    string
		message string
	}{
		{name: "an empty body", body: " \r\n ", message: "Write something first."},
		{name: "a body that is too long", body: strings.Repeat("æ", model.PostBodyMaxLength+1), message: "That is too long. A post can be up to 10,000 characters."},
		{name: "a body with a character that cannot be posted", body: "Cake\x00please", message: "That text has characters that cannot be used here. Remove them and try again."},
	} {
		t.Run("refuses "+test.name+" with a message beside the field, and keeps what was typed", func(t *testing.T) {
			a := newApp(t)
			signe := sqlitetest.CreateUser(t, a.db, "Signe")

			res := a.post(t, a.signIn(t, signe), "/posts", url.Values{"body": {test.body}, "newest": {"2031-03-13"}, "field": {"p_before"}}, true)
			is.Equal(t, http.StatusOK, res.Code)

			decoded, patches := a.decode(t, res), a.patches(t, res)
			is.Equal(t, 1, len(decoded))
			composer := one(t, patches[0], id("composer"))
			alert := one(t, composer, role("alert"))
			is.Equal(t, test.message, text(alert))

			// A browser reading HTML makes a line feed of a carriage return and a line feed, and shows
			// the replacement character for a NUL.
			field := one(t, composer, tag("textarea"))
			typed := strings.ReplaceAll(strings.ReplaceAll(test.body, "\r\n", "\n"), "\x00", "\ufffd")
			is.Equal(t, typed, text(field))
			is.Equal(t, "true", attr(field, "aria-invalid"))
			is.Equal(t, attr(alert, "id"), attr(field, "aria-describedby"))
			is.Equal(t, "composer-body-p_before", attr(field, "id"))
			is.Equal(t, "2031-03-13", attr(one(t, composer, named("newest")), "value"))

			is.Equal(t, 0, a.count(t, "posts"))
		})
	}

	t.Run("refuses a request larger than any post as too long, without reading it all", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")

		res := a.post(t, a.signIn(t, signe), "/posts", url.Values{"body": {strings.Repeat("a", 1<<20)}}, true)
		is.Equal(t, http.StatusOK, res.Code)

		composer := one(t, a.patches(t, res)[0], id("composer"))
		is.Equal(t, "That is too long. A post can be up to 10,000 characters.", text(one(t, composer, role("alert"))))
		is.Equal(t, 0, a.count(t, "posts"))
	})

	t.Run("answers a browser that does not run the page's script with a redirect to the group page, or a page that says why not", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		cookie := a.signIn(t, signe)

		res := a.post(t, cookie, "/posts", url.Values{"body": {"Cake."}}, false)
		is.Equal(t, http.StatusSeeOther, res.Code)
		is.Equal(t, "/", res.Header().Get("Location"))
		is.Equal(t, 1, a.count(t, "posts"))

		res = a.post(t, cookie, "/posts", url.Values{"body": {" "}}, false)
		is.Equal(t, http.StatusUnprocessableEntity, res.Code)
		doc := parse(t, res.Body.String())
		is.Equal(t, "That was not posted", text(one(t, doc, tag("h1"))))
		is.True(t, strings.Contains(text(doc), "Write something first. Go back to try again."), text(doc))
		is.Equal(t, 1, a.count(t, "posts"))
	})
}

func TestTimeline_earlier(t *testing.T) {
	t.Run("loads the page after the cursor, and puts the posts of a day that goes on under the heading the page already has", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		thomas := sqlitetest.CreateUser(t, a.db, "Thomas")
		// Five posts on Wednesday, twenty on Thursday, and five on Friday, so that the first page of
		// twenty ends in the middle of Thursday.
		for i := range 5 {
			a.createPost(t, thomas, time.Date(2031, 3, 12, 9, i, 0, 0, time.UTC), fmt.Sprint("Wednesday ", i))
		}
		for i := range 20 {
			a.createPost(t, thomas, time.Date(2031, 3, 13, 9, i, 0, 0, time.UTC), fmt.Sprint("Thursday ", i))
		}
		for i := range 5 {
			a.createPost(t, thomas, time.Date(2031, 3, 14, 7, i, 0, 0, time.UTC), fmt.Sprint("Friday ", i))
		}
		cookie := a.signIn(t, signe)

		doc := parse(t, a.get(t, cookie, "/").Body.String())
		is.EqualSlice(t, []string{"Today Friday 14 March", "Yesterday Thursday 13 March"}, texts(all(doc, tag("h2"))))
		is.Equal(t, 20, len(all(doc, tag("article"))))
		is.Equal(t, 20, len(all(doc, mark)))
		earlier := one(t, doc, id("earlier"))
		is.Equal(t, "Earlier posts", text(one(t, earlier, tag("button"))))
		after := attr(one(t, earlier, named("after")), "value")

		res := a.post(t, cookie, "/posts/earlier", url.Values{"after": {after}}, true)
		is.Equal(t, http.StatusOK, res.Code)
		decoded, patches := a.decode(t, res), a.patches(t, res)
		is.Equal(t, 3, len(decoded))

		// The rest of Thursday goes under the heading that is already on the page.
		is.Equal(t, "#day-2031-03-13-posts", decoded[0].Selector)
		is.Equal(t, "append", decoded[0].Mode)
		is.Equal(t, 0, len(all(patches[0], tag("h2"))))
		is.EqualSlice(t, []string{"Thursday 4", "Thursday 3", "Thursday 2", "Thursday 1", "Thursday 0"}, texts(all(patches[0], hasAttr("data-body"))))

		// Wednesday is a new day on the page, so it brings its heading.
		is.Equal(t, "#timeline", decoded[1].Selector)
		is.Equal(t, "append", decoded[1].Mode)
		is.EqualSlice(t, []string{"Wednesday 12 March"}, texts(all(patches[1], tag("h2"))))
		is.EqualSlice(t, []string{"Wednesday 4", "Wednesday 3", "Wednesday 2", "Wednesday 1", "Wednesday 0"}, texts(all(patches[1], hasAttr("data-body"))))

		// Every heading is on the two pages together exactly once.
		headings := texts(all(doc, tag("h2")))
		for _, p := range patches {
			headings = append(headings, texts(all(p, tag("h2")))...)
		}
		is.EqualSlice(t, []string{"Today Friday 14 March", "Yesterday Thursday 13 March", "Wednesday 12 March"}, headings)

		// That was the end of the timeline, so the control to load more goes away.
		is.Equal(t, "", decoded[2].Selector)
		earlier = one(t, patches[2], id("earlier"))
		is.Equal(t, 0, len(all(earlier, tag("button"))))
		is.Equal(t, 0, len(all(earlier, named("after"))))
	})

	t.Run("offers the page after the one it loaded while there is one", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		for i := range 45 {
			a.createPost(t, signe, base.Add(-time.Duration(i)*time.Hour), fmt.Sprint("Post ", i))
		}
		cookie := a.signIn(t, signe)

		doc := parse(t, a.get(t, cookie, "/").Body.String())
		after := attr(one(t, one(t, doc, id("earlier")), named("after")), "value")

		patches := a.patches(t, a.post(t, cookie, "/posts/earlier", url.Values{"after": {after}}, true))
		earlier := one(t, patches[len(patches)-1], id("earlier"))
		is.Equal(t, "Earlier posts", text(one(t, earlier, tag("button"))))
		next := attr(one(t, earlier, named("after")), "value")
		is.True(t, next != "" && next != after, next)

		var posts []string
		for _, p := range patches {
			posts = append(posts, texts(all(p, hasAttr("data-body")))...)
		}
		is.Equal(t, 20, len(posts))
		is.Equal(t, "Post 20", posts[0])
		is.Equal(t, "Post 39", posts[19])
	})

	t.Run("does not count as a look at the timeline", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		thomas := sqlitetest.CreateUser(t, a.db, "Thomas")
		for i := range 25 {
			a.createPost(t, thomas, base.Add(-time.Duration(i+1)*time.Minute), fmt.Sprint("Post ", i))
		}
		cookie := a.signIn(t, signe)

		doc := parse(t, a.get(t, cookie, "/").Body.String())
		is.Equal(t, 20, len(all(doc, mark)))
		after := attr(one(t, one(t, doc, id("earlier")), named("after")), "value")

		// Had loading earlier posts been a look, these five would no longer be new the second time.
		for range 2 {
			patches := a.patches(t, a.post(t, cookie, "/posts/earlier", url.Values{"after": {after}}, true))
			is.Equal(t, 5, len(all(patches[0], mark)))
		}
	})

	for _, after := range []string{"", "garbage", "AAAA", strings.Repeat("A", 23), "../../etc/passwd", "<script>alert(1)</script>"} {
		t.Run(fmt.Sprintf("answers the cursor %q with a plain error, and does not count it as a look", after), func(t *testing.T) {
			a := newApp(t)
			signe := sqlitetest.CreateUser(t, a.db, "Signe")
			thomas := sqlitetest.CreateUser(t, a.db, "Thomas")
			a.createPost(t, thomas, base.Add(-time.Hour), "New to Signe.")
			cookie := a.signIn(t, signe)

			for _, datastar := range []bool{true, false} {
				res := a.post(t, cookie, "/posts/earlier", url.Values{"after": {after}}, datastar)
				is.Equal(t, http.StatusBadRequest, res.Code)

				doc := parse(t, res.Body.String())
				is.Equal(t, "That did not work", text(one(t, doc, tag("h1"))))
				is.Equal(t, 0, len(all(doc, tag("article"))))
				is.Equal(t, 2, len(all(doc, tag("script"))), "expected only the page's own two scripts")
			}

			// Had an empty cursor been read as the first page, that would have been a look, and the
			// mark would be gone.
			doc := parse(t, a.get(t, cookie, "/").Body.String())
			is.EqualSlice(t, []string{"new"}, texts(all(doc, mark)))
		})
	}

	t.Run("sends a browser that does not run the page's script back to the group page", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")

		after := model.TimelineCursor{Created: model.Time{T: base}, Seq: 5}.String()
		res := a.post(t, a.signIn(t, signe), "/posts/earlier", url.Values{"after": {after}}, false)
		is.Equal(t, http.StatusSeeOther, res.Code)
		is.Equal(t, "/", res.Header().Get("Location"))
	})
}

func TestTimeline_comments(t *testing.T) {
	t.Run("opens the comments of a post in place, oldest first, with a field to add one, and clears the post's mark for the reader", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		thomas := sqlitetest.CreateUser(t, a.db, "Thomas")
		post := a.createPost(t, signe, base.Add(-26*time.Hour), "The coffee machine is sulking.")
		other := a.createPost(t, signe, base.Add(-25*time.Hour), "Another post.")
		a.createComment(t, thomas, post, base.Add(-25*time.Hour), "Have you tried descaling it?")
		a.createComment(t, signe, post, base.Add(-24*time.Hour-30*time.Minute), "It was descaled\non Tuesday.")
		a.createComment(t, thomas, post, base.Add(-time.Hour), "They come Thursday.")
		a.createComment(t, thomas, other, base.Add(-time.Hour), "On the other post.")
		cookie := a.signIn(t, signe)

		doc := parse(t, a.get(t, cookie, "/").Body.String())
		is.Equal(t, "3 comments2 new", text(one(t, doc, id("post-"+post.ID.String()+"-comments-summary"))))
		is.Equal(t, "", text(one(t, doc, id("post-"+post.ID.String()+"-comments"))))

		res := a.post(t, cookie, "/comments/open", url.Values{"post": {post.ID.String()}}, true)
		is.Equal(t, http.StatusOK, res.Code)
		decoded, patches := a.decode(t, res), a.patches(t, res)
		is.Equal(t, 1, len(decoded))
		is.Equal(t, "", decoded[0].Selector)
		is.Equal(t, "", decoded[0].Mode)

		is.Equal(t, "3 comments", text(one(t, patches[0], id("post-"+post.ID.String()+"-comments-summary"))))

		comments := one(t, patches[0], id("post-"+post.ID.String()+"-comments"))
		items := all(comments, tag("li"))
		is.Equal(t, 3, len(items))
		is.EqualSlice(t, []string{"Have you tried descaling it?", "It was descaled\non Tuesday.", "They come Thursday."}, texts(all(comments, hasAttr("data-body"))))
		// The first two were written the day before, and the third today, at 09:00 in the group's time zone.
		is.EqualSlice(t, []string{"Yesterday 09:00", "Yesterday 09:30", "09:00"}, texts(all(comments, tag("time"))))
		is.True(t, strings.HasPrefix(text(items[0]), "Thomas"), text(items[0]))
		is.True(t, strings.HasPrefix(text(items[1]), "Signe"), text(items[1]))
		is.EqualSlice(t, []string{"new"}, texts(all(items[0], mark)))
		is.Equal(t, 0, len(all(items[1], mark)))
		is.EqualSlice(t, []string{"new"}, texts(all(items[2], mark)))

		form := one(t, comments, tag("form"))
		is.Equal(t, "/comments", attr(form, "action"))
		is.Equal(t, post.ID.String(), attr(one(t, form, named("post")), "value"))
		is.Equal(t, "", text(one(t, form, tag("textarea"))))
		is.Equal(t, "Comment", text(one(t, form, tag("button"))))

		// The mark is gone for this post, and stays on the other one.
		doc = parse(t, a.get(t, cookie, "/").Body.String())
		is.Equal(t, "3 comments", text(one(t, doc, id("post-"+post.ID.String()+"-comments-summary"))))
		is.Equal(t, "1 comment1 new", text(one(t, doc, id("post-"+other.ID.String()+"-comments-summary"))))

		// Opened again, nothing in them is new any more.
		patches = a.patches(t, a.post(t, cookie, "/comments/open", url.Values{"post": {post.ID.String()}}, true))
		is.Equal(t, 0, len(all(patches[0], mark)))
	})

	t.Run("opens a post without comments with just the field to add one", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		post := a.createPost(t, signe, base.Add(-time.Hour), "Cake.")

		patches := a.patches(t, a.post(t, a.signIn(t, signe), "/comments/open", url.Values{"post": {post.ID.String()}}, true))

		is.Equal(t, "Comment", text(one(t, patches[0], id("post-"+post.ID.String()+"-comments-summary"))))
		comments := one(t, patches[0], id("post-"+post.ID.String()+"-comments"))
		is.Equal(t, 0, len(all(comments, tag("li"))))
		is.Equal(t, 1, len(all(comments, tag("textarea"))))
	})

	t.Run("adds a comment, and answers with the comments with the new one last and an empty field", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		thomas := sqlitetest.CreateUser(t, a.db, "Thomas")
		post := a.createPost(t, thomas, base.Add(-2*time.Hour), "Cake in the kitchen.")
		a.createComment(t, thomas, post, base.Add(-time.Hour), "It is carrot.")
		cookie := a.signIn(t, signe)

		res := a.post(t, cookie, "/comments", url.Values{"post": {post.ID.String()}, "body": {"  Happy birthday!\r\nSave me a piece.  "}}, true)
		is.Equal(t, http.StatusOK, res.Code)
		patches := a.patches(t, res)
		is.Equal(t, 1, len(patches))

		is.Equal(t, "2 comments", text(one(t, patches[0], id("post-"+post.ID.String()+"-comments-summary"))))
		comments := one(t, patches[0], id("post-"+post.ID.String()+"-comments"))
		is.EqualSlice(t, []string{"It is carrot.", "Happy birthday!\nSave me a piece."}, texts(all(comments, hasAttr("data-body"))))
		items := all(comments, tag("li"))
		is.True(t, strings.HasPrefix(text(items[1]), "Signe"), text(items[1]))
		is.EqualSlice(t, []string{"10:00"}, texts(all(items[1], tag("time"))))
		is.Equal(t, 0, len(all(items[1], mark)))

		form := one(t, comments, tag("form"))
		field := one(t, form, tag("textarea"))
		is.Equal(t, "", text(field))
		is.True(t, attr(field, "id") != "post-"+post.ID.String()+"-comment-body", "the field kept its ID")
		is.Equal(t, attr(field, "id"), "post-"+post.ID.String()+"-comment-body-"+attr(one(t, form, named("field")), "value"))
		is.Equal(t, 0, len(all(form, role("alert"))))

		// The comment is new to Thomas, and the post does not move or get a mark of its own.
		doc := parse(t, a.get(t, a.signIn(t, thomas), "/").Body.String())
		is.Equal(t, "2 comments1 new", text(one(t, doc, id("post-"+post.ID.String()+"-comments-summary"))))
		is.EqualSlice(t, []string{"1 new"}, texts(all(doc, mark)))
	})

	for _, test := range []struct {
		name    string
		body    string
		message string
	}{
		{name: "an empty body", body: "", message: "Write something first."},
		{name: "a body that is too long", body: strings.Repeat("a", model.CommentBodyMaxLength+1), message: "That is too long. A comment can be up to 2,000 characters."},
		{name: "a body with a character that cannot be posted", body: "Cake\x07", message: "That text has characters that cannot be used here. Remove them and try again."},
	} {
		t.Run("refuses a comment with "+test.name+" with a message beside the field, and keeps what was typed", func(t *testing.T) {
			a := newApp(t)
			signe := sqlitetest.CreateUser(t, a.db, "Signe")
			post := a.createPost(t, signe, base.Add(-time.Hour), "Cake.")

			res := a.post(t, a.signIn(t, signe), "/comments", url.Values{"post": {post.ID.String()}, "body": {test.body}, "field": {"c_before"}}, true)
			is.Equal(t, http.StatusOK, res.Code)

			patches := a.patches(t, res)
			is.Equal(t, 1, len(patches))
			form := one(t, patches[0], id("post-"+post.ID.String()+"-comment-form"))
			is.Equal(t, test.message, text(one(t, form, role("alert"))))
			field := one(t, form, tag("textarea"))
			is.Equal(t, test.body, text(field))
			is.Equal(t, "true", attr(field, "aria-invalid"))
			is.Equal(t, "post-"+post.ID.String()+"-comment-body-c_before", attr(field, "id"))
			is.Equal(t, post.ID.String(), attr(one(t, form, named("post")), "value"))

			is.Equal(t, 0, a.count(t, "comments"))
		})
	}

	t.Run("answers a post that is not there with not found", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		cookie := a.signIn(t, signe)

		for _, post := range []string{"", "p_nope", "<script>alert(1)</script>"} {
			res := a.post(t, cookie, "/comments/open", url.Values{"post": {post}}, true)
			is.Equal(t, http.StatusNotFound, res.Code, post)

			res = a.post(t, cookie, "/comments", url.Values{"post": {post}, "body": {"Hello"}}, true)
			is.Equal(t, http.StatusNotFound, res.Code, post)
		}
		is.Equal(t, 0, a.count(t, "comments"))
	})

	t.Run("answers a browser that does not run the page's script with a redirect to the group page, or a page that says why not", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		thomas := sqlitetest.CreateUser(t, a.db, "Thomas")
		post := a.createPost(t, signe, base.Add(-2*time.Hour), "Cake.")
		a.createComment(t, thomas, post, base.Add(-time.Hour), "Thank you.")
		cookie := a.signIn(t, signe)

		// Opening is not recorded, since nothing was shown.
		res := a.post(t, cookie, "/comments/open", url.Values{"post": {post.ID.String()}}, false)
		is.Equal(t, http.StatusSeeOther, res.Code)
		is.Equal(t, "/", res.Header().Get("Location"))
		doc := parse(t, a.get(t, cookie, "/").Body.String())
		is.Equal(t, "1 comment1 new", text(one(t, doc, id("post-"+post.ID.String()+"-comments-summary"))))

		res = a.post(t, cookie, "/comments", url.Values{"post": {post.ID.String()}, "body": {"You are welcome."}}, false)
		is.Equal(t, http.StatusSeeOther, res.Code)
		is.Equal(t, "/", res.Header().Get("Location"))
		is.Equal(t, 2, a.count(t, "comments"))

		res = a.post(t, cookie, "/comments", url.Values{"post": {post.ID.String()}, "body": {""}}, false)
		is.Equal(t, http.StatusUnprocessableEntity, res.Code)
		is.Equal(t, "That comment was not added", text(one(t, parse(t, res.Body.String()), tag("h1"))))
		is.Equal(t, 2, a.count(t, "comments"))
	})
}

func TestTimeline_text(t *testing.T) {
	for _, body := range []string{
		"<script>alert(1)</script>",
		`<img src="x" onerror="alert(1)">`,
		`<a href="https://example.com/">a link</a>`,
		"</p></article></section><h2>Today</h2>",
		"&lt;b&gt; &amp;amp; <!-- -->",
	} {
		t.Run(fmt.Sprintf("shows the text %q of a post, a comment, and a name as text, wherever they come out", body), func(t *testing.T) {
			a := newApp(t)
			signe := sqlitetest.CreateUser(t, a.db, body)
			cookie := a.signIn(t, signe)

			// The page with nothing on it but the composer, to count against.
			empty := parse(t, a.get(t, cookie, "/").Body.String())

			res := a.post(t, cookie, "/posts", url.Values{"body": {body}}, true)
			is.Equal(t, http.StatusOK, res.Code)
			article := one(t, a.patches(t, res)[1], tag("article"))
			is.Equal(t, body, text(one(t, article, hasAttr("data-body"))))
			is.Equal(t, body, text(one(t, article, tag("h3"))))
			postID := attr(article, "data-post")

			res = a.post(t, cookie, "/comments", url.Values{"post": {postID}, "body": {body}}, true)
			is.Equal(t, http.StatusOK, res.Code)
			comments := one(t, a.patches(t, res)[0], id("post-"+postID+"-comments"))
			item := one(t, comments, tag("li"))
			is.Equal(t, body, text(one(t, item, hasAttr("data-body"))))
			is.True(t, strings.HasPrefix(text(item), body), text(item))

			doc := parse(t, a.get(t, cookie, "/").Body.String())
			is.Equal(t, body, text(one(t, doc, hasAttr("data-body"))))
			is.Equal(t, body, text(one(t, doc, tag("h3"))))

			// Nothing in the text became an element: no script, image, or link that the page does not
			// have without it, and no heading or section beyond the one day and its one post.
			for _, name := range []string{"script", "img", "a", "b", "h1"} {
				is.Equal(t, len(all(empty, tag(name))), len(all(doc, tag(name))), name)
				is.Equal(t, 0, len(all(article, tag(name))), name)
				is.Equal(t, 0, len(all(comments, tag(name))), name)
			}
			is.Equal(t, 1, len(all(doc, tag("h2"))))
			is.Equal(t, 1, len(all(doc, tag("section"))))
			is.Equal(t, 1, len(all(doc, tag("article"))))

			// A refusal shows what was typed as text too.
			res = a.post(t, cookie, "/posts", url.Values{"body": {body + "\x00"}}, true)
			composer := one(t, a.patches(t, res)[0], id("composer"))
			is.Equal(t, body+"\ufffd", text(one(t, composer, tag("textarea"))))
			is.Equal(t, 1, len(all(composer, tag("textarea"))))
			is.Equal(t, 0, len(all(composer, tag("script"))))
		})
	}
}

func TestTimeline_policy(t *testing.T) {
	t.Run("serves every page with a policy that does not allow evaluating strings as scripts, and allows Datastar by its origin and the page's nonce", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		a.createPost(t, signe, base.Add(-time.Hour), "Cake.")
		cookie := a.signIn(t, signe)

		nonces := map[string]bool{}
		for _, request := range []struct {
			cookie *http.Cookie
			path   string
		}{{cookie, "/"}, {nil, "/"}, {cookie, "/nope"}, {cookie, "/"}} {
			res := a.get(t, request.cookie, request.path)
			policy := res.Header().Get("Content-Security-Policy")
			is.True(t, !strings.Contains(policy, "unsafe-eval"), policy)
			is.True(t, !strings.Contains(policy, "unsafe-inline"), policy)

			doc := parse(t, res.Body.String())

			// Datastar reads the nonce from the html element, and the policy allows scripts with it.
			nonce := attr(one(t, doc, tag("html")), "data-nonce")
			is.True(t, len(nonce) >= 20, "nonce "+nonce)
			is.Equal(t, "'self' 'nonce-"+nonce+"'", directives(t, policy)["script-src"])
			is.True(t, !nonces[nonce], "the nonce "+nonce+" was used before")
			nonces[nonce] = true

			// Datastar itself is a script from the app's own origin, which 'self' allows.
			var datastar int
			for _, script := range all(doc, tag("script")) {
				src := attr(script, "src")
				is.True(t, strings.HasPrefix(src, "/") && !strings.HasPrefix(src, "//"), src)
				is.Equal(t, "", text(script))
				if strings.HasPrefix(src, "/scripts/datastar.") {
					datastar++
					is.Equal(t, "module", attr(script, "type"))
				}
			}
			is.Equal(t, 1, datastar)

			// Nothing on the page needs 'unsafe-inline': no inline style, and no inline event handler.
			is.Equal(t, 0, len(all(doc, tag("style"))))
			for _, n := range all(doc, func(*nethtml.Node) bool { return true }) {
				for _, at := range n.Attr {
					is.True(t, at.Key != "style" && !strings.HasPrefix(at.Key, "on"), "inline "+at.Key+" on "+n.Data)
				}
			}

			// A page with a nonce must not be kept and shown again under another response's policy.
			if request.path == "/" {
				is.Equal(t, "no-store", res.Header().Get("Cache-Control"))
			}
		}
	})

	t.Run("references no other host from any page or patch", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		thomas := sqlitetest.CreateUser(t, a.db, "Thomas")
		var post model.Post
		for i := range 25 {
			post = a.createPost(t, thomas, base.Add(-time.Duration(25-i)*time.Hour), fmt.Sprint("Post ", i))
		}
		a.createComment(t, thomas, post, base.Add(-time.Minute), "A comment.")
		cookie := a.signIn(t, signe)

		page := a.get(t, cookie, "/")
		after := attr(one(t, one(t, parse(t, page.Body.String()), id("earlier")), named("after")), "value")

		docs := []*nethtml.Node{
			parse(t, a.get(t, nil, "/").Body.String()),
			parse(t, page.Body.String()),
			parse(t, a.get(t, cookie, "/nope").Body.String()),
			parse(t, a.post(t, cookie, "/posts/earlier", url.Values{"after": {"garbage"}}, true).Body.String()),
			parse(t, a.post(t, cookie, "/posts", url.Values{"body": {""}}, false).Body.String()),
		}
		docs = append(docs, a.patches(t, a.post(t, cookie, "/posts", url.Values{"body": {"Hello"}}, true))...)
		docs = append(docs, a.patches(t, a.post(t, cookie, "/posts", url.Values{"body": {""}}, true))...)
		docs = append(docs, a.patches(t, a.post(t, cookie, "/posts/earlier", url.Values{"after": {after}}, true))...)
		docs = append(docs, a.patches(t, a.post(t, cookie, "/comments/open", url.Values{"post": {post.ID.String()}}, true))...)
		docs = append(docs, a.patches(t, a.post(t, cookie, "/comments", url.Values{"post": {post.ID.String()}, "body": {"Hello"}}, true))...)
		is.True(t, len(docs) >= 12, fmt.Sprint(len(docs)))

		var urls int
		for _, doc := range docs {
			for _, n := range all(doc, func(*nethtml.Node) bool { return true }) {
				is.True(t, n.Data != "style" && n.Data != "iframe" && n.Data != "object" && n.Data != "embed" && n.Data != "base", n.Data)

				for _, at := range n.Attr {
					// No attribute anywhere names a scheme or a host, whether or not it is one that loads.
					is.True(t, !strings.Contains(at.Val, "//"), at.Key+" on "+n.Data+": "+at.Val)

					switch at.Key {
					case "href", "src", "action", "formaction", "srcset", "poster", "data", "ping", "manifest", "background", "cite":
						urls++
						is.True(t, strings.HasPrefix(at.Val, "/"), at.Key+" on "+n.Data+": "+at.Val)
					}

					// What Datastar requests is a path of the app's own too.
					for _, match := range requested.FindAllStringSubmatch(at.Val, -1) {
						urls++
						is.True(t, regexp.MustCompile(`^'/[a-z/]*'$`).MatchString(match[1]), at.Key+" on "+n.Data+": "+at.Val)
					}
				}
			}
		}
		is.True(t, urls > 20, "expected the check to have seen URLs")
	})

	t.Run("keeps what someone reads out of every URL the page asks for", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		var post model.Post
		for i := range 25 {
			post = a.createPost(t, signe, base.Add(-time.Duration(25-i)*time.Hour), fmt.Sprint("Post ", i))
		}
		cookie := a.signIn(t, signe)

		doc := parse(t, a.get(t, cookie, "/").Body.String())
		after := attr(one(t, one(t, doc, id("earlier")), named("after")), "value")
		docs := append([]*nethtml.Node{doc}, a.patches(t, a.post(t, cookie, "/comments/open", url.Values{"post": {post.ID.String()}}, true))...)

		// A request's URL goes into its trace, beside who made it. So the post whose comments someone
		// opens, and the cursor of how far back they have read, are only ever sent in the body.
		var urls int
		for _, doc := range docs {
			for _, n := range all(doc, func(*nethtml.Node) bool { return true }) {
				for _, at := range n.Attr {
					if at.Key == "action" || at.Key == "href" {
						urls++
						is.True(t, !strings.Contains(at.Val, "p_") && !strings.Contains(at.Val, after) && !strings.Contains(at.Val, "?"), at.Val)
					}
					for _, match := range requested.FindAllStringSubmatch(at.Val, -1) {
						urls++
						is.True(t, !strings.Contains(match[1], "p_") && !strings.Contains(match[1], after) && !strings.Contains(match[1], "?"), at.Val)
					}
					is.True(t, !strings.Contains(at.Val, "@get("), "a GET puts what it asks for in the URL: "+at.Val)
				}
			}
		}
		is.True(t, urls > 40, "expected the check to have seen URLs")
		for _, form := range all(doc, tag("form")) {
			is.Equal(t, "post", attr(form, "method"))
		}
	})

	t.Run("logs neither what was written, nor an email address, nor whose comments someone opened", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		thomas := sqlitetest.CreateUser(t, a.db, "Thomas")
		cookie := a.signIn(t, signe)

		a.get(t, cookie, "/")
		res := a.post(t, cookie, "/posts", url.Values{"body": {"A secret about the coffee machine"}}, true)
		postID := attr(one(t, a.patches(t, res)[1], tag("article")), "data-post")
		a.post(t, cookie, "/posts", url.Values{"body": {"An unposted secret\x00"}}, true)
		a.post(t, cookie, "/comments", url.Values{"post": {postID}, "body": {"A secret about the kettle"}}, true)
		a.post(t, cookie, "/comments", url.Values{"post": {postID}, "body": {"An uncommented secret\x00"}}, true)
		a.post(t, cookie, "/comments", url.Values{"post": {"p_nope"}, "body": {"A secret about nothing"}}, true)
		for _, secret := range []string{"secret", "coffee", "kettle", signe.Email.String(), thomas.Email.String()} {
			is.True(t, !strings.Contains(a.logs.String(), secret), a.logs.String())
		}
		is.True(t, strings.Contains(a.logs.String(), postID), "expected the test to see the logs: "+a.logs.String())

		a.logs.Reset()
		thomasCookie := a.signIn(t, thomas)
		a.get(t, thomasCookie, "/")
		res = a.post(t, thomasCookie, "/comments/open", url.Values{"post": {postID}}, true)
		is.Equal(t, http.StatusOK, res.Code)
		a.post(t, thomasCookie, "/comments/open", url.Values{"post": {"p_nope"}}, true)
		is.True(t, !strings.Contains(a.logs.String(), postID) && !strings.Contains(a.logs.String(), "p_nope"), a.logs.String())
	})
}

func TestInjectHTTPRouter(t *testing.T) {
	t.Run("shows no link to a preview unless it is given one", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")

		for _, cookie := range []*http.Cookie{nil, a.signIn(t, signe)} {
			page := a.get(t, cookie, "/").Body.String()
			is.Equal(t, 0, len(all(parse(t, page), tag("a"))))
			is.True(t, !strings.Contains(page, "preview"), page)
		}
	})

	t.Run("shows the link it is given, to someone who is signed in and to someone who is not", func(t *testing.T) {
		a := newApp(t, func(opts *apphttp.InjectHTTPRouterOptions) {
			opts.PreviewLink = func(signedIn bool) Node {
				return A(Href("/preview"), Text(fmt.Sprint("signed in: ", signedIn)))
			}
		})
		signe := sqlitetest.CreateUser(t, a.db, "Signe")

		link := one(t, parse(t, a.get(t, nil, "/").Body.String()), tag("a"))
		is.Equal(t, "/preview", attr(link, "href"))
		is.Equal(t, "signed in: false", text(link))

		link = one(t, one(t, parse(t, a.get(t, a.signIn(t, signe), "/").Body.String()), tag("header")), tag("a"))
		is.Equal(t, "signed in: true", text(link))
	})
}

func FuzzTimeline(f *testing.F) {
	f.Add("Cake in the kitchen.", "2031-03-14", "", "")
	f.Add("", "", "", "")
	f.Add("<script>alert(1)</script>", "<b>", "\"><script>", "garbage")
	f.Add("a\r\nb\x00", "2031-03-13", "p_1", model.TimelineCursor{Created: model.Time{T: base}, Seq: 1}.String())
	f.Add(strings.Repeat("æ", model.PostBodyMaxLength+1), "\xff", "\r\nevent: x", "AQAAAAAAAAAAAAAAAAAAAAAB")

	f.Fuzz(func(t *testing.T, body, newest, field, after string) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		post := a.createPost(t, signe, base.Add(-time.Hour), "Cake.")
		cookie := a.signIn(t, signe)

		// Whatever is sent, the answer is patches or a plain refusal, never a failure of the server,
		// and patches always decode and parse, with nothing in them that the page does not put there.
		res := a.post(t, cookie, "/posts", url.Values{"body": {body}, "newest": {newest}, "field": {field}}, true)
		is.Equal(t, http.StatusOK, res.Code)
		patches := a.patches(t, res)
		is.Equal(t, 1, len(all(patches[0], id("composer"))))
		is.Equal(t, 1, len(all(patches[0], tag("textarea"))))
		for _, p := range patches {
			is.Equal(t, 0, len(all(p, tag("script"))))
		}

		res = a.post(t, cookie, "/comments", url.Values{"post": {post.ID.String()}, "body": {body}, "field": {field}}, true)
		is.Equal(t, http.StatusOK, res.Code)
		patches = a.patches(t, res)
		is.Equal(t, 1, len(all(patches[0], tag("textarea"))))
		is.Equal(t, 0, len(all(patches[0], tag("script"))))

		res = a.post(t, cookie, "/comments/open", url.Values{"post": {field}}, true)
		is.True(t, res.Code == http.StatusNotFound || res.Code == http.StatusOK, fmt.Sprint(res.Code))

		res = a.post(t, cookie, "/posts/earlier", url.Values{"after": {after}}, true)
		is.True(t, res.Code == http.StatusBadRequest || res.Code == http.StatusOK, fmt.Sprint(res.Code))
		if res.Code == http.StatusOK {
			a.patches(t, res)
		}
	})
}

// app is the group page's routes on a router set up the way the server sets its router up, on a real
// database, with a clock the test sets.
type app struct {
	db      *sqlite.Database
	sm      *scs.SessionManager
	handler http.Handler
	clock   *clock
	logs    *logs
}

// clock that a test sets, and that the app reads the time from.
type clock struct {
	t time.Time
}

func (c *clock) Now() time.Time {
	return c.t
}

// logs of the app, which a test can read while the app writes them.
type logs struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *logs) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *logs) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

func (l *logs) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.b.Reset()
}

// newApp for the group "The Loft" in Copenhagen, at the time of [base].
func newApp(t *testing.T, opts ...func(*apphttp.InjectHTTPRouterOptions)) *app {
	t.Helper()

	loc, err := time.LoadLocation("Europe/Copenhagen")
	is.NotError(t, err)
	group := model.Group{Name: "The Loft", TimeZone: loc}

	a := &app{db: sqlitetest.NewDatabase(t), sm: scs.New(), clock: &clock{t: base}, logs: &logs{}}
	log := slog.New(slog.NewTextHandler(a.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

	fat := service.NewFat(service.NewFatOptions{Log: log})
	service.Setup(fat, nil, a.db, nil, a.clock.Now, group)

	options := apphttp.InjectHTTPRouterOptions{Group: group, Now: a.clock.Now}
	for _, opt := range opts {
		opt(&options)
	}

	// The middleware the server puts in front of every page, in its order and its nesting.
	r := gluehttp.NewRouter(gluehttp.NewRouterOpts{SM: a.sm})
	r.Group(func(r *gluehttp.Router) {
		r.Use(httph.NoClickjacking, httph.ContentSecurityPolicy(apphttp.CSP(false)))
		r.Use(a.sm.LoadAndSave, gluehttp.Authenticate(log, a.sm, a.db))
		r.NotFound(gluehttp.NotFound(html.Page))

		r.Group(func(r *gluehttp.Router) {
			apphttp.InjectHTTPRouter(log, fat, options)(r)
		})
	})
	a.handler = r.Mux

	return a
}

// signIn the given user by putting them in a session of the app's own session manager, which is what
// signing in does, and return the session's cookie.
func (a *app) signIn(t *testing.T, user model.User) *http.Cookie {
	t.Helper()

	ctx, err := a.sm.Load(t.Context(), "")
	is.NotError(t, err)
	a.sm.Put(ctx, gluehttp.SessionUserIDKey, user.ID.String())
	token, _, err := a.sm.Commit(ctx)
	is.NotError(t, err)

	return &http.Cookie{Name: a.sm.Cookie.Name, Value: token}
}

func (a *app) get(t *testing.T, cookie *http.Cookie, path string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, path, nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	res := httptest.NewRecorder()
	a.handler.ServeHTTP(res, req)
	return res
}

// post the given form, as Datastar does or as a browser does by itself.
func (a *app) post(t *testing.T, cookie *http.Cookie, path string, form url.Values, datastar bool) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if datastar {
		req.Header.Set("Datastar-Request", "true")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	res := httptest.NewRecorder()
	a.handler.ServeHTTP(res, req)
	return res
}

// decode the patches in the given answer to Datastar.
func (a *app) decode(t *testing.T, res *httptest.ResponseRecorder) []apphttp.DecodedPatch {
	t.Helper()

	is.Equal(t, "text/event-stream", res.Header().Get("Content-Type"))
	decoded, err := apphttp.DecodePatches(res.Body.String())
	is.NotError(t, err)
	is.True(t, len(decoded) > 0, "no patches in "+res.Body.String())
	return decoded
}

// patches in the given answer to Datastar, each with its elements parsed under a node of its own.
func (a *app) patches(t *testing.T, res *httptest.ResponseRecorder) []*nethtml.Node {
	t.Helper()

	var patches []*nethtml.Node
	for _, p := range a.decode(t, res) {
		body := &nethtml.Node{Type: nethtml.ElementNode, DataAtom: atom.Body, Data: "body"}
		nodes, err := nethtml.ParseFragment(strings.NewReader(p.Elements), body)
		is.NotError(t, err)
		for _, n := range nodes {
			body.AppendChild(n)
		}
		patches = append(patches, body)
	}
	return patches
}

func (a *app) createPost(t *testing.T, author model.User, at time.Time, body string) model.Post {
	t.Helper()

	p, err := a.db.CreatePost(t.Context(), model.Post{Created: model.Time{T: at}, AuthorID: author.ID, Body: body})
	is.NotError(t, err)
	return p
}

func (a *app) createComment(t *testing.T, author model.User, post model.Post, at time.Time, body string) model.Comment {
	t.Helper()

	c, err := a.db.CreateComment(t.Context(), model.Comment{Created: model.Time{T: at}, PostID: post.ID, AuthorID: author.ID, Body: body})
	is.NotError(t, err)
	return c
}

// count the rows of the given table.
func (a *app) count(t *testing.T, table string) int {
	t.Helper()

	var count int
	err := a.db.H.Get(t.Context(), &count, "select count(*) from "+table)
	is.NotError(t, err)
	return count
}

func parse(t *testing.T, page string) *nethtml.Node {
	t.Helper()

	doc, err := nethtml.Parse(strings.NewReader(page))
	is.NotError(t, err)
	return doc
}

// all elements under the given node that match, in the order of the document.
func all(n *nethtml.Node, match func(*nethtml.Node) bool) []*nethtml.Node {
	var found []*nethtml.Node
	for d := range n.Descendants() {
		if d.Type == nethtml.ElementNode && match(d) {
			found = append(found, d)
		}
	}
	return found
}

// one element under the given node that matches, and no more.
func one(t *testing.T, n *nethtml.Node, match func(*nethtml.Node) bool) *nethtml.Node {
	t.Helper()

	found := all(n, match)
	if len(found) != 1 {
		var b strings.Builder
		_ = nethtml.Render(&b, n)
		t.Fatalf("expected one matching element, found %v in:\n%s", len(found), b.String())
	}
	return found[0]
}

func tag(name string) func(*nethtml.Node) bool {
	return func(n *nethtml.Node) bool { return n.Data == name }
}

func id(id string) func(*nethtml.Node) bool {
	return func(n *nethtml.Node) bool { return attr(n, "id") == id }
}

func named(name string) func(*nethtml.Node) bool {
	return func(n *nethtml.Node) bool { return attr(n, "name") == name }
}

func role(role string) func(*nethtml.Node) bool {
	return func(n *nethtml.Node) bool { return attr(n, "role") == role }
}

func hasAttr(key string) func(*nethtml.Node) bool {
	return func(n *nethtml.Node) bool {
		for _, a := range n.Attr {
			if a.Key == key {
				return true
			}
		}
		return false
	}
}

// mark is the "new" mark.
func mark(n *nethtml.Node) bool {
	return attr(n, "data-mark") == "new"
}

func attr(n *nethtml.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func attrs(nodes []*nethtml.Node, key string) []string {
	var values []string
	for _, n := range nodes {
		values = append(values, attr(n, key))
	}
	return values
}

// text of everything under the given node, as it is in the document.
func text(n *nethtml.Node) string {
	var b strings.Builder
	for d := range n.Descendants() {
		if d.Type == nethtml.TextNode {
			b.WriteString(d.Data)
		}
	}
	return b.String()
}

func texts(nodes []*nethtml.Node) []string {
	var values []string
	for _, n := range nodes {
		values = append(values, text(n))
	}
	return values
}
