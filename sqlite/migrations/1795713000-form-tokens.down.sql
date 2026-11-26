create index comments_author_id_idx on comments (author_id);
drop index comments_author_id_form_token_idx;
alter table comments drop column form_token;

create index posts_author_id_idx on posts (author_id);
drop index posts_author_id_form_token_idx;
alter table posts drop column form_token;
