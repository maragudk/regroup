drop index comments_author_id_form_token_idx;
alter table comments drop column form_token;

drop index posts_author_id_form_token_idx;
alter table posts drop column form_token;
