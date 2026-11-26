package sqlite

import (
	"context"
	stdsql "database/sql"
	"errors"

	"maragu.dev/glue/sql"

	"app/model"
)

// CreatePost with the time posted, the author, and the body as given.
// The post is stored after every post before it, which breaks ties between equal times on the timeline.
func (d *Database) CreatePost(ctx context.Context, p model.Post) (model.Post, error) {
	var created model.Post
	err := d.H.InTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		seq, err := nextSeq(ctx, tx, "posts")
		if err != nil {
			return err
		}

		query := `
			insert into posts (seq, created, author_id, body)
			values (?, ?, ?, ?)
			returning id, created, author_id, body`
		return tx.Get(ctx, &created, query, seq, p.Created, p.AuthorID, p.Body)
	})
	if err != nil {
		return model.Post{}, err
	}
	return created, nil
}

// CreateComment with the time, the post, the author, and the body as given.
// Returns [model.ErrorPostNotFound] if there is no such post.
func (d *Database) CreateComment(ctx context.Context, c model.Comment) (model.Comment, error) {
	var created model.Comment
	err := d.H.InTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if err := postExists(ctx, tx, c.PostID); err != nil {
			return err
		}

		seq, err := nextSeq(ctx, tx, "comments")
		if err != nil {
			return err
		}

		query := `
			insert into comments (seq, created, post_id, author_id, body)
			values (?, ?, ?, ?, ?)
			returning id, created, post_id, author_id, body`
		return tx.Get(ctx, &created, query, seq, c.Created, c.PostID, c.AuthorID, c.Body)
	})
	if err != nil {
		return model.Comment{}, err
	}
	return created, nil
}

// timelinePostRow is a [model.TimelinePost] with its place in the order posts were stored in.
type timelinePostRow struct {
	model.TimelinePost
	Seq int64
}

// GetTimeline page for the given reader: at most limit posts after the given cursor, newest first,
// and the cursor for the next page, which is nil if there is none. The limit must be at least 1.
//
// Each post is marked with what is new to this reader and nobody else. A post is new if someone else
// posted it since the reader's previous look at the timeline, and after the reader joined.
//
// A nil cursor is the first page, which is a look at the whole timeline: its marks are computed against
// the reader's previous look, and it then becomes the reader's latest look. A page after the first is
// marked against the look before the reader's latest look, so it keeps the marks of its first page as
// long as the reader has not looked again since.
//
// Returns [model.ErrorUserNotFound] if there is no such reader.
func (d *Database) GetTimeline(ctx context.Context, reader model.UserID, after *model.TimelineCursor, limit int) ([]model.TimelinePost, *model.TimelineCursor, error) {
	if limit < 1 {
		return nil, nil, errors.New("limit must be at least 1")
	}

	var rows []timelinePostRow
	get := func(ctx context.Context, q querier) error {
		query := `select basis_post_seq from timeline_reads where user_id = ?`
		if after == nil {
			query = `
				update timeline_reads
				set basis_post_seq = looked_post_seq, looked_post_seq = (select value from sequences where name = 'posts')
				where user_id = ?
				returning basis_post_seq`
		}

		var basis int64
		if err := q.Get(ctx, &basis, query, reader); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return model.ErrorUserNotFound
			}
			return err
		}

		args := []any{
			stdsql.Named("reader", reader),
			stdsql.Named("basis", basis),
			stdsql.Named("limit", limit+1),
		}
		var where string
		if after != nil {
			where = `where (p.created, p.seq) < (:after_created, :after_seq)`
			args = append(args, stdsql.Named("after_created", after.Created), stdsql.Named("after_seq", after.Seq))
		}

		query = `
			with reader as (
				select * from timeline_reads where user_id = :reader
			)
			select
				p.id,
				p.created,
				p.author_id,
				p.body,
				p.seq,
				u.name as author_name,
				(select count(*) from comments c where c.post_id = p.id) as comments,
				p.author_id != :reader and p.seq > :basis as "new",
				(
					select count(*)
					from comments c
					where c.post_id = p.id and c.author_id != :reader and c.seq > max(
						(select joined_comment_seq from reader),
						coalesce((select seen_comment_seq from comment_reads cr where cr.user_id = :reader and cr.post_id = p.id), 0)
					)
				) as new_comments
			from posts p
				join users u on u.id = p.author_id
			` + where + `
			order by p.created desc, p.seq desc
			limit :limit`

		return q.Select(ctx, &rows, query, args...)
	}

	// Only a look writes, so the pages after the first read without taking the write lock.
	var err error
	if after == nil {
		err = d.H.InTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
			return get(ctx, tx)
		})
	} else {
		err = get(ctx, d.H)
	}
	if err != nil {
		return nil, nil, err
	}

	var next *model.TimelineCursor
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		next = &model.TimelineCursor{Created: last.Created, Seq: last.Seq}
	}

	posts := make([]model.TimelinePost, 0, len(rows))
	for _, r := range rows {
		posts = append(posts, r.TimelinePost)
	}
	return posts, next, nil
}

// GetComments of the given post for the given reader, oldest first, and record that the reader opened them.
//
// Each comment is marked with whether it was new to this reader and nobody else: someone else wrote it,
// after the reader joined, and since the reader last opened the post's comments.
//
// Returns [model.ErrorPostNotFound] if there is no such post, and [model.ErrorUserNotFound] if there is
// no such reader.
func (d *Database) GetComments(ctx context.Context, reader model.UserID, postID model.PostID) ([]model.TimelineComment, error) {
	var comments []model.TimelineComment
	err := d.H.InTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if err := postExists(ctx, tx, postID); err != nil {
			return err
		}

		var joined int64
		if err := tx.Get(ctx, &joined, `select joined_comment_seq from timeline_reads where user_id = ?`, reader); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return model.ErrorUserNotFound
			}
			return err
		}

		query := `
			select
				c.id,
				c.created,
				c.post_id,
				c.author_id,
				c.body,
				u.name as author_name,
				c.author_id != :reader and c.seq > max(
					:joined,
					coalesce((select seen_comment_seq from comment_reads where user_id = :reader and post_id = :post), 0)
				) as "new"
			from comments c
				join users u on u.id = c.author_id
			where c.post_id = :post
			order by c.created, c.seq`
		if err := tx.Select(ctx, &comments, query, stdsql.Named("reader", reader), stdsql.Named("joined", joined), stdsql.Named("post", postID)); err != nil {
			return err
		}

		query = `
			insert into comment_reads (user_id, post_id, seen_comment_seq)
			values (:reader, :post, (select value from sequences where name = 'comments'))
			on conflict (user_id, post_id) do update set seen_comment_seq = excluded.seen_comment_seq`
		return tx.Exec(ctx, query, stdsql.Named("reader", reader), stdsql.Named("post", postID))
	})
	if err != nil {
		return nil, err
	}
	return comments, nil
}

// querier is what both [sql.Helper] and [sql.Tx] query with.
type querier interface {
	Get(ctx context.Context, dest any, query string, args ...any) error
	Select(ctx context.Context, dest any, query string, args ...any) error
}

// nextSeq of the named sequence, counting it up by one.
func nextSeq(ctx context.Context, tx *sql.Tx, name string) (int64, error) {
	var seq int64
	if err := tx.Get(ctx, &seq, `update sequences set value = value + 1 where name = ? returning value`, name); err != nil {
		return 0, err
	}
	return seq, nil
}

// postExists or return [model.ErrorPostNotFound].
func postExists(ctx context.Context, tx *sql.Tx, id model.PostID) error {
	var exists bool
	if err := tx.Get(ctx, &exists, `select exists (select 1 from posts where id = ?)`, id); err != nil {
		return err
	}
	if !exists {
		return model.ErrorPostNotFound
	}
	return nil
}
