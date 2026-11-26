package service_test

import (
	"bytes"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"maragu.dev/is"

	"app/model"
	"app/service"
	"app/servicetest"
	"app/sqlite"
	"app/sqlitetest"
)

var base = time.Date(2026, 11, 26, 9, 0, 0, 0, time.UTC)

// clock that a test sets, and that the operations read the time from.
type clock struct {
	t time.Time
}

func (c *clock) Now() time.Time {
	return c.t
}

func TestFat_CreatePost(t *testing.T) {
	t.Run("posts the cleaned body by the acting user at the time the clock says", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		fat := servicetest.NewFat(t)
		service.CreatePost(fat, db, (&clock{t: base}).Now)

		p, err := fat.CreatePost(t.Context(), signe.ID, "  Cake in the kitchen.\r\nCarrot.  \n")
		is.NotError(t, err)
		is.Equal(t, signe.ID, p.AuthorID)
		is.Equal(t, "Cake in the kitchen.\nCarrot.", p.Body)
		is.True(t, p.Created.T.Equal(base))
	})

	t.Run("refuses a body that cleaning refuses", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		fat := servicetest.NewFat(t)
		service.CreatePost(fat, db, (&clock{t: base}).Now)

		_, err := fat.CreatePost(t.Context(), signe.ID, " \n ")
		is.Error(t, model.ErrorBodyEmpty, err)

		_, err = fat.CreatePost(t.Context(), signe.ID, strings.Repeat("a", model.PostBodyMaxLength+1))
		is.Error(t, model.ErrorBodyTooLong, err)
	})

	t.Run("refuses an inactive user and a user that does not exist", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		sqlitetest.DeactivateUser(t, db, signe.ID)
		fat := servicetest.NewFat(t)
		service.CreatePost(fat, db, (&clock{t: base}).Now)

		_, err := fat.CreatePost(t.Context(), signe.ID, "Hello")
		is.Error(t, model.ErrorUserInactive, err)

		_, err = fat.CreatePost(t.Context(), "u_nope", "Hello")
		is.Error(t, model.ErrorUserNotFound, err)
	})

	t.Run("logs the post's ID, and never its text or the author's email address", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		var logs bytes.Buffer
		fat := service.NewFat(service.NewFatOptions{Log: slog.New(slog.NewTextHandler(&logs, nil))})
		service.CreatePost(fat, db, (&clock{t: base}).Now)

		p, err := fat.CreatePost(t.Context(), signe.ID, "A secret about the coffee machine")
		is.NotError(t, err)

		is.True(t, strings.Contains(logs.String(), p.ID.String()), logs.String())
		is.True(t, !strings.Contains(logs.String(), "coffee"), logs.String())
		is.True(t, !strings.Contains(logs.String(), signe.Email.String()), logs.String())
	})
}

func TestFat_CreateComment(t *testing.T) {
	t.Run("comments with the cleaned body by the acting user at the time the clock says", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		thomas := sqlitetest.CreateUser(t, db, "Thomas")
		c := &clock{t: base}
		fat := servicetest.NewFat(t)
		service.CreatePost(fat, db, c.Now)
		service.CreateComment(fat, db, c.Now)

		p, err := fat.CreatePost(t.Context(), signe.ID, "Cake")
		is.NotError(t, err)

		c.t = base.Add(time.Minute)
		comment, err := fat.CreateComment(t.Context(), thomas.ID, p.ID, " Thanks!\n")
		is.NotError(t, err)
		is.Equal(t, p.ID, comment.PostID)
		is.Equal(t, thomas.ID, comment.AuthorID)
		is.Equal(t, "Thanks!", comment.Body)
		is.True(t, comment.Created.T.Equal(base.Add(time.Minute)))
	})

	t.Run("refuses a body longer than a comment's maximum", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		fat := servicetest.NewFat(t)
		service.CreatePost(fat, db, time.Now)
		service.CreateComment(fat, db, (&clock{t: base}).Now)

		p, err := fat.CreatePost(t.Context(), signe.ID, "Cake")
		is.NotError(t, err)

		_, err = fat.CreateComment(t.Context(), signe.ID, p.ID, strings.Repeat("a", model.CommentBodyMaxLength+1))
		is.Error(t, model.ErrorBodyTooLong, err)
	})

	t.Run("refuses a post that does not exist", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		fat := servicetest.NewFat(t)
		service.CreateComment(fat, db, (&clock{t: base}).Now)

		_, err := fat.CreateComment(t.Context(), signe.ID, "p_nope", "Hello")
		is.Error(t, model.ErrorPostNotFound, err)
	})

	t.Run("refuses an inactive user", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		p, err := db.CreatePost(t.Context(), model.Post{Created: model.Time{T: base}, AuthorID: signe.ID, Body: "Cake"})
		is.NotError(t, err)
		sqlitetest.DeactivateUser(t, db, signe.ID)
		fat := servicetest.NewFat(t)
		service.CreateComment(fat, db, (&clock{t: base}).Now)

		_, err = fat.CreateComment(t.Context(), signe.ID, p.ID, "Hello")
		is.Error(t, model.ErrorUserInactive, err)
	})
}

func TestFat_GetTimeline(t *testing.T) {
	copenhagen, err := time.LoadLocation("Europe/Copenhagen")
	is.NotError(t, err)
	group := model.Group{Name: "The Loft", TimeZone: copenhagen}

	t.Run("groups the page into days in the group's time zone, labelled from the clock", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		thomas := sqlitetest.CreateUser(t, db, "Thomas")
		c := &clock{}
		fat := newTimelineFat(t, db, c, group)

		// 23:30 UTC on the 24th is already the 25th in Copenhagen.
		c.t = time.Date(2026, 11, 24, 23, 30, 0, 0, time.UTC)
		p1 := createPost(t, fat, thomas, "Release shipped")
		c.t = time.Date(2026, 11, 26, 8, 0, 0, 0, copenhagen)
		p2 := createPost(t, fat, thomas, "Cake in the kitchen")

		c.t = time.Date(2026, 11, 26, 9, 0, 0, 0, copenhagen)
		timeline, err := fat.GetTimeline(t.Context(), signe.ID, "", 0)
		is.NotError(t, err)
		is.Equal(t, "", timeline.Next)
		is.Equal(t, 2, len(timeline.Days))

		is.Equal(t, model.Date{Year: 2026, Month: time.November, Day: 26}, timeline.Days[0].Date)
		is.True(t, timeline.Days[0].Today)
		is.Equal(t, p2.ID, timeline.Days[0].Posts[0].ID)
		is.True(t, timeline.Days[0].Posts[0].New)
		is.Equal(t, "Thomas", timeline.Days[0].Posts[0].AuthorName)

		is.Equal(t, model.Date{Year: 2026, Month: time.November, Day: 25}, timeline.Days[1].Date)
		is.True(t, timeline.Days[1].Yesterday)
		is.Equal(t, p1.ID, timeline.Days[1].Posts[0].ID)
	})

	t.Run("pages with the next cursor until there is none", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		c := &clock{t: base}
		fat := newTimelineFat(t, db, c, group)

		var created []model.PostID
		for i := range 5 {
			c.t = base.Add(time.Duration(i) * time.Minute)
			created = append([]model.PostID{createPost(t, fat, signe, fmt.Sprint("Post ", i)).ID}, created...)
		}

		var walked []model.PostID
		var cursor string
		for {
			timeline, err := fat.GetTimeline(t.Context(), signe.ID, cursor, 2)
			is.NotError(t, err)
			for _, day := range timeline.Days {
				for _, p := range day.Posts {
					walked = append(walked, p.ID)
				}
			}
			if timeline.Next == "" {
				break
			}
			cursor = timeline.Next
		}

		is.EqualSlice(t, created, walked)
	})

	t.Run("limits a page to 20 posts when no limit is given, and to 100 at most", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		fat := newTimelineFat(t, db, &clock{t: base}, group)
		for range 101 {
			createPost(t, fat, signe, "Hello")
		}

		for _, test := range []struct{ limit, expected int }{{0, 20}, {-1, 20}, {1, 1}, {100, 100}, {101, 100}} {
			timeline, err := fat.GetTimeline(t.Context(), signe.ID, "", test.limit)
			is.NotError(t, err)
			is.Equal(t, test.expected, len(timeline.Days[0].Posts), fmt.Sprint("limit ", test.limit))
			is.True(t, timeline.Next != "")
		}
	})

	t.Run("refuses a cursor it did not make", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		fat := newTimelineFat(t, db, &clock{t: base}, group)

		_, err := fat.GetTimeline(t.Context(), signe.ID, "garbage", 0)
		is.Error(t, model.ErrorCursorInvalid, err)
	})

	t.Run("refuses an inactive user", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		sqlitetest.DeactivateUser(t, db, signe.ID)
		fat := newTimelineFat(t, db, &clock{t: base}, group)

		_, err := fat.GetTimeline(t.Context(), signe.ID, "", 0)
		is.Error(t, model.ErrorUserInactive, err)
	})

	t.Run("marks what is new to the acting user only", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		thomas := sqlitetest.CreateUser(t, db, "Thomas")
		fat := newTimelineFat(t, db, &clock{t: base}, group)
		p := createPost(t, fat, thomas, "Anyone know a good accountant?")
		_, err := fat.CreateComment(t.Context(), signe.ID, p.ID, "Karen does.")
		is.NotError(t, err)

		timeline, err := fat.GetTimeline(t.Context(), thomas.ID, "", 0)
		is.NotError(t, err)
		is.True(t, !timeline.Days[0].Posts[0].New)
		is.Equal(t, 1, timeline.Days[0].Posts[0].NewComments)

		timeline, err = fat.GetTimeline(t.Context(), signe.ID, "", 0)
		is.NotError(t, err)
		is.True(t, timeline.Days[0].Posts[0].New)
		is.Equal(t, 0, timeline.Days[0].Posts[0].NewComments)
	})
}

func TestFat_GetComments(t *testing.T) {
	t.Run("gets the comments oldest first and records that they were opened", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		thomas := sqlitetest.CreateUser(t, db, "Thomas")
		c := &clock{t: base}
		fat := newTimelineFat(t, db, c, model.Group{Name: "The Loft", TimeZone: time.UTC})
		p := createPost(t, fat, signe, "Coffee machine is sulking again")
		c.t = base.Add(time.Minute)
		c1, err := fat.CreateComment(t.Context(), thomas.ID, p.ID, "Have you tried descaling it?")
		is.NotError(t, err)
		c.t = base.Add(2 * time.Minute)
		c2, err := fat.CreateComment(t.Context(), signe.ID, p.ID, "It's the third time this week.")
		is.NotError(t, err)

		comments, err := fat.GetComments(t.Context(), signe.ID, p.ID)
		is.NotError(t, err)
		is.Equal(t, 2, len(comments))
		is.Equal(t, c1.ID, comments[0].ID)
		is.True(t, comments[0].New)
		is.Equal(t, c2.ID, comments[1].ID)
		is.True(t, !comments[1].New)

		timeline, err := fat.GetTimeline(t.Context(), signe.ID, "", 0)
		is.NotError(t, err)
		is.Equal(t, 0, timeline.Days[0].Posts[0].NewComments)
	})

	t.Run("refuses a post that does not exist", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		fat := servicetest.NewFat(t)
		service.GetComments(fat, db)

		_, err := fat.GetComments(t.Context(), signe.ID, "p_nope")
		is.Error(t, model.ErrorPostNotFound, err)
	})

	t.Run("refuses an inactive user", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		p, err := db.CreatePost(t.Context(), model.Post{Created: model.Time{T: base}, AuthorID: signe.ID, Body: "Cake"})
		is.NotError(t, err)
		sqlitetest.DeactivateUser(t, db, signe.ID)
		fat := servicetest.NewFat(t)
		service.GetComments(fat, db)

		_, err = fat.GetComments(t.Context(), signe.ID, p.ID)
		is.Error(t, model.ErrorUserInactive, err)
	})
}

func TestFat_timelineWiring(t *testing.T) {
	operations := []struct {
		name string
		wire func(f *service.Fat, db *sqlite.Database)
		call func(f *service.Fat)
	}{
		{
			name: "CreatePost",
			wire: func(f *service.Fat, db *sqlite.Database) { service.CreatePost(f, db, time.Now) },
			call: func(f *service.Fat) { _, _ = f.CreatePost(t.Context(), "u_x", "Hi") },
		},
		{
			name: "CreateComment",
			wire: func(f *service.Fat, db *sqlite.Database) { service.CreateComment(f, db, time.Now) },
			call: func(f *service.Fat) { _, _ = f.CreateComment(t.Context(), "u_x", "p_x", "Hi") },
		},
		{
			name: "GetTimeline",
			wire: func(f *service.Fat, db *sqlite.Database) { service.GetTimeline(f, db, time.Now, model.Group{}) },
			call: func(f *service.Fat) { _, _ = f.GetTimeline(t.Context(), "u_x", "", 0) },
		},
		{
			name: "GetComments",
			wire: func(f *service.Fat, db *sqlite.Database) { service.GetComments(f, db) },
			call: func(f *service.Fat) { _, _ = f.GetComments(t.Context(), "u_x", "p_x") },
		},
	}

	for _, op := range operations {
		t.Run(op.name+" panics naming the wiring function when called unwired", func(t *testing.T) {
			defer func() {
				is.Equal(t, "service: "+op.name+" not wired; call service."+op.name+" or service.Setup", fmt.Sprint(recover()))
			}()

			op.call(servicetest.NewFat(t))
		})

		t.Run(op.name+" panics when wired twice", func(t *testing.T) {
			defer func() {
				is.Equal(t, "service: "+op.name+" already wired", fmt.Sprint(recover()))
			}()

			fat := servicetest.NewFat(t)
			db := sqlitetest.NewDatabase(t)
			op.wire(fat, db)
			op.wire(fat, db)
		})
	}
}

// newTimelineFat with the timeline's operations wired to the given database and clock.
func newTimelineFat(t *testing.T, db *sqlite.Database, c *clock, group model.Group) *service.Fat {
	t.Helper()

	fat := servicetest.NewFat(t)
	service.CreatePost(fat, db, c.Now)
	service.CreateComment(fat, db, c.Now)
	service.GetTimeline(fat, db, c.Now, group)
	service.GetComments(fat, db)
	return fat
}

func createPost(t *testing.T, fat *service.Fat, author model.User, body string) model.Post {
	t.Helper()

	p, err := fat.CreatePost(t.Context(), author.ID, body)
	is.NotError(t, err)
	return p
}
