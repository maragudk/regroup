package preview_test

import (
	"slices"
	"testing"
	"time"

	"maragu.dev/is"

	"app/model"
	"app/preview"
	"app/sqlite"
	"app/sqlitetest"
)

func TestSeed(t *testing.T) {
	copenhagen, err := time.LoadLocation("Europe/Copenhagen")
	is.NotError(t, err)
	now := time.Date(2026, 11, 26, 14, 0, 0, 0, copenhagen)

	t.Run("fills an empty database with fourteen people, including the named ones", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)

		seeded, err := preview.Seed(t.Context(), db, now, copenhagen)
		is.NotError(t, err)
		is.True(t, seeded)

		names := userNames(t, db)
		is.Equal(t, 14, len(names))
		for _, name := range []string{"Markus", "Signe", "Thomas", "Amira", "Jonas", "Karen"} {
			is.True(t, slices.Contains(names, name), name)
		}
	})

	t.Run("posts today, yesterday, and earlier in the week, with comments, and nothing in the future", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		_, err := preview.Seed(t.Context(), db, now, copenhagen)
		is.NotError(t, err)

		days := model.GroupByDay(timeline(t, db, "Markus"), copenhagen, now)
		is.True(t, days[0].Today)
		is.True(t, days[1].Yesterday)
		is.True(t, len(days) >= 4, "expected days earlier in the week")
		for _, day := range days {
			for _, p := range day.Posts {
				is.True(t, !p.Created.T.After(now), "post in the future")
			}
		}

		var comments int
		for _, p := range timeline(t, db, "Markus") {
			comments += p.Comments
		}
		is.True(t, comments >= 10, "expected comments")
	})

	t.Run("marks what came after everyone's look the evening before as new", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		_, err := preview.Seed(t.Context(), db, now, copenhagen)
		is.NotError(t, err)

		days := model.GroupByDay(timeline(t, db, "Thomas"), copenhagen, now)
		for _, p := range days[0].Posts {
			is.True(t, p.New, p.Body)
		}
		var newComments int
		for _, day := range days[1:] {
			for _, p := range day.Posts {
				is.True(t, !p.New, p.Body)
				newComments += p.NewComments
			}
		}
		is.True(t, newComments > 0, "expected a new comment on an older post")
	})

	t.Run("puts nothing in the future when seeded just after midnight", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		early := time.Date(2026, 11, 26, 0, 10, 0, 0, copenhagen)

		_, err := preview.Seed(t.Context(), db, early, copenhagen)
		is.NotError(t, err)

		for _, p := range timeline(t, db, "Markus") {
			is.True(t, !p.Created.T.After(early), "post in the future")
		}
		var latest model.Time
		is.NotError(t, db.H.Get(t.Context(), &latest, `select max(created) from comments`))
		is.True(t, !latest.T.After(early), "comment in the future")
	})

	t.Run("leaves a database with people in it alone", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		sqlitetest.CreateUser(t, db, "Someone")

		seeded, err := preview.Seed(t.Context(), db, now, copenhagen)
		is.NotError(t, err)
		is.True(t, !seeded)
		is.EqualSlice(t, []string{"Someone"}, userNames(t, db))
	})
}

func userNames(t *testing.T, db *sqlite.Database) []string {
	t.Helper()

	var names []string
	is.NotError(t, db.H.Select(t.Context(), &names, `select name from users order by name`))
	return names
}

func timeline(t *testing.T, db *sqlite.Database, name string) []model.TimelinePost {
	t.Helper()

	var id model.UserID
	is.NotError(t, db.H.Get(t.Context(), &id, `select id from users where name = ?`, name))
	posts, _, err := db.GetTimeline(t.Context(), id, nil, 100)
	is.NotError(t, err)
	return posts
}
