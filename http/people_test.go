package http_test

import (
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

func TestPeople_signInLink(t *testing.T) {
	t.Run("answers asking for a link with an address in the group and one that is not byte for byte the same, and sends a link to the one in the group only", func(t *testing.T) {
		a := newApp(t)
		sqlitetest.CreatePerson(t, a.db, "Signe", "signe@example.com", model.RoleMember)

		known := a.post(t, nil, "/sign-in", url.Values{"email": {"signe@example.com"}}, false)
		knownJobs := jobstest.Take(t, a.db)
		unknown := a.post(t, nil, "/sign-in", url.Values{"email": {"nobody@example.com"}}, false)
		unknownJobs := jobstest.Take(t, a.db)

		is.Equal(t, http.StatusOK, known.Code)
		is.Equal(t, known.Code, unknown.Code)
		is.Equal(t, known.Body.String(), unknown.Body.String())
		is.Equal(t, "If that address belongs to someone in the group, a link to sign in is on its way to it.", known.Body.String())
		// The headers are the same but for the nonce of the content security policy, which every answer has
		// a new one of.
		is.Equal(t, len(known.Header()), len(unknown.Header()))
		nonce := regexp.MustCompile("'nonce-[^']*'")
		for k := range known.Header() {
			is.Equal(t, nonce.ReplaceAllString(known.Header().Get(k), ""), nonce.ReplaceAllString(unknown.Header().Get(k), ""), k)
		}
		is.Equal(t, 1, len(knownJobs))
		is.Equal(t, 1, len(unknownJobs))
		is.Equal(t, knownJobs[0].Name, unknownJobs[0].Name)

		for _, j := range append(knownJobs, unknownJobs...) {
			is.NotError(t, jobs.Funcs(jobs.RegisterOpts{Emails: a.fat})[j.Name](t.Context(), j.Data))
		}
		sent := a.sender.Take()
		is.Equal(t, 1, len(sent))
		is.Equal(t, model.EmailAddress("signe@example.com"), sent[0].To)
	})

	t.Run("says when an address is not one", func(t *testing.T) {
		a := newApp(t)

		res := a.post(t, nil, "/sign-in", url.Values{"email": {"signe"}}, false)
		is.Equal(t, http.StatusBadRequest, res.Code)
		is.Equal(t, "That is not an email address.", res.Body.String())
		is.Equal(t, 0, len(jobstest.Take(t, a.db)))
	})

	t.Run("opens a page that says the link works without using it up, and signs in with a new session on confirming it, once", func(t *testing.T) {
		a := newApp(t)
		sqlitetest.CreatePerson(t, a.db, "Signe", "signe@example.com", model.RoleMember)
		token := a.link(t, "signe@example.com")

		for range 2 {
			res := a.get(t, nil, "/sign-in/link?token="+token)
			is.Equal(t, http.StatusOK, res.Code)
			is.Equal(t, "This link works.", res.Body.String())
			is.Equal(t, "no-referrer", res.Header().Get("Referrer-Policy"))
			is.Equal(t, "no-store", res.Header().Get("Cache-Control"))
		}

		// A session from before, which signing in must not keep.
		before := a.signIn(t, model.User{ID: "u_nobody"})
		res := a.post(t, before, "/sign-in/link", url.Values{"token": {token}}, false)
		is.Equal(t, http.StatusSeeOther, res.Code)
		is.Equal(t, "/", res.Header().Get("Location"))
		cookie := a.sessionCookie(res)
		is.True(t, cookie != nil, "expected a session cookie")
		is.True(t, cookie.Value != before.Value, "expected a new session")
		is.Equal(t, "Signe", a.whoIsSignedIn(t, cookie))

		res = a.post(t, nil, "/sign-in/link", url.Values{"token": {token}}, false)
		is.Equal(t, http.StatusBadRequest, res.Code)
		is.True(t, a.sessionCookie(res) == nil)
	})

	t.Run("refuses a used, expired, or garbage link, and one of someone made inactive, all with one answer", func(t *testing.T) {
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

		var answers []string
		for _, token := range []string{used, inactive, expired, "garbage", ""} {
			res := a.get(t, nil, "/sign-in/link?token="+url.QueryEscape(token))
			is.Equal(t, http.StatusBadRequest, res.Code)
			answers = append(answers, res.Body.String())
			res = a.post(t, nil, "/sign-in/link", url.Values{"token": {token}}, false)
			is.Equal(t, http.StatusBadRequest, res.Code)
			answers = append(answers, res.Body.String())
		}
		for _, answer := range answers {
			is.Equal(t, "This link does not work. Ask for a new one.", answer)
		}
	})

	t.Run("has the first admin choose a name, and refuses one that cannot be used without using up the link", func(t *testing.T) {
		a := newApp(t, withFirstAdmin("markus@example.com"))
		token := a.link(t, "Markus@Example.com")

		res := a.get(t, nil, "/sign-in/link?token="+token)
		is.Equal(t, http.StatusOK, res.Code)
		is.Equal(t, "This link works. Choose your name to join the group.", res.Body.String())

		res = a.post(t, nil, "/sign-in/link", url.Values{"token": {token}, "name": {" "}}, false)
		is.Equal(t, http.StatusBadRequest, res.Code)
		is.Equal(t, "That name cannot be used.", res.Body.String())

		res = a.post(t, nil, "/sign-in/link", url.Values{"token": {token}, "name": {"Markus"}}, false)
		is.Equal(t, http.StatusSeeOther, res.Code)
		is.Equal(t, "Markus", a.whoIsSignedIn(t, a.sessionCookie(res)))
	})
}

func TestPeople_session(t *testing.T) {
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
}

func TestPeople_invitation(t *testing.T) {
	t.Run("opens a page that says the invitation works without using it up, and has the person join and sign in on accepting it, once", func(t *testing.T) {
		a := newApp(t)
		signe := sqlitetest.CreatePerson(t, a.db, "Signe", "signe@example.com", model.RoleModerator)
		_, err := a.fat.Invite(t.Context(), signe.ID, "amira@example.com")
		is.NotError(t, err)
		tokens := a.runJobs(t)
		is.Equal(t, 1, len(tokens))

		for range 2 {
			res := a.get(t, nil, "/invitation?token="+tokens[0])
			is.Equal(t, http.StatusOK, res.Code)
			is.Equal(t, "This invitation works. Choose your name to join the group.", res.Body.String())
			is.Equal(t, "no-referrer", res.Header().Get("Referrer-Policy"))
		}

		res := a.post(t, nil, "/invitation", url.Values{"token": {tokens[0]}, "name": {""}}, false)
		is.Equal(t, http.StatusBadRequest, res.Code)
		is.Equal(t, "That name cannot be used.", res.Body.String())

		res = a.post(t, nil, "/invitation", url.Values{"token": {tokens[0]}, "name": {"Amira"}}, false)
		is.Equal(t, http.StatusSeeOther, res.Code)
		is.Equal(t, "Amira", a.whoIsSignedIn(t, a.sessionCookie(res)))

		for _, res := range []*httptest.ResponseRecorder{
			a.get(t, nil, "/invitation?token="+tokens[0]),
			a.post(t, nil, "/invitation", url.Values{"token": {tokens[0]}, "name": {"Amira"}}, false),
			a.post(t, nil, "/invitation", url.Values{"token": {"garbage"}, "name": {"Amira"}}, false),
		} {
			is.Equal(t, http.StatusBadRequest, res.Code)
			is.Equal(t, "This invitation does not work. Ask the person who invited you for a new one.", res.Body.String())
		}
	})
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

	t.Run("takes the fields of a form from its body only, not from the query, where they would be in the request's span", func(t *testing.T) {
		a := newApp(t)
		sqlitetest.CreatePerson(t, a.db, "Signe", "signe@example.com", model.RoleMember)

		res := a.post(t, nil, "/sign-in?email=signe@example.com", nil, false)
		is.Equal(t, http.StatusBadRequest, res.Code)
		is.Equal(t, 0, len(jobstest.Take(t, a.db)))
	})
}
