-- People: one role each, who invited whom, sign-in links, and invitations that are waiting.
--
-- The template's roles, users_roles, permissions, and roles_permissions tables go, since every person has
-- exactly one role and the rights of each role are stated in the code. Its tokens table goes too, since
-- it kept tokens themselves and needed a person to exist; sign-in links and invitations keep only the
-- hash of their token. Times in the new tables come from the inserting code, not from SQL defaults.

alter table users add column role text not null default 'member' check ( role in ('admin', 'moderator', 'member') );

-- invited_by is who invited the person. It is null for the first admin, who was named in the
-- configuration, and for people who were there before invitations.
alter table users add column invited_by text references users (id);

update users set role = 'admin' where id in (select user_id from users_roles where role = 'admin');

-- Addresses are stored as they are compared: trimmed and lowercased.
update users set email = lower(trim(email));

drop table roles_permissions;
drop table permissions;
drop table users_roles;
drop table roles;
drop table tokens;

-- sign_in_links holds every sign-in link sent in the last day, by the hash of its token. A link belongs
-- to an address, not to a person, so that the first admin can have one before they exist. used is when
-- it was used up, by signing in with it or with another link to the same address.
create table sign_in_links (
  hash text primary key,
  created text not null,
  expires text not null,
  email text not null,
  used text
) strict;

create index sign_in_links_email_created_idx on sign_in_links (email, created);

-- invitations holds the invitations that are waiting, one for each address at most: inviting an address
-- again replaces its invitation, and accepting or withdrawing one deletes it. Who invited whom is kept in
-- users.invited_by once it is accepted. hash is null until the invitation's email is sent, which is when
-- its token is made.
create table invitations (
  id text primary key default ('i_' || lower(hex(randomblob(16)))),
  created text not null,
  expires text not null,
  email text unique not null,
  invited_by text not null references users (id) on delete cascade,
  hash text unique
) strict;

create index invitations_invited_by_idx on invitations (invited_by);
