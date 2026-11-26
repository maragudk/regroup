package preview_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	gluehttp "maragu.dev/glue/http"
	"maragu.dev/is"

	apphttp "app/http"
	"app/model"
	"app/preview"
	"app/service"
	"app/sqlitetest"
)

func TestPeopleLink(t *testing.T) {
	t.Run("links to the list of people, in words for someone who is signed in and for someone who is not", func(t *testing.T) {
		var b strings.Builder
		is.NotError(t, preview.PeopleLink(true).Render(&b))
		is.Equal(t, `<a href="/preview">Switch person</a>`, b.String())

		b.Reset()
		is.NotError(t, preview.PeopleLink(false).Render(&b))
		is.Equal(t, `<a href="/preview">This is a preview. Choose someone to sign in as.</a>`, b.String())
	})
}

func TestRoutes(t *testing.T) {
	t.Run("lands the browser on the group page as the person chosen, with a link to switch person, from a group page that said only that it is private", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		loc, err := time.LoadLocation("Europe/Copenhagen")
		is.NotError(t, err)
		group := model.Group{Name: "The Loft", TimeZone: loc}
		now := func() time.Time { return time.Date(2031, 3, 14, 9, 0, 0, 0, time.UTC) }
		log := slog.New(slog.DiscardHandler)

		fat := service.NewFat(service.NewFatOptions{})
		service.Setup(fat, nil, db, nil, now, group)
		_, err = fat.CreatePost(t.Context(), signe.ID, "Cake in the kitchen.")
		is.NotError(t, err)

		sm := scs.New()
		r := gluehttp.NewRouter(gluehttp.NewRouterOpts{SM: sm})
		r.Use(sm.LoadAndSave, gluehttp.Authenticate(log, sm, db))
		apphttp.InjectHTTPRouter(log, fat, apphttp.InjectHTTPRouterOptions{Group: group, Now: now, Extra: preview.PeopleLink})(r)
		preview.Routes(r, log, db)
		server := httptest.NewServer(r.Mux)
		t.Cleanup(server.Close)

		jar, err := cookiejar.New(nil)
		is.NotError(t, err)
		client := &http.Client{Jar: jar}

		res, err := client.Get(server.URL + "/")
		is.NotError(t, err)
		body := readBody(t, res)
		is.True(t, strings.Contains(body, "This group is private."), body)
		is.True(t, strings.Contains(body, `<a href="/preview">This is a preview. Choose someone to sign in as.</a>`), body)
		is.True(t, !strings.Contains(body, "The Loft") && !strings.Contains(body, "Cake") && !strings.Contains(body, "Signe"), body)

		// The client follows the redirect, as a browser does.
		res, err = client.PostForm(server.URL+"/preview/sign-in", url.Values{"user": {signe.ID.String()}})
		is.NotError(t, err)
		body = readBody(t, res)
		is.Equal(t, http.StatusOK, res.StatusCode)
		is.Equal(t, "/", res.Request.URL.Path)
		is.True(t, strings.Contains(body, "<title>The Loft</title>"), body)
		is.True(t, strings.Contains(body, "Cake in the kitchen."), body)
		is.True(t, strings.Contains(body, `<a href="/preview">Switch person</a>`), body)
	})

	t.Run("lists everyone, and signs the browser in as the one chosen", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		sqlitetest.CreateUser(t, db, "Thomas")
		amira := sqlitetest.CreateUser(t, db, "Amira")
		sqlitetest.DeactivateUser(t, db, amira.ID)

		sm := scs.New()
		r := gluehttp.NewRouter(gluehttp.NewRouterOpts{SM: sm})
		r.Use(sm.LoadAndSave, gluehttp.Authenticate(slog.New(slog.DiscardHandler), sm, db))
		preview.Routes(r, slog.New(slog.DiscardHandler), db)
		r.Mux.Get("/whoami", func(w http.ResponseWriter, r *http.Request) {
			if id := gluehttp.GetUserIDFromContext(r.Context()); id != nil {
				_, _ = io.WriteString(w, id.String())
			}
		})
		server := httptest.NewServer(r.Mux)
		t.Cleanup(server.Close)

		res, err := http.Get(server.URL + "/preview")
		is.NotError(t, err)
		body := readBody(t, res)
		is.Equal(t, http.StatusOK, res.StatusCode)
		is.True(t, strings.Contains(body, ">Signe</button>"), body)
		is.True(t, strings.Contains(body, ">Thomas</button>"), body)
		is.True(t, strings.Contains(body, signe.ID.String()), body)
		is.True(t, !strings.Contains(body, "Amira"), body)

		client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		res, err = client.PostForm(server.URL+"/preview/sign-in", url.Values{"user": {signe.ID.String()}})
		is.NotError(t, err)
		readBody(t, res)
		is.Equal(t, http.StatusFound, res.StatusCode)
		is.Equal(t, "/", res.Header.Get("Location"))
		cookies := res.Cookies()
		is.True(t, len(cookies) > 0, "expected a session cookie")

		req, err := http.NewRequest(http.MethodGet, server.URL+"/whoami", nil)
		is.NotError(t, err)
		for _, c := range cookies {
			req.AddCookie(c)
		}
		res, err = http.DefaultClient.Do(req)
		is.NotError(t, err)
		is.Equal(t, signe.ID.String(), readBody(t, res))
	})

	t.Run("refuses to sign in as someone who does not exist", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		sm := scs.New()
		r := gluehttp.NewRouter(gluehttp.NewRouterOpts{SM: sm})
		r.Use(sm.LoadAndSave)
		preview.Routes(r, slog.New(slog.DiscardHandler), db)

		req := httptest.NewRequest(http.MethodPost, "/preview/sign-in", strings.NewReader(url.Values{"user": {string(model.UserID("u_nope"))}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		res := httptest.NewRecorder()
		r.Mux.ServeHTTP(res, req)

		is.Equal(t, http.StatusBadRequest, res.Code)
		is.Equal(t, 0, len(res.Result().Cookies()))
	})
}

func readBody(t *testing.T, res *http.Response) string {
	t.Helper()

	defer func() { _ = res.Body.Close() }()
	b, err := io.ReadAll(res.Body)
	is.NotError(t, err)
	return string(b)
}
