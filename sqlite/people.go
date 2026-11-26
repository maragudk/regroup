package sqlite

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"time"

	"maragu.dev/glue/jobs"
	"maragu.dev/glue/sql"

	"app/model"
)

func userByEmail(ctx context.Context, q querier, u *model.User, email model.EmailAddress) error {
	if err := q.Get(ctx, u, `select * from users where email = ?`, email); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.ErrorUserNotFound
		}
		return err
	}
	return nil
}

// ListMembers of the group, who are the active users, in the order they joined.
func (d *Database) ListMembers(ctx context.Context) ([]model.User, error) {
	var users []model.User
	if err := d.H.Select(ctx, &users, `select * from users where active = 1 order by rowid`); err != nil {
		return nil, err
	}
	return users, nil
}

// CreateJob with the given name and data, encoded as JSON, on the queue of regular jobs.
func (d *Database) CreateJob(ctx context.Context, name model.JobName, data any) error {
	body, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return jobs.Create(ctx, d.H.JobsQ, name.String(), jobs.Message{Body: body})
}

// CreateSignInLink for the address it names, unless at least the given number of links were made for the
// address since the given time, in which case it stores nothing and returns false. Links made more than a
// day before the link's creation are deleted, since they can no longer count towards the limit.
func (d *Database) CreateSignInLink(ctx context.Context, l model.SignInLink, limit int, since model.Time) (bool, error) {
	var created bool
	err := d.H.InTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		dayBefore := model.Time{T: l.Created.T.Add(-24 * time.Hour)}
		if err := tx.Exec(ctx, `delete from sign_in_links where created < ?`, dayBefore); err != nil {
			return err
		}

		var count int
		if err := tx.Get(ctx, &count, `select count(*) from sign_in_links where email = ? and created > ?`, l.Email, since); err != nil {
			return err
		}
		if count >= limit {
			return nil
		}

		query := `insert into sign_in_links (hash, created, expires, email) values (?, ?, ?, ?)`
		if err := tx.Exec(ctx, query, l.Hash, l.Created, l.Expires, l.Email); err != nil {
			return err
		}
		created = true
		return nil
	})
	return created, err
}

// DeleteSignInLink with the given hash, if there is one.
func (d *Database) DeleteSignInLink(ctx context.Context, hash string) error {
	return d.H.Exec(ctx, `delete from sign_in_links where hash = ?`, hash)
}

// CheckSignInLink with the given hash, at the given time: what signing in with it would do. A link works
// if it is not used up and has not expired, and the address it was sent to is one of these:
//   - the address of an active person, who it signs in, or
//   - the given address of the first admin, while the group has nobody in it, active or not. Signing in
//     then makes them join the group, so [model.SignInLinkCheck.Joining] is true.
//
// Anything else, including a link that was never made, is [model.ErrorLinkInvalid].
func (d *Database) CheckSignInLink(ctx context.Context, hash string, now model.Time, firstAdmin model.EmailAddress) (model.SignInLinkCheck, error) {
	var l model.SignInLink
	if err := d.H.Get(ctx, &l, `select * from sign_in_links where hash = ? and used is null and expires > ?`, hash, now); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.SignInLinkCheck{}, model.ErrorLinkInvalid
		}
		return model.SignInLinkCheck{}, err
	}

	_, joining, err := signInTarget(ctx, d.H, l.Email, firstAdmin)
	if err != nil {
		return model.SignInLinkCheck{}, err
	}
	return model.SignInLinkCheck{Joining: joining}, nil
}

// SignIn with the link with the given hash, at the given time: by the rules of [Database.CheckSignInLink],
// sign in the active person the link was sent to, or have the first admin join the group as an active
// admin with the given name, which must be cleaned already and not be empty. Signing in uses up every
// link to the same address that is not used up yet.
//
// A link that does not work is [model.ErrorLinkInvalid]. A first admin without a name is
// [model.ErrorNameEmpty], and uses up nothing.
func (d *Database) SignIn(ctx context.Context, hash string, now model.Time, firstAdmin model.EmailAddress, name string) (model.User, error) {
	var u model.User
	err := d.H.InTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		var email model.EmailAddress
		query := `update sign_in_links set used = ? where hash = ? and used is null and expires > ? returning email`
		if err := tx.Get(ctx, &email, query, now, hash, now); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return model.ErrorLinkInvalid
			}
			return err
		}

		user, joining, err := signInTarget(ctx, tx, email, firstAdmin)
		if err != nil {
			return err
		}

		if joining {
			if name == "" {
				return model.ErrorNameEmpty
			}
			if err := insertUser(ctx, tx, &u, model.User{Created: now, Name: name, Email: email, Role: model.RoleAdmin}); err != nil {
				return err
			}
		} else {
			u = user
		}

		return tx.Exec(ctx, `update sign_in_links set used = ? where email = ? and used is null`, now, email)
	})
	if err != nil {
		return model.User{}, err
	}
	return u, nil
}

// GetSignInTarget is who a working sign-in link to the given address signs in, by the rules of
// [Database.CheckSignInLink]: the active person with the address, or nobody if the address is the given
// first admin's and the group has nobody in it, in which case joining is true. Anything else is
// [model.ErrorLinkInvalid].
func (d *Database) GetSignInTarget(ctx context.Context, email, firstAdmin model.EmailAddress) (model.User, bool, error) {
	return signInTarget(ctx, d.H, email, firstAdmin)
}

// signInTarget is who a working sign-in link to the given address signs in: the active person with the
// address, or nobody if the given first admin's address is the address and the group has nobody in it,
// in which case joining is true. Anything else is [model.ErrorLinkInvalid].
func signInTarget(ctx context.Context, tx querier, email, firstAdmin model.EmailAddress) (model.User, bool, error) {
	var u model.User
	err := userByEmail(ctx, tx, &u, email)
	switch {
	case err == nil && u.Active:
		return u, false, nil
	case err == nil:
		return model.User{}, false, model.ErrorLinkInvalid
	case !errors.Is(err, model.ErrorUserNotFound):
		return model.User{}, false, err
	}

	if firstAdmin == "" || email != firstAdmin {
		return model.User{}, false, model.ErrorLinkInvalid
	}

	var count int
	if err := tx.Get(ctx, &count, `select count(*) from users`); err != nil {
		return model.User{}, false, err
	}
	if count > 0 {
		return model.User{}, false, model.ErrorLinkInvalid
	}
	return model.User{}, true, nil
}

// insertUser into the group, active and confirmed, with an account of their own, with the time, name,
// address, role, and inviter of the given user, and put what was stored in into. They join the group now:
// nothing posted or commented before is new to them.
func insertUser(ctx context.Context, tx *sql.Tx, into *model.User, u model.User) error {
	var accountID model.AccountID
	query := `insert into accounts (created, updated, name) values (?, ?, ?) returning id`
	if err := tx.Get(ctx, &accountID, query, u.Created, u.Created, u.Name); err != nil {
		return err
	}

	query = `
		insert into users (created, updated, account_id, name, email, confirmed, active, role, invited_by)
		values (?, ?, ?, ?, ?, 1, 1, ?, ?)
		returning *`
	return tx.Get(ctx, into, query, u.Created, u.Created, accountID, u.Name, u.Email, u.Role, u.InvitedBy)
}

// CreateInvitation to the address it names, from the person it names, with the times it has, and create
// the job that sends it, in one transaction. It replaces the invitation that is waiting for the same
// address, if there is one, which then no longer works. Invitations that have expired by its time are
// deleted.
//
// It is sent from the form with the given token. If the person it names has made an invitation from a
// form with the same token before, CreateInvitation makes nothing: it returns the invitation that is
// waiting for the address now, or none if none is, if the address is the one invited then, and
// [model.ErrorFormResentChanged] if it is not.
//
// Returns [model.ErrorAlreadyMember] if the address is an active person's, and [model.ErrorFormerMember]
// if it is the address of a person who is no longer active.
func (d *Database) CreateInvitation(ctx context.Context, inv model.Invitation, token model.FormToken) (model.Invitation, error) {
	var created model.Invitation
	err := d.H.InTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		var sentHash string
		err := tx.Get(ctx, &sentHash, `select email_hash from invitation_form_tokens where invited_by = ? and form_token = ?`, inv.InvitedBy, token)
		switch {
		case err == nil && sentHash != emailHash(inv.Email):
			return model.ErrorFormResentChanged
		case err == nil:
			query := `select ` + invitationColumns + ` from invitations i join users u on u.id = i.invited_by where i.email = ? and i.expires > ?`
			if err := tx.Get(ctx, &created, query, inv.Email, inv.Created); err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			return nil
		case !errors.Is(err, sql.ErrNoRows):
			return err
		}

		var u model.User
		err = userByEmail(ctx, tx, &u, inv.Email)
		switch {
		case err == nil && u.Active:
			return model.ErrorAlreadyMember
		case err == nil:
			return model.ErrorFormerMember
		case !errors.Is(err, model.ErrorUserNotFound):
			return err
		}

		if err := tx.Exec(ctx, `delete from invitations where email = ? or expires <= ?`, inv.Email, inv.Created); err != nil {
			return err
		}

		query := `insert into invitation_form_tokens (invited_by, form_token, email_hash, created) values (?, ?, ?, ?)`
		if err := tx.Exec(ctx, query, inv.InvitedBy, token, emailHash(inv.Email), inv.Created); err != nil {
			return err
		}

		var id model.InvitationID
		query = `insert into invitations (created, expires, email, invited_by) values (?, ?, ?, ?) returning id`
		if err := tx.Get(ctx, &id, query, inv.Created, inv.Expires, inv.Email, inv.InvitedBy); err != nil {
			return err
		}

		if err := invitationByID(ctx, tx, &created, id); err != nil {
			return err
		}

		body, err := json.Marshal(model.SendInvitationJobData{InvitationID: id})
		if err != nil {
			return err
		}
		return jobs.CreateTx(ctx, tx.Tx.Tx, d.H.JobsQ, model.JobNameSendInvitation.String(), jobs.Message{Body: body})
	})
	if err != nil {
		return model.Invitation{}, err
	}
	return created, nil
}

// emailHash of the given address, which must be parsed already: SHA-256, in lowercase hexadecimal.
func emailHash(e model.EmailAddress) string {
	h := sha256.Sum256([]byte(e))
	return hex.EncodeToString(h[:])
}

// invitationColumns of an invitation, with the name of the person who sent it.
const invitationColumns = `i.id, i.created, i.expires, i.email, i.invited_by, u.name as invited_by_name`

func invitationByID(ctx context.Context, q querier, inv *model.Invitation, id model.InvitationID) error {
	query := `select ` + invitationColumns + ` from invitations i join users u on u.id = i.invited_by where i.id = ?`
	if err := q.Get(ctx, inv, query, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.ErrorInvitationNotFound
		}
		return err
	}
	return nil
}

// SetInvitationHash of the invitation with the given ID, which is the hash of the token of its link, if it
// is still waiting and has not expired at the given time. The token of a hash set before no longer works. Returns [model.ErrorInvitationNotFound] if there is no such invitation.
func (d *Database) SetInvitationHash(ctx context.Context, id model.InvitationID, hash string, now model.Time) (model.Invitation, error) {
	var inv model.Invitation
	err := d.H.InTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		var found model.InvitationID
		query := `update invitations set hash = ? where id = ? and expires > ? returning id`
		if err := tx.Get(ctx, &found, query, hash, id, now); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return model.ErrorInvitationNotFound
			}
			return err
		}
		return invitationByID(ctx, tx, &inv, id)
	})
	if err != nil {
		return model.Invitation{}, err
	}
	return inv, nil
}

// GetInvitationByHash of its link's token, if it is waiting and has not expired at the given time.
// Returns [model.ErrorLinkInvalid] if there is no such invitation.
func (d *Database) GetInvitationByHash(ctx context.Context, hash string, now model.Time) (model.Invitation, error) {
	var inv model.Invitation
	query := `select ` + invitationColumns + ` from invitations i join users u on u.id = i.invited_by where i.hash = ? and i.expires > ?`
	if err := d.H.Get(ctx, &inv, query, hash, now); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.Invitation{}, model.ErrorLinkInvalid
		}
		return model.Invitation{}, err
	}
	return inv, nil
}

// GetInvitations that are waiting and have not expired at the given time, the newest first.
func (d *Database) GetInvitations(ctx context.Context, now model.Time) ([]model.Invitation, error) {
	var invs []model.Invitation
	query := `select ` + invitationColumns + ` from invitations i join users u on u.id = i.invited_by where i.expires > ? order by i.created desc, i.rowid desc`
	if err := d.H.Select(ctx, &invs, query, now); err != nil {
		return nil, err
	}
	return invs, nil
}

// DeleteInvitation with the given ID, if it is waiting and has not expired at the given time, so that it no
// longer works. Returns [model.ErrorInvitationNotFound] if there is no such invitation.
func (d *Database) DeleteInvitation(ctx context.Context, id model.InvitationID, now model.Time) error {
	var found model.InvitationID
	if err := d.H.Get(ctx, &found, `delete from invitations where id = ? and expires > ? returning id`, id, now); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.ErrorInvitationNotFound
		}
		return err
	}
	return nil
}

// AcceptInvitation with the given hash of its link's token, at the given time, if it is waiting and has
// not expired: the invitation is used up, and a new active member joins the group with its address, the
// given name, which must be cleaned already, and the person who sent it as who invited them. They join
// now, so nothing posted or commented before is new to them.
//
// Returns [model.ErrorLinkInvalid] if there is no such invitation, and [model.ErrorAlreadyMember] if a
// person with the address exists after all, which uses up nothing.
func (d *Database) AcceptInvitation(ctx context.Context, hash string, now model.Time, name string) (model.User, model.Invitation, error) {
	var u model.User
	var inv model.Invitation
	err := d.H.InTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		query := `select ` + invitationColumns + ` from invitations i join users u on u.id = i.invited_by where i.hash = ? and i.expires > ?`
		if err := tx.Get(ctx, &inv, query, hash, now); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return model.ErrorLinkInvalid
			}
			return err
		}

		if err := tx.Exec(ctx, `delete from invitations where id = ?`, inv.ID); err != nil {
			return err
		}

		var existing model.User
		err := userByEmail(ctx, tx, &existing, inv.Email)
		switch {
		case err == nil:
			return model.ErrorAlreadyMember
		case !errors.Is(err, model.ErrorUserNotFound):
			return err
		}

		return insertUser(ctx, tx, &u, model.User{Created: now, Name: name, Email: inv.Email, Role: model.RoleMember, InvitedBy: &inv.InvitedBy})
	})
	if err != nil {
		return model.User{}, model.Invitation{}, err
	}
	return u, inv, nil
}

// ChangeRole of the target to the given role, as the actor, if the given check of the two, as they are
// in the same transaction, returns nil, and return the role the target had before. The target is nil if
// there is no such person. Returns what the check returns otherwise, [model.ErrorUserNotFound] if there
// is no such actor, and [model.ErrorMemberNotFound] if there is no such target and the check passes.
func (d *Database) ChangeRole(ctx context.Context, actor, target model.UserID, role model.Role, check func(actor model.User, target *model.User) error) (model.Role, error) {
	var before model.Role
	err := d.H.InTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		var a model.User
		if err := tx.Get(ctx, &a, `select * from users where id = ?`, actor); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return model.ErrorUserNotFound
			}
			return err
		}

		t := &model.User{}
		if err := tx.Get(ctx, t, `select * from users where id = ?`, target); err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			t = nil
		}

		if err := check(a, t); err != nil {
			return err
		}
		if t == nil {
			return model.ErrorMemberNotFound
		}

		before = t.Role
		return tx.Exec(ctx, `update users set role = ? where id = ?`, role, target)
	})
	if err != nil {
		return "", err
	}
	return before, nil
}
