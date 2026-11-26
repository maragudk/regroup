package preview

import (
	"errors"
	"log/slog"
	"net/http"

	gluehttp "maragu.dev/glue/http"
	. "maragu.dev/gomponents"
	. "maragu.dev/gomponents/html"

	"app/html"
	"app/model"
	"app/sqlite"
)

// PeopleLink to the preview's list of people, for someone who is signed in and wants to be someone else,
// and for someone who is not signed in.
func PeopleLink(signedIn bool) Node {
	if signedIn {
		return A(Href("/preview"), Text("Switch person"))
	}
	return A(Href("/preview"), Text("This is a preview. Choose someone to sign in as."))
}

// Routes of the preview: a list of everyone active in the group at /preview, signing in as any of them by
// putting their ID in the session under [gluehttp.SessionUserIDKey], and the emails in the given outbox at
// /preview/emails.
func Routes(r *gluehttp.Router, log *slog.Logger, db *sqlite.Database, outbox *Outbox) {
	r.Get("/preview/emails", func(props html.PageProps) (Node, error) {
		return emailsPage(props, outbox)
	})

	r.Get("/preview", func(props html.PageProps) (Node, error) {
		var users []model.User
		if err := db.H.Select(props.Ctx, &users, `select * from users where active = 1 order by name`); err != nil {
			return html.ErrorPage(props), err
		}

		props.Title = "Preview: sign in as"
		return html.Page(props,
			H1(Text("Sign in as")),
			P(A(Href("/preview/emails"), Text("See the emails the preview kept instead of sending them"))),
			Ul(Class("grid gap-2"),
				Map(users, func(u model.User) Node {
					return Li(
						Form(Method("post"), Action("/preview/sign-in"),
							Input(Type("hidden"), Name("user"), Value(u.ID.String())),
							Button(Type("submit"), Class("cursor-pointer underline decoration-stone-400 decoration-1 underline-offset-4 hover:decoration-stone-900"), Text(u.Name)),
						),
					)
				}),
			),
		), nil
	})

	r.Post("/preview/sign-in", func(props html.PageProps) (Node, error) {
		id := model.UserID(props.R.FormValue("user"))
		if _, err := db.GetUser(props.Ctx, id); err != nil {
			if errors.Is(err, model.ErrorUserNotFound) {
				http.Error(props.W, "no such user", http.StatusBadRequest)
				return nil, nil
			}
			return html.ErrorPage(props), err
		}

		if err := r.SM.RenewToken(props.Ctx); err != nil {
			return html.ErrorPage(props), err
		}
		r.SM.Put(props.Ctx, gluehttp.SessionUserIDKey, id.String())

		log.InfoContext(props.Ctx, "Signed in for the preview", "userID", id)

		http.Redirect(props.W, props.R, "/", http.StatusFound)
		return nil, nil
	})
}
