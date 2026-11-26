package service_test

import (
	"bytes"
	"errors"
	"log/slog"
	"math/rand/v2"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"maragu.dev/glue/email"
	"maragu.dev/glue/oteltest"
	"maragu.dev/is"

	"app/jobs"
	"app/jobstest"
	"app/model"
	"app/peopletest"
	"app/service"
	"app/servicetest"
	"app/sqlite"
	"app/sqlitetest"
)

// people is the business logic of a group, The Loft, on a real database, with a clock the test sets, a
// sender that keeps what it is asked to send, and the jobs run when the test says.
type people struct {
	db     *sqlite.Database
	fat    *service.Fat
	sender *servicetest.Sender
	clock  *clock
}

// newPeople with the given address of the first admin, which may be empty, logging to the given logger, or
// through the test if it is nil.
func newPeople(t *testing.T, firstAdmin model.EmailAddress, log *slog.Logger) *people {
	t.Helper()

	p := &people{db: sqlitetest.NewDatabase(t), sender: &servicetest.Sender{}, clock: &clock{t: base}}
	if log == nil {
		p.fat = servicetest.NewFat(t)
	} else {
		p.fat = service.NewFat(service.NewFatOptions{Log: log})
	}
	service.Setup(p.fat, nil, p.db, p.sender, p.clock.Now, rand.NewChaCha8([32]byte{}), model.Group{Name: "The Loft", TimeZone: time.UTC}, firstAdmin)
	return p
}

// runJobs that are waiting, and return the emails they sent.
func (p *people) runJobs(t *testing.T) []email.SendOptions {
	t.Helper()

	jobstest.Run(t, p.db, jobs.Funcs(jobs.RegisterOpts{Emails: p.fat}))
	return p.sender.Take()
}

// link for the given address: ask for one, run the jobs, and return the token of the one email sent.
func (p *people) link(t *testing.T, address string) string {
	t.Helper()

	is.NotError(t, p.fat.RequestSignInLink(t.Context(), address))
	sent := p.runJobs(t)
	is.Equal(t, 1, len(sent), "expected one email")
	return sent[0].Keywords["token"]
}

// invite the given address as the given person, run the jobs, and return the invitation and the token of
// the one email sent.
func (p *people) invite(t *testing.T, actor model.User, address string) (model.Invitation, string) {
	t.Helper()

	inv, err := p.fat.Invite(t.Context(), actor.ID, address)
	is.NotError(t, err)
	sent := p.runJobs(t)
	is.Equal(t, 1, len(sent), "expected one email")
	return inv, sent[0].Keywords["token"]
}

func (p *people) person(t *testing.T, name string, role model.Role) model.User {
	t.Helper()

	return sqlitetest.CreatePerson(t, p.db, name, model.EmailAddress(strings.ToLower(name)+"@example.com"), role)
}

func TestFat_RequestSignInLink(t *testing.T) {
	t.Run("answers an address in the group and one that is not with the same, after the same kind of work, and sends a link only to the one in the group", func(t *testing.T) {
		p := newPeople(t, "", nil)
		p.person(t, "Signe", model.RoleMember)

		known := p.fat.RequestSignInLink(t.Context(), "Signe@Example.com")
		knownJobs := jobstest.Take(t, p.db)
		unknown := p.fat.RequestSignInLink(t.Context(), "Stranger@Example.com")
		unknownJobs := jobstest.Take(t, p.db)

		is.True(t, known == nil && unknown == nil)
		is.Equal(t, 1, len(knownJobs))
		is.Equal(t, 1, len(unknownJobs))
		is.Equal(t, knownJobs[0].Name, unknownJobs[0].Name)
		is.Equal(t, model.JobNameSendSignInLink, knownJobs[0].Name)
		// The work differs only in the address it is for.
		is.Equal(t, string(knownJobs[0].Data), strings.ReplaceAll(string(unknownJobs[0].Data), "stranger@", "signe@"))

		for _, j := range append(knownJobs, unknownJobs...) {
			is.NotError(t, jobs.Funcs(jobs.RegisterOpts{Emails: p.fat})[j.Name](t.Context(), j.Data))
		}
		sent := p.sender.Take()
		is.Equal(t, 1, len(sent))
		is.Equal(t, model.EmailAddress("signe@example.com"), sent[0].To)
	})

	t.Run("refuses what is not an email address, and makes no work for it", func(t *testing.T) {
		p := newPeople(t, "", nil)

		for _, address := range []string{"", "signe", "signe@localhost", "Signe <signe@example.com>"} {
			is.Error(t, model.ErrorEmailInvalid, p.fat.RequestSignInLink(t.Context(), address), address)
		}
		is.Equal(t, 0, len(jobstest.Take(t, p.db)))
	})
}

func TestFat_SendSignInLink(t *testing.T) {
	t.Run("sends an active person an email naming the group, with a link whose token works and is stored only as its hash", func(t *testing.T) {
		p := newPeople(t, "", nil)
		p.person(t, "Signe", model.RoleMember)

		is.NotError(t, p.fat.RequestSignInLink(t.Context(), "signe@example.com"))
		sent := p.runJobs(t)
		is.Equal(t, 1, len(sent))
		e := sent[0]
		is.Equal(t, "sign-in", e.Template)
		is.Equal(t, "Sign in to The Loft", e.Subject)
		is.Equal(t, "Your link to sign in to The Loft. It works once, for the next 15 minutes.", e.Preheader)
		is.Equal(t, model.EmailAddress("signe@example.com"), e.To)
		is.Equal(t, "Signe", e.ToName)
		is.Equal(t, "The Loft", e.Keywords["group"])

		token, err := model.ParseLinkToken(e.Keywords["token"])
		is.NotError(t, err)
		is.Equal(t, 1, count(t, p.db, "sign_in_links where hash = '"+token.Hash()+"'"))
		is.Equal(t, 0, count(t, p.db, "sign_in_links where hash = '"+token.String()+"'"))
		var expires string
		is.NotError(t, p.db.H.Get(t.Context(), &expires, `select expires from sign_in_links`))
		is.Equal(t, model.Time{T: base.Add(15 * time.Minute)}.String(), expires)
	})

	t.Run("sends nothing to an address nobody in the group has, or to someone who is no longer active", func(t *testing.T) {
		p := newPeople(t, "", nil)
		signe := p.person(t, "Signe", model.RoleMember)
		sqlitetest.DeactivateUser(t, p.db, signe.ID)

		is.NotError(t, p.fat.RequestSignInLink(t.Context(), "signe@example.com"))
		is.NotError(t, p.fat.RequestSignInLink(t.Context(), "thomas@example.com"))
		is.Equal(t, 0, len(p.runJobs(t)))
		is.Equal(t, 0, count(t, p.db, "sign_in_links"))
	})

	t.Run("sends the first admin a link while the group has nobody in it, and not once it has someone", func(t *testing.T) {
		p := newPeople(t, "markus@example.com", nil)

		is.NotError(t, p.fat.RequestSignInLink(t.Context(), " Markus@example.com"))
		sent := p.runJobs(t)
		is.Equal(t, 1, len(sent))
		is.Equal(t, "", sent[0].ToName)

		p.person(t, "Signe", model.RoleAdmin)
		is.NotError(t, p.fat.RequestSignInLink(t.Context(), "markus@example.com"))
		is.Equal(t, 0, len(p.runJobs(t)))
	})

	t.Run("sends at most five links to an address in an hour, and then nothing, until an hour after the first", func(t *testing.T) {
		p := newPeople(t, "", nil)
		p.person(t, "Signe", model.RoleMember)
		p.person(t, "Thomas", model.RoleMember)

		for i := range 7 {
			p.clock.t = base.Add(time.Duration(i) * time.Minute)
			is.NotError(t, p.fat.RequestSignInLink(t.Context(), "signe@example.com"))
			is.Equal(t, min(1, max(0, 5-i)), len(p.runJobs(t)), i)
		}
		p.link(t, "thomas@example.com")

		p.clock.t = base.Add(time.Hour - time.Millisecond)
		is.NotError(t, p.fat.RequestSignInLink(t.Context(), "signe@example.com"))
		is.Equal(t, 0, len(p.runJobs(t)))

		p.clock.t = base.Add(time.Hour)
		p.link(t, "signe@example.com")
	})

	t.Run("fails if the email cannot be sent, so that the job is tried again", func(t *testing.T) {
		p := newPeople(t, "", nil)
		p.person(t, "Signe", model.RoleMember)
		p.sender.Fail(errors.New("no"))

		err := p.fat.SendSignInLink(t.Context(), "signe@example.com")
		is.True(t, err != nil)
	})
}

func TestFat_firstAdmin(t *testing.T) {
	t.Run("keeps the first admin an admin who can sign in when the configuration names someone else later, and lets the new address in nowhere", func(t *testing.T) {
		p := newPeople(t, "markus@example.com", nil)
		markus, err := p.fat.SignIn(t.Context(), p.link(t, "markus@example.com"), "Markus")
		is.NotError(t, err)

		// The host changes the setting, and the app starts again with it.
		changed := service.NewFat(service.NewFatOptions{})
		service.Setup(changed, nil, p.db, p.sender, p.clock.Now, rand.NewChaCha8([32]byte{2}), model.Group{Name: "The Loft", TimeZone: time.UTC}, "signe@example.com")
		p.fat = changed

		is.NotError(t, p.fat.RequestSignInLink(t.Context(), "signe@example.com"))
		is.Equal(t, 0, len(p.runJobs(t)))

		u, err := p.fat.SignIn(t.Context(), p.link(t, "markus@example.com"), "")
		is.NotError(t, err)
		is.Equal(t, markus.ID, u.ID)
		is.Equal(t, model.RoleAdmin, u.Role)
		is.Equal(t, 1, count(t, p.db, "users"))
	})
}

func TestFat_SignIn(t *testing.T) {
	t.Run("signs in the person, once, and not with any other link they had", func(t *testing.T) {
		p := newPeople(t, "", nil)
		signe := p.person(t, "Signe", model.RoleMember)
		first := p.link(t, "signe@example.com")
		second := p.link(t, "signe@example.com")

		check, err := p.fat.CheckSignInLink(t.Context(), second)
		is.NotError(t, err)
		is.True(t, !check.Joining)

		u, err := p.fat.SignIn(t.Context(), second, "Not used")
		is.NotError(t, err)
		is.Equal(t, signe.ID, u.ID)
		is.Equal(t, "Signe", u.Name)

		for _, token := range []string{first, second} {
			_, err = p.fat.CheckSignInLink(t.Context(), token)
			is.Error(t, model.ErrorLinkInvalid, err)
			_, err = p.fat.SignIn(t.Context(), token, "")
			is.Error(t, model.ErrorLinkInvalid, err)
		}
	})

	t.Run("works for 15 minutes, and not from then on", func(t *testing.T) {
		p := newPeople(t, "", nil)
		p.person(t, "Signe", model.RoleMember)
		token := p.link(t, "signe@example.com")

		p.clock.t = base.Add(15 * time.Minute)
		_, err := p.fat.CheckSignInLink(t.Context(), token)
		is.Error(t, model.ErrorLinkInvalid, err)
		_, err = p.fat.SignIn(t.Context(), token, "")
		is.Error(t, model.ErrorLinkInvalid, err)

		p.clock.t = base.Add(15*time.Minute - time.Millisecond)
		_, err = p.fat.SignIn(t.Context(), token, "")
		is.NotError(t, err)
	})

	t.Run("refuses garbage, an invitation's link, and a link of someone made inactive, all with the one refusal", func(t *testing.T) {
		p := newPeople(t, "", nil)
		signe := p.person(t, "Signe", model.RoleModerator)
		token := p.link(t, "signe@example.com")
		_, invitation := p.invite(t, signe, "amira@example.com")
		sqlitetest.DeactivateUser(t, p.db, signe.ID)

		for _, s := range []string{token, invitation, "", "garbage", strings.Repeat("A", 43)} {
			_, err := p.fat.CheckSignInLink(t.Context(), s)
			is.Error(t, model.ErrorLinkInvalid, err, s)
			_, err = p.fat.SignIn(t.Context(), s, "Signe")
			is.Error(t, model.ErrorLinkInvalid, err, s)
		}
	})

	t.Run("has the first admin choose their name and join as an admin, and refuses a name that cannot be used without using up the link", func(t *testing.T) {
		p := newPeople(t, "markus@example.com", nil)
		token := p.link(t, "markus@example.com")

		check, err := p.fat.CheckSignInLink(t.Context(), token)
		is.NotError(t, err)
		is.True(t, check.Joining)

		_, err = p.fat.SignIn(t.Context(), token, " \n ")
		is.Error(t, model.ErrorNameEmpty, err)
		_, err = p.fat.SignIn(t.Context(), token, "Markus\nAdmin")
		is.Error(t, model.ErrorNameInvalid, err)
		_, err = p.fat.SignIn(t.Context(), token, strings.Repeat("M", model.NameMaxLength+1))
		is.Error(t, model.ErrorNameTooLong, err)
		is.Equal(t, 0, count(t, p.db, "users"))

		p.clock.t = base.Add(time.Minute)
		u, err := p.fat.SignIn(t.Context(), token, "  Markus ")
		is.NotError(t, err)
		is.Equal(t, "Markus", u.Name)
		is.Equal(t, model.RoleAdmin, u.Role)
		is.True(t, u.Active)
		is.True(t, u.Created.T.Equal(base.Add(time.Minute)))
	})
}

func TestFat_Invite(t *testing.T) {
	t.Run("lets a moderator invite an address, which gets an email naming who invited and the group, with a link that works for 7 days", func(t *testing.T) {
		p := newPeople(t, "", nil)
		signe := p.person(t, "Signe", model.RoleModerator)

		inv, err := p.fat.Invite(t.Context(), signe.ID, " Amira@Example.com ")
		is.NotError(t, err)
		is.Equal(t, model.EmailAddress("amira@example.com"), inv.Email)
		is.Equal(t, signe.ID, inv.InvitedBy)
		is.True(t, inv.Expires.T.Equal(base.Add(7*24*time.Hour)))

		sent := p.runJobs(t)
		is.Equal(t, 1, len(sent))
		e := sent[0]
		is.Equal(t, "invitation", e.Template)
		is.Equal(t, "Signe invited you to The Loft", e.Subject)
		is.Equal(t, "Signe has invited you to join The Loft. The link works for 7 days.", e.Preheader)
		is.Equal(t, model.EmailAddress("amira@example.com"), e.To)
		is.Equal(t, "", e.ToName)
		is.Equal(t, "Signe", e.Keywords["inviter"])
		is.Equal(t, "The Loft", e.Keywords["group"])

		check, err := p.fat.CheckInvitation(t.Context(), e.Keywords["token"])
		is.NotError(t, err)
		is.Equal(t, "Signe", check.InvitedByName)

		p.clock.t = base.Add(7*24*time.Hour - time.Millisecond)
		_, err = p.fat.CheckInvitation(t.Context(), e.Keywords["token"])
		is.NotError(t, err)
		p.clock.t = base.Add(7 * 24 * time.Hour)
		_, err = p.fat.CheckInvitation(t.Context(), e.Keywords["token"])
		is.Error(t, model.ErrorLinkInvalid, err)
		_, err = p.fat.AcceptInvitation(t.Context(), e.Keywords["token"], "Amira")
		is.Error(t, model.ErrorLinkInvalid, err)
	})

	t.Run("lets an admin invite, and refuses a member, someone inactive, and someone who does not exist", func(t *testing.T) {
		p := newPeople(t, "", nil)
		markus := p.person(t, "Markus", model.RoleAdmin)
		member := p.person(t, "Thomas", model.RoleMember)
		inactive := p.person(t, "Karen", model.RoleModerator)
		sqlitetest.DeactivateUser(t, p.db, inactive.ID)

		_, err := p.fat.Invite(t.Context(), markus.ID, "a@example.com")
		is.NotError(t, err)
		_, err = p.fat.Invite(t.Context(), member.ID, "b@example.com")
		is.Error(t, model.ErrorNotAllowed, err)
		_, err = p.fat.Invite(t.Context(), inactive.ID, "b@example.com")
		is.Error(t, model.ErrorUserInactive, err)
		_, err = p.fat.Invite(t.Context(), "u_nope", "b@example.com")
		is.Error(t, model.ErrorUserNotFound, err)
		is.Equal(t, 1, len(p.runJobs(t)))
	})

	t.Run("refuses an address that is not one, a member's in any spelling, and that of someone no longer active", func(t *testing.T) {
		p := newPeople(t, "", nil)
		signe := p.person(t, "Signe", model.RoleModerator)
		karen := p.person(t, "Karen", model.RoleMember)
		sqlitetest.DeactivateUser(t, p.db, karen.ID)

		_, err := p.fat.Invite(t.Context(), signe.ID, "not an address")
		is.Error(t, model.ErrorEmailInvalid, err)
		_, err = p.fat.Invite(t.Context(), signe.ID, "SIGNE@example.com")
		is.Error(t, model.ErrorAlreadyMember, err)
		_, err = p.fat.Invite(t.Context(), signe.ID, "karen@example.com")
		is.Error(t, model.ErrorFormerMember, err)
		is.Equal(t, 0, len(p.runJobs(t)))
	})

	t.Run("replaces an earlier invitation to the same address, whose link then no longer works", func(t *testing.T) {
		p := newPeople(t, "", nil)
		signe := p.person(t, "Signe", model.RoleModerator)
		markus := p.person(t, "Markus", model.RoleAdmin)
		_, first := p.invite(t, signe, "amira@example.com")
		_, second := p.invite(t, markus, "amira@example.com")

		_, err := p.fat.AcceptInvitation(t.Context(), first, "Amira")
		is.Error(t, model.ErrorLinkInvalid, err)
		check, err := p.fat.CheckInvitation(t.Context(), second)
		is.NotError(t, err)
		is.Equal(t, "Markus", check.InvitedByName)

		invs, err := p.fat.GetInvitations(t.Context(), signe.ID)
		is.NotError(t, err)
		is.Equal(t, 1, len(invs))
		is.Equal(t, markus.ID, invs[0].InvitedBy)
	})

	t.Run("sends nothing for an invitation withdrawn or replaced before its email went", func(t *testing.T) {
		p := newPeople(t, "", nil)
		signe := p.person(t, "Signe", model.RoleModerator)
		a, err := p.fat.Invite(t.Context(), signe.ID, "a@example.com")
		is.NotError(t, err)
		is.NotError(t, p.fat.WithdrawInvitation(t.Context(), signe.ID, a.ID))
		_, err = p.fat.Invite(t.Context(), signe.ID, "b@example.com")
		is.NotError(t, err)
		_, err = p.fat.Invite(t.Context(), signe.ID, "b@example.com")
		is.NotError(t, err)

		sent := p.runJobs(t)
		is.Equal(t, 1, len(sent))
		is.Equal(t, model.EmailAddress("b@example.com"), sent[0].To)
	})
}

func TestFat_GetInvitations(t *testing.T) {
	t.Run("lets moderators and admins see the invitations that are waiting, with their addresses, and members not", func(t *testing.T) {
		p := newPeople(t, "", nil)
		signe := p.person(t, "Signe", model.RoleModerator)
		markus := p.person(t, "Markus", model.RoleAdmin)
		thomas := p.person(t, "Thomas", model.RoleMember)
		p.invite(t, signe, "amira@example.com")

		for _, actor := range []model.User{signe, markus} {
			invs, err := p.fat.GetInvitations(t.Context(), actor.ID)
			is.NotError(t, err)
			is.Equal(t, 1, len(invs))
			is.Equal(t, model.EmailAddress("amira@example.com"), invs[0].Email)
			is.Equal(t, "Signe", invs[0].InvitedByName)
		}

		invs, err := p.fat.GetInvitations(t.Context(), thomas.ID)
		is.Error(t, model.ErrorNotAllowed, err)
		is.Equal(t, 0, len(invs))
	})

	t.Run("leaves out invitations that have expired", func(t *testing.T) {
		p := newPeople(t, "", nil)
		signe := p.person(t, "Signe", model.RoleModerator)
		p.invite(t, signe, "amira@example.com")

		p.clock.t = base.Add(7 * 24 * time.Hour)
		invs, err := p.fat.GetInvitations(t.Context(), signe.ID)
		is.NotError(t, err)
		is.Equal(t, 0, len(invs))
	})
}

func TestFat_WithdrawInvitation(t *testing.T) {
	t.Run("lets a moderator withdraw an invitation, whose link then no longer works, and refuses a member", func(t *testing.T) {
		p := newPeople(t, "", nil)
		signe := p.person(t, "Signe", model.RoleModerator)
		thomas := p.person(t, "Thomas", model.RoleMember)
		inv, token := p.invite(t, signe, "amira@example.com")

		is.Error(t, model.ErrorNotAllowed, p.fat.WithdrawInvitation(t.Context(), thomas.ID, inv.ID))
		_, err := p.fat.CheckInvitation(t.Context(), token)
		is.NotError(t, err)

		is.NotError(t, p.fat.WithdrawInvitation(t.Context(), signe.ID, inv.ID))
		_, err = p.fat.CheckInvitation(t.Context(), token)
		is.Error(t, model.ErrorLinkInvalid, err)
		_, err = p.fat.AcceptInvitation(t.Context(), token, "Amira")
		is.Error(t, model.ErrorLinkInvalid, err)
		is.Error(t, model.ErrorInvitationNotFound, p.fat.WithdrawInvitation(t.Context(), signe.ID, inv.ID))
	})
}

func TestFat_AcceptInvitation(t *testing.T) {
	t.Run("has the invitee join as an active member with their name, invited by who sent it, once, and nothing posted before is new to them", func(t *testing.T) {
		p := newPeople(t, "", nil)
		signe := p.person(t, "Signe", model.RoleModerator)
		_, link := p.invite(t, signe, "amira@example.com")
		_, err := p.fat.CreatePost(t.Context(), signe.ID, token(t), "Before")
		is.NotError(t, err)

		p.clock.t = base.Add(time.Hour)
		_, err = p.fat.AcceptInvitation(t.Context(), link, "\t")
		is.Error(t, model.ErrorNameEmpty, err)
		amira, err := p.fat.AcceptInvitation(t.Context(), link, " Amira ")
		is.NotError(t, err)
		is.Equal(t, "Amira", amira.Name)
		is.Equal(t, model.RoleMember, amira.Role)
		is.Equal(t, signe.ID, *amira.InvitedBy)
		is.True(t, amira.Created.T.Equal(base.Add(time.Hour)))

		_, err = p.fat.AcceptInvitation(t.Context(), link, "Amira")
		is.Error(t, model.ErrorLinkInvalid, err)
		is.Equal(t, 2, count(t, p.db, "users"))

		_, err = p.fat.CreatePost(t.Context(), signe.ID, token(t), "After")
		is.NotError(t, err)
		timeline, err := p.fat.GetTimeline(t.Context(), amira.ID, "", 0)
		is.NotError(t, err)
		posts := timeline.Days[0].Posts
		is.Equal(t, "After", posts[0].Body)
		is.True(t, posts[0].New)
		is.Equal(t, "Before", posts[1].Body)
		is.True(t, !posts[1].New)
	})

	t.Run("refuses garbage and a sign-in link with the one refusal", func(t *testing.T) {
		p := newPeople(t, "", nil)
		p.person(t, "Signe", model.RoleModerator)
		token := p.link(t, "signe@example.com")

		for _, s := range []string{token, "", "garbage"} {
			_, err := p.fat.CheckInvitation(t.Context(), s)
			is.Error(t, model.ErrorLinkInvalid, err, s)
			_, err = p.fat.AcceptInvitation(t.Context(), s, "Amira")
			is.Error(t, model.ErrorLinkInvalid, err, s)
		}
	})
}

func TestFat_ListMembers(t *testing.T) {
	t.Run("gives every member the names and roles of the active members, and only admins their email addresses", func(t *testing.T) {
		p := newPeople(t, "", nil)
		markus := p.person(t, "Markus", model.RoleAdmin)
		signe := p.person(t, "Signe", model.RoleModerator)
		thomas := p.person(t, "Thomas", model.RoleMember)
		karen := p.person(t, "Karen", model.RoleMember)
		sqlitetest.DeactivateUser(t, p.db, karen.ID)

		for _, actor := range []model.User{markus, signe, thomas} {
			members, err := p.fat.ListMembers(t.Context(), actor.ID)
			is.NotError(t, err)
			is.Equal(t, 3, len(members))
			is.Equal(t, "Markus", members[0].Name)
			is.Equal(t, model.RoleAdmin, members[0].Role)
			is.Equal(t, "Signe", members[1].Name)
			is.Equal(t, model.RoleModerator, members[1].Role)
			is.Equal(t, "Thomas", members[2].Name)
			is.Equal(t, model.RoleMember, members[2].Role)
			for _, m := range members {
				if actor.Role == model.RoleAdmin {
					is.Equal(t, model.EmailAddress(strings.ToLower(m.Name)+"@example.com"), m.Email)
				} else {
					is.Equal(t, model.EmailAddress(""), m.Email)
				}
			}
		}

		_, err := p.fat.ListMembers(t.Context(), karen.ID)
		is.Error(t, model.ErrorUserInactive, err)
	})
}

func TestFat_ChangeRole(t *testing.T) {
	t.Run("lets an admin change another person's role to any role", func(t *testing.T) {
		p := newPeople(t, "", nil)
		markus := p.person(t, "Markus", model.RoleAdmin)
		signe := p.person(t, "Signe", model.RoleMember)

		for _, role := range []model.Role{model.RoleModerator, model.RoleAdmin, model.RoleMember, model.RoleMember} {
			is.NotError(t, p.fat.ChangeRole(t.Context(), markus.ID, signe.ID, role.String()))
			u, err := p.db.GetUser(t.Context(), signe.ID)
			is.NotError(t, err)
			is.Equal(t, role, u.Role)
		}
	})

	t.Run("lets nobody change their own role, and no moderator or member anyone's", func(t *testing.T) {
		p := newPeople(t, "", nil)
		markus := p.person(t, "Markus", model.RoleAdmin)
		signe := p.person(t, "Signe", model.RoleModerator)
		thomas := p.person(t, "Thomas", model.RoleMember)

		is.Error(t, model.ErrorNotAllowed, p.fat.ChangeRole(t.Context(), markus.ID, markus.ID, "member"))
		is.Error(t, model.ErrorNotAllowed, p.fat.ChangeRole(t.Context(), signe.ID, signe.ID, "admin"))
		is.Error(t, model.ErrorNotAllowed, p.fat.ChangeRole(t.Context(), signe.ID, thomas.ID, "moderator"))
		is.Error(t, model.ErrorNotAllowed, p.fat.ChangeRole(t.Context(), thomas.ID, thomas.ID, "admin"))
		is.Error(t, model.ErrorNotAllowed, p.fat.ChangeRole(t.Context(), thomas.ID, markus.ID, "member"))
		// Someone who may not change roles learns nothing about whether the person is there.
		is.Error(t, model.ErrorNotAllowed, p.fat.ChangeRole(t.Context(), thomas.ID, "u_nope", "member"))

		for _, u := range []model.User{markus, signe, thomas} {
			after, err := p.db.GetUser(t.Context(), u.ID)
			is.NotError(t, err)
			is.Equal(t, u.Role, after.Role)
		}
	})

	t.Run("leaves an admin when two admins take each other's role away, one after the other", func(t *testing.T) {
		p := newPeople(t, "", nil)
		markus := p.person(t, "Markus", model.RoleAdmin)
		signe := p.person(t, "Signe", model.RoleAdmin)

		is.NotError(t, p.fat.ChangeRole(t.Context(), markus.ID, signe.ID, "member"))
		is.Error(t, model.ErrorNotAllowed, p.fat.ChangeRole(t.Context(), signe.ID, markus.ID, "member"))
		is.Equal(t, 1, count(t, p.db, "users where role = 'admin' and active = 1"))
	})

	t.Run("refuses a role that is not one, someone inactive, and someone who is not an active member", func(t *testing.T) {
		p := newPeople(t, "", nil)
		markus := p.person(t, "Markus", model.RoleAdmin)
		signe := p.person(t, "Signe", model.RoleMember)
		karen := p.person(t, "Karen", model.RoleAdmin)
		sqlitetest.DeactivateUser(t, p.db, karen.ID)

		is.Error(t, model.ErrorRoleInvalid, p.fat.ChangeRole(t.Context(), markus.ID, signe.ID, "owner"))
		is.Error(t, model.ErrorUserInactive, p.fat.ChangeRole(t.Context(), karen.ID, signe.ID, "admin"))
		is.Error(t, model.ErrorMemberNotFound, p.fat.ChangeRole(t.Context(), markus.ID, karen.ID, "member"))
		is.Error(t, model.ErrorMemberNotFound, p.fat.ChangeRole(t.Context(), markus.ID, "u_nope", "member"))
		is.Error(t, model.ErrorUserNotFound, p.fat.ChangeRole(t.Context(), "u_nope", signe.ID, "admin"))
	})
}

func TestFat_people_telemetry(t *testing.T) {
	t.Run("records IDs, and never an email address, a token, a link, or a name, in logs or traces", func(t *testing.T) {
		spans := oteltest.NewSpanRecorder(t)
		var logs bytes.Buffer
		p := newPeople(t, "markus@example.com", slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))

		// Everything there is to do, the refusals included.
		markusToken := p.link(t, "Markus@example.com")
		markus, err := p.fat.SignIn(t.Context(), markusToken, "Markus Mørk")
		is.NotError(t, err)
		is.NotError(t, p.fat.RequestSignInLink(t.Context(), "stranger@example.com"))
		_ = p.runJobs(t)
		inv, inviteToken := p.invite(t, markus, "amira@example.com")
		_, err = p.fat.CheckInvitation(t.Context(), inviteToken)
		is.NotError(t, err)
		amira, err := p.fat.AcceptInvitation(t.Context(), inviteToken, "Amira Haddad")
		is.NotError(t, err)
		_, err = p.fat.AcceptInvitation(t.Context(), inviteToken, "Amira Haddad")
		is.Error(t, model.ErrorLinkInvalid, err)
		is.NotError(t, p.fat.ChangeRole(t.Context(), markus.ID, amira.ID, "moderator"))
		amiraToken := p.link(t, "amira@example.com")
		_, err = p.fat.SignIn(t.Context(), amiraToken, "")
		is.NotError(t, err)
		_, err = p.fat.SignIn(t.Context(), amiraToken, "")
		is.Error(t, model.ErrorLinkInvalid, err)
		_, err = p.fat.Invite(t.Context(), amira.ID, "markus@example.com")
		is.Error(t, model.ErrorAlreadyMember, err)
		other, _ := p.invite(t, amira, "jonas@example.com")
		is.NotError(t, p.fat.WithdrawInvitation(t.Context(), amira.ID, other.ID))
		_, err = p.fat.ListMembers(t.Context(), markus.ID)
		is.NotError(t, err)
		_, err = p.fat.GetInvitations(t.Context(), markus.ID)
		is.NotError(t, err)

		secrets := []string{"markus@example.com", "amira@example.com", "jonas@example.com", "stranger@example.com", "Markus", "Amira", "Mørk", "Haddad",
			markusToken, inviteToken, amiraToken, "sign-in/link", "invitation?token"}
		for _, s := range []string{markusToken, inviteToken, amiraToken} {
			secrets = append(secrets, model.LinkToken(s).Hash())
		}

		for _, s := range secrets {
			is.True(t, !strings.Contains(logs.String(), s), "the logs hold "+s+":\n"+logs.String())
		}
		is.True(t, strings.Contains(logs.String(), markus.ID.String()), logs.String())
		is.True(t, strings.Contains(logs.String(), inv.ID.String()), logs.String())

		ended := spans.Ended()
		is.True(t, len(ended) > 20, "expected the operations to be traced")
		for _, span := range ended {
			texts := []string{span.Name(), span.Status().Description}
			attributes := span.Attributes()
			for _, e := range span.Events() {
				texts = append(texts, e.Name)
				attributes = append(attributes, e.Attributes...)
			}
			for _, a := range attributes {
				texts = append(texts, string(a.Key), a.Value.String())
			}
			for _, text := range texts {
				for _, s := range secrets {
					is.True(t, !strings.Contains(text, s), "span "+span.Name()+" holds "+s+" in "+text)
				}
			}
		}
		is.True(t, hasAttribute(ended, attribute.String("user.id", markus.ID.String())))
		is.True(t, hasAttribute(ended, attribute.String("invitation.id", inv.ID.String())))
	})
}

// hasAttribute reports whether any of the spans has the attribute.
func hasAttribute(spans []sdktrace.ReadOnlySpan, a attribute.KeyValue) bool {
	for _, s := range spans {
		if oteltest.HasAttribute(s.Attributes(), a) {
			return true
		}
	}
	return false
}

// count the rows of the given table and condition, such as "users where active = 1".
func count(t *testing.T, db *sqlite.Database, from string) int {
	t.Helper()

	var n int
	is.NotError(t, db.H.Get(t.Context(), &n, "select count(*) from "+from))
	return n
}

func TestFat_peopleHistory(t *testing.T) {
	t.Run("signs in, invites, and changes roles as the rules say, after any sequence of actions by several people", func(t *testing.T) {
		peopletest.Test(t, &peopleDriver{db: sqlitetest.NewDatabase(t)}, 30)
	})
}

// peopleDriver runs a history of people against the business logic on a real database, with its jobs.
type peopleDriver struct {
	t      testing.TB
	db     *sqlite.Database
	fat    *service.Fat
	sender *servicetest.Sender
	clock  *clock
	forms  uint64
}

func (d *peopleDriver) Reset(t testing.TB, now time.Time, firstAdmin model.EmailAddress, seed [32]byte) {
	d.t = t
	for _, query := range []string{
		"update users set invited_by = null", "delete from sign_in_links", "delete from invitations",
		"delete from comments", "delete from posts", "delete from users", "delete from accounts",
	} {
		if err := d.db.H.Exec(t.Context(), query); err != nil {
			t.Fatal(err)
		}
	}
	jobstest.Take(t, d.db)

	d.clock = &clock{t: now}
	d.sender = &servicetest.Sender{}
	d.fat = service.NewFat(service.NewFatOptions{})
	service.Setup(d.fat, nil, d.db, d.sender, d.clock.Now, rand.NewChaCha8(seed), model.Group{Name: "The Loft", TimeZone: time.UTC}, firstAdmin)
}

func (d *peopleDriver) SetClock(now time.Time) {
	d.clock.t = now
}

func (d *peopleDriver) Deactivate(id model.UserID) {
	sqlitetest.DeactivateUser(d.t, d.db, id)
}

func (d *peopleDriver) People() []peopletest.Person {
	var users []model.User
	if err := d.db.H.Select(d.t.Context(), &users, "select * from users order by rowid"); err != nil {
		d.t.Fatal(err)
	}
	var people []peopletest.Person
	for _, u := range users {
		people = append(people, peopletest.Person{ID: u.ID, Name: u.Name, Email: u.Email, Role: u.Role, Active: u.Active, InvitedBy: u.InvitedBy})
	}
	return people
}

func (d *peopleDriver) Emails() []peopletest.Email {
	jobstest.Run(d.t, d.db, jobs.Funcs(jobs.RegisterOpts{Emails: d.fat}))
	var sent []peopletest.Email
	for _, e := range d.sender.Take() {
		sent = append(sent, peopletest.Email{To: e.To, Template: e.Template, Subject: e.Subject, Token: e.Keywords["token"]})
	}
	return sent
}

// answer is the given error if it is one from the business domain, and fails the test otherwise.
func (d *peopleDriver) answer(err error) error {
	var domainErr model.Error
	if err != nil && !errors.As(err, &domainErr) {
		d.t.Fatal(err)
	}
	return err
}

func (d *peopleDriver) RequestSignInLink(address string) error {
	return d.answer(d.fat.RequestSignInLink(d.t.Context(), address))
}

func (d *peopleDriver) CheckSignInLink(token string) (bool, error) {
	check, err := d.fat.CheckSignInLink(d.t.Context(), token)
	return check.Joining, d.answer(err)
}

func (d *peopleDriver) SignIn(token, name string) (model.UserID, error) {
	u, err := d.fat.SignIn(d.t.Context(), token, name)
	return u.ID, d.answer(err)
}

func (d *peopleDriver) Invite(actor model.UserID, address string) (model.InvitationID, error) {
	inv, err := d.fat.Invite(d.t.Context(), actor, address)
	return inv.ID, d.answer(err)
}

func (d *peopleDriver) GetInvitations(actor model.UserID) ([]model.Invitation, error) {
	invs, err := d.fat.GetInvitations(d.t.Context(), actor)
	return invs, d.answer(err)
}

func (d *peopleDriver) WithdrawInvitation(actor model.UserID, id model.InvitationID) error {
	return d.answer(d.fat.WithdrawInvitation(d.t.Context(), actor, id))
}

func (d *peopleDriver) CheckInvitation(token string) (string, error) {
	check, err := d.fat.CheckInvitation(d.t.Context(), token)
	return check.InvitedByName, d.answer(err)
}

func (d *peopleDriver) AcceptInvitation(token, name string) (model.UserID, error) {
	u, err := d.fat.AcceptInvitation(d.t.Context(), token, name)
	return u.ID, d.answer(err)
}

func (d *peopleDriver) ChangeRole(actor, target model.UserID, role string) error {
	return d.answer(d.fat.ChangeRole(d.t.Context(), actor, target, role))
}

func (d *peopleDriver) ListMembers(actor model.UserID) ([]model.Member, error) {
	members, err := d.fat.ListMembers(d.t.Context(), actor)
	return members, d.answer(err)
}

func (d *peopleDriver) Post(actor model.UserID) (model.PostID, error) {
	d.forms++
	var b [16]byte
	for i := range 8 {
		b[i] = byte(d.forms >> (8 * i))
	}
	form, err := model.NewFormToken(bytes.NewReader(b[:]))
	if err != nil {
		d.t.Fatal(err)
	}
	p, err := d.fat.CreatePost(d.t.Context(), actor, form.String(), "Note")
	return p.ID, d.answer(err)
}

func (d *peopleDriver) Look(actor model.UserID) ([]peopletest.Post, error) {
	timeline, err := d.fat.GetTimeline(d.t.Context(), actor, "", 100)
	var posts []peopletest.Post
	for _, day := range timeline.Days {
		for _, p := range day.Posts {
			posts = append(posts, peopletest.Post{ID: p.ID, New: p.New})
		}
	}
	return posts, d.answer(err)
}
