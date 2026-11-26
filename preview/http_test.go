package preview_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/alexedwards/scs/v2"
	gluehttp "maragu.dev/glue/http"
	"maragu.dev/is"

	"app/model"
	"app/preview"
	"app/sqlitetest"
)

func TestRoutes(t *testing.T) {
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
