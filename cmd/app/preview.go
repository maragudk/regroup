//go:build preview

// The preview build: it seeds an empty database with a made-up office group, lets anyone sign in as
// anyone at /preview, and keeps the emails it would have sent, to show them at /preview/emails, instead
// of sending them. Only the preview build tag links the preview package into the app.

package main

import (
	"context"
	"log/slog"
	"time"

	"maragu.dev/glue/email"
	gluehttp "maragu.dev/glue/http"

	"app/model"
	"app/preview"
	"app/sqlite"
)

func init() {
	defaultGroupName = "The Loft"
	previewLink = preview.PeopleLink

	setupPreview = func(ctx context.Context, log *slog.Logger, db *sqlite.Database, now func() time.Time, group model.Group) (func(*gluehttp.Router), email.Sender, error) {
		log.WarnContext(ctx, "This is the preview build: anyone can sign in as anyone at /preview, and it sends no email")

		seeded, err := preview.Seed(ctx, db, now(), group.TimeZone)
		if err != nil {
			return nil, nil, err
		}
		if seeded {
			log.InfoContext(ctx, "Seeded the empty database with the preview's made-up group")
		}

		outbox := preview.NewOutbox(log.With("component", "preview.Outbox"), now)
		return func(r *gluehttp.Router) {
			preview.Routes(r, log, db, outbox)
		}, outbox, nil
	}
}
