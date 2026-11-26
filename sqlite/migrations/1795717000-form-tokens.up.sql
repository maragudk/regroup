-- form_token is the token of the form a post or a comment was sent from, if it was sent from one. Each
-- author sends a token once: sending it again gets what was made the first time, so that trying again
-- after an answer that was lost does not make the same thing twice. Posts and comments that were not sent
-- from a form, such as those of the preview's seed, have none.

alter table posts add column form_token text;
create unique index posts_author_id_form_token_idx on posts (author_id, form_token);

alter table comments add column form_token text;
create unique index comments_author_id_form_token_idx on comments (author_id, form_token);
