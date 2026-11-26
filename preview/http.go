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

// Routes of the preview: a list of everyone active in the group at /preview, and signing in as any of
// them by putting their ID in the session under [gluehttp.SessionUserIDKey].
func Routes(r *gluehttp.Router, log *slog.Logger, db *sqlite.Database) {
	r.Get("/preview", func(props html.PageProps) (Node, error) {
		var users []model.User
		if err := db.H.Select(props.Ctx, &users, `select * from users where active = 1 order by name`); err != nil {
			return html.ErrorPage(), err
		}

		return html.Page(html.PageProps{Title: "Preview: sign in as"},
			H1(Text("Sign in as")),
			Ul(
				Map(users, func(u model.User) Node {
					return Li(
						Form(Method("post"), Action("/preview/sign-in"),
							Input(Type("hidden"), Name("user"), Value(u.ID.String())),
							Button(Type("submit"), Class("cursor-pointer"), Text(u.Name)),
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
			return html.ErrorPage(), err
		}

		if err := r.SM.RenewToken(props.Ctx); err != nil {
			return html.ErrorPage(), err
		}
		r.SM.Put(props.Ctx, gluehttp.SessionUserIDKey, id.String())

		log.InfoContext(props.Ctx, "Signed in for the preview", "userID", id)

		http.Redirect(props.W, props.R, "/", http.StatusFound)
		return nil, nil
	})
}
