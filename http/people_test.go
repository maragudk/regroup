package http_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	nethtml "golang.org/x/net/html"
	gluehttp "maragu.dev/glue/http"
	"maragu.dev/is"

	apphttp "app/http"
	"app/jobs"
	"app/jobstest"
	"app/model"
	"app/sqlitetest"
)

// runJobs of the app that are waiting, and return the tokens of the links in the emails they sent.
func (a *app) runJobs(t *testing.T) []string {
	t.Helper()

	jobstest.Run(t, a.db, jobs.Funcs(jobs.RegisterOpts{Emails: a.fat}))
	var tokens []string
	for _, e := range a.sender.Take() {
		tokens = append(tokens, e.Keywords["token"])
	}
	return tokens
}

// link for the given address, asked for through the app, from the one email sent.
func (a *app) link(t *testing.T, address string) string {
	t.Helper()

	res := a.post(t, nil, "/sign-in", url.Values{"email": {address}}, false)
	is.Equal(t, http.StatusOK, res.Code)
	tokens := a.runJobs(t)
	is.Equal(t, 1, len(tokens))
	return tokens[0]
}

// invitation for the given address from the given person, made through the business logic, and the
// token of the link in its email.
func (a *app) invitation(t *testing.T, from model.User, address string) (model.Invitation, string) {
	t.Helper()

	inv, err := a.fat.Invite(t.Context(), from.ID, newToken(t), address)
	is.NotError(t, err)
	tokens := a.runJobs(t)
	is.Equal(t, 1, len(tokens))
	return inv, tokens[0]
}

// whoIsSignedIn on the group page with the given cookie: the name it shows, or "" if it says the group is
// private.
func (a *app) whoIsSignedIn(t *testing.T, cookie *http.Cookie) string {
	t.Helper()

	res := a.get(t, cookie, "/")
	is.Equal(t, http.StatusOK, res.Code)
	body := res.Body.String()
	if strings.Contains(body, "This group is private.") {
		return ""
	}
	i := strings.Index(body, "data-person")
	is.True(t, i >= 0, body)
	rest := body[i:]
	rest = rest[strings.IndexByte(rest, '>')+1:]
	return rest[:strings.IndexByte(rest, '<')]
}

// sessionCookie of the answer, if it sets one.
func (a *app) sessionCookie(res *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range res.Result().Cookies() {
		if c.Name == a.sm.Cookie.Name {
			return c
		}
	}
	return nil
}

// send the form with the given marker under the given node as a browser does, with what is in its
// fields, the given values in place of those of the same names, and the given cookie, and return the
// answer.
func (a *app) send(t *testing.T, cookie *http.Cookie, n *nethtml.Node, marker string, values url.Values) *httptest.ResponseRecorder {
	t.Helper()

	form := one(t, n, func(n *nethtml.Node) bool { return n.Data == "form" && attr(n, "data-form") == marker })
	is.Equal(t, "post", attr(form, "method"))
	sent := url.Values{}
	for _, input := range all(form, tag("input")) {
		sent.Set(attr(input, "name"), attr(input, "value"))
	}
	for k, v := range values {
		sent[k] = v
	}
	return a.post(t, cookie, attr(form, "action"), sent, false)
}

// page of the given answer, which must be one, parsed.
func page(t *testing.T, res *httptest.ResponseRecorder) *nethtml.Node {
	t.Helper()

	is.True(t, strings.HasPrefix(res.Body.String(), "<!doctype html>"), res.Body.String())
	return parse(t, res.Body.String())
}

// nonces of the content security policy, which every answer has a new one of, in a page and in a header.
var nonces = regexp.MustCompile(`data-nonce="[^"]*"|'nonce-[^']*'`)

// sameAnswer checks that the given answers are the same, status, headers, and body, but for the nonce.
func sameAnswer(t *testing.T, a, b *httptest.ResponseRecorder) {
	t.Helper()

	is.Equal(t, a.Code, b.Code)
	is.Equal(t, nonces.ReplaceAllString(a.Body.String(), ""), nonces.ReplaceAllString(b.Body.String(), ""))
	is.Equal(t, len(a.Header()), len(b.Header()))
	for k := range a.Header() {
		is.Equal(t, nonces.ReplaceAllString(a.Header().Get(k), ""), nonces.ReplaceAllString(b.Header().Get(k), ""), k)
	}
}

// messages on the given page, by their keys.
func messages(doc *nethtml.Node) []string {
	return attrs(all(doc, hasAttr("data-message")), "data-message")
}

func TestPeople_signIn(t *testing.T) {
	t.Run("shows someone who is not signed in the same page at / and /sign-in: that the group is private, with one field to ask for a link, and nothing of the group", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreatePerson(t, a.db, "Signe", "signe@example.com", model.RoleMember)
		a.createPost(t, signe, base.Add(-time.Hour), "Cake in the kitchen.")

		root := a.get(t, nil, "/")
		signIn := a.get(t, nil, "/sign-in")
		is.Equal(t, http.StatusOK, signIn.Code)
		sameAnswer(t, root, signIn)

		doc := page(t, signIn)
		is.Equal(t, "This group is private.", text(one(t, doc, tag("h1"))))
		is.Equal(t, "regroup", text(one(t, doc, tag("title"))))
		form := one(t, doc, tag("form"))
		is.Equal(t, "/sign-in", attr(form, "action"))
		field := one(t, form, named("email"))
		is.Equal(t, "email", attr(field, "type"))
		is.Equal(t, "email", attr(field, "autocomplete"))
		is.Equal(t, "Email address", text(one(t, doc, func(n *nethtml.Node) bool { return n.Data == "label" && attr(n, "for") == attr(field, "id") })))
		is.EqualSlice(t, []string{"Send me a link"}, texts(all(doc, tag("button"))))
		for _, secret := range []string{"The Loft", "Cake", "Signe", "signe@"} {
			is.True(t, !strings.Contains(signIn.Body.String(), secret), secret)
		}
	})

	t.Run("sends someone who is signed in from /sign-in to the group page, and gives someone no longer active the page to ask for a link", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreatePerson(t, a.db, "Signe", "signe@example.com", model.RoleMember)
		karen := sqlitetest.CreatePerson(t, a.db, "Karen", "karen@example.com", model.RoleMember)
		karenCookie := a.signIn(t, karen)
		sqlitetest.DeactivateUser(t, a.db, karen.ID)

		res := a.get(t, a.signIn(t, signe), "/sign-in")
		is.Equal(t, http.StatusSeeOther, res.Code)
		is.Equal(t, "/", res.Header().Get("Location"))

		res = a.get(t, karenCookie, "/sign-in")
		is.Equal(t, http.StatusOK, res.Code)
		is.Equal(t, "This group is private.", text(one(t, page(t, res), tag("h1"))))
	})

	t.Run("answers asking for a link with a member's address in any spelling, the first admin's, a former member's, and one nobody has, with the same page but for the nonce", func(t *testing.T) {
		a := newApp(t, withFirstAdmin("first@example.com"))
		sqlitetest.CreatePerson(t, a.db, "Signe", "signe@example.com", model.RoleMember)
		karen := sqlitetest.CreatePerson(t, a.db, "Karen", "karen@example.com", model.RoleMember)
		sqlitetest.DeactivateUser(t, a.db, karen.ID)

		var answers []*httptest.ResponseRecorder
		for _, address := range []string{"signe@example.com", "nobody@example.com", "karen@example.com", " Signe@Example.com ", "first@example.com"} {
			answers = append(answers, a.post(t, nil, "/sign-in", url.Values{"email": {address}}, false))
			is.Equal(t, 1, len(jobstest.Take(t, a.db)), address)
		}
		for _, res := range answers[1:] {
			sameAnswer(t, answers[0], res)
		}

		is.Equal(t, http.StatusOK, answers[0].Code)
		doc := page(t, answers[0])
		is.EqualSlice(t, []string{"sent"}, messages(doc))
		is.Equal(t, "If that address belongs to someone in the group, a link to sign in is on its way to it. It works once, for the next 15 minutes.", text(one(t, doc, hasAttr("data-message"))))
		is.Equal(t, 0, len(all(doc, tag("form"))))
		is.True(t, !strings.Contains(answers[0].Body.String(), "signe"), answers[0].Body.String())
	})

	t.Run("sends a link to a member, and nothing to an address nobody has or a former member's", func(t *testing.T) {
		a := newApp(t)
		sqlitetest.CreatePerson(t, a.db, "Signe", "signe@example.com", model.RoleMember)
		karen := sqlitetest.CreatePerson(t, a.db, "Karen", "karen@example.com", model.RoleMember)
		sqlitetest.DeactivateUser(t, a.db, karen.ID)

		for _, address := range []string{"signe@example.com", "nobody@example.com", "karen@example.com"} {
			is.Equal(t, http.StatusOK, a.post(t, nil, "/sign-in", url.Values{"email": {address}}, false).Code)
		}
		jobstest.Run(t, a.db, jobs.Funcs(jobs.RegisterOpts{Emails: a.fat}))
		sent := a.sender.Take()
		is.Equal(t, 1, len(sent))
		is.Equal(t, model.EmailAddress("signe@example.com"), sent[0].To)
	})

	t.Run("refuses text that is not an email address with a plain message under the field, keeps what was typed, and asks for nothing", func(t *testing.T) {
		a := newApp(t)

		for _, typed := range []string{"signe", "", "<b>signe</b>@", "signe@example.com\nx"} {
			res := a.post(t, nil, "/sign-in", url.Values{"email": {typed}}, false)
			is.Equal(t, http.StatusOK, res.Code)
			doc := page(t, res)
			is.EqualSlice(t, []string{"email-invalid"}, messages(doc))
			is.Equal(t, "That is not an email address.", text(one(t, doc, hasAttr("data-message"))))
			field := one(t, doc, named("email"))
			is.Equal(t, typed, attr(field, "value"))
			is.Equal(t, "true", attr(field, "aria-invalid"))
		}
		is.Equal(t, 0, len(jobstest.Take(t, a.db)))
	})

	t.Run("signs in from asking for a link to the timeline: the link from the email opens a page that names the group, with one button, which signs in and goes to the group page", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreatePerson(t, a.db, "Signe", "signe@example.com", model.RoleMember)
		a.createPost(t, signe, base.Add(-time.Hour), "Cake in the kitchen.")

		res := a.send(t, nil, page(t, a.get(t, nil, "/")), "sign-in-request", url.Values{"email": {"signe@example.com"}})
		is.Equal(t, http.StatusOK, res.Code)
		tokens := a.runJobs(t)
		is.Equal(t, 1, len(tokens))

		for range 2 {
			res = a.get(t, nil, "/sign-in/link?token="+tokens[0])
			is.Equal(t, http.StatusOK, res.Code)
			is.Equal(t, "no-referrer", res.Header().Get("Referrer-Policy"))
			is.Equal(t, "no-store", res.Header().Get("Cache-Control"))
		}
		doc := page(t, res)
		is.Equal(t, "The Loft", text(one(t, doc, tag("h1"))))
		is.Equal(t, "Sign in to The Loft", text(one(t, doc, tag("title"))))
		is.EqualSlice(t, []string{"Sign in"}, texts(all(doc, tag("button"))))
		is.Equal(t, 0, len(all(doc, func(n *nethtml.Node) bool { return n.Data == "input" && attr(n, "type") != "hidden" })))
		is.Equal(t, "/sign-in/link", attr(one(t, doc, tag("form")), "action"))

		res = a.send(t, nil, doc, "sign-in", nil)
		is.Equal(t, http.StatusSeeOther, res.Code)
		is.Equal(t, "/", res.Header().Get("Location"))
		cookie := a.sessionCookie(res)
		is.True(t, cookie != nil, "expected a session cookie")
		timeline := page(t, a.get(t, cookie, "/"))
		is.Equal(t, "Signe", text(one(t, timeline, hasAttr("data-person"))))
		is.EqualSlice(t, []string{"Cake in the kitchen."}, texts(all(timeline, hasAttr("data-body"))))

		// Pressing the button again is a link that does not work any more.
		res = a.send(t, nil, doc, "sign-in", nil)
		is.Equal(t, http.StatusOK, res.Code)
		is.EqualSlice(t, []string{"link-invalid"}, messages(page(t, res)))
		is.True(t, a.sessionCookie(res) == nil)
	})

	t.Run("answers a used, expired, or garbage link, and one of someone made inactive, opened or pressed, with one page that does not say why or name the group, and has the field to ask for a new one", func(t *testing.T) {
		a := newApp(t)
		sqlitetest.CreatePerson(t, a.db, "Signe", "signe@example.com", model.RoleMember)
		thomas := sqlitetest.CreatePerson(t, a.db, "Thomas", "thomas@example.com", model.RoleMember)
		used := a.link(t, "signe@example.com")
		is.Equal(t, http.StatusSeeOther, a.post(t, nil, "/sign-in/link", url.Values{"token": {used}}, false).Code)
		inactive := a.link(t, "thomas@example.com")
		sqlitetest.DeactivateUser(t, a.db, thomas.ID)
		a.clock.t = base.Add(time.Minute)
		expired := a.link(t, "signe@example.com")
		a.clock.t = base.Add(16 * time.Minute)

		var answers []*httptest.ResponseRecorder
		for _, token := range []string{used, inactive, expired, "garbage", ""} {
			answers = append(answers,
				a.get(t, nil, "/sign-in/link?token="+url.QueryEscape(token)),
				a.post(t, nil, "/sign-in/link", url.Values{"token": {token}}, false),
				a.post(t, nil, "/sign-in/link", url.Values{"token": {token}, "name": {""}}, false),
			)
		}
		for _, res := range answers {
			sameAnswer(t, answers[0], res)
		}

		is.Equal(t, http.StatusOK, answers[0].Code)
		doc := page(t, answers[0])
		is.Equal(t, "This group is private.", text(one(t, doc, tag("h1"))))
		is.Equal(t, "regroup", text(one(t, doc, tag("title"))))
		is.EqualSlice(t, []string{"link-invalid"}, messages(doc))
		is.Equal(t, "This link does not work. Ask for a new one.", text(one(t, doc, hasAttr("data-message"))))
		is.Equal(t, "/sign-in", attr(one(t, doc, tag("form")), "action"))
		for _, secret := range []string{"The Loft", "Signe", "Thomas"} {
			is.True(t, !strings.Contains(answers[0].Body.String(), secret), secret)
		}
	})

	t.Run("has the first admin choose the name the group will see, refuses one that cannot be used with why, keeping it and the link, and joins with one that can", func(t *testing.T) {
		a := newApp(t, withFirstAdmin("markus@example.com"))
		token := a.link(t, "Markus@Example.com")

		doc := page(t, a.get(t, nil, "/sign-in/link?token="+token))
		is.Equal(t, "The Loft", text(one(t, doc, tag("h1"))))
		is.Equal(t, "name", attr(one(t, doc, named("name")), "autocomplete"))
		is.EqualSlice(t, []string{"Join"}, texts(all(doc, tag("button"))))

		for _, test := range []struct {
			name, key, message string
		}{
			{" ", "name-empty", "Write the name the group will see first."},
			{strings.Repeat("a", 51), "name-too-long", "That is too long. A name can be up to 50 characters."},
			{"Mar\nkus", "name-invalid", "That name has characters that cannot be used in a name. Remove them and try again."},
		} {
			res := a.send(t, nil, doc, "join-first", url.Values{"name": {test.name}})
			is.Equal(t, http.StatusOK, res.Code)
			refused := page(t, res)
			is.EqualSlice(t, []string{test.key}, messages(refused))
			is.Equal(t, test.message, text(one(t, refused, hasAttr("data-message"))))
			is.Equal(t, test.name, attr(one(t, refused, named("name")), "value"))
			is.Equal(t, token, attr(one(t, refused, named("token")), "value"))
			is.Equal(t, "The Loft", text(one(t, refused, tag("h1"))))
			is.True(t, a.sessionCookie(res) == nil)
			doc = refused
		}

		res := a.send(t, nil, doc, "join-first", url.Values{"name": {"Markus"}})
		is.Equal(t, http.StatusSeeOther, res.Code)
		is.Equal(t, "Markus", a.whoIsSignedIn(t, a.sessionCookie(res)))
	})
}

func TestPeople_session(t *testing.T) {
	t.Run("starts a new session on signing in, which keeps nothing from the one before, and ends the one before", func(t *testing.T) {
		a := newApp(t)
		sqlitetest.CreatePerson(t, a.db, "Signe", "signe@example.com", model.RoleMember)
		thomas := sqlitetest.CreatePerson(t, a.db, "Thomas", "thomas@example.com", model.RoleMember)

		// Thomas is signed in on this browser, and his session holds something of his.
		ctx, err := a.sm.Load(t.Context(), "")
		is.NotError(t, err)
		a.sm.Put(ctx, gluehttp.SessionUserIDKey, thomas.ID.String())
		a.sm.Put(ctx, "draft", "Thomas's draft")
		value, _, err := a.sm.Commit(ctx)
		is.NotError(t, err)
		before := &http.Cookie{Name: a.sm.Cookie.Name, Value: value}
		is.Equal(t, "Thomas", a.whoIsSignedIn(t, before))

		res := a.post(t, before, "/sign-in/link", url.Values{"token": {a.link(t, "signe@example.com")}}, false)
		is.Equal(t, http.StatusSeeOther, res.Code)
		after := a.sessionCookie(res)
		is.True(t, after != nil && after.Value != before.Value, "expected a new session")
		is.Equal(t, "Signe", a.whoIsSignedIn(t, after))
		is.Equal(t, "", a.whoIsSignedIn(t, before))

		ctx, err = a.sm.Load(t.Context(), after.Value)
		is.NotError(t, err)
		is.Equal(t, "", a.sm.GetString(ctx, "draft"))
	})

	t.Run("ends the session of someone made inactive on their next request", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreatePerson(t, a.db, "Signe", "signe@example.com", model.RoleMember)
		res := a.post(t, nil, "/sign-in/link", url.Values{"token": {a.link(t, "signe@example.com")}}, false)
		cookie := a.sessionCookie(res)
		is.Equal(t, "Signe", a.whoIsSignedIn(t, cookie))

		sqlitetest.DeactivateUser(t, a.db, signe.ID)
		is.Equal(t, "", a.whoIsSignedIn(t, cookie))

		// The session is gone, so it stays gone if they are made active again.
		is.NotError(t, a.db.H.Exec(t.Context(), `update users set active = 1 where id = ?`, signe.ID))
		is.Equal(t, "", a.whoIsSignedIn(t, cookie))
	})

	t.Run("signs out with the button beside the person's name on the sign, which lands on the page that says the group is private", func(t *testing.T) {
		a := newApp(t)
		sqlitetest.CreatePerson(t, a.db, "Signe", "signe@example.com", model.RoleMember)
		cookie := a.sessionCookie(a.post(t, nil, "/sign-in/link", url.Values{"token": {a.link(t, "signe@example.com")}}, false))

		for _, path := range []string{"/", "/members"} {
			sign := one(t, page(t, a.get(t, cookie, path)), tag("header"))
			form := one(t, sign, func(n *nethtml.Node) bool { return n.Data == "form" && attr(n, "data-form") == "sign-out" })
			is.Equal(t, "/logout", attr(form, "action"))
			is.Equal(t, "Sign out", text(one(t, form, tag("button"))))
			is.Equal(t, "", attr(form, "data-on:submit"))
		}

		res := a.send(t, cookie, page(t, a.get(t, cookie, "/")), "sign-out", nil)
		is.Equal(t, http.StatusFound, res.Code)
		is.Equal(t, "/", res.Header().Get("Location"))
		is.Equal(t, "", a.whoIsSignedIn(t, cookie))
		is.Equal(t, "This group is private.", text(one(t, page(t, a.get(t, a.sessionCookie(res), "/")), tag("h1"))))
	})
}

func TestPeople_sign(t *testing.T) {
	t.Run("links the member count to the list of members, and the group's name to the group page, on both pages", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreatePerson(t, a.db, "Signe", "signe@example.com", model.RoleMember)
		sqlitetest.CreatePerson(t, a.db, "Thomas", "thomas@example.com", model.RoleMember)
		cookie := a.signIn(t, signe)

		for _, path := range []string{"/", "/members"} {
			sign := one(t, page(t, a.get(t, cookie, path)), tag("header"))
			count := one(t, sign, func(n *nethtml.Node) bool { return n.Data == "a" && attr(n, "href") == "/members" })
			is.Equal(t, "2 members", text(count))
			name := one(t, one(t, sign, tag("h1")), tag("a"))
			is.Equal(t, "/", attr(name, "href"))
			is.Equal(t, "The Loft", text(name))
			is.Equal(t, "Signe", text(one(t, sign, hasAttr("data-person"))))

			// The link to the page itself says so.
			for _, link := range []*nethtml.Node{count, name} {
				if attr(link, "href") == path {
					is.Equal(t, "page", attr(link, "aria-current"))
				} else {
					is.Equal(t, "", attr(link, "aria-current"))
				}
			}
		}
	})
}

func TestPeople_invitation(t *testing.T) {
	t.Run("invites from the list of members to the timeline: the email's link opens a page that names the group and who invited, and joining with a name signs in, after which the list shows the new member", func(t *testing.T) {
		a := newApp(t)
		markus := sqlitetest.CreatePerson(t, a.db, "Markus", "markus@example.com", model.RoleAdmin)
		signe := sqlitetest.CreatePerson(t, a.db, "Signe", "signe@example.com", model.RoleModerator)
		signeCookie := a.signIn(t, signe)

		res := a.send(t, signeCookie, page(t, a.get(t, signeCookie, "/members")), "invite", url.Values{"email": {"Amira@Example.com"}})
		is.Equal(t, http.StatusSeeOther, res.Code)
		is.Equal(t, "/members", res.Header().Get("Location"))
		tokens := a.runJobs(t)
		is.Equal(t, 1, len(tokens))

		for range 2 {
			res = a.get(t, nil, "/invitation?token="+tokens[0])
			is.Equal(t, http.StatusOK, res.Code)
			is.Equal(t, "no-referrer", res.Header().Get("Referrer-Policy"))
		}
		doc := page(t, res)
		is.Equal(t, "The Loft", text(one(t, doc, tag("h1"))))
		is.Equal(t, "Join The Loft", text(one(t, doc, tag("title"))))
		is.Equal(t, "Signe", text(one(t, doc, hasAttr("data-inviter"))))
		is.Equal(t, "name", attr(one(t, doc, named("name")), "autocomplete"))
		is.EqualSlice(t, []string{"Join"}, texts(all(doc, tag("button"))))

		res = a.send(t, nil, doc, "join", url.Values{"name": {""}})
		is.Equal(t, http.StatusOK, res.Code)
		refused := page(t, res)
		is.EqualSlice(t, []string{"name-empty"}, messages(refused))
		is.Equal(t, "Signe", text(one(t, refused, hasAttr("data-inviter"))))
		is.True(t, a.sessionCookie(res) == nil)

		res = a.send(t, nil, refused, "join", url.Values{"name": {"Amira"}})
		is.Equal(t, http.StatusSeeOther, res.Code)
		is.Equal(t, "/", res.Header().Get("Location"))
		is.Equal(t, "Amira", a.whoIsSignedIn(t, a.sessionCookie(res)))

		list := page(t, a.get(t, a.signIn(t, markus), "/members"))
		rows := all(list, hasAttr("data-member"))
		is.EqualSlice(t, []string{"Markus", "Signe", "Amira"}, texts(all(list, hasAttr("data-name"))))
		is.Equal(t, "member", attr(rows[2], "data-role"))
		is.True(t, strings.Contains(text(rows[2]), "amira@example.com"), text(rows[2]))
		is.Equal(t, 0, len(all(list, hasAttr("data-invitation"))))

		// The link is used up.
		res = a.send(t, nil, refused, "join", url.Values{"name": {"Amira"}})
		is.EqualSlice(t, []string{"invitation-invalid"}, messages(page(t, res)))
	})

	t.Run("answers an invitation that was used, withdrawn, replaced, has run out, or is garbage, opened or pressed, with one page that does not say why or name the group", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreatePerson(t, a.db, "Signe", "signe@example.com", model.RoleModerator)
		_, used := a.invitation(t, signe, "amira@example.com")
		_, err := a.fat.AcceptInvitation(t.Context(), used, "Amira")
		is.NotError(t, err)
		withdrawnInv, withdrawn := a.invitation(t, signe, "nadia@example.com")
		is.NotError(t, a.fat.WithdrawInvitation(t.Context(), signe.ID, withdrawnInv.ID))
		_, replaced := a.invitation(t, signe, "olga@example.com")
		a.invitation(t, signe, "olga@example.com")
		_, expired := a.invitation(t, signe, "petra@example.com")
		a.clock.t = base.Add(model.InvitationLifetime)

		var answers []*httptest.ResponseRecorder
		for _, token := range []string{used, withdrawn, replaced, expired, "garbage", ""} {
			answers = append(answers,
				a.get(t, nil, "/invitation?token="+url.QueryEscape(token)),
				a.post(t, nil, "/invitation", url.Values{"token": {token}, "name": {"Amira"}}, false),
				a.post(t, nil, "/invitation", url.Values{"token": {token}, "name": {""}}, false),
			)
		}
		for _, res := range answers {
			sameAnswer(t, answers[0], res)
		}

		is.Equal(t, http.StatusOK, answers[0].Code)
		doc := page(t, answers[0])
		is.Equal(t, "This group is private.", text(one(t, doc, tag("h1"))))
		is.EqualSlice(t, []string{"invitation-invalid"}, messages(doc))
		is.Equal(t, "This invitation does not work. Ask the person who invited you for a new one.", text(one(t, doc, hasAttr("data-message"))))
		for _, secret := range []string{"The Loft", "Signe", "Amira"} {
			is.True(t, !strings.Contains(answers[0].Body.String(), secret), secret)
		}
	})

	t.Run("answers an invitation to an address that has joined in the meantime as one that does not work", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreatePerson(t, a.db, "Signe", "signe@example.com", model.RoleModerator)
		_, token := a.invitation(t, signe, "amira@example.com")
		sqlitetest.CreatePerson(t, a.db, "Amira", "amira@example.com", model.RoleMember)

		res := a.post(t, nil, "/invitation", url.Values{"token": {token}, "name": {"Amira"}}, false)
		is.Equal(t, http.StatusOK, res.Code)
		is.EqualSlice(t, []string{"invitation-invalid"}, messages(page(t, res)))
	})
}

// people of a made-up group for the pages for members: an admin, a moderator, a member, a moderator who
// the test may make inactive, a post with a comment, and an invitation that is waiting.
type people struct {
	markus, signe, thomas, karen model.User
	invitation                   model.Invitation
	post                         model.Post
}

func seed(t *testing.T, a *app) people {
	t.Helper()

	p := people{
		markus: sqlitetest.CreatePerson(t, a.db, "Markus", "markus@example.com", model.RoleAdmin),
		signe:  sqlitetest.CreatePerson(t, a.db, "Signe", "signe@example.com", model.RoleModerator),
		thomas: sqlitetest.CreatePerson(t, a.db, "Thomas", "thomas@example.com", model.RoleMember),
		karen:  sqlitetest.CreatePerson(t, a.db, "Karen", "karen@example.com", model.RoleModerator),
	}
	p.post = a.createPost(t, p.signe, base.Add(-time.Hour), "A secret about the kettle.")
	a.createComment(t, p.thomas, p.post, base.Add(-time.Minute), "It is carrot.")
	p.invitation, _ = a.invitation(t, p.signe, "nadia@example.com")
	return p
}

func TestPeople_members(t *testing.T) {
	t.Run("shows a member the names and roles in the order people joined, and no address, field, or control but signing out", func(t *testing.T) {
		a := newApp(t)
		p := seed(t, a)
		sqlitetest.DeactivateUser(t, a.db, p.karen.ID)

		res := a.get(t, a.signIn(t, p.thomas), "/members")
		is.Equal(t, http.StatusOK, res.Code)
		doc := page(t, res)
		is.Equal(t, "Members of The Loft", text(one(t, doc, tag("title"))))
		rows := all(doc, hasAttr("data-member"))
		is.EqualSlice(t, []string{p.markus.ID.String(), p.signe.ID.String(), p.thomas.ID.String()}, attrs(rows, "data-member"))
		is.EqualSlice(t, []string{"admin", "moderator", "member"}, attrs(rows, "data-role"))
		is.EqualSlice(t, []string{"Markus", "Signe", "Thomas"}, texts(all(doc, hasAttr("data-name"))))
		is.True(t, !strings.Contains(res.Body.String(), "@example.com"), res.Body.String())
		is.EqualSlice(t, []string{"/logout"}, attrs(all(doc, tag("form")), "action"))
		is.Equal(t, 0, len(all(doc, hasAttr("data-invitation"))))
		is.Equal(t, "3 members", text(one(t, one(t, doc, tag("header")), func(n *nethtml.Node) bool { return attr(n, "href") == "/members" })))
	})

	t.Run("shows a moderator the field to invite, and the invitations that are waiting, with who invited and when each runs out, and no address of a member", func(t *testing.T) {
		a := newApp(t)
		p := seed(t, a)

		res := a.get(t, a.signIn(t, p.signe), "/members")
		doc := page(t, res)
		for _, address := range []string{"markus@", "signe@", "thomas@"} {
			is.True(t, !strings.Contains(res.Body.String(), address), address)
		}
		is.EqualSlice(t, []string{"/logout", "/invitations", "/invitations/withdraw"}, attrs(all(doc, tag("form")), "action"))
		row := one(t, doc, hasAttr("data-invitation"))
		is.Equal(t, p.invitation.ID.String(), attr(row, "data-invitation"))
		// The invitation was made at 10:00 in Copenhagen on Friday 14 March, and works for seven days.
		is.Equal(t, "nadia@example.comInvited by Signe. Runs out on Friday 21 March at 10:00.Withdraw the invitation to nadia@example.com", text(row))
		_, err := model.ParseFormToken(attr(one(t, doc, named("token")), "value"))
		is.NotError(t, err)
		is.Equal(t, 0, len(all(doc, tag("details"))))
	})

	t.Run("shows an admin the addresses, and lets them change the role of everyone but themselves", func(t *testing.T) {
		a := newApp(t)
		p := seed(t, a)
		cookie := a.signIn(t, p.markus)

		doc := page(t, a.get(t, cookie, "/members"))
		list := one(t, doc, id("members"))
		is.EqualSlice(t, []string{"markus@example.com", "signe@example.com", "thomas@example.com", "karen@example.com"}, texts(all(list, hasAttr("data-email"))))
		is.EqualSlice(t, []string{p.signe.ID.String(), p.thomas.ID.String(), p.karen.ID.String()}, attrs(all(list, named("member")), "value"))
		is.Equal(t, 0, len(all(all(doc, hasAttr("data-member"))[0], tag("form"))))

		row := func(doc *nethtml.Node, id model.UserID) *nethtml.Node {
			return one(t, doc, func(n *nethtml.Node) bool { return attr(n, "data-member") == id.String() })
		}
		res := a.send(t, cookie, row(doc, p.thomas.ID), "change-role", url.Values{"role": {"moderator"}})
		is.Equal(t, http.StatusSeeOther, res.Code)
		is.Equal(t, "/members", res.Header().Get("Location"))
		doc = page(t, a.get(t, cookie, "/members"))
		is.Equal(t, "moderator", attr(row(doc, p.thomas.ID), "data-role"))
		is.True(t, strings.Contains(text(row(doc, p.thomas.ID)), "Moderator"))
	})

	for _, test := range []struct {
		name    string
		typed   string
		key     string
		message string
	}{
		{"text that is not an address", "nadia", "email-invalid", "That is not an email address."},
		{"a member's address", " Thomas@Example.com", "already-member", "That address belongs to someone who is in the group already."},
		{"the address of someone who is no longer active", "karen@example.com", "former-member", "That address belongs to someone who was in the group. They cannot be invited again for now."},
	} {
		t.Run("refuses to invite "+test.name+" with a plain message under the field, keeps what was typed, and invites nobody", func(t *testing.T) {
			a := newApp(t)
			p := seed(t, a)
			sqlitetest.DeactivateUser(t, a.db, p.karen.ID)
			cookie := a.signIn(t, p.signe)

			doc := page(t, a.get(t, cookie, "/members"))
			res := a.send(t, cookie, doc, "invite", url.Values{"email": {test.typed}})
			is.Equal(t, http.StatusOK, res.Code)
			refused := page(t, res)
			is.EqualSlice(t, []string{test.key}, messages(refused))
			is.Equal(t, test.message, text(one(t, refused, hasAttr("data-message"))))
			field := one(t, refused, named("email"))
			is.Equal(t, test.typed, attr(field, "value"))
			is.Equal(t, "true", attr(field, "aria-invalid"))
			// The form that comes back has a new token.
			is.True(t, attr(one(t, refused, named("token")), "value") != attr(one(t, doc, named("token")), "value"))
			is.Equal(t, 0, len(jobstest.Take(t, a.db)))
			is.Equal(t, 1, a.count(t, "invitations"))
		})
	}

	t.Run("invites once when the same form is sent again, answers as the first time, and says so when it is sent again with another address", func(t *testing.T) {
		a := newApp(t)
		p := seed(t, a)
		cookie := a.signIn(t, p.signe)
		doc := page(t, a.get(t, cookie, "/members"))

		for range 2 {
			res := a.send(t, cookie, doc, "invite", url.Values{"email": {"amira@example.com"}})
			is.Equal(t, http.StatusSeeOther, res.Code)
			is.Equal(t, "/members", res.Header().Get("Location"))
		}
		is.Equal(t, 1, len(jobstest.Take(t, a.db)))
		is.Equal(t, 2, a.count(t, "invitations"))

		res := a.send(t, cookie, doc, "invite", url.Values{"email": {"olga@example.com"}})
		is.Equal(t, http.StatusOK, res.Code)
		refused := page(t, res)
		is.EqualSlice(t, []string{"resent"}, messages(refused))
		is.Equal(t, "Your earlier try did go through, and that invitation is on its way. This address has not been invited.", text(one(t, refused, hasAttr("data-message"))))
		is.Equal(t, "olga@example.com", attr(one(t, refused, named("email")), "value"))
		is.Equal(t, 0, len(jobstest.Take(t, a.db)))
		is.Equal(t, 2, a.count(t, "invitations"))
	})

	t.Run("refuses a form to invite without a token, or with one that is not one, with a plain error, and invites nobody", func(t *testing.T) {
		a := newApp(t)
		p := seed(t, a)
		cookie := a.signIn(t, p.signe)

		for _, token := range []string{"", "garbage"} {
			res := a.post(t, cookie, "/invitations", url.Values{"email": {"amira@example.com"}, "token": {token}}, false)
			is.Equal(t, http.StatusBadRequest, res.Code)
		}
		is.Equal(t, 0, len(jobstest.Take(t, a.db)))
	})

	t.Run("says once on the list of members that an invitation is on its way", func(t *testing.T) {
		a := newApp(t)
		p := seed(t, a)
		cookie := a.signIn(t, p.signe)

		res := a.send(t, cookie, page(t, a.get(t, cookie, "/members")), "invite", url.Values{"email": {"amira@example.com"}})
		is.Equal(t, http.StatusSeeOther, res.Code)
		doc := page(t, a.get(t, cookie, "/members"))
		is.EqualSlice(t, []string{"invited"}, messages(doc))
		is.Equal(t, "The invitation is on its way.", text(one(t, doc, hasAttr("data-message"))))
		is.EqualSlice(t, []string{"amira@example.com", "nadia@example.com"}, texts(all(one(t, doc, id("invitations")), hasAttr("data-email"))))
		is.Equal(t, 0, len(messages(page(t, a.get(t, cookie, "/members")))))
	})

	t.Run("withdraws an invitation, whose link then does not work, and says so when it is not waiting any more", func(t *testing.T) {
		a := newApp(t)
		p := seed(t, a)
		_, token := a.invitation(t, p.signe, "amira@example.com")
		cookie := a.signIn(t, p.markus)
		doc := page(t, a.get(t, cookie, "/members"))
		row := one(t, doc, func(n *nethtml.Node) bool {
			return attr(n, "data-invitation") != "" && strings.Contains(text(n), "amira@")
		})

		res := a.send(t, cookie, row, "withdraw-invitation", nil)
		is.Equal(t, http.StatusSeeOther, res.Code)
		is.Equal(t, "/members", res.Header().Get("Location"))
		is.EqualSlice(t, []string{p.invitation.ID.String()}, attrs(all(page(t, a.get(t, cookie, "/members")), hasAttr("data-invitation")), "data-invitation"))
		is.EqualSlice(t, []string{"invitation-invalid"}, messages(page(t, a.get(t, nil, "/invitation?token="+token))))

		res = a.send(t, cookie, row, "withdraw-invitation", nil)
		is.Equal(t, http.StatusOK, res.Code)
		refused := page(t, res)
		is.EqualSlice(t, []string{"invitation-not-found"}, messages(refused))
		is.Equal(t, "That invitation is not waiting any more. It may have been accepted, withdrawn, or have run out.", text(one(t, refused, hasAttr("data-message"))))
	})

	for _, test := range []struct {
		name    string
		actor   func(p people) model.User
		target  func(p people) model.UserID
		role    string
		key     string
		message string
	}{
		{"an admin's own role", func(p people) model.User { return p.markus }, func(p people) model.UserID { return p.markus.ID }, "member", "own-role", "Nobody can change their own role."},
		{"a role, by a moderator", func(p people) model.User { return p.signe }, func(p people) model.UserID { return p.thomas.ID }, "moderator", "not-allowed", "Only admins can change roles."},
		{"a role, by a member", func(p people) model.User { return p.thomas }, func(p people) model.UserID { return p.signe.ID }, "member", "not-allowed", "Only admins can change roles."},
		{"the role of someone not in the group", func(p people) model.User { return p.markus }, func(p people) model.UserID { return p.karen.ID }, "member", "member-not-found", "That person is not in the group."},
		{"a role to one that is not one", func(p people) model.User { return p.markus }, func(p people) model.UserID { return p.thomas.ID }, "owner", "role-invalid", "That is not a role."},
	} {
		t.Run("refuses to change "+test.name+" with a plain message, and changes nothing", func(t *testing.T) {
			a := newApp(t)
			p := seed(t, a)
			sqlitetest.DeactivateUser(t, a.db, p.karen.ID)

			res := a.post(t, a.signIn(t, test.actor(p)), "/members/role", url.Values{"member": {test.target(p).String()}, "role": {test.role}}, false)
			is.Equal(t, http.StatusOK, res.Code)
			doc := page(t, res)
			is.EqualSlice(t, []string{test.key}, messages(doc))
			is.Equal(t, test.message, text(one(t, doc, hasAttr("data-message"))))
			is.EqualSlice(t, []string{"admin", "moderator", "member"}, attrs(all(doc, hasAttr("data-member")), "data-role"))
		})
	}

	for _, test := range []struct {
		name, path string
		form       func(p people) url.Values
		message    string
	}{
		{"invite", "/invitations", func(p people) url.Values { return url.Values{"email": {"amira@example.com"}} }, "Only moderators and admins can invite."},
		{"withdraw an invitation", "/invitations/withdraw", func(p people) url.Values { return url.Values{"invitation": {p.invitation.ID.String()}} }, "Only moderators and admins can withdraw invitations."},
	} {
		t.Run("refuses to let a member "+test.name+" with a plain message, and does nothing", func(t *testing.T) {
			a := newApp(t)
			p := seed(t, a)

			form := test.form(p)
			form.Set("token", newToken(t))
			res := a.post(t, a.signIn(t, p.thomas), test.path, form, false)
			is.Equal(t, http.StatusOK, res.Code)
			doc := page(t, res)
			is.EqualSlice(t, []string{"not-allowed"}, messages(doc))
			is.Equal(t, test.message, text(one(t, doc, hasAttr("data-message"))))
			is.True(t, !strings.Contains(res.Body.String(), "@example.com"), res.Body.String())
			is.Equal(t, 1, a.count(t, "invitations"))
			is.Equal(t, 0, len(jobstest.Take(t, a.db)))
		})
	}
}

// visitor of the app, as the table of who gets what sees them.
type visitor string

const (
	admin     visitor = "admin"
	moderator visitor = "moderator"
	member    visitor = "member"
	inactive  visitor = "inactive"
	nobody    visitor = "nobody"
)

func TestPeople_access(t *testing.T) {
	// secrets that each kind of visitor may not get from any route. Karen is made inactive, so nobody
	// gets her name or address. Roles are as the list of members shows them.
	everything := []string{"The Loft", "Markus", "Signe", "Thomas", "Karen", "kettle", "carrot", "@example.com", "data-role", "data-member", "Admin", "Moderator"}
	forbidden := map[visitor][]string{
		admin:     {"Karen", "karen@"},
		moderator: {"Karen", "markus@", "signe@", "thomas@", "karen@"},
		member:    {"Karen", "@example.com"},
		inactive:  everything,
		nobody:    everything,
	}

	type expected map[visitor]int
	all := func(code int) expected {
		return expected{admin: code, moderator: code, member: code, inactive: code, nobody: code}
	}
	staff := func(staff, others int) expected {
		return expected{admin: staff, moderator: staff, member: others, inactive: others, nobody: others}
	}
	members := func(in, out int) expected {
		return expected{admin: in, moderator: in, member: in, inactive: out, nobody: out}
	}

	for _, route := range []struct {
		method, path string
		form         func(p people) url.Values
		datastar     bool
		codes        expected
		// shows is what the answer has for some kinds of visitor, so that the table is not passed by
		// answers that have nothing in them.
		shows map[visitor][]string
	}{
		{method: http.MethodGet, path: "/", codes: all(http.StatusOK), shows: map[visitor][]string{admin: {"The Loft", "kettle"}, member: {"The Loft", "kettle"}, nobody: {"This group is private."}}},
		{method: http.MethodGet, path: "/sign-in", codes: members(http.StatusSeeOther, http.StatusOK)},
		{method: http.MethodPost, path: "/sign-in", form: func(people) url.Values { return url.Values{"email": {"thomas@example.com"}} }, codes: all(http.StatusOK)},
		{method: http.MethodGet, path: "/sign-in/link?token=garbage", codes: all(http.StatusOK)},
		{method: http.MethodPost, path: "/sign-in/link", form: func(people) url.Values { return url.Values{"token": {"garbage"}, "name": {"Signe"}} }, codes: all(http.StatusOK)},
		{method: http.MethodGet, path: "/invitation?token=garbage", codes: all(http.StatusOK)},
		{method: http.MethodPost, path: "/invitation", form: func(people) url.Values { return url.Values{"token": {"garbage"}, "name": {"Signe"}} }, codes: all(http.StatusOK)},
		{method: http.MethodGet, path: "/members", codes: all(http.StatusOK), shows: map[visitor][]string{admin: {"markus@example.com", "nadia@example.com", "Change role"}, moderator: {"nadia@example.com", "Withdraw"}, member: {"Markus", "Moderator"}, nobody: {"This group is private."}}},
		{method: http.MethodPost, path: "/invitations", form: func(people) url.Values { return url.Values{"email": {"amira@example.com"}} }, codes: staff(http.StatusSeeOther, http.StatusOK)},
		{method: http.MethodPost, path: "/invitations/withdraw", form: func(p people) url.Values { return url.Values{"invitation": {p.invitation.ID.String()}} }, codes: staff(http.StatusSeeOther, http.StatusOK)},
		{method: http.MethodPost, path: "/members/role", form: func(p people) url.Values { return url.Values{"member": {p.thomas.ID.String()}, "role": {"moderator"}} },
			codes: expected{admin: http.StatusSeeOther, moderator: http.StatusOK, member: http.StatusOK, inactive: http.StatusOK, nobody: http.StatusOK}},
		{method: http.MethodPost, path: "/logout", codes: all(http.StatusFound)},
		{method: http.MethodPost, path: "/posts", form: func(people) url.Values { return url.Values{"body": {"Hello"}} }, codes: members(http.StatusSeeOther, http.StatusUnauthorized)},
		{method: http.MethodPost, path: "/posts/earlier", form: func(people) url.Values {
			return url.Values{"after": {model.TimelineCursor{Created: model.Time{T: base}, Seq: 5}.String()}}
		}, datastar: true, codes: members(http.StatusOK, http.StatusUnauthorized)},
		{method: http.MethodPost, path: "/comments/open", form: func(p people) url.Values { return url.Values{"post": {p.post.ID.String()}} }, datastar: true, codes: members(http.StatusOK, http.StatusUnauthorized)},
		{method: http.MethodPost, path: "/comments", form: func(p people) url.Values { return url.Values{"post": {p.post.ID.String()}, "body": {"Hello"}} }, codes: members(http.StatusSeeOther, http.StatusUnauthorized)},
		{method: http.MethodGet, path: "/nope", codes: all(http.StatusNotFound)},
	} {
		for _, v := range []visitor{admin, moderator, member, inactive, nobody} {
			t.Run(fmt.Sprintf("answers %v %v for %v with %v and nothing they may not get", route.method, route.path, v, route.codes[v]), func(t *testing.T) {
				a := newApp(t)
				p := seed(t, a)

				var cookie *http.Cookie
				switch v {
				case admin:
					cookie = a.signIn(t, p.markus)
				case moderator:
					cookie = a.signIn(t, p.signe)
				case member:
					cookie = a.signIn(t, p.thomas)
				case inactive:
					// Karen was signed in before she was made inactive.
					cookie = a.signIn(t, p.karen)
				}
				sqlitetest.DeactivateUser(t, a.db, p.karen.ID)

				var res *httptest.ResponseRecorder
				if route.method == http.MethodGet {
					res = a.get(t, cookie, route.path)
				} else {
					form := url.Values{}
					if route.form != nil {
						form = route.form(p)
					}
					if route.path == "/invitations" {
						form.Set("token", newToken(t))
					}
					res = a.post(t, cookie, route.path, form, route.datastar)
				}

				is.Equal(t, route.codes[v], res.Code)
				answer := res.Body.String() + fmt.Sprint(res.Header())
				for _, secret := range forbidden[v] {
					is.True(t, !strings.Contains(answer, secret), "the answer has "+secret+" in it:\n"+answer)
				}
				for _, shown := range route.shows[v] {
					is.True(t, strings.Contains(answer, shown), "the answer does not have "+shown+" in it:\n"+answer)
				}
			})
		}
	}
}

func TestPeople_crossSite(t *testing.T) {
	t.Run("refuses every form of the pages for people that another site makes a browser send, and does nothing", func(t *testing.T) {
		a := newApp(t)
		p := seed(t, a)
		_, token := a.invitation(t, p.signe, "amira@example.com")
		link := a.link(t, "thomas@example.com")
		cookie := a.signIn(t, p.markus)

		for path, form := range map[string]url.Values{
			"/sign-in":              {"email": {"thomas@example.com"}},
			"/sign-in/link":         {"token": {link}},
			"/invitation":           {"token": {token}, "name": {"Amira"}},
			"/invitations":          {"email": {"olga@example.com"}, "token": {newToken(t)}},
			"/invitations/withdraw": {"invitation": {p.invitation.ID.String()}},
			"/members/role":         {"member": {p.thomas.ID.String()}, "role": {"admin"}},
			"/logout":               {},
		} {
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("Origin", "https://evil.example")
			req.Header.Set("Sec-Fetch-Site", "cross-site")
			req.AddCookie(cookie)
			res := httptest.NewRecorder()
			a.handler.ServeHTTP(res, req)

			is.Equal(t, http.StatusForbidden, res.Code, path)
		}

		is.Equal(t, 0, len(jobstest.Take(t, a.db)))
		is.Equal(t, 2, a.count(t, "invitations"))
		is.Equal(t, 4, a.count(t, "users"))
		is.Equal(t, "Markus", a.whoIsSignedIn(t, cookie))
		is.EqualSlice(t, []string{"admin", "moderator", "member", "moderator"}, attrs(all(page(t, a.get(t, cookie, "/members")), hasAttr("data-member")), "data-role"))
		is.Equal(t, http.StatusSeeOther, a.post(t, nil, "/sign-in/link", url.Values{"token": {link}}, false).Code)
		_, err := a.fat.CheckInvitation(t.Context(), token)
		is.NotError(t, err)
	})
}

func FuzzPeople_forms(f *testing.F) {
	f.Add("signe@example.com", "Signe", "garbage", "moderator", "i_1")
	f.Add("", "", "", "", "")
	f.Add("<script>@example.com", "<b>Signe</b>", "%zz", "admin\x00", "\xff")
	f.Add(" Signe@Example.com\r\n", "\u202eSigne", strings.Repeat("a", 43), "member", strings.Repeat("i", 1000))

	f.Fuzz(func(t *testing.T, email, name, token, role, other string) {
		a := newApp(t)
		markus := sqlitetest.CreatePerson(t, a.db, "Markus", "markus@example.com", model.RoleAdmin)
		cookie := a.signIn(t, markus)

		for _, req := range []struct {
			path   string
			form   url.Values
			cookie *http.Cookie
		}{
			{"/sign-in", url.Values{"email": {email}}, nil},
			{"/sign-in/link", url.Values{"token": {token}, "name": {name}}, nil},
			{"/invitation", url.Values{"token": {token}, "name": {name}}, nil},
			{"/invitations", url.Values{"email": {email}, "token": {token}}, cookie},
			{"/invitations", url.Values{"email": {email}, "token": {newToken(t)}}, cookie},
			{"/invitations/withdraw", url.Values{"invitation": {other}}, cookie},
			{"/members/role", url.Values{"member": {other}, "role": {role}}, cookie},
		} {
			res := a.post(t, req.cookie, req.path, req.form, false)
			is.True(t, res.Code < 500, fmt.Sprint(req.path, " answered ", res.Code))
			if res.Code != http.StatusOK {
				continue
			}

			// What was typed comes back only as the value of its field.
			doc := page(t, res)
			for _, field := range all(doc, func(n *nethtml.Node) bool { return n.Data == "input" && attr(n, "type") != "hidden" }) {
				value := attr(field, "value")
				is.True(t, value == "" || value == echoed(email) || value == echoed(name), value)
			}
		}
		for _, path := range []string{"/sign-in/link?token=" + url.QueryEscape(token), "/invitation?token=" + url.QueryEscape(token), "/members"} {
			res := a.get(t, cookie, path)
			is.True(t, res.Code < 500, fmt.Sprint(path, " answered ", res.Code))
		}
	})
}

// echoed text, as a page that puts it back in a field shows it: valid UTF-8, with a browser's reading of
// carriage returns and NULs in an attribute.
func echoed(s string) string {
	return strings.NewReplacer("\r\n", "\n", "\r", "\n", "\x00", "\ufffd").Replace(strings.ToValidUTF8(s, "\ufffd"))
}

func TestPeople_telemetry(t *testing.T) {
	t.Run("keeps the token of a link and an email address out of every span, whatever the request", func(t *testing.T) {
		sr := tracetest.NewSpanRecorder()
		previous := otel.GetTracerProvider()
		otel.SetTracerProvider(apphttp.HideQueryValues(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))))
		t.Cleanup(func() { otel.SetTracerProvider(previous) })

		a := newApp(t)
		sqlitetest.CreatePerson(t, a.db, "Signe", "signe@example.com", model.RoleMember)
		token := a.link(t, "signe@example.com")

		for _, req := range []*http.Request{
			httptest.NewRequest(http.MethodGet, "/sign-in/link?token="+token, nil),
			httptest.NewRequest(http.MethodHead, "/sign-in/link?token="+token, nil),
			httptest.NewRequest(http.MethodDelete, "/invitation?token="+token, nil),
			httptest.NewRequest(http.MethodGet, "/sign-in/link/?token="+token, nil),
			httptest.NewRequest(http.MethodGet, "/nowhere?token="+token, nil),
			httptest.NewRequest(http.MethodGet, "/sign-in/link?token="+token+";x", nil),
			httptest.NewRequest(http.MethodGet, "/sign-in/link?Token="+token+"&x=%zz", nil),
			httptest.NewRequest(http.MethodPost, "/sign-in?email=signe@example.com", nil),
		} {
			a.handler.ServeHTTP(httptest.NewRecorder(), req)
		}
		res := a.post(t, nil, "/sign-in/link", url.Values{"token": {token}}, false)
		is.Equal(t, http.StatusSeeOther, res.Code)

		var queries int
		for _, span := range sr.Ended() {
			for _, attr := range span.Attributes() {
				is.True(t, !strings.Contains(attr.Value.String(), token), "span "+span.Name()+" holds the token in "+string(attr.Key))
				is.True(t, !strings.Contains(attr.Value.String(), "signe@"), "span "+span.Name()+" holds the address in "+string(attr.Key))
				if attr.Key == "url.query" {
					queries++
				}
			}
		}
		is.Equal(t, 8, queries)
	})

	t.Run("keeps every address, token, link, and name out of the logs and the spans of the pages for people, whatever is done on them", func(t *testing.T) {
		sr := tracetest.NewSpanRecorder()
		previous := otel.GetTracerProvider()
		otel.SetTracerProvider(apphttp.HideQueryValues(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))))
		t.Cleanup(func() { otel.SetTracerProvider(previous) })

		a := newApp(t)
		markus := sqlitetest.CreatePerson(t, a.db, "Markus Quill", "markus.quill@example.com", model.RoleAdmin)
		signe := sqlitetest.CreatePerson(t, a.db, "Signe Varga", "signe.varga@example.com", model.RoleModerator)
		admin, moderator := a.signIn(t, markus), a.signIn(t, signe)
		a.logs.Reset()

		var secrets []string
		ask := page(t, a.get(t, nil, "/sign-in"))
		a.send(t, nil, ask, "sign-in-request", url.Values{"email": {"nobody.zed@example.com"}})
		a.send(t, nil, ask, "sign-in-request", url.Values{"email": {"not an address zed"}})
		link := a.link(t, "signe.varga@example.com")
		secrets = append(secrets, link)
		a.send(t, nil, page(t, a.get(t, nil, "/sign-in/link?token="+link)), "sign-in", nil)

		list := page(t, a.get(t, moderator, "/members"))
		a.send(t, moderator, list, "invite", url.Values{"email": {"amira.zephyr@example.com"}})
		a.send(t, moderator, list, "invite", url.Values{"email": {"olga.zed@example.com"}})
		a.send(t, moderator, page(t, a.get(t, moderator, "/members")), "invite", url.Values{"email": {"markus.quill@example.com"}})
		tokens := a.runJobs(t)
		is.Equal(t, 1, len(tokens))
		secrets = append(secrets, tokens[0])
		invitation := page(t, a.get(t, nil, "/invitation?token="+tokens[0]))
		a.send(t, nil, invitation, "join", url.Values{"name": {strings.Repeat("Zephyr", 10)}})
		a.send(t, nil, invitation, "join", url.Values{"name": {"Amira Zephyr"}})

		list = page(t, a.get(t, admin, "/members"))
		a.send(t, admin, one(t, list, func(n *nethtml.Node) bool { return attr(n, "data-member") == signe.ID.String() }), "change-role", url.Values{"role": {"member"}})
		a.post(t, admin, "/members/role", url.Values{"member": {markus.ID.String()}, "role": {"member"}}, false)
		inv, _ := a.invitation(t, markus, "petra.zed@example.com")
		a.post(t, admin, "/invitations/withdraw", url.Values{"invitation": {inv.ID.String()}}, false)
		a.post(t, admin, "/logout", nil, false)

		logs := a.logs.String()
		is.True(t, strings.Contains(logs, "Accepted an invitation"), logs)
		for _, secret := range append(secrets, "@", "example.com", "Amira", "Zephyr", "Quill", "Varga", "zed", "/sign-in/link?", "/invitation?") {
			is.True(t, !strings.Contains(logs, secret), "the logs have "+secret+" in them:\n"+logs)
		}
		var spans int
		for _, span := range sr.Ended() {
			spans++
			for _, attr := range span.Attributes() {
				for _, secret := range append(secrets, "@", "Amira", "Zephyr", "Quill", "Varga", "zed") {
					is.True(t, !strings.Contains(attr.Value.String(), secret), "span "+span.Name()+" holds "+secret+" in "+string(attr.Key))
				}
			}
		}
		is.True(t, spans > 20, fmt.Sprint(spans, " spans"))
	})

	t.Run("takes the fields of a form from its body only, not from the query, where they would be in the request's span", func(t *testing.T) {
		a := newApp(t)
		sqlitetest.CreatePerson(t, a.db, "Signe", "signe@example.com", model.RoleMember)

		res := a.post(t, nil, "/sign-in?email=signe@example.com", nil, false)
		is.Equal(t, http.StatusOK, res.Code)
		is.EqualSlice(t, []string{"email-invalid"}, messages(page(t, res)))
		is.Equal(t, 0, len(jobstest.Take(t, a.db)))
	})
}
