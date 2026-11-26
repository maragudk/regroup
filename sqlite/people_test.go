package sqlite_test

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"maragu.dev/is"
	"maragu.dev/migrate"

	"app/jobstest"
	"app/model"
	"app/sqlite"
	"app/sqlitetest"
)

// at the given number of minutes after [base].
func at(minutes int) model.Time {
	return model.Time{T: base.Add(time.Duration(minutes) * time.Minute)}
}

// createLink to the given address, made at the given number of minutes after [base], which works for
// 15 minutes, and return its hash.
func createLink(t *testing.T, db *sqlite.Database, email model.EmailAddress, minutes int, hash string) string {
	t.Helper()

	created, err := db.CreateSignInLink(t.Context(), model.SignInLink{Hash: hash, Created: at(minutes), Expires: at(minutes + 15), Email: email}, 5, at(minutes-60))
	is.NotError(t, err)
	is.True(t, created, "expected the link to be made")
	return hash
}

func TestDatabase_CreateSignInLink(t *testing.T) {
	t.Run("makes links up to the limit since the given time, and then none", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)

		for i := range 6 {
			created, err := db.CreateSignInLink(t.Context(), model.SignInLink{Hash: "h" + string(rune('a'+i)), Created: at(i), Expires: at(i + 15), Email: "signe@example.com"}, 5, at(i-60))
			is.NotError(t, err)
			is.Equal(t, i < 5, created)
		}
		is.Equal(t, 5, count(t, db, "sign_in_links"))
	})

	t.Run("counts only links to the same address, and only those made after the given time", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		for i := range 5 {
			createLink(t, db, "signe@example.com", i, "h"+string(rune('a'+i)))
		}

		created, err := db.CreateSignInLink(t.Context(), model.SignInLink{Hash: "other", Created: at(10), Expires: at(25), Email: "thomas@example.com"}, 5, at(-50))
		is.NotError(t, err)
		is.True(t, created)

		// An hour after the first, it no longer counts.
		created, err = db.CreateSignInLink(t.Context(), model.SignInLink{Hash: "later", Created: at(60), Expires: at(75), Email: "signe@example.com"}, 5, at(0))
		is.NotError(t, err)
		is.True(t, created)
	})

	t.Run("deletes links made more than a day before", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		createLink(t, db, "signe@example.com", 0, "old")
		createLink(t, db, "thomas@example.com", 24*60+1, "new")

		is.Equal(t, 0, count(t, db, "sign_in_links where hash = 'old'"))
		is.Equal(t, 1, count(t, db, "sign_in_links where hash = 'new'"))
	})
}

func TestDatabase_SignIn(t *testing.T) {
	t.Run("signs in the active person a link was sent to, once, and uses up their other links but not anyone else's", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreatePerson(t, db, "Signe", "signe@example.com", model.RoleMember)
		sqlitetest.CreatePerson(t, db, "Thomas", "thomas@example.com", model.RoleMember)
		first := createLink(t, db, "signe@example.com", 0, "first")
		second := createLink(t, db, "signe@example.com", 1, "second")
		thomas := createLink(t, db, "thomas@example.com", 1, "thomas")

		check, err := db.CheckSignInLink(t.Context(), second, at(2), "")
		is.NotError(t, err)
		is.True(t, !check.Joining)

		u, err := db.SignIn(t.Context(), second, at(2), "", "")
		is.NotError(t, err)
		is.Equal(t, signe.ID, u.ID)

		for _, hash := range []string{first, second} {
			_, err = db.CheckSignInLink(t.Context(), hash, at(3), "")
			is.Error(t, model.ErrorLinkInvalid, err, hash)
			_, err = db.SignIn(t.Context(), hash, at(3), "", "")
			is.Error(t, model.ErrorLinkInvalid, err, hash)
		}

		_, err = db.CheckSignInLink(t.Context(), thomas, at(3), "")
		is.NotError(t, err)
	})

	t.Run("does not use up a link by looking at it", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		sqlitetest.CreatePerson(t, db, "Signe", "signe@example.com", model.RoleMember)
		hash := createLink(t, db, "signe@example.com", 0, "h")

		for range 3 {
			_, err := db.CheckSignInLink(t.Context(), hash, at(1), "")
			is.NotError(t, err)
		}
		_, err := db.SignIn(t.Context(), hash, at(1), "", "")
		is.NotError(t, err)
	})

	t.Run("works until just before the link expires, and not from then on", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		sqlitetest.CreatePerson(t, db, "Signe", "signe@example.com", model.RoleMember)
		hash := createLink(t, db, "signe@example.com", 0, "h")
		justBefore := model.Time{T: at(15).T.Add(-time.Millisecond)}

		_, err := db.CheckSignInLink(t.Context(), hash, justBefore, "")
		is.NotError(t, err)
		_, err = db.CheckSignInLink(t.Context(), hash, at(15), "")
		is.Error(t, model.ErrorLinkInvalid, err)
		_, err = db.SignIn(t.Context(), hash, at(15), "", "")
		is.Error(t, model.ErrorLinkInvalid, err)
		_, err = db.SignIn(t.Context(), hash, justBefore, "", "")
		is.NotError(t, err)
	})

	t.Run("refuses a person who is no longer active, and a link that was never made", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreatePerson(t, db, "Signe", "signe@example.com", model.RoleMember)
		hash := createLink(t, db, "signe@example.com", 0, "h")
		sqlitetest.DeactivateUser(t, db, signe.ID)

		_, err := db.CheckSignInLink(t.Context(), hash, at(1), "")
		is.Error(t, model.ErrorLinkInvalid, err)
		_, err = db.SignIn(t.Context(), hash, at(1), "", "")
		is.Error(t, model.ErrorLinkInvalid, err)
		_, err = db.SignIn(t.Context(), "never", at(1), "", "")
		is.Error(t, model.ErrorLinkInvalid, err)
	})

	t.Run("has the first admin join the empty group as an admin with their name, who then signs in like anyone", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		hash := createLink(t, db, "markus@example.com", 0, "h")
		other := createLink(t, db, "markus@example.com", 1, "other")

		check, err := db.CheckSignInLink(t.Context(), hash, at(2), "markus@example.com")
		is.NotError(t, err)
		is.True(t, check.Joining)

		u, err := db.SignIn(t.Context(), hash, at(2), "markus@example.com", "Markus")
		is.NotError(t, err)
		is.Equal(t, "Markus", u.Name)
		is.Equal(t, model.RoleAdmin, u.Role)
		is.Equal(t, model.EmailAddress("markus@example.com"), u.Email)
		is.True(t, u.Active)
		is.True(t, u.InvitedBy == nil)
		is.Equal(t, at(2).String(), u.Created.String())

		_, err = db.SignIn(t.Context(), other, at(3), "markus@example.com", "Markus again")
		is.Error(t, model.ErrorLinkInvalid, err)
		is.Equal(t, 1, count(t, db, "users"))

		later := createLink(t, db, "markus@example.com", 4, "later")
		check, err = db.CheckSignInLink(t.Context(), later, at(5), "markus@example.com")
		is.NotError(t, err)
		is.True(t, !check.Joining)
	})

	t.Run("lets nobody join as the first admin without a name, and uses up nothing then", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		hash := createLink(t, db, "markus@example.com", 0, "h")

		_, err := db.SignIn(t.Context(), hash, at(1), "markus@example.com", "")
		is.Error(t, model.ErrorNameEmpty, err)
		is.Equal(t, 0, count(t, db, "users"))

		_, err = db.SignIn(t.Context(), hash, at(1), "markus@example.com", "Markus")
		is.NotError(t, err)
	})

	t.Run("lets nobody join as the first admin once the group has anyone in it, active or not, or with another address", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		hash := createLink(t, db, "markus@example.com", 0, "h")

		_, err := db.CheckSignInLink(t.Context(), hash, at(1), "")
		is.Error(t, model.ErrorLinkInvalid, err)
		_, err = db.CheckSignInLink(t.Context(), hash, at(1), "signe@example.com")
		is.Error(t, model.ErrorLinkInvalid, err)

		thomas := sqlitetest.CreatePerson(t, db, "Thomas", "thomas@example.com", model.RoleAdmin)
		sqlitetest.DeactivateUser(t, db, thomas.ID)

		_, err = db.CheckSignInLink(t.Context(), hash, at(1), "markus@example.com")
		is.Error(t, model.ErrorLinkInvalid, err)
		_, err = db.SignIn(t.Context(), hash, at(1), "markus@example.com", "Markus")
		is.Error(t, model.ErrorLinkInvalid, err)
		is.Equal(t, 1, count(t, db, "users"))
	})
}

func TestDatabase_CreateInvitation(t *testing.T) {
	t.Run("makes an invitation with the job that sends it", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreatePerson(t, db, "Signe", "signe@example.com", model.RoleModerator)

		inv, err := db.CreateInvitation(t.Context(), model.Invitation{Created: at(0), Expires: at(60), Email: "amira@example.com", InvitedBy: signe.ID}, formToken(t))
		is.NotError(t, err)
		is.Equal(t, model.EmailAddress("amira@example.com"), inv.Email)
		is.Equal(t, signe.ID, inv.InvitedBy)
		is.Equal(t, "Signe", inv.InvitedByName)
		is.Equal(t, at(60).String(), inv.Expires.String())

		jobs := jobstest.Take(t, db)
		is.Equal(t, 1, len(jobs))
		is.Equal(t, model.JobNameSendInvitation, jobs[0].Name)
		is.Equal(t, `{"InvitationID":"`+inv.ID.String()+`"}`, string(jobs[0].Data))
	})

	t.Run("replaces the invitation that was waiting for the same address, whose link then no longer works", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreatePerson(t, db, "Signe", "signe@example.com", model.RoleModerator)
		first, err := db.CreateInvitation(t.Context(), model.Invitation{Created: at(0), Expires: at(60), Email: "amira@example.com", InvitedBy: signe.ID}, formToken(t))
		is.NotError(t, err)
		_, err = db.SetInvitationHash(t.Context(), first.ID, "first", at(1))
		is.NotError(t, err)

		second, err := db.CreateInvitation(t.Context(), model.Invitation{Created: at(2), Expires: at(62), Email: "amira@example.com", InvitedBy: signe.ID}, formToken(t))
		is.NotError(t, err)
		is.True(t, first.ID != second.ID)

		_, err = db.GetInvitationByHash(t.Context(), "first", at(3))
		is.Error(t, model.ErrorLinkInvalid, err)
		_, err = db.SetInvitationHash(t.Context(), first.ID, "again", at(3))
		is.Error(t, model.ErrorInvitationNotFound, err)
		is.Equal(t, 1, count(t, db, "invitations"))
	})

	t.Run("refuses the address of a member, and of someone who is no longer active", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreatePerson(t, db, "Signe", "signe@example.com", model.RoleModerator)
		thomas := sqlitetest.CreatePerson(t, db, "Thomas", "thomas@example.com", model.RoleMember)
		sqlitetest.DeactivateUser(t, db, thomas.ID)

		_, err := db.CreateInvitation(t.Context(), model.Invitation{Created: at(0), Expires: at(60), Email: "signe@example.com", InvitedBy: signe.ID}, formToken(t))
		is.Error(t, model.ErrorAlreadyMember, err)
		_, err = db.CreateInvitation(t.Context(), model.Invitation{Created: at(0), Expires: at(60), Email: "thomas@example.com", InvitedBy: signe.ID}, formToken(t))
		is.Error(t, model.ErrorFormerMember, err)
		is.Equal(t, 0, count(t, db, "invitations"))
		is.Equal(t, 0, len(jobstest.Take(t, db)))
	})

	t.Run("makes nothing for a form sent again with the same address, and returns the invitation that is waiting for it", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreatePerson(t, db, "Signe", "signe@example.com", model.RoleModerator)
		token := formToken(t)

		first, err := db.CreateInvitation(t.Context(), model.Invitation{Created: at(0), Expires: at(60), Email: "amira@example.com", InvitedBy: signe.ID}, token)
		is.NotError(t, err)
		is.Equal(t, 1, len(jobstest.Take(t, db)))

		again, err := db.CreateInvitation(t.Context(), model.Invitation{Created: at(1), Expires: at(61), Email: "amira@example.com", InvitedBy: signe.ID}, token)
		is.NotError(t, err)
		is.Equal(t, first.ID, again.ID)
		is.Equal(t, at(60).String(), again.Expires.String())
		is.Equal(t, 1, count(t, db, "invitations"))
		is.Equal(t, 0, len(jobstest.Take(t, db)))

		// Once the invitation is gone, the same form still makes nothing.
		is.NotError(t, db.DeleteInvitation(t.Context(), first.ID, at(2)))
		again, err = db.CreateInvitation(t.Context(), model.Invitation{Created: at(3), Expires: at(63), Email: "amira@example.com", InvitedBy: signe.ID}, token)
		is.NotError(t, err)
		is.Equal(t, model.InvitationID(""), again.ID)
		is.Equal(t, 0, count(t, db, "invitations"))
		is.Equal(t, 0, len(jobstest.Take(t, db)))
	})

	t.Run("makes nothing for a form sent again with another address, and says so", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreatePerson(t, db, "Signe", "signe@example.com", model.RoleModerator)
		token := formToken(t)

		_, err := db.CreateInvitation(t.Context(), model.Invitation{Created: at(0), Expires: at(60), Email: "amira@example.com", InvitedBy: signe.ID}, token)
		is.NotError(t, err)
		jobstest.Take(t, db)

		_, err = db.CreateInvitation(t.Context(), model.Invitation{Created: at(1), Expires: at(61), Email: "nadia@example.com", InvitedBy: signe.ID}, token)
		is.Error(t, model.ErrorFormResentChanged, err)
		is.Equal(t, 1, count(t, db, "invitations"))
		is.Equal(t, 0, len(jobstest.Take(t, db)))
	})

	t.Run("lets another person use a token someone else used, and keeps a token that made nothing unused", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreatePerson(t, db, "Signe", "signe@example.com", model.RoleModerator)
		markus := sqlitetest.CreatePerson(t, db, "Markus", "markus@example.com", model.RoleAdmin)
		token := formToken(t)

		_, err := db.CreateInvitation(t.Context(), model.Invitation{Created: at(0), Expires: at(60), Email: "signe@example.com", InvitedBy: signe.ID}, token)
		is.Error(t, model.ErrorAlreadyMember, err)
		_, err = db.CreateInvitation(t.Context(), model.Invitation{Created: at(1), Expires: at(61), Email: "amira@example.com", InvitedBy: signe.ID}, token)
		is.NotError(t, err)
		_, err = db.CreateInvitation(t.Context(), model.Invitation{Created: at(2), Expires: at(62), Email: "nadia@example.com", InvitedBy: markus.ID}, token)
		is.NotError(t, err)
		is.Equal(t, 2, count(t, db, "invitations"))
	})

	t.Run("keeps only the hash of the address with the token, which outlives the invitation", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreatePerson(t, db, "Signe", "signe@example.com", model.RoleModerator)

		_, err := db.CreateInvitation(t.Context(), model.Invitation{Created: at(0), Expires: at(60), Email: "amira@example.com", InvitedBy: signe.ID}, formToken(t))
		is.NotError(t, err)

		var hashes []string
		is.NotError(t, db.H.Select(t.Context(), &hashes, `select email_hash from invitation_form_tokens`))
		sum := sha256.Sum256([]byte("amira@example.com"))
		is.EqualSlice(t, []string{hex.EncodeToString(sum[:])}, hashes)
	})
}

// formToken for a form, new each time.
func formToken(t *testing.T) model.FormToken {
	t.Helper()

	token, err := model.NewFormToken(rand.Reader)
	is.NotError(t, err)
	return token
}

func TestDatabase_GetInvitations(t *testing.T) {
	t.Run("lists the invitations that have not expired, the newest first, and deletes one", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreatePerson(t, db, "Signe", "signe@example.com", model.RoleModerator)
		old, err := db.CreateInvitation(t.Context(), model.Invitation{Created: at(0), Expires: at(10), Email: "old@example.com", InvitedBy: signe.ID}, formToken(t))
		is.NotError(t, err)
		a, err := db.CreateInvitation(t.Context(), model.Invitation{Created: at(1), Expires: at(61), Email: "a@example.com", InvitedBy: signe.ID}, formToken(t))
		is.NotError(t, err)
		b, err := db.CreateInvitation(t.Context(), model.Invitation{Created: at(2), Expires: at(62), Email: "b@example.com", InvitedBy: signe.ID}, formToken(t))
		is.NotError(t, err)

		invs, err := db.GetInvitations(t.Context(), at(10))
		is.NotError(t, err)
		is.Equal(t, 2, len(invs))
		is.Equal(t, b.ID, invs[0].ID)
		is.Equal(t, a.ID, invs[1].ID)
		is.Equal(t, "Signe", invs[0].InvitedByName)

		is.NotError(t, db.DeleteInvitation(t.Context(), a.ID, at(3)))
		is.Error(t, model.ErrorInvitationNotFound, db.DeleteInvitation(t.Context(), a.ID, at(3)))
		invs, err = db.GetInvitations(t.Context(), at(9))
		is.NotError(t, err)
		is.Equal(t, 2, len(invs))
		is.Equal(t, old.ID, invs[1].ID)

		// An invitation that has expired is not waiting, so there is none to withdraw, and the next
		// invitation deletes it.
		is.Error(t, model.ErrorInvitationNotFound, db.DeleteInvitation(t.Context(), old.ID, at(10)))
		_, err = db.CreateInvitation(t.Context(), model.Invitation{Created: at(10), Expires: at(70), Email: "c@example.com", InvitedBy: signe.ID}, formToken(t))
		is.NotError(t, err)
		is.Equal(t, 0, count(t, db, "invitations where id = '"+old.ID.String()+"'"))
	})
}

func TestDatabase_AcceptInvitation(t *testing.T) {
	t.Run("has a new active member join with the invitation's address, the name, and who invited them, once", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreatePerson(t, db, "Signe", "signe@example.com", model.RoleModerator)
		inv, err := db.CreateInvitation(t.Context(), model.Invitation{Created: at(0), Expires: at(60), Email: "amira@example.com", InvitedBy: signe.ID}, formToken(t))
		is.NotError(t, err)
		_, err = db.SetInvitationHash(t.Context(), inv.ID, "h", at(1))
		is.NotError(t, err)

		got, err := db.GetInvitationByHash(t.Context(), "h", at(2))
		is.NotError(t, err)
		is.Equal(t, inv.ID, got.ID)

		u, accepted, err := db.AcceptInvitation(t.Context(), "h", at(3), "Amira")
		is.NotError(t, err)
		is.Equal(t, inv.ID, accepted.ID)
		is.Equal(t, "Amira", u.Name)
		is.Equal(t, model.EmailAddress("amira@example.com"), u.Email)
		is.Equal(t, model.RoleMember, u.Role)
		is.True(t, u.Active)
		is.Equal(t, signe.ID, *u.InvitedBy)
		is.Equal(t, at(3).String(), u.Created.String())

		_, _, err = db.AcceptInvitation(t.Context(), "h", at(4), "Amira again")
		is.Error(t, model.ErrorLinkInvalid, err)
		is.Equal(t, 2, count(t, db, "users"))
		is.Equal(t, 0, count(t, db, "invitations"))
	})

	t.Run("makes nothing posted before the person accepted new to them, and what is posted after new", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreatePerson(t, db, "Signe", "signe@example.com", model.RoleModerator)
		inv, err := db.CreateInvitation(t.Context(), model.Invitation{Created: at(0), Expires: at(60), Email: "amira@example.com", InvitedBy: signe.ID}, formToken(t))
		is.NotError(t, err)
		_, err = db.SetInvitationHash(t.Context(), inv.ID, "h", at(1))
		is.NotError(t, err)
		createPost(t, db, signe, at(2).T)

		amira, _, err := db.AcceptInvitation(t.Context(), "h", at(3), "Amira")
		is.NotError(t, err)
		createPost(t, db, signe, at(4).T)

		posts := getTimeline(t, db, amira)
		is.Equal(t, 2, len(posts))
		is.True(t, posts[0].New)
		is.True(t, !posts[1].New)
	})

	t.Run("refuses an invitation that has expired, been withdrawn, or was never made", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreatePerson(t, db, "Signe", "signe@example.com", model.RoleModerator)
		a, err := db.CreateInvitation(t.Context(), model.Invitation{Created: at(0), Expires: at(60), Email: "a@example.com", InvitedBy: signe.ID}, formToken(t))
		is.NotError(t, err)
		_, err = db.SetInvitationHash(t.Context(), a.ID, "a", at(1))
		is.NotError(t, err)
		b, err := db.CreateInvitation(t.Context(), model.Invitation{Created: at(0), Expires: at(60), Email: "b@example.com", InvitedBy: signe.ID}, formToken(t))
		is.NotError(t, err)
		_, err = db.SetInvitationHash(t.Context(), b.ID, "b", at(1))
		is.NotError(t, err)
		is.NotError(t, db.DeleteInvitation(t.Context(), b.ID, at(1)))

		_, err = db.GetInvitationByHash(t.Context(), "a", at(60))
		is.Error(t, model.ErrorLinkInvalid, err)
		_, _, err = db.AcceptInvitation(t.Context(), "a", at(60), "A")
		is.Error(t, model.ErrorLinkInvalid, err)
		_, _, err = db.AcceptInvitation(t.Context(), "b", at(2), "B")
		is.Error(t, model.ErrorLinkInvalid, err)
		_, _, err = db.AcceptInvitation(t.Context(), "never", at(2), "C")
		is.Error(t, model.ErrorLinkInvalid, err)
		is.Equal(t, 1, count(t, db, "users"))

		_, err = db.SetInvitationHash(t.Context(), a.ID, "late", at(60))
		is.Error(t, model.ErrorInvitationNotFound, err)
	})
}

func TestDatabase_ChangeRole(t *testing.T) {
	t.Run("changes the role if the check of the actor and the target passes, and returns the role before", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		markus := sqlitetest.CreatePerson(t, db, "Markus", "markus@example.com", model.RoleAdmin)
		signe := sqlitetest.CreatePerson(t, db, "Signe", "signe@example.com", model.RoleMember)

		var checked []string
		before, err := db.ChangeRole(t.Context(), markus.ID, signe.ID, model.RoleModerator, func(a model.User, t *model.User) error {
			checked = append(checked, a.Name, t.Name)
			return nil
		})
		is.NotError(t, err)
		is.Equal(t, model.RoleMember, before)
		is.EqualSlice(t, []string{"Markus", "Signe"}, checked)

		u, err := db.GetUser(t.Context(), signe.ID)
		is.NotError(t, err)
		is.Equal(t, model.RoleModerator, u.Role)
	})

	t.Run("changes nothing if the check fails, and gives the check no target for someone who is not there", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		markus := sqlitetest.CreatePerson(t, db, "Markus", "markus@example.com", model.RoleAdmin)
		signe := sqlitetest.CreatePerson(t, db, "Signe", "signe@example.com", model.RoleMember)

		_, err := db.ChangeRole(t.Context(), markus.ID, signe.ID, model.RoleAdmin, func(model.User, *model.User) error { return model.ErrorNotAllowed })
		is.Error(t, model.ErrorNotAllowed, err)
		u, err := db.GetUser(t.Context(), signe.ID)
		is.NotError(t, err)
		is.Equal(t, model.RoleMember, u.Role)

		target := &model.User{}
		_, err = db.ChangeRole(t.Context(), markus.ID, "u_nope", model.RoleAdmin, func(_ model.User, t *model.User) error {
			target = t
			return model.ErrorMemberNotFound
		})
		is.Error(t, model.ErrorMemberNotFound, err)
		is.True(t, target == nil)

		_, err = db.ChangeRole(t.Context(), "u_nope", signe.ID, model.RoleAdmin, func(model.User, *model.User) error { return nil })
		is.Error(t, model.ErrorUserNotFound, err)
	})
}

func TestDatabase_ListMembers(t *testing.T) {
	t.Run("lists the active people in the order they joined", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		thomas := sqlitetest.CreatePerson(t, db, "Thomas", "thomas@example.com", model.RoleMember)
		amira := sqlitetest.CreatePerson(t, db, "Amira", "amira@example.com", model.RoleAdmin)
		signe := sqlitetest.CreatePerson(t, db, "Signe", "signe@example.com", model.RoleMember)
		sqlitetest.DeactivateUser(t, db, amira.ID)

		users, err := db.ListMembers(t.Context())
		is.NotError(t, err)
		is.Equal(t, 2, len(users))
		is.Equal(t, thomas.ID, users[0].ID)
		is.Equal(t, signe.ID, users[1].ID)
	})
}

func TestPeopleMigration(t *testing.T) {
	t.Run("gives the template's admins the admin role and everyone else the member role, stores addresses lowercased, and migrates down and up again", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		migrations := os.DirFS("migrations")
		is.NotError(t, migrate.To(t.Context(), db.H.DB.DB, migrations, "1795713000-form-tokens"))

		admin := sqlitetest.CreateUser(t, db, "Admin")
		member := sqlitetest.CreateUser(t, db, "Member")
		_, err := db.H.DB.ExecContext(t.Context(), `insert into users_roles (user_id, role) values (?, 'admin')`, admin.ID)
		is.NotError(t, err)
		_, err = db.H.DB.ExecContext(t.Context(), `update users set email = ' Member@Example.COM ' where id = ?`, member.ID)
		is.NotError(t, err)

		for _, to := range []func() error{
			func() error { return migrate.Up(t.Context(), db.H.DB.DB, migrations) },
			func() error { return migrate.To(t.Context(), db.H.DB.DB, migrations, "1795713000-form-tokens") },
			func() error { return migrate.Up(t.Context(), db.H.DB.DB, migrations) },
		} {
			is.NotError(t, to())
		}

		u, err := db.GetUser(t.Context(), admin.ID)
		is.NotError(t, err)
		is.Equal(t, model.RoleAdmin, u.Role)
		u, err = db.GetUser(t.Context(), member.ID)
		is.NotError(t, err)
		is.Equal(t, model.RoleMember, u.Role)
		is.Equal(t, model.EmailAddress("member@example.com"), u.Email)
	})

	t.Run("refuses a role that is not one, and a second invitation to the same address, in the schema itself", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")

		_, err := db.H.DB.ExecContext(t.Context(), `update users set role = 'owner' where id = ?`, signe.ID)
		is.True(t, err != nil, "expected the role to be refused")

		insert := `insert into invitations (created, expires, email, invited_by) values ('2031-01-01T00:00:00.000Z', '2031-01-08T00:00:00.000Z', 'a@example.com', ?)`
		_, err = db.H.DB.ExecContext(t.Context(), insert, signe.ID)
		is.NotError(t, err)
		_, err = db.H.DB.ExecContext(t.Context(), insert, signe.ID)
		is.True(t, err != nil, "expected the second invitation to be refused")
	})
}
