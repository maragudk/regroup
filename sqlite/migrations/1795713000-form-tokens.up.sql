-- form_token is the token of the form a post or a comment was sent from, if it was sent from one. Each
-- author sends a token once: sending it again gets what was made the first time, so that trying again
-- after an answer that was lost does not make the same thing twice. Posts and comments that were not sent
-- from a form have none. No token is empty.
--
-- The unique indexes on (author_id, form_token) also serve lookups by author alone, so the indexes on
-- author_id go.

alter table posts add column form_token text check (form_token != '');
create unique index posts_author_id_form_token_idx on posts (author_id, form_token);
drop index posts_author_id_idx;

alter table comments add column form_token text check (form_token != '');
create unique index comments_author_id_form_token_idx on comments (author_id, form_token);
drop index comments_author_id_idx;
