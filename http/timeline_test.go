package http_test

import (
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/alexedwards/scs/v2"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	nethtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	"hegel.dev/go/hegel"
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
		is.Equal(t, "", attr(one(t, one(t, doc, id("composer")), named("days")), "value"))
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

		// The composer names the first few days on the page, newest first.
		is.Equal(t, "2031-03-14,2031-03-13,2031-03-10", attr(one(t, one(t, doc, id("composer")), named("days")), "value"))
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

		field = one(t, one(t, doc, id("post-"+post.ID.String()+"-comment-form")), tag("textarea"))
		is.Equal(t, "Write a comment", text(one(t, doc, func(n *nethtml.Node) bool { return n.Data == "label" && attr(n, "for") == attr(field, "id") })))
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
		is.EqualSlice(t, []string{"Post", "Comment"}, texts(all(doc, tag("button"))))
		is.EqualSlice(t, []string{"/posts", "/comments/open", "/comments", "/posts/earlier"}, attrs(all(doc, tag("form")), "action"))
		is.Equal(t, 0, len(all(doc, tag("input")))-len(all(doc, func(n *nethtml.Node) bool { return n.Data == "input" && attr(n, "type") == "hidden" })))
	})

	t.Run("gives each form that makes something a token of its own", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		for i := range 3 {
			a.createPost(t, signe, base.Add(-time.Duration(i)*time.Hour), "Cake.")
		}
		cookie := a.signIn(t, signe)

		doc := parse(t, a.get(t, cookie, "/").Body.String())
		forms := append(all(doc, id("composer")), all(doc, func(n *nethtml.Node) bool { return strings.HasSuffix(attr(n, "id"), "-comment-form") })...)
		is.Equal(t, 4, len(forms))
		tokens := map[string]bool{}
		for _, form := range forms {
			token := attr(one(t, form, named("token")), "value")
			_, err := model.ParseFormToken(token)
			is.NotError(t, err, token)
			tokens[token] = true
		}
		is.Equal(t, 4, len(tokens))
		is.Equal(t, 4, len(all(doc, named("token"))))
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

		res := a.post(t, cookie, "/posts", url.Values{"body": {"  Cake in the kitchen.\r\n\r\nIt is carrot.  "}, "days": {"2031-03-14,2031-03-13"}}, true)
		is.Equal(t, http.StatusOK, res.Code)
		is.Equal(t, "text/event-stream", res.Header().Get("Content-Type"))

		decoded, patches := a.decode(t, res), a.patches(t, res)
		is.Equal(t, 2, len(decoded))

		is.Equal(t, "", decoded[0].Selector)
		is.Equal(t, "", decoded[0].Mode)
		composer := one(t, patches[0], id("composer"))
		is.Equal(t, "", text(one(t, composer, tag("textarea"))))
		is.Equal(t, "2031-03-14,2031-03-13", attr(one(t, composer, named("days")), "value"))
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

	t.Run("says to screen readers that the post is on the timeline, and gives the focus back to the composer's field", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		cookie := a.signIn(t, signe)

		for _, days := range []string{"", "2031-03-14", "2031-03-13"} {
			patches := a.patches(t, a.post(t, cookie, "/posts", url.Values{"body": {"Cake."}, "days": {days}}, true))
			is.Equal(t, "Your post is on the timeline.", announced(t, patches), days)

			// The words are in an element named after the post, so that the next post's are said too.
			var article *nethtml.Node
			for _, p := range patches {
				if found := all(p, tag("article")); len(found) == 1 {
					article = found[0]
				}
			}
			is.Equal(t, 1, len(all(patches[0], id("announcement-"+attr(article, "data-post")))), days)
			is.EqualSlice(t, []string{"textarea"}, focusTakers(patches), days)
		}

		patches := a.patches(t, a.post(t, cookie, "/posts", url.Values{"body": {" "}}, true))
		is.Equal(t, 0, len(all(patches[0], id("announcement"))))
	})

	t.Run("gives the focus to the composer's field when what was written is refused, or was sent before with other text", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		cookie := a.signIn(t, signe)
		sent := newToken(t)
		a.post(t, cookie, "/posts", url.Values{"body": {"Cake."}, "token": {sent}}, true)

		for _, form := range []url.Values{{"body": {" "}}, {"body": {"Cake!"}, "token": {sent}}} {
			patches := a.patches(t, a.post(t, cookie, "/posts", form, true))
			is.EqualSlice(t, []string{"textarea"}, focusTakers(patches))
			is.Equal(t, "composer-body", attr(one(t, patches[0], hasAttr("data-init")), "id"))
		}

		is.Equal(t, 0, len(all(parse(t, a.get(t, cookie, "/").Body.String()), hasAttr("data-init"))))
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

		res := a.post(t, a.signIn(t, signe), "/posts", url.Values{"body": {"Cake."}, "days": {"2031-03-13,2031-03-10,2031-03-09"}}, true)

		decoded, patches := a.decode(t, res), a.patches(t, res)
		is.Equal(t, 3, len(decoded))
		is.Equal(t, "#timeline", decoded[1].Selector)
		is.Equal(t, "prepend", decoded[1].Mode)
		section := one(t, patches[1], tag("section"))
		is.Equal(t, "Today Friday 14 March", text(one(t, section, tag("h2"))))
		is.Equal(t, "Cake.", text(one(t, one(t, section, id("day-2031-03-14-posts")), hasAttr("data-body"))))

		// The composer now names the new day first, and still only a few days.
		is.Equal(t, "2031-03-14,2031-03-13,2031-03-10", attr(one(t, patches[0], named("days")), "value"))
	})

	t.Run("sets the headings right that a page left open overnight still has as today and yesterday", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		a.createPost(t, signe, base.Add(-25*time.Hour), "Thursday's.")
		a.createPost(t, signe, base.Add(-time.Hour), "Friday's.")
		cookie := a.signIn(t, signe)

		doc := parse(t, a.get(t, cookie, "/").Body.String())
		is.EqualSlice(t, []string{"Today Friday 14 March", "Yesterday Thursday 13 March"}, texts(all(doc, tag("h2"))))
		days := attr(one(t, one(t, doc, id("composer")), named("days")), "value")

		// The page stays open, and the next post is written the morning after.
		a.clock.t = base.Add(24 * time.Hour)
		res := a.post(t, cookie, "/posts", url.Values{"body": {"Saturday's."}, "days": {days}}, true)

		decoded, patches := a.decode(t, res), a.patches(t, res)
		is.Equal(t, 3, len(decoded))
		is.Equal(t, "#timeline", decoded[1].Selector)
		is.Equal(t, "prepend", decoded[1].Mode)
		is.EqualSlice(t, []string{"Today Saturday 15 March"}, texts(all(patches[1], tag("h2"))))

		// The headings that were on the page come again, by their IDs, with what is true now.
		is.Equal(t, "", decoded[2].Selector)
		is.Equal(t, "", decoded[2].Mode)
		is.EqualSlice(t, []string{"Yesterday Friday 14 March", "Thursday 13 March"}, texts(all(patches[2], tag("h2"))))
		for _, date := range []string{"2031-03-14", "2031-03-13"} {
			was := one(t, doc, id("day-"+date+"-heading"))
			is.Equal(t, "h2", was.Data)
			is.Equal(t, "h2", one(t, patches[2], id("day-"+date+"-heading")).Data)
		}

		// A reload shows the same three headings.
		doc = parse(t, a.get(t, cookie, "/").Body.String())
		is.EqualSlice(t, []string{"Today Saturday 15 March", "Yesterday Friday 14 March", "Thursday 13 March"}, texts(all(doc, tag("h2"))))
	})

	t.Run("puts a post under the heading of its day wherever on the page that is, when the clocks have gone back past midnight", func(t *testing.T) {
		// In St. John's on 7 November 2010, 00:01 became 23:01 the day before. So a post written at
		// 23:30 on Saturday comes after a post written just after midnight on Sunday.
		a := newApp(t, inZone("America/St_Johns"))
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		a.createPost(t, signe, time.Date(2010, 11, 6, 20, 0, 0, 0, time.UTC), "Saturday afternoon.")
		a.createPost(t, signe, time.Date(2010, 11, 7, 2, 30, 30, 0, time.UTC), "Sunday, 30 seconds old.")
		a.clock.t = time.Date(2010, 11, 7, 3, 0, 0, 0, time.UTC)
		cookie := a.signIn(t, signe)

		doc := parse(t, a.get(t, cookie, "/").Body.String())
		is.EqualSlice(t, []string{"Sunday 7 November", "Today Saturday 6 November"}, texts(all(doc, tag("h2"))))
		days := attr(one(t, one(t, doc, id("composer")), named("days")), "value")
		is.Equal(t, "2010-11-07,2010-11-06", days)

		res := a.post(t, cookie, "/posts", url.Values{"body": {"Saturday again."}, "days": {days}}, true)

		decoded, patches := a.decode(t, res), a.patches(t, res)
		is.Equal(t, 2, len(decoded))
		is.Equal(t, "#day-2010-11-06-posts", decoded[1].Selector)
		is.Equal(t, "prepend", decoded[1].Mode)
		is.Equal(t, 0, len(all(patches[1], tag("h2"))))
		is.Equal(t, days, attr(one(t, patches[0], named("days")), "value"))
	})

	t.Run("puts the first post in the place of the words that say the timeline is empty", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")

		res := a.post(t, a.signIn(t, signe), "/posts", url.Values{"body": {"Cake."}, "days": {""}}, true)

		decoded, patches := a.decode(t, res), a.patches(t, res)
		is.Equal(t, 2, len(decoded))
		is.Equal(t, "#timeline", decoded[1].Selector)
		is.Equal(t, "inner", decoded[1].Mode)
		is.Equal(t, "Today Friday 14 March", text(one(t, patches[1], tag("h2"))))
		is.Equal(t, "Cake.", text(one(t, patches[1], hasAttr("data-body"))))
		is.Equal(t, "2031-03-14", attr(one(t, patches[0], named("days")), "value"))
	})

	t.Run("reads the days a page names leniently, and never as anything but dates", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")

		for days, expected := range map[string]string{
			"garbage":                         "2031-03-14",
			"2031-03-13,,x,2031-03-13":        "2031-03-14,2031-03-13",
			"2031-02-30,#timeline,2031-03-12": "2031-03-14,2031-03-12",
			"2031-03-13,2031-03-12,2031-03-11,2031-03-10,2031-03-09": "2031-03-14,2031-03-13,2031-03-12",
			"<script>alert(1)</script>":                              "2031-03-14",
		} {
			res := a.post(t, a.signIn(t, signe), "/posts", url.Values{"body": {"Cake."}, "days": {days}}, true)

			decoded, patches := a.decode(t, res), a.patches(t, res)
			is.Equal(t, expected, attr(one(t, patches[0], named("days")), "value"), days)
			for _, d := range decoded {
				is.True(t, regexp.MustCompile(`^(|#timeline|#day-\d{4}-\d{2}-\d{2}-posts)$`).MatchString(d.Selector), d.Selector)
			}
			for _, p := range patches {
				for _, h := range all(p, tag("h2")) {
					is.True(t, regexp.MustCompile(`^day-\d{4}-\d{2}-\d{2}-heading$`).MatchString(attr(h, "id")), attr(h, "id"))
				}
			}
		}
	})

	t.Run("does not count as a look at the timeline", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		thomas := sqlitetest.CreateUser(t, a.db, "Thomas")
		a.createPost(t, thomas, base.Add(-time.Hour), "New to Signe.")
		cookie := a.signIn(t, signe)

		res := a.post(t, cookie, "/posts", url.Values{"body": {"Cake."}, "days": {"2031-03-14"}}, true)
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

			res := a.post(t, a.signIn(t, signe), "/posts", url.Values{"body": {test.body}, "days": {"2031-03-13,2031-03-12"}, "field": {"p_before"}}, true)
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
			is.Equal(t, "2031-03-13,2031-03-12", attr(one(t, composer, named("days")), "value"))

			is.Equal(t, 0, a.count(t, "posts"))
		})
	}

	t.Run("refuses a body many times too long like any other, with everything the page sent kept", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		typed := strings.Repeat("æ", 100_000)

		res := a.post(t, a.signIn(t, signe), "/posts", url.Values{"body": {typed}, "days": {"2031-03-13"}, "field": {"p_before"}}, true)
		is.Equal(t, http.StatusOK, res.Code)

		composer := one(t, a.patches(t, res)[0], id("composer"))
		is.Equal(t, "That is too long. A post can be up to 10,000 characters.", text(one(t, composer, role("alert"))))
		is.Equal(t, typed, text(one(t, composer, tag("textarea"))))
		is.Equal(t, "composer-body-p_before", attr(one(t, composer, tag("textarea")), "id"))
		is.Equal(t, "2031-03-13", attr(one(t, composer, named("days")), "value"))
		is.Equal(t, 0, a.count(t, "posts"))
	})

	t.Run("answers a request too large to read with an error and no patch, so that nothing on the page changes", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		post := a.createPost(t, signe, base.Add(-time.Hour), "Cake.")
		cookie := a.signIn(t, signe)

		for path, form := range map[string]url.Values{
			"/posts":         {"days": {"2031-03-14"}, "field": {"p_before"}, "body": {strings.Repeat("a", 2<<20)}},
			"/comments":      {"post": {post.ID.String()}, "field": {"c_before"}, "body": {strings.Repeat("a", 2<<20)}},
			"/comments/open": {"post": {strings.Repeat("a", 2<<20)}},
			"/posts/earlier": {"after": {strings.Repeat("a", 2<<20)}},
		} {
			for _, datastar := range []bool{true, false} {
				res := a.post(t, cookie, path, form, datastar)
				is.Equal(t, http.StatusRequestEntityTooLarge, res.Code, path)
				is.True(t, !strings.Contains(res.Header().Get("Content-Type"), "event-stream"), path)
				is.Equal(t, "That is too long", text(one(t, parse(t, res.Body.String()), tag("h1"))))
			}
		}
		is.Equal(t, 1, a.count(t, "posts"))
		is.Equal(t, 0, a.count(t, "comments"))
	})

	t.Run("answers a body that is not a form with a plain error", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		cookie := a.signIn(t, signe)

		for _, path := range []string{"/posts", "/comments", "/comments/open", "/posts/earlier"} {
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader("body=%zz&post=%"))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("Datastar-Request", "true")
			req.AddCookie(cookie)
			res := httptest.NewRecorder()
			a.handler.ServeHTTP(res, req)

			is.Equal(t, http.StatusBadRequest, res.Code, path)
			is.Equal(t, "That did not work", text(one(t, parse(t, res.Body.String()), tag("h1"))))
		}
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
		is.True(t, strings.Contains(text(doc), "Write something first."), text(doc))
		is.Equal(t, 1, a.count(t, "posts"))
	})
}

func TestTimeline_again(t *testing.T) {
	t.Run("posts once when the same form is sent again with the same text, answers again as the first time, and gives a new token each time", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		cookie := a.signIn(t, signe)
		sent := newToken(t)

		var posts, tokens []string
		for _, body := range []string{"Cake.", "Cake.", " Cake.\r\n"} {
			res := a.post(t, cookie, "/posts", url.Values{"body": {body}, "days": {"2031-03-14"}, "token": {sent}}, true)
			is.Equal(t, http.StatusOK, res.Code)
			patches := a.patches(t, res)
			is.Equal(t, 2, len(patches))
			article := one(t, patches[1], tag("article"))
			is.Equal(t, "Cake.", text(one(t, article, hasAttr("data-body"))))
			posts = append(posts, attr(article, "data-post"))
			tokens = append(tokens, attr(one(t, patches[0], named("token")), "value"))
			is.Equal(t, "", text(one(t, patches[0], tag("textarea"))))
			is.Equal(t, 0, len(all(patches[0], role("alert"))))
			is.Equal(t, "Your post is on the timeline.", announced(t, patches))
		}
		is.Equal(t, 1, a.count(t, "posts"))
		is.Equal(t, posts[0], posts[1])
		is.Equal(t, posts[0], posts[2])
		for i, token := range tokens {
			_, err := model.ParseFormToken(token)
			is.NotError(t, err)
			is.True(t, token != sent, "the composer kept the token that was sent")
			is.True(t, !slices.Contains(tokens[:i], token), "the composer got a token it had before")
		}

		res := a.post(t, cookie, "/posts", url.Values{"body": {"Cake."}, "token": {sent}}, false)
		is.Equal(t, http.StatusSeeOther, res.Code)
		is.Equal(t, 1, a.count(t, "posts"))
	})

	t.Run("comments once when the same form is sent again with the same text, answers again as the first time, and gives a new token each time", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		post := a.createPost(t, signe, base.Add(-time.Hour), "Cake.")
		cookie := a.signIn(t, signe)
		sent := newToken(t)

		var tokens []string
		for _, body := range []string{"Yes.", "Yes.", "\tYes.\n"} {
			res := a.post(t, cookie, "/comments", url.Values{"post": {post.ID.String()}, "body": {body}, "token": {sent}}, true)
			is.Equal(t, http.StatusOK, res.Code)
			patches := a.patches(t, res)
			patch := patches[0]
			is.EqualSlice(t, []string{"Yes."}, texts(all(one(t, patch, id("post-"+post.ID.String()+"-comments")), hasAttr("data-body"))))
			form := one(t, patch, id("post-"+post.ID.String()+"-comment-form"))
			tokens = append(tokens, attr(one(t, form, named("token")), "value"))
			is.Equal(t, "", text(one(t, form, tag("textarea"))))
			is.Equal(t, 0, len(all(form, role("alert"))))
			is.Equal(t, "Your comment was added.", announced(t, patches))
		}
		is.Equal(t, 1, a.count(t, "comments"))
		for i, token := range tokens {
			_, err := model.ParseFormToken(token)
			is.NotError(t, err)
			is.True(t, token != sent, "the form kept the token that was sent")
			is.True(t, !slices.Contains(tokens[:i], token), "the form got a token it had before")
		}

		res := a.post(t, cookie, "/comments", url.Values{"post": {post.ID.String()}, "body": {"Yes."}, "token": {sent}}, false)
		is.Equal(t, http.StatusSeeOther, res.Code)
		is.Equal(t, 1, a.count(t, "comments"))
	})

	t.Run("keeps other text sent again with the same form in the composer, with a message that the first went through and this did not, and answers with the first post", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		cookie := a.signIn(t, signe)
		sent := newToken(t)

		first := a.patches(t, a.post(t, cookie, "/posts", url.Values{"body": {"Cake."}, "days": {"2031-03-14"}, "token": {sent}}, true))
		post := attr(one(t, first[1], tag("article")), "data-post")

		tokens := []string{sent}
		for _, body := range []string{"Cake!", "", strings.Repeat("a", model.PostBodyMaxLength+1)} {
			res := a.post(t, cookie, "/posts", url.Values{"body": {body}, "days": {"2031-03-14"}, "field": {""}, "token": {sent}}, true)
			is.Equal(t, http.StatusOK, res.Code)
			decoded, patches := a.decode(t, res), a.patches(t, res)
			is.Equal(t, 2, len(patches))

			composer := one(t, patches[0], id("composer"))
			field := one(t, composer, tag("textarea"))
			is.Equal(t, body, text(field))
			is.Equal(t, "composer-body", attr(field, "id"))
			is.Equal(t, "true", attr(field, "aria-invalid"))
			is.Equal(t, "Your earlier try did go through, and is on the timeline as it was sent. This text has not been posted.", text(one(t, composer, role("alert"))))
			is.Equal(t, "2031-03-14", attr(one(t, composer, named("days")), "value"))
			token := attr(one(t, composer, named("token")), "value")
			_, err := model.ParseFormToken(token)
			is.NotError(t, err)
			is.True(t, !slices.Contains(tokens, token), "the composer got a token it had before")
			tokens = append(tokens, token)

			// What the first sending made is shown as the first time, without saying it was posted now.
			is.Equal(t, "#day-2031-03-14-posts", decoded[1].Selector)
			article := one(t, patches[1], tag("article"))
			is.Equal(t, post, attr(article, "data-post"))
			is.Equal(t, "Cake.", text(one(t, article, hasAttr("data-body"))))
			is.Equal(t, 0, len(all(patches[0], id("announcement"))))
		}
		is.Equal(t, 1, a.count(t, "posts"))

		// The new token the form got makes the text a post of its own.
		res := a.post(t, cookie, "/posts", url.Values{"body": {"Cake!"}, "token": {tokens[1]}}, true)
		is.Equal(t, "Cake!", text(one(t, a.patches(t, res)[1], hasAttr("data-body"))))
		is.Equal(t, 2, a.count(t, "posts"))
	})

	t.Run("keeps other text sent again with the same comment form in it, with a message that the first went through and this did not, and answers with the comments", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		post := a.createPost(t, signe, base.Add(-time.Hour), "Cake.")
		cookie := a.signIn(t, signe)
		sent := newToken(t)

		a.post(t, cookie, "/comments", url.Values{"post": {post.ID.String()}, "body": {"Yes."}, "token": {sent}}, true)

		for _, body := range []string{"Yes!", "", strings.Repeat("a", model.CommentBodyMaxLength+1)} {
			res := a.post(t, cookie, "/comments", url.Values{"post": {post.ID.String()}, "body": {body}, "field": {"c_before"}, "token": {sent}}, true)
			is.Equal(t, http.StatusOK, res.Code)
			patches := a.patches(t, res)
			is.Equal(t, 1, len(patches))

			is.EqualSlice(t, []string{"Yes."}, texts(all(one(t, patches[0], id("post-"+post.ID.String()+"-comments")), hasAttr("data-body"))))
			form := one(t, patches[0], id("post-"+post.ID.String()+"-comment-form"))
			field := one(t, form, tag("textarea"))
			is.Equal(t, body, text(field))
			is.Equal(t, "post-"+post.ID.String()+"-comment-body-c_before", attr(field, "id"))
			is.Equal(t, "true", attr(field, "aria-invalid"))
			is.Equal(t, "Your earlier try did go through, and is with the comments as it was sent. This text has not been added.", text(one(t, form, role("alert"))))
			token := attr(one(t, form, named("token")), "value")
			_, err := model.ParseFormToken(token)
			is.NotError(t, err)
			is.True(t, token != sent)
			is.Equal(t, 0, len(all(patches[0], id("announcement"))))
		}
		is.Equal(t, 1, a.count(t, "comments"))
	})

	t.Run("tells a browser that does not run the page's script that other text sent again with the same form was not posted, and makes nothing", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		post := a.createPost(t, signe, base.Add(-time.Hour), "Cake.")
		cookie := a.signIn(t, signe)

		for path, forms := range map[string][2]url.Values{
			"/posts":    {{"body": {"Cake."}, "token": {newToken(t)}}, {"body": {"Cake!"}}},
			"/comments": {{"body": {"Yes."}, "token": {newToken(t)}, "post": {post.ID.String()}}, {"body": {"Yes!"}, "post": {post.ID.String()}}},
		} {
			is.Equal(t, http.StatusSeeOther, a.post(t, cookie, path, forms[0], false).Code, path)
			forms[1].Set("token", forms[0].Get("token"))
			res := a.post(t, cookie, path, forms[1], false)
			is.Equal(t, http.StatusConflict, res.Code, path)
			doc := parse(t, res.Body.String())
			is.True(t, strings.Contains(text(doc), "Your earlier try did go through"), text(doc))
		}
		is.Equal(t, 2, a.count(t, "posts"))
		is.Equal(t, 1, a.count(t, "comments"))
	})

	t.Run("gives a refused form a new token, and makes nothing", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		post := a.createPost(t, signe, base.Add(-time.Hour), "Cake.")
		cookie := a.signIn(t, signe)

		for path, form := range map[string]url.Values{
			"/posts":    {"body": {" "}, "token": {newToken(t)}},
			"/comments": {"body": {" "}, "token": {newToken(t)}, "post": {post.ID.String()}},
		} {
			res := a.post(t, cookie, path, form, true)
			is.Equal(t, http.StatusOK, res.Code, path)
			token := attr(one(t, a.patches(t, res)[0], named("token")), "value")
			_, err := model.ParseFormToken(token)
			is.NotError(t, err, path)
			is.True(t, token != form.Get("token"), path)
		}
		is.Equal(t, 1, a.count(t, "posts"))
		is.Equal(t, 0, a.count(t, "comments"))
	})

	t.Run("refuses a form without a token, or with one that is not one, with a plain error, and makes nothing", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		post := a.createPost(t, signe, base.Add(-time.Hour), "Cake.")
		cookie := a.signIn(t, signe)

		for _, token := range []string{"", "not a token", "q6urq6urq6urq6urq6urqx", newToken(t) + "A"} {
			for _, datastar := range []bool{true, false} {
				for path, form := range map[string]url.Values{
					"/posts":    {"body": {"Cake."}, "token": {token}},
					"/comments": {"body": {"Yes."}, "token": {token}, "post": {post.ID.String()}},
				} {
					res := a.post(t, cookie, path, form, datastar)
					is.Equal(t, http.StatusBadRequest, res.Code, path+" "+token)
					is.True(t, res.Header().Get("Content-Type") != "text/event-stream", path)
				}
			}
		}
		is.Equal(t, 1, a.count(t, "posts"))
		is.Equal(t, 0, a.count(t, "comments"))
	})

	t.Run("makes exactly one post or comment for each form, however often and in whatever order each is sent", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		post := a.createPost(t, signe, base.Add(-time.Hour), "Cake.")
		cookie := a.signIn(t, signe)

		hegel.Test(t, func(ht *hegel.T) {
			_, err := a.db.H.DB.ExecContext(t.Context(), "delete from comments")
			is.NotError(ht, err)
			_, err = a.db.H.DB.ExecContext(t.Context(), "delete from posts where id != ?", post.ID)
			is.NotError(ht, err)

			// Forms to post and forms to comment, each with its token, sent in any order, as often as
			// it comes up, by Datastar or by the browser itself.
			type form struct {
				path   string
				values url.Values
			}
			// The tokens are drawn too, so that a failure can be replayed.
			token := func() string {
				t, err := model.NewFormToken(bytes.NewReader(hegel.Draw(ht, hegel.Binary(16, 16))))
				is.NotError(ht, err)
				return t.String()
			}
			var forms []form
			for range hegel.Draw(ht, hegel.Integers(1, 3)) {
				forms = append(forms, form{path: "/posts", values: url.Values{"body": {"Cake."}, "token": {token()}}})
			}
			for range hegel.Draw(ht, hegel.Integers(0, 3)) {
				forms = append(forms, form{path: "/comments", values: url.Values{"body": {"Yes."}, "token": {token()}, "post": {post.ID.String()}}})
			}

			sends := hegel.Draw(ht, hegel.Lists(hegel.Integers(0, len(forms)-1)).MinSize(1).MaxSize(15))
			type submission struct{ path, token string }
			sent := map[submission]bool{}
			for _, i := range sends {
				datastar := hegel.Draw(ht, hegel.Booleans())
				res := a.post(t, cookie, forms[i].path, forms[i].values, datastar)
				if want := map[bool]int{true: http.StatusOK, false: http.StatusSeeOther}[datastar]; res.Code != want {
					ht.Fatalf("sending %v answered %v, not %v", forms[i].path, res.Code, want)
				}
				// Two forms of a kind with the same token, which drawing can give, are one submission.
				sent[submission{path: forms[i].path, token: forms[i].values.Get("token")}] = true
			}

			var posts, comments int
			for s := range sent {
				if s.path == "/posts" {
					posts++
				} else {
					comments++
				}
			}
			if n := a.count(t, "posts") - 1; n != posts {
				ht.Fatalf("%v sends of %v forms to post made %v posts", len(sends), posts, n)
			}
			if n := a.count(t, "comments"); n != comments {
				ht.Fatalf("%v sends of %v forms to comment made %v comments", len(sends), comments, n)
			}
		})
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
		is.Equal(t, "2031-03-14,2031-03-13", attr(one(t, earlier, named("days")), "value"))

		// The cursor alone says which day the page ended on, whether or not the page names its days.
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

		// The first post loaded takes the focus, and no other, whatever day it is on.
		var focused []string
		for _, p := range patches {
			for _, n := range all(p, hasAttr("data-init")) {
				focused = append(focused, text(one(t, n, hasAttr("data-body"))))
			}
		}
		is.EqualSlice(t, []string{"Thursday 4"}, focused)

		// That was the end of the timeline, so the control to load more goes away.
		is.Equal(t, "", decoded[2].Selector)
		earlier = one(t, patches[2], id("earlier"))
		is.Equal(t, 0, len(all(earlier, tag("button"))))
		is.Equal(t, 0, len(all(earlier, named("after"))))
	})

	t.Run("moves the keyboard focus to the first post it loads, and says how many it loaded, and when there are no more", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		for i := range 41 {
			a.createPost(t, signe, base.Add(-time.Duration(i)*time.Minute), fmt.Sprint("Post ", i))
		}
		cookie := a.signIn(t, signe)

		after := attr(one(t, parse(t, a.get(t, cookie, "/").Body.String()), named("after")), "value")
		for _, expected := range []struct {
			first, said string
		}{
			{first: "Post 20", said: "Loaded 20 earlier posts."},
			{first: "Post 40", said: "Loaded 1 earlier post. There are no posts before it."},
		} {
			patches := a.patches(t, a.post(t, cookie, "/posts/earlier", url.Values{"after": {after}}, true))

			var focused []*nethtml.Node
			for _, p := range patches {
				focused = append(focused, all(p, hasAttr("data-init"))...)
			}
			is.Equal(t, 1, len(focused))
			is.Equal(t, "article", focused[0].Data)
			is.Equal(t, expected.first, text(one(t, focused[0], hasAttr("data-body"))))
			is.True(t, strings.HasSuffix(attr(focused[0], "data-init"), "el.focus()"), attr(focused[0], "data-init"))
			is.Equal(t, "-1", attr(focused[0], "tabindex"))

			is.Equal(t, expected.said, announced(t, patches))

			if earlier := all(patches[len(patches)-1], named("after")); len(earlier) > 0 {
				after = attr(earlier[0], "value")
			}
		}
	})

	t.Run("moves the focus to the first post it loads when that is under a day heading it brings", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		for i := range 20 {
			a.createPost(t, signe, base.Add(-time.Duration(i)*time.Minute), fmt.Sprint("Today ", i))
		}
		for i := range 3 {
			a.createPost(t, signe, base.Add(-24*time.Hour-time.Duration(i)*time.Minute), fmt.Sprint("Yesterday ", i))
		}
		cookie := a.signIn(t, signe)

		after := attr(one(t, parse(t, a.get(t, cookie, "/").Body.String()), named("after")), "value")
		res := a.post(t, cookie, "/posts/earlier", url.Values{"after": {after}, "days": {"2031-03-14"}}, true)
		decoded, patches := a.decode(t, res), a.patches(t, res)
		is.Equal(t, "#timeline", decoded[0].Selector)

		var focused []string
		for _, p := range patches {
			for _, n := range all(p, hasAttr("data-init")) {
				focused = append(focused, text(one(t, n, hasAttr("data-body"))))
			}
		}
		is.EqualSlice(t, []string{"Yesterday 0"}, focused)
	})

	t.Run("says when there turn out to be no earlier posts at all", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		post := a.createPost(t, signe, base, "Cake.")
		cookie := a.signIn(t, signe)

		after := model.TimelineCursor{Created: post.Created, Seq: 1}.String()
		patches := a.patches(t, a.post(t, cookie, "/posts/earlier", url.Values{"after": {after}}, true))
		is.Equal(t, "There are no earlier posts.", announced(t, patches))
	})

	t.Run("loads five earlier posts and says so in plural, with no more to come", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		for i := range 25 {
			a.createPost(t, signe, base.Add(-time.Duration(i)*time.Minute), fmt.Sprint("Post ", i))
		}
		cookie := a.signIn(t, signe)

		after := attr(one(t, parse(t, a.get(t, cookie, "/").Body.String()), named("after")), "value")
		patches := a.patches(t, a.post(t, cookie, "/posts/earlier", url.Values{"after": {after}}, true))
		is.Equal(t, "Loaded 5 earlier posts. There are no posts before these.", announced(t, patches))
	})

	t.Run("gives a day one heading across pages when the clocks have gone back past midnight too", func(t *testing.T) {
		// In St. John's on 7 November 2010, 00:01 became 23:01 the day before. The newest twenty posts
		// are one from Sunday and nineteen from the Saturday hour that came after it on the clock. The
		// page after them begins with the post from the first minute of Sunday, which is a day the
		// first page has a heading for, but not the day the first page ended on.
		a := newApp(t, inZone("America/St_Johns"))
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		a.createPost(t, signe, time.Date(2010, 11, 6, 20, 0, 0, 0, time.UTC), "Saturday afternoon.")
		a.createPost(t, signe, time.Date(2010, 11, 7, 2, 30, 30, 0, time.UTC), "Sunday, 30 seconds old.")
		for i := range 19 {
			a.createPost(t, signe, time.Date(2010, 11, 7, 2, 40+i, 0, 0, time.UTC), fmt.Sprint("Saturday again ", i))
		}
		a.createPost(t, signe, time.Date(2010, 11, 7, 4, 0, 0, 0, time.UTC), "Sunday, half past midnight.")
		a.clock.t = time.Date(2010, 11, 7, 12, 0, 0, 0, time.UTC)
		cookie := a.signIn(t, signe)

		doc := parse(t, a.get(t, cookie, "/").Body.String())
		is.EqualSlice(t, []string{"Today Sunday 7 November", "Yesterday Saturday 6 November"}, texts(all(doc, tag("h2"))))
		earlier := one(t, doc, id("earlier"))
		form := url.Values{"after": {attr(one(t, earlier, named("after")), "value")}, "days": {attr(one(t, earlier, named("days")), "value")}}
		is.Equal(t, "2010-11-07,2010-11-06", form.Get("days"))

		res := a.post(t, cookie, "/posts/earlier", form, true)
		decoded, patches := a.decode(t, res), a.patches(t, res)
		is.Equal(t, 3, len(decoded))
		is.Equal(t, "#day-2010-11-07-posts", decoded[0].Selector)
		is.EqualSlice(t, []string{"Sunday, 30 seconds old."}, texts(all(patches[0], hasAttr("data-body"))))
		is.Equal(t, "#day-2010-11-06-posts", decoded[1].Selector)
		is.EqualSlice(t, []string{"Saturday afternoon."}, texts(all(patches[1], hasAttr("data-body"))))
		for _, p := range patches {
			is.Equal(t, 0, len(all(p, tag("h2"))))
			is.Equal(t, 0, len(all(p, tag("section"))))
		}
	})

	t.Run("names the days at the end of the page with each page it loads, so that the next one knows them", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		for i := range 45 {
			a.createPost(t, signe, base.Add(-time.Duration(i)*8*time.Hour), fmt.Sprint("Post ", i))
		}
		cookie := a.signIn(t, signe)

		doc := parse(t, a.get(t, cookie, "/").Body.String())
		earlier := one(t, doc, id("earlier"))
		form := url.Values{"after": {attr(one(t, earlier, named("after")), "value")}, "days": {attr(one(t, earlier, named("days")), "value")}}
		is.Equal(t, "2031-03-10,2031-03-09,2031-03-08", form.Get("days"))

		patches := a.patches(t, a.post(t, cookie, "/posts/earlier", form, true))
		earlier = one(t, patches[len(patches)-1], id("earlier"))
		is.Equal(t, "2031-03-03,2031-03-02,2031-03-01", attr(one(t, earlier, named("days")), "value"))
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
				res := a.post(t, cookie, "/posts/earlier", url.Values{"after": {after}, "days": {after}}, datastar)
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

	t.Run("tells a browser that does not run the page's script that this takes JavaScript, without counting as a look", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		thomas := sqlitetest.CreateUser(t, a.db, "Thomas")
		a.createPost(t, thomas, base.Add(-time.Hour), "New to Signe.")
		cookie := a.signIn(t, signe)

		after := model.TimelineCursor{Created: model.Time{T: base}, Seq: 5}.String()
		res := a.post(t, cookie, "/posts/earlier", url.Values{"after": {after}}, false)
		is.Equal(t, http.StatusBadRequest, res.Code)
		is.Equal(t, "", res.Header().Get("Location"))
		page := res.Body.String()
		is.Equal(t, "That needs JavaScript", text(one(t, parse(t, page), tag("h1"))))
		is.True(t, !strings.Contains(page, "New to Signe."), page)

		doc := parse(t, a.get(t, cookie, "/").Body.String())
		is.EqualSlice(t, []string{"new"}, texts(all(doc, mark)))
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

		// The field to add a comment is on the page from the start, under the post it is for.
		form := one(t, one(t, doc, id("post-"+post.ID.String())), func(n *nethtml.Node) bool { return n.Data == "form" && attr(n, "action") == "/comments" })
		is.Equal(t, post.ID.String(), attr(one(t, form, named("post")), "value"))
		is.Equal(t, "", text(one(t, form, tag("textarea"))))
		is.Equal(t, "Comment", text(one(t, form, tag("button"))))

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

		// The answer has no field in it, so that it leaves alone what someone has begun to write.
		is.Equal(t, 0, len(all(patches[0], tag("form"))))
		is.Equal(t, 0, len(all(patches[0], tag("textarea"))))

		// The mark is gone for this post, and stays on the other one.
		doc = parse(t, a.get(t, cookie, "/").Body.String())
		is.Equal(t, "3 comments", text(one(t, doc, id("post-"+post.ID.String()+"-comments-summary"))))
		is.Equal(t, "1 comment1 new", text(one(t, doc, id("post-"+other.ID.String()+"-comments-summary"))))

		// Opened again, nothing in them is new any more.
		patches = a.patches(t, a.post(t, cookie, "/comments/open", url.Values{"post": {post.ID.String()}}, true))
		is.Equal(t, 0, len(all(patches[0], mark)))
	})

	t.Run("opens a post without comments with nothing in the place for them", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		post := a.createPost(t, signe, base.Add(-time.Hour), "Cake.")

		patches := a.patches(t, a.post(t, a.signIn(t, signe), "/comments/open", url.Values{"post": {post.ID.String()}}, true))

		is.Equal(t, "Comment", text(one(t, patches[0], id("post-"+post.ID.String()+"-comments-summary"))))
		comments := one(t, patches[0], id("post-"+post.ID.String()+"-comments"))
		is.Equal(t, 0, len(all(comments, tag("li"))))
		is.Equal(t, "", text(comments))
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

		form := one(t, patches[0], id("post-"+post.ID.String()+"-comment-form"))
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

	t.Run("gives the focus to the post's comment field after a comment, after a refusal, and after other text sent before, and not when comments are only opened", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		post := a.createPost(t, signe, base.Add(-time.Hour), "Cake.")
		cookie := a.signIn(t, signe)
		sent := newToken(t)

		for _, form := range []url.Values{{"body": {"Yes."}, "token": {sent}}, {"body": {" "}}, {"body": {"Yes!"}, "token": {sent}}} {
			form.Set("post", post.ID.String())
			patches := a.patches(t, a.post(t, cookie, "/comments", form, true))
			is.EqualSlice(t, []string{"textarea"}, focusTakers(patches))
			field := one(t, patches[0], hasAttr("data-init"))
			is.True(t, strings.HasPrefix(attr(field, "id"), "post-"+post.ID.String()+"-comment-body"), attr(field, "id"))
		}

		patches := a.patches(t, a.post(t, cookie, "/comments/open", url.Values{"post": {post.ID.String()}}, true))
		is.Equal(t, 0, len(focusTakers(patches)))
	})

	t.Run("says to screen readers that the comment was added, and nothing when comments are only opened", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		post := a.createPost(t, signe, base.Add(-time.Hour), "Cake.")
		cookie := a.signIn(t, signe)

		var said []string
		for range 2 {
			patches := a.patches(t, a.post(t, cookie, "/comments", url.Values{"post": {post.ID.String()}, "body": {"Yes."}}, true))
			is.Equal(t, "Your comment was added.", announced(t, patches))

			// The words are in an element named after the comment, so that the next comment's are said too.
			comments := all(patches[0], hasAttr("data-comment"))
			newest := attr(comments[len(comments)-1], "data-comment")
			is.Equal(t, 1, len(all(patches[0], id("announcement-"+newest))))
			said = append(said, newest)
		}
		is.True(t, said[0] != said[1])

		patches := a.patches(t, a.post(t, cookie, "/comments/open", url.Values{"post": {post.ID.String()}}, true))
		is.Equal(t, 0, len(all(patches[0], id("announcement"))))
	})

	t.Run("answers with text even when what it puts back on the page was not", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		post := a.createPost(t, signe, base, "Cake.")
		cookie := a.signIn(t, signe)

		// A comment on a post that is not there is not found, whatever else is sent, so the comment is
		// on a post that is.
		for path, form := range map[string]url.Values{
			"/posts":    {"body": {""}, "field": {"\r\nevent: \xb7"}, "days": {"\xff"}},
			"/comments": {"body": {"\xff"}, "field": {"\xb7"}, "post": {post.ID.String()}},
		} {
			res := a.post(t, cookie, path, form, true)
			is.Equal(t, http.StatusOK, res.Code, path)
			is.True(t, utf8.Valid(res.Body.Bytes()), path+": the answer is not valid UTF-8")
			is.Equal(t, 1, len(a.patches(t, res)), path)
		}
	})

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
		is.Equal(t, http.StatusBadRequest, res.Code)
		is.Equal(t, "That needs JavaScript", text(one(t, parse(t, res.Body.String()), tag("h1"))))
		is.True(t, !strings.Contains(res.Body.String(), "Thank you."), res.Body.String())
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
			comments := a.patches(t, res)[0]
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

			// A refusal shows what was typed as text too, with what is not text at all replaced.
			for _, bad := range []string{"\x00", "\xff"} {
				res = a.post(t, cookie, "/posts", url.Values{"body": {body + bad}}, true)
				is.True(t, utf8.Valid(res.Body.Bytes()), "the answer is not valid UTF-8")
				composer := one(t, a.patches(t, res)[0], id("composer"))
				is.Equal(t, body+"\ufffd", text(one(t, composer, tag("textarea"))))
				is.Equal(t, 1, len(all(composer, tag("textarea"))))
				is.Equal(t, 0, len(all(composer, tag("script"))))
			}
		})
	}
}

func TestTimeline_policy(t *testing.T) {
	// requested matches the path in each request that a Datastar expression makes.
	requested := regexp.MustCompile(`@(?:get|post|put|patch|delete)\(([^,)]*)`)

	t.Run("serves every page with a policy that does not allow evaluating strings as scripts, and allows Datastar by its origin and the page's nonce, and lets none of its own pages be stored", func(t *testing.T) {
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

			// A page with a nonce must not be kept and shown again under another response's policy. The
			// not-found page is not one of the app's routes, and has nothing of the group on it.
			if request.path != "/nope" {
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
		is.True(t, urls > 60, "expected the check to have seen URLs")
		for _, form := range all(doc, tag("form")) {
			is.Equal(t, "post", attr(form, "method"))
		}

		// No ID is written into an expression either, where it would be code and not data.
		for _, n := range all(doc, func(*nethtml.Node) bool { return true }) {
			for _, at := range n.Attr {
				if strings.HasPrefix(at.Key, "data-on") {
					is.True(t, !strings.Contains(at.Val, "p_"), at.Val)
				}
			}
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

func TestTimeline_failures(t *testing.T) {
	// requests that each route gets, as Datastar sends them.
	requests := func(post model.Post) map[string]url.Values {
		return map[string]url.Values{
			"/posts":         {"body": {"A secret about the kettle"}},
			"/posts/earlier": {"after": {model.TimelineCursor{Created: model.Time{T: base}, Seq: 5}.String()}},
			"/comments/open": {"post": {post.ID.String()}},
			"/comments":      {"post": {post.ID.String()}, "body": {"A secret about the kettle"}},
		}
	}

	for _, test := range []struct {
		name string
		set  func(*store)
	}{
		{name: "is no longer active", set: func(s *store) { s.inactive = true }},
		{name: "is gone", set: func(s *store) { s.gone = true }},
	} {
		t.Run("treats someone who turns out to be someone who "+test.name+" in the middle of a request as not signed in", func(t *testing.T) {
			a := newApp(t)
			signe := sqlitetest.CreateUser(t, a.db, "Signe")
			post := a.createPost(t, signe, base.Add(-time.Hour), "Cake in the kitchen.")
			a.createComment(t, signe, post, base.Add(-time.Minute), "It is carrot.")
			cookie := a.signIn(t, signe)

			// The session still passes, and then the business logic finds otherwise.
			test.set(a.store)

			res := a.get(t, cookie, "/")
			is.Equal(t, http.StatusOK, res.Code)
			page := res.Body.String()
			is.Equal(t, "This group is private.", text(one(t, parse(t, page), tag("h1"))))
			for _, secret := range []string{"The Loft", "Cake", "carrot", "Signe"} {
				is.True(t, !strings.Contains(page, secret), page)
			}

			for path, form := range requests(post) {
				res := a.post(t, cookie, path, form, true)
				is.Equal(t, http.StatusUnauthorized, res.Code, path)
				body := res.Body.String()
				is.Equal(t, "This group is private.", text(one(t, parse(t, body), tag("h1"))))
				for _, secret := range []string{"The Loft", "Cake", "carrot", "Signe"} {
					is.True(t, !strings.Contains(body, secret), body)
				}
			}
			is.Equal(t, 1, a.count(t, "posts"))
			is.Equal(t, 1, a.count(t, "comments"))
		})
	}

	t.Run("answers a failure underneath with an error page that has nothing of the group on it, and logs it without what was written", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		post := a.createPost(t, signe, base.Add(-time.Hour), "Cake in the kitchen.")
		cookie := a.signIn(t, signe)

		for path, operation := range map[string]string{
			"/posts":         "CreatePost",
			"/posts/earlier": "GetTimeline",
			"/comments/open": "GetComments",
			"/comments":      "CreateComment",
		} {
			a.store.fail = map[string]error{operation: errors.New("the disk is full")}
			a.logs.Reset()

			res := a.post(t, cookie, path, requests(post)[path], true)
			is.Equal(t, http.StatusInternalServerError, res.Code, path)
			is.True(t, !strings.Contains(res.Header().Get("Content-Type"), "event-stream"), path)
			body := res.Body.String()
			is.Equal(t, "Something went wrong", text(one(t, parse(t, body), tag("h1"))))
			for _, secret := range []string{"The Loft", "Cake", "Signe", "kettle", "disk"} {
				is.True(t, !strings.Contains(body, secret), body)
			}

			is.True(t, strings.Contains(a.logs.String(), "the disk is full"), a.logs.String())
			is.True(t, !strings.Contains(a.logs.String(), "kettle") && !strings.Contains(a.logs.String(), signe.Email.String()), a.logs.String())
		}
		is.Equal(t, 1, a.count(t, "posts"))
		is.Equal(t, 0, a.count(t, "comments"))
	})

	t.Run("does not count a group page that failed as a look", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		thomas := sqlitetest.CreateUser(t, a.db, "Thomas")
		a.createPost(t, thomas, base.Add(-time.Hour), "New to Signe.")
		cookie := a.signIn(t, signe)

		// Everything that can fail comes before the look, so a page that fails uses up no marks.
		for _, operation := range []string{"CountActiveUsers", "GetTimeline"} {
			a.store.fail = map[string]error{operation: errors.New("the disk is full")}

			res := a.get(t, cookie, "/")
			is.Equal(t, http.StatusInternalServerError, res.Code, operation)
			body := res.Body.String()
			is.Equal(t, "Something went wrong", text(one(t, parse(t, body), tag("h1"))))
			is.True(t, !strings.Contains(body, "The Loft") && !strings.Contains(body, "New to Signe."), body)
		}

		a.store.fail = nil
		doc := parse(t, a.get(t, cookie, "/").Body.String())
		is.EqualSlice(t, []string{"new"}, texts(all(doc, mark)))
	})

	t.Run("refuses a request that another site makes a member's browser send", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		post := a.createPost(t, signe, base.Add(-time.Hour), "Cake in the kitchen.")
		cookie := a.signIn(t, signe)

		for path, form := range requests(post) {
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("Origin", "https://evil.example")
			req.Header.Set("Sec-Fetch-Site", "cross-site")
			req.AddCookie(cookie)
			res := httptest.NewRecorder()
			a.handler.ServeHTTP(res, req)

			is.Equal(t, http.StatusForbidden, res.Code, path)
			is.True(t, !strings.Contains(res.Body.String(), "Cake"), res.Body.String())
		}
		is.Equal(t, 1, a.count(t, "posts"))
		is.Equal(t, 0, a.count(t, "comments"))
	})
}

func TestTimeline_traces(t *testing.T) {
	t.Run("records in no span what was written, an email address, or whose comments someone opened", func(t *testing.T) {
		// The app gets its tracers from the global provider when it is set up, so the recorder goes in
		// first, and the provider that was there comes back afterwards.
		recorder := tracetest.NewSpanRecorder()
		previous := otel.GetTracerProvider()
		otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder)))
		t.Cleanup(func() {
			otel.SetTracerProvider(previous)
		})

		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		thomas := sqlitetest.CreateUser(t, a.db, "Thomas")
		cookie := a.signIn(t, signe)

		// recorded is everything the spans that have ended so far say, each as one line.
		recorded := func() []string {
			var lines []string
			for _, span := range recorder.Ended() {
				line := span.Name() + " " + span.Status().Description
				for _, attribute := range span.Attributes() {
					line += " " + string(attribute.Key) + "=" + attribute.Value.String()
				}
				for _, event := range span.Events() {
					line += " " + event.Name
					for _, attribute := range event.Attributes {
						line += " " + string(attribute.Key) + "=" + attribute.Value.String()
					}
				}
				lines = append(lines, line)
			}
			return lines
		}

		a.get(t, cookie, "/")
		res := a.post(t, cookie, "/posts", url.Values{"body": {"A secret about the coffee machine"}}, true)
		postID := attr(one(t, a.patches(t, res)[1], tag("article")), "data-post")
		a.post(t, cookie, "/posts", url.Values{"body": {"An unposted secret\x00"}}, true)
		a.post(t, cookie, "/comments", url.Values{"post": {postID}, "body": {"A secret about the kettle"}}, true)
		a.post(t, cookie, "/comments", url.Values{"post": {postID}, "body": {""}, "field": {"secret"}}, true)
		a.store.fail = map[string]error{"CreatePost": errors.New("the disk is full")}
		a.post(t, cookie, "/posts", url.Values{"body": {"A secret that fails"}}, true)
		a.store.fail = nil

		written := recorded()
		is.True(t, len(written) > 10, "expected the recorder to have seen spans")
		for _, line := range written {
			for _, secret := range []string{"secret", "coffee", "kettle", signe.Email.String(), thomas.Email.String()} {
				is.True(t, !strings.Contains(line, secret), line)
			}
		}
		// The check sees what it should: the requests with their paths and who made them, and the
		// failure, which is not a secret.
		joined := strings.Join(written, "\n")
		is.True(t, strings.Contains(joined, "url.path=/comments"), joined)
		is.True(t, strings.Contains(joined, signe.ID.String()), joined)
		is.True(t, strings.Contains(joined, "the disk is full"), joined)
		is.True(t, strings.Contains(joined, postID), joined)

		// Thomas reads: he opens the comments, and loads earlier posts. Neither the post nor the cursor
		// is in anything recorded from then on, though the requests are, with who made them.
		before := len(written)
		thomasCookie := a.signIn(t, thomas)
		a.get(t, thomasCookie, "/")
		res = a.post(t, thomasCookie, "/comments/open", url.Values{"post": {postID}}, true)
		is.Equal(t, http.StatusOK, res.Code)
		a.post(t, thomasCookie, "/comments/open", url.Values{"post": {"p_nope"}}, true)
		after := model.TimelineCursor{Created: model.Time{T: base}, Seq: 5}.String()
		res = a.post(t, thomasCookie, "/posts/earlier", url.Values{"after": {after}, "days": {"2031-03-14"}}, true)
		is.Equal(t, http.StatusOK, res.Code)

		read := recorded()[before:]
		for _, line := range read {
			for _, secret := range []string{postID, "p_nope", after, "2031-03-14"} {
				is.True(t, !strings.Contains(line, secret), line)
			}
		}
		joined = strings.Join(read, "\n")
		is.True(t, strings.Contains(joined, "url.path=/comments/open"), joined)
		is.True(t, strings.Contains(joined, "url.path=/posts/earlier"), joined)
		is.True(t, strings.Contains(joined, thomas.ID.String()), joined)
	})
}

func TestInjectHTTPRouter(t *testing.T) {
	t.Run("panics without randomness for the tokens of forms", func(t *testing.T) {
		defer func() {
			is.True(t, recover() != nil, "no panic")
		}()
		apphttp.InjectHTTPRouter(slog.New(slog.DiscardHandler), service.NewFat(service.NewFatOptions{}), apphttp.InjectHTTPRouterOptions{Now: time.Now})(gluehttp.NewRouter(gluehttp.NewRouterOpts{SM: scs.New()}))
	})

	t.Run("gives forms tokens from the randomness it is given", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		cookie := a.signIn(t, signe)

		// The app reads its tokens from ChaCha8 with a seed of zeros, so the composer's is its first 16 bytes.
		first := make([]byte, 16)
		_, err := rand.NewChaCha8([32]byte{}).Read(first)
		is.NotError(t, err)
		expected, err := model.NewFormToken(bytes.NewReader(first))
		is.NotError(t, err)

		doc := parse(t, a.get(t, cookie, "/").Body.String())
		is.Equal(t, expected.String(), attr(one(t, one(t, doc, id("composer")), named("token")), "value"))
	})

	t.Run("shows nothing extra unless it is given something", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")

		for _, cookie := range []*http.Cookie{nil, a.signIn(t, signe)} {
			page := a.get(t, cookie, "/").Body.String()
			is.Equal(t, 0, len(all(parse(t, page), tag("a"))))
			is.True(t, !strings.Contains(page, "preview"), page)
		}
	})

	t.Run("shows the extra it is given, to someone who is signed in and to someone who is not", func(t *testing.T) {
		a := newApp(t, func(c *config) {
			c.extra = func(signedIn bool) Node {
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
	f.Add("Cake in the kitchen.", "2031-03-14", "", "", "q6urq6urq6urq6urq6urqw")
	f.Add("", "", "", "", "")
	f.Add("<script>alert(1)</script>", "<b>,2031-03-13", "\"><script>", "garbage", "\"><script>")
	f.Add("a\r\nb\x00", "2031-03-13,2031-03-12", "p_1", model.TimelineCursor{Created: model.Time{T: base}, Seq: 1}.String(), "q6urq6urq6urq6urq6urqx")
	f.Add(strings.Repeat("æ", model.PostBodyMaxLength+1), "\xff", "\r\nevent: x", "AQAAAAAAAAAAAAAAAAAAAAAB", "q6urq6urq6u\nrq6urq6urqw")
	f.Add("x", "0000-00-00,9999-12-31,2031-13-01,#timeline", "x'+alert(1)+'", "Af__x3zt0ygAAAAAAAAAAAA", "AQAAAZvNMb8AAAAAAAAAACo")
	// Found by fuzzing: a refusal put a field value that is not valid UTF-8 back into the answer as it came.
	f.Add("", "\xff", "\r\nevent: \xb7", "AQAAAAAAAAAAAAAAAAAAAAAB", "\xff")

	f.Fuzz(func(t *testing.T, body, days, field, after, token string) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		post := a.createPost(t, signe, base.Add(-time.Hour), "Cake.")
		cookie := a.signIn(t, signe)

		// Whatever is sent, the answer is patches or a plain refusal, never a failure of the server,
		// and patches always decode and parse, with nothing in them that the page does not put there,
		// and aimed only at the places the page has.
		res := a.post(t, cookie, "/posts", url.Values{"body": {body}, "days": {days}, "field": {field}}, true)
		is.Equal(t, http.StatusOK, res.Code)
		is.True(t, utf8.Valid(res.Body.Bytes()), "the answer is not valid UTF-8")
		patches := a.patches(t, res)
		is.Equal(t, 1, len(all(patches[0], id("composer"))))
		is.Equal(t, 1, len(all(patches[0], tag("textarea"))))
		for _, p := range patches {
			is.Equal(t, 0, len(all(p, tag("script"))))
		}
		for _, d := range a.decode(t, res) {
			is.True(t, regexp.MustCompile(`^(|#timeline|#day-\d{4}-\d{2}-\d{2}-posts)$`).MatchString(d.Selector), d.Selector)
			is.True(t, slices.Contains([]string{"", "inner", "prepend"}, d.Mode), d.Mode)
		}

		res = a.post(t, cookie, "/comments", url.Values{"post": {post.ID.String()}, "body": {body}, "field": {field}}, true)
		is.Equal(t, http.StatusOK, res.Code)
		is.True(t, utf8.Valid(res.Body.Bytes()), "the answer is not valid UTF-8")
		patches = a.patches(t, res)
		is.Equal(t, 1, len(all(patches[0], tag("textarea"))))
		is.Equal(t, 0, len(all(patches[0], tag("script"))))

		res = a.post(t, cookie, "/comments/open", url.Values{"post": {field}}, true)
		is.Equal(t, http.StatusNotFound, res.Code)

		res = a.post(t, cookie, "/posts/earlier", url.Values{"after": {after}, "days": {days}}, true)
		is.True(t, res.Code == http.StatusBadRequest || res.Code == http.StatusOK, fmt.Sprint(res.Code))
		if res.Code == http.StatusOK {
			for _, d := range a.decode(t, res) {
				is.True(t, regexp.MustCompile(`^(|#timeline|#day-\d{4}-\d{2}-\d{2}-posts)$`).MatchString(d.Selector), d.Selector)
			}
		}

		// A form with any token is answered with patches or refused as a request the page could not
		// have sent, and only a token that could have been made makes anything.
		_, tokenErr := model.ParseFormToken(token)
		for path, form := range map[string]url.Values{
			"/posts":    {"body": {"Fuzzed."}, "token": {token}},
			"/comments": {"body": {"Fuzzed."}, "token": {token}, "post": {post.ID.String()}},
		} {
			posts, comments := a.count(t, "posts"), a.count(t, "comments")
			res = a.post(t, cookie, path, form, true)
			if tokenErr != nil {
				is.Equal(t, http.StatusBadRequest, res.Code, path)
				is.Equal(t, posts, a.count(t, "posts"), path)
				is.Equal(t, comments, a.count(t, "comments"), path)
				continue
			}
			is.Equal(t, http.StatusOK, res.Code, path)
			a.patches(t, res)
		}
	})
}

// FuzzTimeline_form sends bytes that may not be a form at all, to every route that reads one.
func FuzzTimeline_form(f *testing.F) {
	f.Add([]byte("body=Cake&days=2031-03-14"))
	f.Add([]byte("body=%zz&post=%"))
	f.Add([]byte("body=%00%ff&after=%0d%0a&post=;&field==&&"))
	f.Add([]byte{0xff, 0xfe, '=', 0, '&'})
	f.Add([]byte(strings.Repeat("a=b&", 2000)))

	f.Fuzz(func(t *testing.T, form []byte) {
		a := newApp(t)
		signe := sqlitetest.CreateUser(t, a.db, "Signe")
		cookie := a.signIn(t, signe)

		for _, path := range []string{"/posts", "/comments", "/comments/open", "/posts/earlier"} {
			req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(form))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("Datastar-Request", "true")
			req.AddCookie(cookie)
			res := httptest.NewRecorder()
			a.handler.ServeHTTP(res, req)

			is.True(t, res.Code < 500, fmt.Sprint(path, " answered ", res.Code))
			if res.Header().Get("Content-Type") == "text/event-stream" {
				a.patches(t, res)
			}
		}
	})
}

// app is the app's routes behind the middleware that the server puts in front of every page, on a real
// database, with a clock the test sets.
type app struct {
	db      *sqlite.Database
	store   *store
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

// store is the real database, which a test can make fail for the business logic: with the given error
// from the operation of the given name, or by having everyone turn out inactive or gone when the
// business logic looks them up.
type store struct {
	*sqlite.Database
	fail     map[string]error
	inactive bool
	gone     bool
}

func (s *store) GetUser(ctx context.Context, id model.UserID) (model.User, error) {
	if s.gone {
		return model.User{}, model.ErrorUserNotFound
	}
	u, err := s.Database.GetUser(ctx, id)
	if s.inactive {
		u.Active = false
	}
	return u, err
}

func (s *store) CountActiveUsers(ctx context.Context) (int, error) {
	if err := s.fail["CountActiveUsers"]; err != nil {
		return 0, err
	}
	return s.Database.CountActiveUsers(ctx)
}

func (s *store) CreatePost(ctx context.Context, p model.Post, token model.FormToken) (model.Post, error) {
	if err := s.fail["CreatePost"]; err != nil {
		return model.Post{}, err
	}
	return s.Database.CreatePost(ctx, p, token)
}

func (s *store) CreateComment(ctx context.Context, c model.Comment, token model.FormToken) (model.Comment, error) {
	if err := s.fail["CreateComment"]; err != nil {
		return model.Comment{}, err
	}
	return s.Database.CreateComment(ctx, c, token)
}

func (s *store) GetTimeline(ctx context.Context, reader model.UserID, after *model.TimelineCursor, limit int) ([]model.TimelinePost, *model.TimelineCursor, error) {
	if err := s.fail["GetTimeline"]; err != nil {
		return nil, nil, err
	}
	return s.Database.GetTimeline(ctx, reader, after, limit)
}

func (s *store) GetComments(ctx context.Context, reader model.UserID, postID model.PostID) ([]model.TimelineComment, error) {
	if err := s.fail["GetComments"]; err != nil {
		return nil, err
	}
	return s.Database.GetComments(ctx, reader, postID)
}

// config of an [app].
type config struct {
	zone  string
	extra func(signedIn bool) Node
}

// inZone sets the group's time zone.
func inZone(zone string) func(*config) {
	return func(c *config) {
		c.zone = zone
	}
}

// newApp for the group "The Loft", in Copenhagen unless the options say otherwise, at the time of [base].
func newApp(t *testing.T, opts ...func(*config)) *app {
	t.Helper()

	c := config{zone: "Europe/Copenhagen"}
	for _, opt := range opts {
		opt(&c)
	}

	loc, err := time.LoadLocation(c.zone)
	is.NotError(t, err)
	group := model.Group{Name: "The Loft", TimeZone: loc}

	a := &app{db: sqlitetest.NewDatabase(t), sm: scs.New(), clock: &clock{t: base}, logs: &logs{}}
	a.store = &store{Database: a.db, fail: map[string]error{}}
	log := slog.New(slog.NewTextHandler(a.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// The session's user is looked up in the real database, and everything after that in the store.
	fat := service.NewFat(service.NewFatOptions{Log: log})
	service.GetUser(fat, a.db)
	service.CreateComment(fat, a.store, a.clock.Now)
	service.CreatePost(fat, a.store, a.clock.Now)
	service.GetComments(fat, a.store)
	service.GetMembers(fat, a.store)
	service.GetTimeline(fat, a.store, a.clock.Now, group)

	// The same middleware as [gluehttp.Server] puts in front of every page, in its order and nesting.
	protection := http.NewCrossOriginProtection()
	r := gluehttp.NewRouter(gluehttp.NewRouterOpts{SM: a.sm})
	r.Use(gluehttp.OpenTelemetry, protection.Handler)
	r.Group(func(r *gluehttp.Router) {
		r.Use(httph.NoClickjacking, httph.ContentSecurityPolicy(apphttp.CSP(false)))
		r.Use(a.sm.LoadAndSave, gluehttp.Authenticate(log, a.sm, a.db))
		r.NotFound(gluehttp.NotFound(html.Page))

		r.Group(func(r *gluehttp.Router) {
			apphttp.InjectHTTPRouter(log, fat, apphttp.InjectHTTPRouterOptions{Group: group, Now: a.clock.Now, Random: rand.NewChaCha8([32]byte{}), Extra: c.extra})(r)
		})
	})
	a.handler = r.Mux

	return a
}

// signIn the given user by putting them in a session of the app's session manager, and return the
// session's cookie.
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

// post the given form, as Datastar does or as a browser does by itself. A form to post or to comment
// carries a new token, as one from the page does, unless the test gives it one or an empty one.
func (a *app) post(t *testing.T, cookie *http.Cookie, path string, form url.Values, datastar bool) *httptest.ResponseRecorder {
	t.Helper()

	if (path == "/posts" || path == "/comments") && !form.Has("token") {
		form = maps.Clone(form)
		if form == nil {
			form = url.Values{}
		}
		form.Set("token", newToken(t))
	}

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

	p, err := a.db.CreatePost(t.Context(), model.Post{Created: model.Time{T: at}, AuthorID: author.ID, Body: body}, "")
	is.NotError(t, err)
	return p
}

func (a *app) createComment(t *testing.T, author model.User, post model.Post, at time.Time, body string) model.Comment {
	t.Helper()

	c, err := a.db.CreateComment(t.Context(), model.Comment{Created: model.Time{T: at}, PostID: post.ID, AuthorID: author.ID, Body: body}, "")
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

// announced is what the given patches say to screen readers: the text of the one live region in them,
// which has one element in it, named after what the words are about.
func announced(t *testing.T, patches []*nethtml.Node) string {
	t.Helper()

	var regions []*nethtml.Node
	for _, p := range patches {
		regions = append(regions, all(p, id("announcement"))...)
	}
	is.Equal(t, 1, len(regions))
	is.Equal(t, "status", attr(regions[0], "role"))
	is.Equal(t, "polite", attr(regions[0], "aria-live"))
	said := one(t, regions[0], func(n *nethtml.Node) bool { return strings.HasPrefix(attr(n, "id"), "announcement-") })
	return text(said)
}

// focusTakers in the given patches: the tags of the elements that take the focus when they are put on
// the page.
func focusTakers(patches []*nethtml.Node) []string {
	var tags []string
	for _, p := range patches {
		for _, n := range all(p, hasAttr("data-init")) {
			if strings.Contains(attr(n, "data-init"), "focus()") {
				tags = append(tags, n.Data)
			}
		}
	}
	return tags
}

// newToken for a form, as the page has.
func newToken(t testing.TB) string {
	t.Helper()

	token, err := model.NewFormToken(cryptorand.Reader)
	is.NotError(t, err)
	return token.String()
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
