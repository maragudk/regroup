//go:build preview

// The preview build: it seeds an empty database with a made-up office group, and lets anyone sign in as
// anyone at /preview. Only the preview build tag links the preview package into the app.

package main

import (
	"context"
	"log/slog"
	"time"

	gluehttp "maragu.dev/glue/http"

	"app/model"
	"app/preview"
	"app/sqlite"
)

func init() {
	defaultGroupName = "The Loft"

	setupPreview = func(ctx context.Context, log *slog.Logger, db *sqlite.Database, now func() time.Time, group model.Group) (func(*gluehttp.Router), error) {
		log.WarnContext(ctx, "This is the preview build: anyone can sign in as anyone at /preview")

		seeded, err := preview.Seed(ctx, db, now(), group.TimeZone)
		if err != nil {
			return nil, err
		}
		if seeded {
			log.InfoContext(ctx, "Seeded the empty database with the preview's made-up group")
		}

		return func(r *gluehttp.Router) {
			preview.Routes(r, log, db)
		}, nil
	}
}
