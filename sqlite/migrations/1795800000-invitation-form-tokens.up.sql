-- invitation_form_tokens holds the token of each form that an invitation was made from, with who sent it
-- and the SHA-256 of the address it invited, in lowercase hexadecimal. Each person sends a token once:
-- sending the same form again makes no second invitation, so that trying again after an answer that was
-- lost does not send a second email, nor replace the first invitation. Only the hash of the address is
-- kept, since a row outlives the invitation it made, which can be withdrawn or accepted.
create table invitation_form_tokens (
  invited_by text not null references users (id) on delete cascade,
  form_token text not null check (form_token != ''),
  email_hash text not null,
  created text not null,
  primary key (invited_by, form_token)
) strict;
