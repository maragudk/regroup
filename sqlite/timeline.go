package sqlite

import (
	"context"
	stdsql "database/sql"
	"errors"

	"maragu.dev/glue/sql"

	"app/model"
)

// CreatePost with the time posted, the author, and the body it is given, which are not checked here.
// The post is stored after every post before it, which breaks ties between equal times on the timeline.
func (d *Database) CreatePost(ctx context.Context, p model.Post) (model.Post, error) {
	query := `
		insert into posts (seq, created, author_id, body)
		values ((select coalesce(max(seq), 0) + 1 from posts), ?, ?, ?)
		returning id, created, author_id, body`

	var created model.Post
	if err := d.H.Get(ctx, &created, query, p.Created, p.AuthorID, p.Body); err != nil {
		return model.Post{}, err
	}
	return created, nil
}

// CreateComment with the time, the post, the author, and the body it is given, which are not checked here.
// Returns [model.ErrorPostNotFound] if there is no such post.
func (d *Database) CreateComment(ctx context.Context, c model.Comment) (model.Comment, error) {
	var created model.Comment
	err := d.H.InTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if err := postExists(ctx, tx, c.PostID); err != nil {
			return err
		}

		query := `
			insert into comments (seq, created, post_id, author_id, body)
			values ((select coalesce(max(seq), 0) + 1 from comments), ?, ?, ?, ?)
			returning id, created, post_id, author_id, body`
		return tx.Get(ctx, &created, query, c.Created, c.PostID, c.AuthorID, c.Body)
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
// and the cursor for the next page, which is nil if there is none.
//
// Each post is marked with what is new to this reader and nobody else. A post is new if someone else
// posted it since the reader's previous look at the timeline. A nil cursor is the first page, which is a
// look: its marks are computed against the look before it, and it then becomes the reader's latest look.
// The pages after it keep the marks of that first page.
//
// Returns [model.ErrorUserNotFound] if there is no such reader.
func (d *Database) GetTimeline(ctx context.Context, reader model.UserID, after *model.TimelineCursor, limit int) ([]model.TimelinePost, *model.TimelineCursor, error) {
	var rows []timelinePostRow
	err := d.H.InTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		query := `select basis_post_seq from timeline_reads where user_id = ?`
		if after == nil {
			query = `
				update timeline_reads
				set basis_post_seq = looked_post_seq, looked_post_seq = (select coalesce(max(seq), 0) from posts)
				where user_id = ?
				returning basis_post_seq`
		}

		var basis int64
		if err := tx.Get(ctx, &basis, query, reader); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return model.ErrorUserNotFound
			}
			return err
		}

		var afterCreated model.Time
		var afterSeq int64
		if after != nil {
			afterCreated, afterSeq = after.Created, after.Seq
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
			where :after_seq = 0 or (p.created, p.seq) < (:after_created, :after_seq)
			order by p.created desc, p.seq desc
			limit :limit`

		return tx.Select(ctx, &rows, query,
			stdsql.Named("reader", reader),
			stdsql.Named("basis", basis),
			stdsql.Named("after_created", afterCreated),
			stdsql.Named("after_seq", afterSeq),
			stdsql.Named("limit", limit+1),
		)
	})
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
			values (:reader, :post, (select coalesce(max(seq), 0) from comments where post_id = :post))
			on conflict (user_id, post_id) do update set seen_comment_seq = excluded.seen_comment_seq`
		return tx.Exec(ctx, query, stdsql.Named("reader", reader), stdsql.Named("post", postID))
	})
	if err != nil {
		return nil, err
	}
	return comments, nil
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
