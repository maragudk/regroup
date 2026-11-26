package service_test

import (
	"bytes"
	"context"
	"crypto/rand"
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

// base is not today, so that a test can tell the clock it gives from the real one.
var base = time.Date(2031, 3, 14, 9, 0, 0, 0, time.UTC)

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

		p, err := fat.CreatePost(t.Context(), signe.ID, token(t), "  Cake in the kitchen.\r\nCarrot.  \n")
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

		_, err := fat.CreatePost(t.Context(), signe.ID, token(t), " \n ")
		is.Error(t, model.ErrorBodyEmpty, err)

		_, err = fat.CreatePost(t.Context(), signe.ID, token(t), strings.Repeat("a", model.PostBodyMaxLength+1))
		is.Error(t, model.ErrorBodyTooLong, err)
	})

	t.Run("refuses an inactive user and a user that does not exist", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		sqlitetest.DeactivateUser(t, db, signe.ID)
		fat := servicetest.NewFat(t)
		service.CreatePost(fat, db, (&clock{t: base}).Now)

		_, err := fat.CreatePost(t.Context(), signe.ID, token(t), "Hello")
		is.Error(t, model.ErrorUserInactive, err)

		_, err = fat.CreatePost(t.Context(), "u_nope", token(t), "Hello")
		is.Error(t, model.ErrorUserNotFound, err)
	})

	t.Run("logs the post's ID, and never its text or the author's email address", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		var logs bytes.Buffer
		fat := service.NewFat(service.NewFatOptions{Log: slog.New(slog.NewTextHandler(&logs, nil))})
		service.CreatePost(fat, db, (&clock{t: base}).Now)

		p, err := fat.CreatePost(t.Context(), signe.ID, token(t), "A secret about the coffee machine")
		is.NotError(t, err)

		is.True(t, strings.Contains(logs.String(), p.ID.String()), logs.String())
		is.True(t, !strings.Contains(logs.String(), "coffee"), logs.String())
		is.True(t, !strings.Contains(logs.String(), signe.Email.String()), logs.String())
	})
}

func TestFat_CreatePost_again(t *testing.T) {
	t.Run("returns the post made the first time when the same token is sent again with the same text once cleaned, and makes nothing", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		c := &clock{t: base}
		fat := servicetest.NewFat(t)
		service.CreatePost(fat, db, c.Now)
		sent := token(t)

		first, err := fat.CreatePost(t.Context(), signe.ID, sent, "Cake.")
		is.NotError(t, err)

		c.t = base.Add(time.Minute)
		for _, body := range []string{"Cake.", "  Cake.\r\n"} {
			again, err := fat.CreatePost(t.Context(), signe.ID, sent, body)
			is.NotError(t, err)
			is.Equal(t, first, again)
		}

		posts, _, err := db.GetTimeline(t.Context(), signe.ID, nil, 10)
		is.NotError(t, err)
		is.Equal(t, 1, len(posts))
	})

	t.Run("returns the post made the first time and says the text was not posted when the same token is sent again with other text, and makes nothing", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		c := &clock{t: base}
		fat := servicetest.NewFat(t)
		service.CreatePost(fat, db, c.Now)
		sent := token(t)

		first, err := fat.CreatePost(t.Context(), signe.ID, sent, "Cake.")
		is.NotError(t, err)

		c.t = base.Add(time.Minute)
		for _, body := range []string{"Cake!", "cake.", " ", "", strings.Repeat("a", model.PostBodyMaxLength+1), "Cake.\xff"} {
			again, err := fat.CreatePost(t.Context(), signe.ID, sent, body)
			is.Error(t, model.ErrorFormResentChanged, err)
			is.Equal(t, first, again)
		}

		posts, _, err := db.GetTimeline(t.Context(), signe.ID, nil, 10)
		is.NotError(t, err)
		is.Equal(t, 1, len(posts))
	})

	t.Run("says the text was not posted when another sending with the same token made the post in the meantime, with other text", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		fat := servicetest.NewFat(t)
		service.CreatePost(fat, &unseenTokens{Database: db}, (&clock{t: base}).Now)
		sent := token(t)

		first, err := fat.CreatePost(t.Context(), signe.ID, sent, "Cake.")
		is.NotError(t, err)

		again, err := fat.CreatePost(t.Context(), signe.ID, sent, "Cake.")
		is.NotError(t, err)
		is.Equal(t, first, again)

		again, err = fat.CreatePost(t.Context(), signe.ID, sent, "Cake!")
		is.Error(t, model.ErrorFormResentChanged, err)
		is.Equal(t, first, again)
	})

	t.Run("makes a post for each new token", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		fat := servicetest.NewFat(t)
		service.CreatePost(fat, db, (&clock{t: base}).Now)

		p1, err := fat.CreatePost(t.Context(), signe.ID, token(t), "Cake.")
		is.NotError(t, err)
		p2, err := fat.CreatePost(t.Context(), signe.ID, token(t), "Cake.")
		is.NotError(t, err)
		is.True(t, p1.ID != p2.ID)
	})

	t.Run("refuses a token that is not one, before anything about the body", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		fat := servicetest.NewFat(t)
		service.CreatePost(fat, db, (&clock{t: base}).Now)

		for _, sent := range []string{"", "not a token", "q6urq6urq6urq6urq6urqx"} {
			_, err := fat.CreatePost(t.Context(), signe.ID, sent, "Cake.")
			is.Error(t, model.ErrorFormTokenInvalid, err)
			_, err = fat.CreatePost(t.Context(), signe.ID, sent, " ")
			is.Error(t, model.ErrorFormTokenInvalid, err)
		}
	})
}

func TestFat_logging(t *testing.T) {
	t.Run("logs IDs, and never the text of a post or a comment or anyone's email address, through every operation", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		thomas := sqlitetest.CreateUser(t, db, "Thomas")
		var logs bytes.Buffer
		fat := service.NewFat(service.NewFatOptions{Log: slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))})
		c := &clock{t: base}
		service.CreatePost(fat, db, c.Now)
		service.CreateComment(fat, db, c.Now)
		service.GetTimeline(fat, db, c.Now, model.Group{Name: "The Loft", TimeZone: time.UTC})
		service.GetComments(fat, db)

		p, err := fat.CreatePost(t.Context(), signe.ID, token(t), "A secret about the coffee machine")
		is.NotError(t, err)
		comment, err := fat.CreateComment(t.Context(), thomas.ID, p.ID, token(t), "A secret about the printer")
		is.NotError(t, err)
		_, err = fat.GetTimeline(t.Context(), thomas.ID, "", 0)
		is.NotError(t, err)
		_, err = fat.GetComments(t.Context(), signe.ID, p.ID)
		is.NotError(t, err)
		_, err = fat.CreatePost(t.Context(), signe.ID, token(t), " ")
		is.Error(t, model.ErrorBodyEmpty, err)

		is.True(t, strings.Contains(logs.String(), comment.ID.String()), logs.String())
		for _, secret := range []string{"secret", signe.Email.String(), thomas.Email.String()} {
			is.True(t, !strings.Contains(logs.String(), secret), logs.String())
		}
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

		p, err := fat.CreatePost(t.Context(), signe.ID, token(t), "Cake")
		is.NotError(t, err)

		c.t = base.Add(time.Minute)
		comment, err := fat.CreateComment(t.Context(), thomas.ID, p.ID, token(t), " Thanks!\n")
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
		service.CreatePost(fat, db, (&clock{t: base}).Now)
		service.CreateComment(fat, db, (&clock{t: base}).Now)

		p, err := fat.CreatePost(t.Context(), signe.ID, token(t), "Cake")
		is.NotError(t, err)

		_, err = fat.CreateComment(t.Context(), signe.ID, p.ID, token(t), strings.Repeat("a", model.CommentBodyMaxLength+1))
		is.Error(t, model.ErrorBodyTooLong, err)
	})

	t.Run("refuses a post that does not exist, before anything about the body", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		fat := servicetest.NewFat(t)
		service.CreateComment(fat, db, (&clock{t: base}).Now)

		for _, body := range []string{"Hello", "", " \n ", strings.Repeat("a", model.CommentBodyMaxLength+1), "Hello \xff"} {
			_, err := fat.CreateComment(t.Context(), signe.ID, "p_nope", token(t), body)
			is.Error(t, model.ErrorPostNotFound, err)
		}
	})

	t.Run("returns the comment made the first time when the same token is sent again, whatever the post, and says the text was not added if it is other text, and makes nothing", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		c := &clock{t: base}
		fat := servicetest.NewFat(t)
		service.CreatePost(fat, db, c.Now)
		service.CreateComment(fat, db, c.Now)
		p := createPost(t, fat, signe, "Cake")
		sent := token(t)

		first, err := fat.CreateComment(t.Context(), signe.ID, p.ID, sent, "Gone now.")
		is.NotError(t, err)

		c.t = base.Add(time.Minute)
		for _, again := range []struct {
			post    model.PostID
			body    string
			changed bool
		}{
			{p.ID, "Gone now.", false},
			{p.ID, " Gone now.\n", false},
			{"p_nope", "Gone now.", false},
			{p.ID, "Gone now!", true},
			{p.ID, "", true},
			{"p_nope", strings.Repeat("a", model.CommentBodyMaxLength+1), true},
		} {
			comment, err := fat.CreateComment(t.Context(), signe.ID, again.post, sent, again.body)
			if again.changed {
				is.Error(t, model.ErrorFormResentChanged, err)
			} else {
				is.NotError(t, err)
			}
			is.Equal(t, first, comment)
		}

		comments, err := db.GetComments(t.Context(), signe.ID, p.ID)
		is.NotError(t, err)
		is.Equal(t, 1, len(comments))
	})

	t.Run("says the text was not added when another sending with the same token made the comment in the meantime, with other text", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		c := &clock{t: base}
		fat := servicetest.NewFat(t)
		service.CreatePost(fat, db, c.Now)
		service.CreateComment(fat, &unseenTokens{Database: db}, c.Now)
		p := createPost(t, fat, signe, "Cake")
		sent := token(t)

		first, err := fat.CreateComment(t.Context(), signe.ID, p.ID, sent, "Yes.")
		is.NotError(t, err)

		again, err := fat.CreateComment(t.Context(), signe.ID, p.ID, sent, "Yes.")
		is.NotError(t, err)
		is.Equal(t, first, again)

		again, err = fat.CreateComment(t.Context(), signe.ID, p.ID, sent, "Yes!")
		is.Error(t, model.ErrorFormResentChanged, err)
		is.Equal(t, first, again)
	})

	t.Run("refuses a token that is not one", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		fat := servicetest.NewFat(t)
		service.CreatePost(fat, db, (&clock{t: base}).Now)
		service.CreateComment(fat, db, (&clock{t: base}).Now)
		p := createPost(t, fat, signe, "Cake")

		for _, sent := range []string{"", "not a token"} {
			_, err := fat.CreateComment(t.Context(), signe.ID, p.ID, sent, "Yes.")
			is.Error(t, model.ErrorFormTokenInvalid, err)
		}
	})

	t.Run("refuses an inactive user", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		p, err := db.CreatePost(t.Context(), model.Post{Created: model.Time{T: base}, AuthorID: signe.ID, Body: "Cake"}, "")
		is.NotError(t, err)
		sqlitetest.DeactivateUser(t, db, signe.ID)
		fat := servicetest.NewFat(t)
		service.CreateComment(fat, db, (&clock{t: base}).Now)

		_, err = fat.CreateComment(t.Context(), signe.ID, p.ID, token(t), "Hello")
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

		// 23:30 UTC on the 12th is already the 13th in Copenhagen.
		c.t = time.Date(2031, 3, 12, 23, 30, 0, 0, time.UTC)
		p1 := createPost(t, fat, thomas, "Release shipped")
		c.t = time.Date(2031, 3, 14, 8, 0, 0, 0, copenhagen)
		p2 := createPost(t, fat, thomas, "Cake in the kitchen")

		c.t = time.Date(2031, 3, 14, 9, 0, 0, 0, copenhagen)
		timeline, err := fat.GetTimeline(t.Context(), signe.ID, "", 0)
		is.NotError(t, err)
		is.Equal(t, "", timeline.Next)
		is.Equal(t, 2, len(timeline.Days))

		is.Equal(t, model.Date{Year: 2031, Month: time.March, Day: 14}, timeline.Days[0].Date)
		is.True(t, timeline.Days[0].Today)
		is.Equal(t, p2.ID, timeline.Days[0].Posts[0].ID)
		is.True(t, timeline.Days[0].Posts[0].New)
		is.Equal(t, "Thomas", timeline.Days[0].Posts[0].AuthorName)
		is.Equal(t, copenhagen, timeline.Days[0].Posts[0].Created.T.Location())

		is.Equal(t, model.Date{Year: 2031, Month: time.March, Day: 13}, timeline.Days[1].Date)
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

	t.Run("refuses an inactive user, on the first page and the ones after it", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		fat := newTimelineFat(t, db, &clock{t: base}, group)
		createPost(t, fat, signe, "One")
		createPost(t, fat, signe, "Two")
		timeline, err := fat.GetTimeline(t.Context(), signe.ID, "", 1)
		is.NotError(t, err)
		sqlitetest.DeactivateUser(t, db, signe.ID)

		_, err = fat.GetTimeline(t.Context(), signe.ID, "", 0)
		is.Error(t, model.ErrorUserInactive, err)

		_, err = fat.GetTimeline(t.Context(), signe.ID, timeline.Next, 0)
		is.Error(t, model.ErrorUserInactive, err)
	})

	t.Run("marks what is new to the acting user only", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		thomas := sqlitetest.CreateUser(t, db, "Thomas")
		fat := newTimelineFat(t, db, &clock{t: base}, group)
		p := createPost(t, fat, thomas, "Anyone know a good accountant?")
		_, err := fat.CreateComment(t.Context(), signe.ID, p.ID, token(t), "Karen does.")
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
		c1, err := fat.CreateComment(t.Context(), thomas.ID, p.ID, token(t), "Have you tried descaling it?")
		is.NotError(t, err)
		c.t = base.Add(2 * time.Minute)
		c2, err := fat.CreateComment(t.Context(), signe.ID, p.ID, token(t), "It's the third time this week.")
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
		p, err := db.CreatePost(t.Context(), model.Post{Created: model.Time{T: base}, AuthorID: signe.ID, Body: "Cake"}, "")
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
			call: func(f *service.Fat) { _, _ = f.CreatePost(t.Context(), "u_x", "", "Hi") },
		},
		{
			name: "CreateComment",
			wire: func(f *service.Fat, db *sqlite.Database) { service.CreateComment(f, db, time.Now) },
			call: func(f *service.Fat) { _, _ = f.CreateComment(t.Context(), "u_x", "p_x", "", "Hi") },
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

// unseenTokens is the real database, except that a look-up by form token finds nothing, as when another
// sending with the same token is stored between the look-up and the insert.
type unseenTokens struct {
	*sqlite.Database
}

func (u *unseenTokens) GetPostByFormToken(context.Context, model.UserID, model.FormToken) (model.Post, error) {
	return model.Post{}, model.ErrorPostNotFound
}

func (u *unseenTokens) GetCommentByFormToken(context.Context, model.UserID, model.FormToken) (model.Comment, error) {
	return model.Comment{}, model.ErrorCommentNotFound
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

	p, err := fat.CreatePost(t.Context(), author.ID, token(t), body)
	is.NotError(t, err)
	return p
}

// token for a form, new each time.
func token(t *testing.T) string {
	t.Helper()

	formToken, err := model.NewFormToken(rand.Reader)
	is.NotError(t, err)
	return formToken.String()
}
