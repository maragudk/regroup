package http

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/alexedwards/scs/v2"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	gluehttp "maragu.dev/glue/http"
	. "maragu.dev/gomponents"

	"app/html"
	"app/model"
)

// peopleService is the business logic of signing in and of invitations.
type peopleService interface {
	RequestSignInLink(ctx context.Context, address string) error
	CheckSignInLink(ctx context.Context, token string) (model.SignInLinkCheck, error)
	SignIn(ctx context.Context, token, name string) (model.User, error)
	CheckInvitation(ctx context.Context, token string) (model.InvitationCheck, error)
	AcceptInvitation(ctx context.Context, token, name string) (model.User, error)
}

// people adds the routes for asking for a sign-in link, for the page a sign-in link opens, and for the
// page an invitation's link opens, with what each of them does. They answer in plain text, as the bare
// routes they are, until the pages are built:
//   - POST /sign-in with an email address asks for a sign-in link. Every address that is one gets the
//     same answer.
//   - GET /sign-in/link?token=… says whether the link works, without using it up, and POST /sign-in/link
//     with the token, and a name for the first admin, signs in and goes to the group page.
//   - GET /invitation?token=… says whether the invitation works, without using it up, and
//     POST /invitation with the token and a name accepts it, signs in, and goes to the group page.
func people(r *Router, log *slog.Logger, svc peopleService) {
	r.Post("/sign-in", func(props html.PageProps) (Node, error) {
		err := svc.RequestSignInLink(props.Ctx, props.R.FormValue("email"))
		switch {
		case errors.Is(err, model.ErrorEmailInvalid):
			return plain(props.W, http.StatusBadRequest, "That is not an email address.")
		case err != nil:
			log.ErrorContext(props.Ctx, "Error asking for a sign-in link", "error", err)
			return html.ErrorPage(props), err
		}
		return plain(props.W, http.StatusOK, "If that address belongs to someone in the group, a link to sign in is on its way to it.")
	})

	r.Get("/sign-in/link", func(props html.PageProps) (Node, error) {
		check, err := svc.CheckSignInLink(props.Ctx, props.R.URL.Query().Get("token"))
		switch {
		case errors.Is(err, model.ErrorLinkInvalid):
			return plain(props.W, http.StatusBadRequest, linkRefusal)
		case err != nil:
			log.ErrorContext(props.Ctx, "Error checking a sign-in link", "error", err)
			return html.ErrorPage(props), err
		case check.Joining:
			return plain(props.W, http.StatusOK, "This link works. Choose your name to join the group.")
		}
		return plain(props.W, http.StatusOK, "This link works.")
	})

	r.Post("/sign-in/link", func(props html.PageProps) (Node, error) {
		u, err := svc.SignIn(props.Ctx, props.R.FormValue("token"), props.R.FormValue("name"))
		return signedIn(props, log, r.SM, u, err)
	})

	r.Get("/invitation", func(props html.PageProps) (Node, error) {
		_, err := svc.CheckInvitation(props.Ctx, props.R.URL.Query().Get("token"))
		switch {
		case errors.Is(err, model.ErrorLinkInvalid):
			return plain(props.W, http.StatusBadRequest, invitationRefusal)
		case err != nil:
			log.ErrorContext(props.Ctx, "Error checking an invitation", "error", err)
			return html.ErrorPage(props), err
		}
		return plain(props.W, http.StatusOK, "This invitation works. Choose your name to join the group.")
	})

	r.Post("/invitation", func(props html.PageProps) (Node, error) {
		u, err := svc.AcceptInvitation(props.Ctx, props.R.FormValue("token"), props.R.FormValue("name"))
		if errors.Is(err, model.ErrorLinkInvalid) {
			return plain(props.W, http.StatusBadRequest, invitationRefusal)
		}
		return signedIn(props, log, r.SM, u, err)
	})
}

const (
	// linkRefusal is the one answer to a sign-in link that does not work, whatever the reason.
	linkRefusal = "This link does not work. Ask for a new one."

	// invitationRefusal is the one answer to an invitation that does not work, whatever the reason.
	invitationRefusal = "This invitation does not work. Ask the person who invited you for a new one."
)

// signedIn answers signing in, or accepting an invitation, with the given outcome: on success, the
// session is the user's, and the browser goes to the group page.
func signedIn(props html.PageProps, log *slog.Logger, sm *scs.SessionManager, u model.User, err error) (Node, error) {
	switch {
	case errors.Is(err, model.ErrorLinkInvalid):
		return plain(props.W, http.StatusBadRequest, linkRefusal)
	case errors.Is(err, model.ErrorNameEmpty), errors.Is(err, model.ErrorNameInvalid), errors.Is(err, model.ErrorNameTooLong):
		return plain(props.W, http.StatusBadRequest, "That name cannot be used.")
	case err != nil:
		log.ErrorContext(props.Ctx, "Error signing in", "error", err)
		return html.ErrorPage(props), err
	}

	if err := SignInSession(props.Ctx, sm, u.ID); err != nil {
		log.ErrorContext(props.Ctx, "Error putting the user in the session", "error", err, "userID", u.ID)
		return html.ErrorPage(props), err
	}

	http.Redirect(props.W, props.R, "/", http.StatusSeeOther)
	return nil, nil
}

// SignInSession of the request in the given context as the given user: the session gets a new token, so
// that a session from before cannot be taken over, and holds the user's ID where glue's authentication
// middleware reads it. Every request after that checks that the user is still active, and ends the
// session if they are not.
func SignInSession(ctx context.Context, sm *scs.SessionManager, id model.UserID) error {
	if err := sm.RenewToken(ctx); err != nil {
		return err
	}
	sm.Put(ctx, gluehttp.SessionUserIDKey, id.String())
	return nil
}

// plain text as the answer, with the given status.
func plain(w http.ResponseWriter, status int, text string) (Node, error) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, text)
	return nil, nil
}

// hideTokens keeps the token of a link in an email out of the telemetry and out of the Referer header,
// for a request whose query has one, such as the page a sign-in link opens. The server's span records
// the query as it came, so it is recorded again with each token's value hidden, which replaces it, and
// the answer tells the browser to send no Referer from the page.
func hideTokens(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if q := r.URL.Query(); q.Has("token") {
			w.Header().Set("Referrer-Policy", "no-referrer")
			for i := range q["token"] {
				q["token"][i] = "hidden"
			}
			trace.SpanFromContext(r.Context()).SetAttributes(attribute.String("url.query", q.Encode()))
		}
		next.ServeHTTP(w, r)
	})
}
