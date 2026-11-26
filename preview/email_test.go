package preview_test

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	"maragu.dev/glue/email"
	gluehttp "maragu.dev/glue/http"
	"maragu.dev/is"

	apphttp "app/http"
	"app/jobs"
	"app/jobstest"
	"app/model"
	"app/preview"
	"app/service"
	"app/sqlitetest"
)

func TestOutbox(t *testing.T) {
	t.Run("shows the emails the app would have sent, the newest first, with links into the app that work", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		loc, err := time.LoadLocation("Europe/Copenhagen")
		is.NotError(t, err)
		now := func() time.Time { return time.Date(2031, 3, 14, 9, 0, 0, 0, time.UTC) }
		group := model.Group{Name: "The Loft", TimeZone: loc}
		_, err = preview.Seed(t.Context(), db, now(), loc)
		is.NotError(t, err)
		var logs bytes.Buffer
		log := slog.New(slog.NewTextHandler(&logs, nil))
		outbox := preview.NewOutbox(log, now)

		fat := service.NewFat(service.NewFatOptions{Log: log})
		service.Setup(fat, nil, db, outbox, now, rand.Reader, group, "")

		sm := scs.New()
		r := gluehttp.NewRouter(gluehttp.NewRouterOpts{SM: sm})
		r.Use(sm.LoadAndSave, gluehttp.Authenticate(log, sm, db))
		apphttp.InjectHTTPRouter(log, fat, apphttp.InjectHTTPRouterOptions{Group: group, Now: now, Random: rand.Reader, Extra: preview.PeopleLink})(r)
		preview.Routes(r, log, db, outbox)

		res := httptest.NewRecorder()
		r.Mux.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/preview/emails", nil))
		is.Equal(t, http.StatusOK, res.Code)
		is.True(t, strings.Contains(res.Body.String(), "Nothing yet."), res.Body.String())

		var markus model.User
		is.NotError(t, db.H.Get(t.Context(), &markus, "select * from users where name = 'Markus'"))
		is.NotError(t, fat.RequestSignInLink(t.Context(), "signe@example.com"))
		_, err = fat.Invite(t.Context(), markus.ID, "nadia@example.com")
		is.NotError(t, err)
		jobstest.Run(t, db, jobs.Funcs(jobs.RegisterOpts{Emails: fat}))

		res = httptest.NewRecorder()
		r.Mux.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/preview/emails", nil))
		body := res.Body.String()
		is.Equal(t, http.StatusOK, res.Code)
		invitation := strings.Index(body, "Markus invited you to The Loft")
		signIn := strings.Index(body, "Sign in to The Loft")
		is.True(t, invitation > 0 && signIn > invitation, "expected the invitation first, then the sign-in email:\n"+body)
		is.True(t, strings.Contains(body, "To nadia@example.com"), body)
		is.True(t, strings.Contains(body, "To signe@example.com"), body)

		links := regexp.MustCompile(`href="(/(?:sign-in/link|invitation)\?token=[A-Za-z0-9_-]{43})"`).FindAllStringSubmatch(body, -1)
		is.Equal(t, 2, len(links), body)
		for _, link := range links {
			res = httptest.NewRecorder()
			r.Mux.ServeHTTP(res, httptest.NewRequest(http.MethodGet, link[1], nil))
			is.Equal(t, http.StatusOK, res.Code, link[1])
		}

		token := strings.TrimPrefix(links[0][1], "/invitation?token=")
		req := httptest.NewRequest(http.MethodPost, "/invitation", strings.NewReader(url.Values{"token": {token}, "name": {"Nadia"}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		res = httptest.NewRecorder()
		r.Mux.ServeHTTP(res, req)
		is.Equal(t, http.StatusSeeOther, res.Code)

		is.True(t, !strings.Contains(logs.String(), "@example.com"), logs.String())
		is.True(t, strings.Contains(logs.String(), "Kept an email for the preview instead of sending it"), logs.String())
	})

	t.Run("keeps the newest hundred", func(t *testing.T) {
		outbox := preview.NewOutbox(slog.New(slog.DiscardHandler), time.Now)
		for i := range 101 {
			is.NotError(t, outbox.SendTransactional(t.Context(), email.SendOptions{Subject: fmt.Sprint("Email ", i), Template: "sign-in", To: "a@example.com"}))
		}

		sm := scs.New()
		r := gluehttp.NewRouter(gluehttp.NewRouterOpts{SM: sm})
		r.Use(sm.LoadAndSave)
		preview.Routes(r, slog.New(slog.DiscardHandler), sqlitetest.NewDatabase(t), outbox)
		res := httptest.NewRecorder()
		r.Mux.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/preview/emails", nil))

		is.True(t, strings.Contains(res.Body.String(), ">Email 100<"), res.Body.String())
		is.True(t, strings.Contains(res.Body.String(), ">Email 1<"), res.Body.String())
		is.True(t, !strings.Contains(res.Body.String(), ">Email 0<"), res.Body.String())
	})
}
