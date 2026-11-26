drop table invitations;
drop table sign_in_links;

create table tokens (
  value text primary key,
  created text not null default (strftime('%Y-%m-%dT%H:%M:%fZ')),
  updated text not null default (strftime('%Y-%m-%dT%H:%M:%fZ')),
  user_id text not null references users (id) on delete cascade,
  used int not null default 0 check ( used in (0, 1) ),
  expires text not null default (strftime('%Y-%m-%dT%H:%M:%fZ', 'now', '7 days'))
) strict;

create trigger tokens_updated_timestamp after update on tokens begin
  update tokens set updated = strftime('%Y-%m-%dT%H:%M:%fZ') where value = old.value;
end;

create table roles (
  role text primary key
) strict;

insert into roles (role) values ('admin');

create table users_roles (
  user_id text not null references users (id) on delete cascade,
  role text not null references roles (role) on delete cascade,
  primary key (user_id, role)
) strict;

create index users_roles_role_idx on users_roles (role);

create table permissions (
  permission text primary key
) strict;

create table roles_permissions (
  role text not null references roles (role) on delete cascade,
  permission text not null references permissions (permission) on delete cascade,
  primary key (role, permission)
) strict;

create index roles_permissions_permission_idx on roles_permissions (permission);

insert into users_roles (user_id, role) select id, 'admin' from users where role = 'admin';

alter table users drop column invited_by;
alter table users drop column role;
