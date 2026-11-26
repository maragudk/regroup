//go:build preview

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
	defaultGroupName = preview.GroupName

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
