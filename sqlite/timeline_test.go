package sqlite_test

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"hegel.dev/go/hegel"
	"maragu.dev/is"
	"maragu.dev/migrate"

	"app/model"
	"app/sqlite"
	"app/sqlitetest"
)

var base = time.Date(2026, 11, 26, 9, 0, 0, 0, time.UTC)

func TestDatabase_CreatePost(t *testing.T) {
	t.Run("stores a post with the time, author, and body given, and an ID", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")

		p, err := db.CreatePost(t.Context(), model.Post{
			Created:  model.Time{T: base.Add(123 * time.Millisecond)},
			AuthorID: signe.ID,
			Body:     "Cake in the kitchen.\n\nIt's carrot.",
		}, "")
		is.NotError(t, err)
		is.True(t, strings.HasPrefix(p.ID.String(), "p_"), p.ID.String())
		is.True(t, p.Created.T.Equal(base.Add(123*time.Millisecond)))
		is.Equal(t, signe.ID, p.AuthorID)
		is.Equal(t, "Cake in the kitchen.\n\nIt's carrot.", p.Body)
	})

	t.Run("returns the post the author made with a token before, and stores nothing, when the author sends that token again", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")

		first, err := db.CreatePost(t.Context(), model.Post{Created: model.Time{T: base}, AuthorID: signe.ID, Body: "Cake."}, "q6urq6urq6urq6urq6urqw")
		is.NotError(t, err)

		again, err := db.CreatePost(t.Context(), model.Post{Created: model.Time{T: base.Add(time.Minute)}, AuthorID: signe.ID, Body: "Cake!"}, "q6urq6urq6urq6urq6urqw")
		is.NotError(t, err)
		is.Equal(t, first, again)
		is.Equal(t, 1, count(t, db, "posts"))

		// The next post is stored after the first, as if nothing had happened in between.
		next, err := db.CreatePost(t.Context(), model.Post{Created: model.Time{T: base}, AuthorID: signe.ID, Body: "More cake."}, "")
		is.NotError(t, err)
		is.EqualSlice(t, []model.PostID{next.ID, first.ID}, ids(getTimeline(t, db, signe)))
	})

	t.Run("makes a post for each author who sends the same token, and for each post sent without one", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		thomas := sqlitetest.CreateUser(t, db, "Thomas")

		for _, author := range []model.User{signe, thomas} {
			_, err := db.CreatePost(t.Context(), model.Post{Created: model.Time{T: base}, AuthorID: author.ID, Body: "Cake."}, "q6urq6urq6urq6urq6urqw")
			is.NotError(t, err)
		}
		for range 2 {
			_, err := db.CreatePost(t.Context(), model.Post{Created: model.Time{T: base}, AuthorID: signe.ID, Body: "Cake."}, "")
			is.NotError(t, err)
		}
		is.Equal(t, 4, count(t, db, "posts"))
	})

	t.Run("makes one post however many times the same token is sent at once", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")

		posts := make([]model.Post, 8)
		errs := make([]error, len(posts))
		var wg sync.WaitGroup
		for i := range posts {
			wg.Go(func() {
				posts[i], errs[i] = db.CreatePost(t.Context(), model.Post{Created: model.Time{T: base}, AuthorID: signe.ID, Body: fmt.Sprint("Cake ", i)}, "q6urq6urq6urq6urq6urqw")
			})
		}
		wg.Wait()

		for i := range posts {
			is.NotError(t, errs[i])
			is.Equal(t, posts[0], posts[i])
		}
		is.Equal(t, 1, count(t, db, "posts"))
	})

	t.Run("makes exactly one post for each author and token, however often and in whatever order each is sent", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		people := []model.User{sqlitetest.CreateUser(t, db, "Signe"), sqlitetest.CreateUser(t, db, "Thomas")}

		hegel.Test(t, func(ht *hegel.T) {
			_, err := db.H.DB.ExecContext(t.Context(), "delete from posts")
			is.NotError(ht, err)

			sends := hegel.Draw(ht, hegel.Lists(submission(len(people))).MinSize(1).MaxSize(20))

			first := map[submitted]model.Post{}
			var untokened int
			for i, s := range sends {
				p, err := db.CreatePost(t.Context(), model.Post{Created: model.Time{T: base}, AuthorID: people[s.author].ID, Body: fmt.Sprint("Send ", i)}, s.token)
				if err != nil {
					ht.Fatalf("send %v of %+v failed: %v", i, s, err)
				}
				if s.token == "" {
					untokened++
					continue
				}
				if f, ok := first[s]; ok && f != p {
					ht.Fatalf("send %v of %+v gave %+v, not what the first send gave, %+v", i, s, p, f)
				}
				if _, ok := first[s]; !ok {
					first[s] = p
				}
			}
			if n := count(ht, db, "posts"); n != len(first)+untokened {
				ht.Fatalf("%v sends of %v submissions with tokens and %v without stored %v posts", len(sends), len(first), untokened, n)
			}
		})
	})
}

func TestDatabase_CreateComment(t *testing.T) {
	t.Run("stores a comment on a post", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		p := createPost(t, db, signe, base)

		c, err := db.CreateComment(t.Context(), model.Comment{
			Created:  model.Time{T: base.Add(time.Minute)},
			PostID:   p.ID,
			AuthorID: signe.ID,
			Body:     "Gone now.",
		}, "")
		is.NotError(t, err)
		is.True(t, strings.HasPrefix(c.ID.String(), "c_"), c.ID.String())
		is.True(t, c.Created.T.Equal(base.Add(time.Minute)))
		is.Equal(t, p.ID, c.PostID)
		is.Equal(t, signe.ID, c.AuthorID)
		is.Equal(t, "Gone now.", c.Body)
	})

	t.Run("refuses a comment on a post that does not exist", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")

		_, err := db.CreateComment(t.Context(), model.Comment{Created: model.Time{T: base}, PostID: "p_nope", AuthorID: signe.ID, Body: "Hello?"}, "")
		is.Error(t, model.ErrorPostNotFound, err)
	})

	t.Run("returns the comment the author made with a token before, and stores nothing, when the author sends that token again, whatever the post", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		p := createPost(t, db, signe, base)
		other := createPost(t, db, signe, base)

		first, err := db.CreateComment(t.Context(), model.Comment{Created: model.Time{T: base}, PostID: p.ID, AuthorID: signe.ID, Body: "Gone now."}, "q6urq6urq6urq6urq6urqw")
		is.NotError(t, err)

		for _, post := range []model.Post{p, other} {
			again, err := db.CreateComment(t.Context(), model.Comment{Created: model.Time{T: base.Add(time.Minute)}, PostID: post.ID, AuthorID: signe.ID, Body: "Gone!"}, "q6urq6urq6urq6urq6urqw")
			is.NotError(t, err)
			is.Equal(t, first, again)
		}
		is.Equal(t, 1, count(t, db, "comments"))
	})

	t.Run("makes a comment for each author who sends the same token, and for each comment sent without one", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		thomas := sqlitetest.CreateUser(t, db, "Thomas")
		p := createPost(t, db, signe, base)

		for _, author := range []model.User{signe, thomas} {
			_, err := db.CreateComment(t.Context(), model.Comment{Created: model.Time{T: base}, PostID: p.ID, AuthorID: author.ID, Body: "Yes."}, "q6urq6urq6urq6urq6urqw")
			is.NotError(t, err)
		}
		for range 2 {
			_, err := db.CreateComment(t.Context(), model.Comment{Created: model.Time{T: base}, PostID: p.ID, AuthorID: signe.ID, Body: "Yes."}, "")
			is.NotError(t, err)
		}
		is.Equal(t, 4, count(t, db, "comments"))
	})

	t.Run("makes one comment however many times the same token is sent at once", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		p := createPost(t, db, signe, base)

		comments := make([]model.Comment, 8)
		errs := make([]error, len(comments))
		var wg sync.WaitGroup
		for i := range comments {
			wg.Go(func() {
				comments[i], errs[i] = db.CreateComment(t.Context(), model.Comment{Created: model.Time{T: base}, PostID: p.ID, AuthorID: signe.ID, Body: fmt.Sprint("Yes ", i)}, "q6urq6urq6urq6urq6urqw")
			})
		}
		wg.Wait()

		for i := range comments {
			is.NotError(t, errs[i])
			is.Equal(t, comments[0], comments[i])
		}
		is.Equal(t, 1, count(t, db, "comments"))
	})

	t.Run("makes exactly one comment for each author and token, however often and in whatever order each is sent", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		people := []model.User{sqlitetest.CreateUser(t, db, "Signe"), sqlitetest.CreateUser(t, db, "Thomas")}
		p := createPost(t, db, people[0], base)

		hegel.Test(t, func(ht *hegel.T) {
			_, err := db.H.DB.ExecContext(t.Context(), "delete from comments")
			is.NotError(ht, err)

			sends := hegel.Draw(ht, hegel.Lists(submission(len(people))).MinSize(1).MaxSize(20))

			first := map[submitted]model.Comment{}
			var untokened int
			for i, s := range sends {
				c, err := db.CreateComment(t.Context(), model.Comment{Created: model.Time{T: base}, PostID: p.ID, AuthorID: people[s.author].ID, Body: fmt.Sprint("Send ", i)}, s.token)
				if err != nil {
					ht.Fatalf("send %v of %+v failed: %v", i, s, err)
				}
				if s.token == "" {
					untokened++
					continue
				}
				if f, ok := first[s]; ok && f != c {
					ht.Fatalf("send %v of %+v gave %+v, not what the first send gave, %+v", i, s, c, f)
				}
				if _, ok := first[s]; !ok {
					first[s] = c
				}
			}
			if n := count(ht, db, "comments"); n != len(first)+untokened {
				ht.Fatalf("%v sends of %v submissions with tokens and %v without stored %v comments", len(sends), len(first), untokened, n)
			}
		})
	})
}

func TestDatabase_GetPost(t *testing.T) {
	t.Run("gets a post by its ID", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		p := createPost(t, db, signe, base)

		got, err := db.GetPost(t.Context(), p.ID)
		is.NotError(t, err)
		is.Equal(t, p, got)
	})

	t.Run("refuses a post that does not exist", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)

		_, err := db.GetPost(t.Context(), "p_nope")
		is.Error(t, model.ErrorPostNotFound, err)
	})
}

func TestDatabase_GetPostByFormToken(t *testing.T) {
	t.Run("gets the post the author made with the token, and nobody else's", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		thomas := sqlitetest.CreateUser(t, db, "Thomas")
		p, err := db.CreatePost(t.Context(), model.Post{Created: model.Time{T: base}, AuthorID: signe.ID, Body: "Cake."}, "q6urq6urq6urq6urq6urqw")
		is.NotError(t, err)

		got, err := db.GetPostByFormToken(t.Context(), signe.ID, "q6urq6urq6urq6urq6urqw")
		is.NotError(t, err)
		is.Equal(t, p, got)

		_, err = db.GetPostByFormToken(t.Context(), thomas.ID, "q6urq6urq6urq6urq6urqw")
		is.Error(t, model.ErrorPostNotFound, err)

		_, err = db.GetPostByFormToken(t.Context(), signe.ID, "AAAAAAAAAAAAAAAAAAAAAA")
		is.Error(t, model.ErrorPostNotFound, err)
	})

	t.Run("never finds a post that was made without a token", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		createPost(t, db, signe, base)

		_, err := db.GetPostByFormToken(t.Context(), signe.ID, "")
		is.Error(t, model.ErrorPostNotFound, err)
	})
}

func TestDatabase_GetCommentByFormToken(t *testing.T) {
	t.Run("gets the comment the author made with the token, and nobody else's", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		thomas := sqlitetest.CreateUser(t, db, "Thomas")
		p := createPost(t, db, signe, base)
		c, err := db.CreateComment(t.Context(), model.Comment{Created: model.Time{T: base}, PostID: p.ID, AuthorID: signe.ID, Body: "Yes."}, "q6urq6urq6urq6urq6urqw")
		is.NotError(t, err)

		got, err := db.GetCommentByFormToken(t.Context(), signe.ID, "q6urq6urq6urq6urq6urqw")
		is.NotError(t, err)
		is.Equal(t, c, got)

		_, err = db.GetCommentByFormToken(t.Context(), thomas.ID, "q6urq6urq6urq6urq6urqw")
		is.Error(t, model.ErrorCommentNotFound, err)
	})

	t.Run("never finds a comment that was made without a token", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		createComment(t, db, signe, createPost(t, db, signe, base), base)

		_, err := db.GetCommentByFormToken(t.Context(), signe.ID, "")
		is.Error(t, model.ErrorCommentNotFound, err)
	})
}

func TestDatabase_GetTimeline(t *testing.T) {
	t.Run("gets posts newest first, with the author's name and the number of comments", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		thomas := sqlitetest.CreateUser(t, db, "Thomas")
		p1 := createPost(t, db, signe, base)
		p2 := createPost(t, db, thomas, base.Add(time.Hour))
		createComment(t, db, thomas, p1, base.Add(2*time.Hour))
		createComment(t, db, signe, p1, base.Add(3*time.Hour))

		posts, next, err := db.GetTimeline(t.Context(), signe.ID, nil, 10)
		is.NotError(t, err)
		is.True(t, next == nil)
		is.EqualSlice(t, []model.PostID{p2.ID, p1.ID}, ids(posts))
		is.Equal(t, "Thomas", posts[0].AuthorName)
		is.Equal(t, 0, posts[0].Comments)
		is.Equal(t, "Signe", posts[1].AuthorName)
		is.Equal(t, 2, posts[1].Comments)
		is.Equal(t, p1.Body, posts[1].Body)
		is.True(t, posts[1].Created.T.Equal(base))
	})

	t.Run("puts the later stored of two posts with the same time first", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		p1 := createPost(t, db, signe, base)
		p2 := createPost(t, db, signe, base)
		p3 := createPost(t, db, signe, base)

		posts, _, err := db.GetTimeline(t.Context(), signe.ID, nil, 10)
		is.NotError(t, err)
		is.EqualSlice(t, []model.PostID{p3.ID, p2.ID, p1.ID}, ids(posts))
	})

	t.Run("orders by the time posted, not the order stored", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		p1 := createPost(t, db, signe, base.Add(time.Hour))
		p2 := createPost(t, db, signe, base)

		posts, _, err := db.GetTimeline(t.Context(), signe.ID, nil, 10)
		is.NotError(t, err)
		is.EqualSlice(t, []model.PostID{p1.ID, p2.ID}, ids(posts))
	})

	t.Run("pages with a cursor that holds across equal times and posts added between pages", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		p1 := createPost(t, db, signe, base)
		p2 := createPost(t, db, signe, base.Add(time.Minute))
		p3 := createPost(t, db, signe, base.Add(time.Minute))
		p4 := createPost(t, db, signe, base.Add(time.Minute))
		p5 := createPost(t, db, signe, base.Add(2*time.Minute))

		page1, next, err := db.GetTimeline(t.Context(), signe.ID, nil, 2)
		is.NotError(t, err)
		is.EqualSlice(t, []model.PostID{p5.ID, p4.ID}, ids(page1))
		is.True(t, next != nil)

		// A newer post, and one with the same time as the last one shown, which sorts before it.
		createPost(t, db, signe, base.Add(time.Hour))
		createPost(t, db, signe, base.Add(time.Minute))

		page2, next, err := db.GetTimeline(t.Context(), signe.ID, next, 2)
		is.NotError(t, err)
		is.EqualSlice(t, []model.PostID{p3.ID, p2.ID}, ids(page2))
		is.True(t, next != nil)

		page3, next, err := db.GetTimeline(t.Context(), signe.ID, next, 2)
		is.NotError(t, err)
		is.EqualSlice(t, []model.PostID{p1.ID}, ids(page3))
		is.True(t, next == nil)
	})

	t.Run("has no next page when the last page is exactly full", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		createPost(t, db, signe, base)
		createPost(t, db, signe, base.Add(time.Minute))

		posts, next, err := db.GetTimeline(t.Context(), signe.ID, nil, 2)
		is.NotError(t, err)
		is.Equal(t, 2, len(posts))
		is.True(t, next == nil)
	})

	t.Run("gets an empty timeline", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")

		posts, next, err := db.GetTimeline(t.Context(), signe.ID, nil, 10)
		is.NotError(t, err)
		is.Equal(t, 0, len(posts))
		is.True(t, next == nil)
	})

	t.Run("refuses a reader that does not exist", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)

		_, _, err := db.GetTimeline(t.Context(), "u_nope", nil, 10)
		is.Error(t, model.ErrorUserNotFound, err)
	})

	t.Run("marks posts by others since the reader joined as new on the first look", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		thomas := sqlitetest.CreateUser(t, db, "Thomas")
		p := createPost(t, db, thomas, base)

		posts, _, err := db.GetTimeline(t.Context(), signe.ID, nil, 10)
		is.NotError(t, err)
		is.Equal(t, p.ID, posts[0].ID)
		is.True(t, posts[0].New)
	})

	t.Run("never marks a person's own post as new to them", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		createPost(t, db, signe, base)

		posts, _, err := db.GetTimeline(t.Context(), signe.ID, nil, 10)
		is.NotError(t, err)
		is.True(t, !posts[0].New)
	})

	t.Run("marks only the posts since the previous look as new", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		thomas := sqlitetest.CreateUser(t, db, "Thomas")
		p1 := createPost(t, db, thomas, base)

		getTimeline(t, db, signe)

		// The same time as the first post: what counts is that it came after the look.
		p2 := createPost(t, db, thomas, base)

		posts := getTimeline(t, db, signe)
		is.EqualSlice(t, []model.PostID{p2.ID, p1.ID}, ids(posts))
		is.True(t, posts[0].New)
		is.True(t, !posts[1].New)

		// Looking again with nothing posted in between leaves nothing new.
		posts = getTimeline(t, db, signe)
		is.True(t, !posts[0].New)
		is.True(t, !posts[1].New)
	})

	t.Run("marks a page after the first against the look before the reader's latest look", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		thomas := sqlitetest.CreateUser(t, db, "Thomas")
		createPost(t, db, thomas, base)
		createPost(t, db, thomas, base.Add(time.Minute))

		_, next, err := db.GetTimeline(t.Context(), signe.ID, nil, 1)
		is.NotError(t, err)

		// Looking again, say in another tab, makes everything on the timeline seen.
		getTimeline(t, db, signe)

		page2, _, err := db.GetTimeline(t.Context(), signe.ID, next, 1)
		is.NotError(t, err)
		is.True(t, !page2[0].New)
	})

	t.Run("marks a post as new even after the newest post before it is gone", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		thomas := sqlitetest.CreateUser(t, db, "Thomas")
		createPost(t, db, thomas, base)
		gone := createPost(t, db, thomas, base.Add(time.Minute))
		getTimeline(t, db, signe)

		is.NotError(t, db.H.Exec(t.Context(), `delete from posts where id = ?`, gone.ID))
		p := createPost(t, db, thomas, base.Add(2*time.Minute))

		posts := getTimeline(t, db, signe)
		is.Equal(t, p.ID, posts[0].ID)
		is.True(t, posts[0].New)
	})

	t.Run("refuses a limit below 1", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		createPost(t, db, signe, base)

		for _, limit := range []int{0, -1} {
			_, _, err := db.GetTimeline(t.Context(), signe.ID, nil, limit)
			is.True(t, err != nil, fmt.Sprint("limit ", limit))
		}
	})

	t.Run("keeps the marks of the first page on the pages after it", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		thomas := sqlitetest.CreateUser(t, db, "Thomas")
		createPost(t, db, thomas, base)
		createPost(t, db, thomas, base.Add(time.Minute))
		createPost(t, db, thomas, base.Add(2*time.Minute))

		page1, next, err := db.GetTimeline(t.Context(), signe.ID, nil, 1)
		is.NotError(t, err)
		is.True(t, page1[0].New)

		page2, next, err := db.GetTimeline(t.Context(), signe.ID, next, 1)
		is.NotError(t, err)
		is.True(t, page2[0].New)

		page3, _, err := db.GetTimeline(t.Context(), signe.ID, next, 1)
		is.NotError(t, err)
		is.True(t, page3[0].New)
	})

	t.Run("does not mark posts from before the reader joined as new", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		thomas := sqlitetest.CreateUser(t, db, "Thomas")
		p1 := createPost(t, db, thomas, base)
		amira := sqlitetest.CreateUser(t, db, "Amira")
		p2 := createPost(t, db, thomas, base.Add(time.Minute))

		posts := getTimeline(t, db, amira)
		is.EqualSlice(t, []model.PostID{p2.ID, p1.ID}, ids(posts))
		is.True(t, posts[0].New)
		is.True(t, !posts[1].New)
	})

	t.Run("counts comments by others since the reader last opened them as new", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		thomas := sqlitetest.CreateUser(t, db, "Thomas")
		p := createPost(t, db, signe, base)
		createComment(t, db, thomas, p, base.Add(time.Minute))
		createComment(t, db, signe, p, base.Add(2*time.Minute))
		createComment(t, db, thomas, p, base.Add(3*time.Minute))

		posts := getTimeline(t, db, signe)
		is.Equal(t, 3, posts[0].Comments)
		is.Equal(t, 2, posts[0].NewComments)

		getComments(t, db, signe, p)

		posts = getTimeline(t, db, signe)
		is.Equal(t, 0, posts[0].NewComments)

		createComment(t, db, thomas, p, base.Add(4*time.Minute))

		posts = getTimeline(t, db, signe)
		is.Equal(t, 4, posts[0].Comments)
		is.Equal(t, 1, posts[0].NewComments)
	})

	t.Run("counts new comments per post, so opening one post's comments leaves another's new", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		thomas := sqlitetest.CreateUser(t, db, "Thomas")
		p1 := createPost(t, db, thomas, base)
		p2 := createPost(t, db, thomas, base.Add(time.Minute))
		createComment(t, db, thomas, p1, base.Add(2*time.Minute))
		createComment(t, db, thomas, p2, base.Add(3*time.Minute))
		createComment(t, db, thomas, p2, base.Add(4*time.Minute))

		posts := getTimeline(t, db, signe)
		is.EqualSlice(t, []model.PostID{p2.ID, p1.ID}, ids(posts))
		is.Equal(t, 2, posts[0].NewComments)
		is.Equal(t, 1, posts[1].NewComments)

		getComments(t, db, signe, p1)

		posts = getTimeline(t, db, signe)
		is.Equal(t, 2, posts[0].NewComments)
		is.Equal(t, 0, posts[1].NewComments)
	})

	t.Run("keeps comments new until the post's comments are opened, however often the timeline is looked at", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		thomas := sqlitetest.CreateUser(t, db, "Thomas")
		p := createPost(t, db, signe, base)
		createComment(t, db, thomas, p, base.Add(time.Minute))

		getTimeline(t, db, signe)
		getTimeline(t, db, signe)

		posts := getTimeline(t, db, signe)
		is.Equal(t, 1, posts[0].NewComments)
	})

	t.Run("does not count comments from before the reader joined as new", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		thomas := sqlitetest.CreateUser(t, db, "Thomas")
		p := createPost(t, db, thomas, base)
		createComment(t, db, thomas, p, base.Add(time.Minute))
		amira := sqlitetest.CreateUser(t, db, "Amira")
		createComment(t, db, thomas, p, base.Add(2*time.Minute))

		posts := getTimeline(t, db, amira)
		is.Equal(t, 2, posts[0].Comments)
		is.Equal(t, 1, posts[0].NewComments)
	})

	t.Run("keeps one reader's marks out of what another reader sees", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		thomas := sqlitetest.CreateUser(t, db, "Thomas")
		amira := sqlitetest.CreateUser(t, db, "Amira")
		p := createPost(t, db, amira, base)
		createComment(t, db, amira, p, base.Add(time.Minute))

		// Thomas looks at the timeline and opens the comments, which must leave Signe's marks alone.
		getTimeline(t, db, thomas)
		getComments(t, db, thomas, p)

		posts := getTimeline(t, db, signe)
		is.True(t, posts[0].New)
		is.Equal(t, 1, posts[0].NewComments)

		posts = getTimeline(t, db, thomas)
		is.True(t, !posts[0].New)
		is.Equal(t, 0, posts[0].NewComments)
	})

	t.Run("walks every post exactly once, in order, across equal times and posts added between pages", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")

		hegel.Test(t, func(ht *hegel.T) {
			if err := db.H.Exec(t.Context(), `delete from posts`); err != nil {
				ht.Fatal(err)
			}

			// Few distinct times, so that many posts share one.
			at := hegel.Map(hegel.Integers(0, 4), func(m int) time.Time { return base.Add(time.Duration(m) * time.Minute) })

			// stored is every post in the order it was stored in, which breaks ties between equal times.
			type storedPost struct {
				id      model.PostID
				created time.Time
			}
			var stored []storedPost
			store := func(created time.Time) storedPost {
				p, err := db.CreatePost(t.Context(), model.Post{Created: model.Time{T: created}, AuthorID: signe.ID, Body: "Hi"}, "")
				if err != nil {
					ht.Fatal(err)
				}
				sp := storedPost{id: p.ID, created: created}
				stored = append(stored, sp)
				return sp
			}

			for _, created := range hegel.Draw(ht, hegel.Lists(at).MaxSize(20)) {
				store(created)
			}
			limit := hegel.Draw(ht, hegel.Integers(1, 6))

			// expected holds the posts a walk must yield. A post added between pages is yielded only if it
			// sorts after the last post shown, which for an equal time it never does, since it was stored later.
			expected := slices.Clone(stored)
			var walked []model.PostID
			var cursor *model.TimelineCursor
			for page := 0; ; page++ {
				if page > len(stored)+1 {
					ht.Fatalf("walked more pages than there are posts")
				}

				posts, next, err := db.GetTimeline(t.Context(), signe.ID, cursor, limit)
				if err != nil {
					ht.Fatal(err)
				}
				if len(posts) > limit {
					ht.Fatalf("got %v posts with limit %v", len(posts), limit)
				}
				walked = append(walked, ids(posts)...)
				if next == nil {
					break
				}
				if len(posts) == 0 {
					ht.Fatalf("got an empty page with a next cursor")
				}
				cursor = next

				lastShown := posts[len(posts)-1].Created.T
				for _, created := range hegel.Draw(ht, hegel.Lists(at).MaxSize(3)) {
					sp := store(created)
					if created.Before(lastShown) {
						expected = append(expected, sp)
					}
				}
			}

			// Newest first, and the later stored first among equal times.
			order := map[model.PostID]int{}
			for i, sp := range stored {
				order[sp.id] = i
			}
			slices.SortFunc(expected, func(a, b storedPost) int {
				if c := b.created.Compare(a.created); c != 0 {
					return c
				}
				return order[b.id] - order[a.id]
			})
			var expectedIDs []model.PostID
			for _, sp := range expected {
				expectedIDs = append(expectedIDs, sp.id)
			}

			if !slices.Equal(expectedIDs, walked) {
				ht.Fatalf("walked %v, expected %v", walked, expectedIDs)
			}
		})
	})
}

func TestDatabase_GetComments(t *testing.T) {
	t.Run("gets the comments of a post oldest first, with the author's name", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		thomas := sqlitetest.CreateUser(t, db, "Thomas")
		p := createPost(t, db, signe, base)
		other := createPost(t, db, signe, base)
		c2 := createComment(t, db, thomas, p, base.Add(2*time.Minute))
		c1 := createComment(t, db, signe, p, base.Add(time.Minute))
		c3 := createComment(t, db, signe, p, base.Add(2*time.Minute))
		createComment(t, db, signe, other, base.Add(time.Minute))

		comments := getComments(t, db, signe, p)
		is.EqualSlice(t, []model.CommentID{c1.ID, c2.ID, c3.ID}, commentIDs(comments))
		is.Equal(t, "Signe", comments[0].AuthorName)
		is.Equal(t, "Thomas", comments[1].AuthorName)
		is.Equal(t, p.ID, comments[1].PostID)
		is.Equal(t, c2.Body, comments[1].Body)
	})

	t.Run("marks comments by others as new until opened, and never the reader's own", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		thomas := sqlitetest.CreateUser(t, db, "Thomas")
		p := createPost(t, db, signe, base)
		createComment(t, db, thomas, p, base.Add(time.Minute))
		createComment(t, db, signe, p, base.Add(2*time.Minute))

		comments := getComments(t, db, signe, p)
		is.True(t, comments[0].New)
		is.True(t, !comments[1].New)

		createComment(t, db, thomas, p, base.Add(3*time.Minute))

		comments = getComments(t, db, signe, p)
		is.True(t, !comments[0].New)
		is.True(t, !comments[1].New)
		is.True(t, comments[2].New)

		comments = getComments(t, db, signe, p)
		is.True(t, !comments[2].New)
	})

	t.Run("does not mark comments from before the reader joined as new", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		thomas := sqlitetest.CreateUser(t, db, "Thomas")
		p := createPost(t, db, thomas, base)
		createComment(t, db, thomas, p, base.Add(time.Minute))
		amira := sqlitetest.CreateUser(t, db, "Amira")
		createComment(t, db, thomas, p, base.Add(2*time.Minute))

		comments := getComments(t, db, amira, p)
		is.True(t, !comments[0].New)
		is.True(t, comments[1].New)
	})

	t.Run("opening one post's comments leaves another post's comments new", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		thomas := sqlitetest.CreateUser(t, db, "Thomas")
		p1 := createPost(t, db, thomas, base)
		p2 := createPost(t, db, thomas, base)
		createComment(t, db, thomas, p1, base)
		createComment(t, db, thomas, p2, base)

		getComments(t, db, signe, p1)

		comments := getComments(t, db, signe, p2)
		is.True(t, comments[0].New)
	})

	t.Run("gets no comments for a post without any", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		p := createPost(t, db, signe, base)

		is.Equal(t, 0, len(getComments(t, db, signe, p)))
	})

	t.Run("refuses a post that does not exist", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")

		_, err := db.GetComments(t.Context(), signe.ID, "p_nope")
		is.Error(t, model.ErrorPostNotFound, err)
	})

	t.Run("refuses a reader that does not exist", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		p := createPost(t, db, signe, base)

		_, err := db.GetComments(t.Context(), "u_nope", p.ID)
		is.Error(t, model.ErrorUserNotFound, err)
	})
}

func TestTimelineMigration(t *testing.T) {
	t.Run("gives people from before the migration a read state, and migrates down and up again", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		migrations := os.DirFS("migrations")

		is.NotError(t, migrate.To(t.Context(), db.H.DB.DB, migrations, "1755714454-auth"))
		before := sqlitetest.CreateUser(t, db, "Before")
		is.NotError(t, migrate.To(t.Context(), db.H.DB.DB, migrations, "1795703426-timeline"))
		is.NotError(t, migrate.Up(t.Context(), db.H.DB.DB, migrations))

		thomas := sqlitetest.CreateUser(t, db, "Thomas")
		createPost(t, db, thomas, base)

		posts := getTimeline(t, db, before)
		is.True(t, posts[0].New)
	})
}

func TestFormTokenMigration(t *testing.T) {
	t.Run("keeps the posts and comments from before it, and migrates down and up again", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		migrations := os.DirFS("migrations")
		is.NotError(t, migrate.To(t.Context(), db.H.DB.DB, migrations, "1795703426-timeline"))
		signe := sqlitetest.CreateUser(t, db, "Signe")
		_, err := db.H.DB.ExecContext(t.Context(), `insert into sequences (name, value) values ('posts', 1) on conflict do update set value = 1`)
		is.NotError(t, err)
		_, err = db.H.DB.ExecContext(t.Context(), `insert into posts (id, seq, created, author_id, body) values ('p_before', 1, '2026-11-26T09:00:00.000Z', ?, 'Before')`, signe.ID)
		is.NotError(t, err)
		_, err = db.H.DB.ExecContext(t.Context(), `insert into comments (id, seq, created, post_id, author_id, body) values ('c_before', 1, '2026-11-26T09:00:00.000Z', 'p_before', ?, 'Before too')`, signe.ID)
		is.NotError(t, err)

		// Both stay through the migration, down again, and up again.
		for _, to := range []func() error{
			func() error { return migrate.Up(t.Context(), db.H.DB.DB, migrations) },
			func() error { return migrate.To(t.Context(), db.H.DB.DB, migrations, "1795703426-timeline") },
			func() error { return migrate.Up(t.Context(), db.H.DB.DB, migrations) },
		} {
			is.NotError(t, to())
			is.Equal(t, 1, count(t, db, "posts where id = 'p_before' and body = 'Before'"))
			is.Equal(t, 1, count(t, db, "comments where id = 'c_before' and body = 'Before too'"))
		}

		_, err = db.CreatePost(t.Context(), model.Post{Created: model.Time{T: base}, AuthorID: signe.ID, Body: "After"}, "q6urq6urq6urq6urq6urqw")
		is.NotError(t, err)
	})

	t.Run("refuses a second post or comment by the same author with the same token, and an empty token, in the schema itself", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		insertPost := `insert into posts (seq, created, author_id, body, form_token) values (?, '2026-11-26T09:00:00.000Z', ?, 'Cake', ?)`
		insertComment := `insert into comments (seq, created, post_id, author_id, body, form_token) values (?, '2026-11-26T09:00:00.000Z', 'p_1', ?, 'Yes', ?)`

		_, err := db.H.DB.ExecContext(t.Context(), `insert into posts (id, seq, created, author_id, body) values ('p_1', 100, '2026-11-26T09:00:00.000Z', ?, 'Cake')`, signe.ID)
		is.NotError(t, err)
		for _, insert := range []string{insertPost, insertComment} {
			_, err = db.H.DB.ExecContext(t.Context(), insert, 1, signe.ID, "q6urq6urq6urq6urq6urqw")
			is.NotError(t, err)
			_, err = db.H.DB.ExecContext(t.Context(), insert, 2, signe.ID, "q6urq6urq6urq6urq6urqw")
			is.True(t, err != nil && strings.Contains(err.Error(), "UNIQUE"), fmt.Sprint(err))
			_, err = db.H.DB.ExecContext(t.Context(), insert, 3, signe.ID, "")
			is.True(t, err != nil && strings.Contains(err.Error(), "CHECK"), fmt.Sprint(err))
		}
	})
}

// submitted is what makes a submission of a form the same submission: who sends it, and its token.
type submitted struct {
	author int
	token  model.FormToken
}

// submission by one of the given number of people, with one of a few tokens, so that the same one comes
// up again often. Sometimes it has no token, which makes every send of it a submission of its own.
func submission(people int) hegel.Generator[submitted] {
	return hegel.Composite(func(tc hegel.TestCase) submitted {
		return submitted{
			author: hegel.Draw(tc, hegel.Integers(0, people-1)),
			token:  hegel.Draw(tc, hegel.SampledFrom([]model.FormToken{"q6urq6urq6urq6urq6urqw", "AAAAAAAAAAAAAAAAAAAAAA", "-_-_-_-_-_-_-_-_-_-_-w", ""})),
		}
	})
}

// count the rows of the given table.
func count(t testing.TB, db *sqlite.Database, table string) int {
	t.Helper()

	var n int
	is.NotError(t, db.H.Get(t.Context(), &n, "select count(*) from "+table))
	return n
}

func createPost(t *testing.T, db *sqlite.Database, author model.User, created time.Time) model.Post {
	t.Helper()

	p, err := db.CreatePost(t.Context(), model.Post{Created: model.Time{T: created}, AuthorID: author.ID, Body: fmt.Sprint("A post by ", author.Name)}, "")
	is.NotError(t, err)
	return p
}

func createComment(t *testing.T, db *sqlite.Database, author model.User, p model.Post, created time.Time) model.Comment {
	t.Helper()

	c, err := db.CreateComment(t.Context(), model.Comment{Created: model.Time{T: created}, PostID: p.ID, AuthorID: author.ID, Body: fmt.Sprint("A comment by ", author.Name)}, "")
	is.NotError(t, err)
	return c
}

func getTimeline(t *testing.T, db *sqlite.Database, reader model.User) []model.TimelinePost {
	t.Helper()

	posts, _, err := db.GetTimeline(t.Context(), reader.ID, nil, 100)
	is.NotError(t, err)
	return posts
}

func getComments(t *testing.T, db *sqlite.Database, reader model.User, p model.Post) []model.TimelineComment {
	t.Helper()

	comments, err := db.GetComments(t.Context(), reader.ID, p.ID)
	is.NotError(t, err)
	return comments
}

func ids(posts []model.TimelinePost) []model.PostID {
	var ids []model.PostID
	for _, p := range posts {
		ids = append(ids, p.ID)
	}
	return ids
}

func commentIDs(comments []model.TimelineComment) []model.CommentID {
	var ids []model.CommentID
	for _, c := range comments {
		ids = append(ids, c.ID)
	}
	return ids
}
