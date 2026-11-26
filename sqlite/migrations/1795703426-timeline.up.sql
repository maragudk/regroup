-- Posts and comments take their times from the app, not from SQL defaults, so that tests and generated
-- histories can replay them with a clock of their own.
--
-- seq is the order rows were stored in. It breaks ties between equal times on the timeline, and the
-- read state below is kept as the highest seq a reader has seen, so that what is new does not depend on
-- two clocks agreeing to the millisecond.

create table posts (
  id text primary key default ('p_' || lower(hex(randomblob(16)))),
  seq integer not null unique check ( seq > 0 ),
  created text not null,
  author_id text not null references users (id),
  body text not null
) strict;

create index posts_created_seq_idx on posts (created, seq);
create index posts_author_id_idx on posts (author_id);

create table comments (
  id text primary key default ('c_' || lower(hex(randomblob(16)))),
  seq integer not null unique check ( seq > 0 ),
  created text not null,
  post_id text not null references posts (id),
  author_id text not null references users (id),
  body text not null
) strict;

create index comments_post_id_created_seq_idx on comments (post_id, created, seq);
create index comments_author_id_idx on comments (author_id);

-- timeline_reads holds one person's private read state of the timeline:
-- - joined_comment_seq: the highest comment seq when the person joined. No comment up to it is new to them.
-- - looked_post_seq: the highest post seq at their latest look at the timeline.
-- - basis_post_seq: the highest post seq at the look before that, which marks on the latest look are computed against.
create table timeline_reads (
  user_id text primary key references users (id) on delete cascade,
  joined_comment_seq integer not null,
  looked_post_seq integer not null,
  basis_post_seq integer not null
) strict;

-- comment_reads holds the highest comment seq of a post at the person's latest opening of its comments.
create table comment_reads (
  user_id text not null references users (id) on delete cascade,
  post_id text not null references posts (id) on delete cascade,
  seen_comment_seq integer not null,
  primary key (user_id, post_id)
) strict;

create index comment_reads_post_id_idx on comment_reads (post_id);

-- Nothing from before a person joined is new to them, so their read state starts at what exists then.
create trigger users_timeline_reads after insert on users begin
  insert into timeline_reads (user_id, joined_comment_seq, looked_post_seq, basis_post_seq)
  values (
    new.id,
    (select coalesce(max(seq), 0) from comments),
    (select coalesce(max(seq), 0) from posts),
    (select coalesce(max(seq), 0) from posts)
  );
end;

insert into timeline_reads (user_id, joined_comment_seq, looked_post_seq, basis_post_seq)
select id, 0, 0, 0 from users;
