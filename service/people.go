package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"maragu.dev/glue/email"

	"app/model"
)

// emailSender sends an email, such as a sign-in link.
type emailSender interface {
	SendTransactional(ctx context.Context, opts email.SendOptions) error
}

// jobCreator is the store a job is created in.
type jobCreator interface {
	CreateJob(ctx context.Context, name model.JobName, data any) error
}

// RequestSignInLink wires [Fat.RequestSignInLink] to the given store.
//
// Panics if the operation is already wired.
func RequestSignInLink(f *Fat, db jobCreator) {
	if f.requestSignInLink != nil {
		panic("service: RequestSignInLink already wired")
	}

	f.requestSignInLink = func(ctx context.Context, address string) error {
		if !model.Allowed(nil, model.ActionSignIn, nil) {
			return model.ErrorNotAllowed
		}

		e, err := model.ParseEmailAddress(address)
		if err != nil {
			return err
		}

		// Whether the address gets a link is decided in the job, so that the answer and the time it takes
		// are the same whoever the address is.
		return db.CreateJob(ctx, model.JobNameSendSignInLink, model.SendSignInLinkJobData{Email: e})
	}
}

// RequestSignInLink for the given email address, by anyone, signed in or not. An address that is not one
// by [model.ParseEmailAddress] is [model.ErrorEmailInvalid]. Any other address gets the same answer, nil,
// after the same work: a job of [model.JobNameSendSignInLink] for the address, which [Fat.SendSignInLink]
// does. So nobody can tell from asking whether an address is in the group.
//
// Panics unless the operation was wired, by [Setup] or by the function of the same name.
func (f *Fat) RequestSignInLink(ctx context.Context, address string) error {
	if f.requestSignInLink == nil {
		panic("service: RequestSignInLink not wired; call service.RequestSignInLink or service.Setup")
	}

	ctx, span := f.tracer.Start(ctx, "service.RequestSignInLink")
	defer span.End()

	err := f.requestSignInLink(ctx, address)
	recordError(span, err)
	return err
}

// signInLinkSender is the store a sign-in link is made in.
type signInLinkSender interface {
	GetSignInTarget(ctx context.Context, email, firstAdmin model.EmailAddress) (model.User, bool, error)
	CreateSignInLink(ctx context.Context, l model.SignInLink, limit int, since model.Time) (bool, error)
	DeleteSignInLink(ctx context.Context, hash string) error
}

// SendSignInLink wires [Fat.SendSignInLink] to the given store, sender, clock, and randomness, for the
// given group, and the given address of the first admin, which is empty if there is none.
// The randomness must be safe for concurrent use, as [crypto/rand.Reader] is.
//
// Panics if the operation is already wired.
func SendSignInLink(f *Fat, db signInLinkSender, sender emailSender, now func() time.Time, random io.Reader, group model.Group, firstAdmin model.EmailAddress) {
	if f.sendSignInLink != nil {
		panic("service: SendSignInLink already wired")
	}

	f.sendSignInLink = func(ctx context.Context, address model.EmailAddress) error {
		span := trace.SpanFromContext(ctx)

		u, joining, err := db.GetSignInTarget(ctx, address, firstAdmin)
		if errors.Is(err, model.ErrorLinkInvalid) {
			span.SetAttributes(attribute.String("sign_in.outcome", "not sent: not a member"))
			f.log.InfoContext(ctx, "Sent no sign-in link, since the address is not an active member's")
			return nil
		}
		if err != nil {
			return err
		}
		if !joining {
			span.SetAttributes(attribute.String("user.id", u.ID.String()))
		}

		token, err := model.NewLinkToken(random)
		if err != nil {
			return err
		}

		t := now()
		created, err := db.CreateSignInLink(ctx, model.SignInLink{
			Hash:    token.Hash(),
			Created: model.Time{T: t},
			Expires: model.Time{T: t.Add(model.SignInLinkLifetime)},
			Email:   address,
		}, model.SignInLinksPerHour, model.Time{T: t.Add(-time.Hour)})
		if err != nil {
			return err
		}
		if !created {
			span.SetAttributes(attribute.String("sign_in.outcome", "not sent: too many"))
			f.log.InfoContext(ctx, "Sent no sign-in link, since the address had too many in the last hour", "userID", u.ID)
			return nil
		}

		err = sender.SendTransactional(ctx, email.SendOptions{
			Keywords:  model.Keywords{"token": token.String(), "group": group.Name, "lifetime": lifetime(model.SignInLinkLifetime)},
			Preheader: "Your link to sign in to " + group.Name + ". It works once, for the next " + lifetime(model.SignInLinkLifetime) + ".",
			Subject:   "Sign in to " + group.Name,
			Template:  "sign-in",
			To:        address,
			ToName:    u.Name,
		})
		if err != nil {
			// A link that was not sent does not count towards the limit, so that trying again can send one.
			if deleteErr := db.DeleteSignInLink(ctx, token.Hash()); deleteErr != nil {
				return errors.Join(err, deleteErr)
			}
			return err
		}

		if joining {
			span.SetAttributes(attribute.String("sign_in.outcome", "sent: first admin"))
			f.log.InfoContext(ctx, "Sent a sign-in link to the first admin, who has not joined yet")
			return nil
		}
		span.SetAttributes(attribute.String("sign_in.outcome", "sent"))
		f.log.InfoContext(ctx, "Sent a sign-in link", "userID", u.ID)
		return nil
	}
}

// SendSignInLink to the given address, which [model.ParseEmailAddress] returned, if it is the address of
// an active person, or the first admin's while the group has nobody in it. Any other address gets
// nothing, and so does an address that was sent [model.SignInLinksPerHour] links in the hour before. A
// link whose email could not be sent is not kept, and does not count.
// The link works once, for [model.SignInLinkLifetime], and only the hash of its token is stored.
//
// Panics unless the operation was wired, by [Setup] or by the function of the same name.
func (f *Fat) SendSignInLink(ctx context.Context, address model.EmailAddress) error {
	if f.sendSignInLink == nil {
		panic("service: SendSignInLink not wired; call service.SendSignInLink or service.Setup")
	}

	ctx, span := f.tracer.Start(ctx, "service.SendSignInLink")
	defer span.End()

	err := f.sendSignInLink(ctx, address)
	recordError(span, err)
	return err
}

// signInLinkChecker is the store a sign-in link is looked at in.
type signInLinkChecker interface {
	CheckSignInLink(ctx context.Context, hash string, now model.Time, firstAdmin model.EmailAddress) (model.SignInLinkCheck, error)
}

// CheckSignInLink wires [Fat.CheckSignInLink] to the given store and clock, with the given address of the
// first admin.
//
// Panics if the operation is already wired.
func CheckSignInLink(f *Fat, db signInLinkChecker, now func() time.Time, firstAdmin model.EmailAddress) {
	if f.checkSignInLink != nil {
		panic("service: CheckSignInLink already wired")
	}

	f.checkSignInLink = func(ctx context.Context, token string) (model.SignInLinkCheck, error) {
		if !model.Allowed(nil, model.ActionSignIn, nil) {
			return model.SignInLinkCheck{}, model.ErrorNotAllowed
		}

		t, err := model.ParseLinkToken(token)
		if err != nil {
			return model.SignInLinkCheck{}, err
		}
		return db.CheckSignInLink(ctx, t.Hash(), model.Time{T: now()}, firstAdmin)
	}
}

// CheckSignInLink with the given token, by anyone: whether signing in with it would work now, without
// using it up, and whether the person would join the group by it, so that they choose their name first.
// A link that is used up, has expired, is to someone who is not active, or was never made, is
// [model.ErrorLinkInvalid], whichever it is.
//
// Panics unless the operation was wired, by [Setup] or by the function of the same name.
func (f *Fat) CheckSignInLink(ctx context.Context, token string) (model.SignInLinkCheck, error) {
	if f.checkSignInLink == nil {
		panic("service: CheckSignInLink not wired; call service.CheckSignInLink or service.Setup")
	}

	ctx, span := f.tracer.Start(ctx, "service.CheckSignInLink")
	defer span.End()

	check, err := f.checkSignInLink(ctx, token)
	recordError(span, err)
	return check, err
}

// signer is the store a sign-in link is used in.
type signer interface {
	signInLinkChecker
	SignIn(ctx context.Context, hash string, now model.Time, firstAdmin model.EmailAddress, name string) (model.User, error)
}

// SignIn wires [Fat.SignIn] to the given store and clock, with the given address of the first admin.
//
// Panics if the operation is already wired.
func SignIn(f *Fat, db signer, now func() time.Time, firstAdmin model.EmailAddress) {
	if f.signIn != nil {
		panic("service: SignIn already wired")
	}

	f.signIn = func(ctx context.Context, token, name string) (model.User, error) {
		if !model.Allowed(nil, model.ActionSignIn, nil) {
			return model.User{}, model.ErrorNotAllowed
		}

		t, err := model.ParseLinkToken(token)
		if err != nil {
			return model.User{}, err
		}

		at := model.Time{T: now()}
		check, err := db.CheckSignInLink(ctx, t.Hash(), at, firstAdmin)
		if err != nil {
			return model.User{}, err
		}

		var cleaned string
		if check.Joining {
			if cleaned, err = model.CleanName(name); err != nil {
				return model.User{}, err
			}
		}

		u, err := db.SignIn(ctx, t.Hash(), at, firstAdmin, cleaned)
		if err != nil {
			return model.User{}, err
		}

		trace.SpanFromContext(ctx).SetAttributes(attribute.String("user.id", u.ID.String()), attribute.Bool("sign_in.joined", check.Joining))
		if check.Joining {
			f.log.InfoContext(ctx, "The first admin joined the group", "userID", u.ID)
		} else {
			f.log.InfoContext(ctx, "Signed in", "userID", u.ID)
		}
		return u, nil
	}
}

// SignIn with the sign-in link with the given token, by anyone, and return who it signs in. The link is
// used up, and so are all other links to the same address. A link that does not work, by the rules of
// [Fat.CheckSignInLink], is [model.ErrorLinkInvalid], whichever the reason.
//
// If the link is the first admin's, they join the group as an active admin with the given name, cleaned
// by [model.CleanName], and a name that is refused uses up nothing. Anyone else's name is as it is, and
// the given name is not used.
//
// Panics unless the operation was wired, by [Setup] or by the function of the same name.
func (f *Fat) SignIn(ctx context.Context, token, name string) (model.User, error) {
	if f.signIn == nil {
		panic("service: SignIn not wired; call service.SignIn or service.Setup")
	}

	ctx, span := f.tracer.Start(ctx, "service.SignIn")
	defer span.End()

	u, err := f.signIn(ctx, token, name)
	recordError(span, err)
	return u, err
}

// inviter is the store an invitation is made in, together with the job of
// [model.JobNameSendInvitation] that sends it.
type inviter interface {
	userGetter
	CreateInvitation(ctx context.Context, inv model.Invitation) (model.Invitation, error)
}

// Invite wires [Fat.Invite] to the given store and clock.
//
// Panics if the operation is already wired.
func Invite(f *Fat, db inviter, now func() time.Time) {
	if f.invite != nil {
		panic("service: Invite already wired")
	}

	f.invite = func(ctx context.Context, actor model.UserID, address string) (model.Invitation, error) {
		if _, err := authorize(ctx, db, actor, model.ActionInvite); err != nil {
			return model.Invitation{}, err
		}

		e, err := model.ParseEmailAddress(address)
		if err != nil {
			return model.Invitation{}, err
		}

		t := now()
		inv, err := db.CreateInvitation(ctx, model.Invitation{
			Created:   model.Time{T: t},
			Expires:   model.Time{T: t.Add(model.InvitationLifetime)},
			Email:     e,
			InvitedBy: actor,
		})
		if err != nil {
			return model.Invitation{}, err
		}

		trace.SpanFromContext(ctx).SetAttributes(attribute.String("invitation.id", inv.ID.String()))
		f.log.InfoContext(ctx, "Invited someone", "invitationID", inv.ID, "invitedBy", actor)
		return inv, nil
	}
}

// Invite the given email address to the group, as the acting user, who must be active and may invite by
// [model.Allowed]. The invitation works once, for [model.InvitationLifetime], and is sent by
// [Fat.SendInvitation] in a job. It replaces the invitation that was waiting for the address, if there was
// one, which then no longer works.
//
// An address that is not one is [model.ErrorEmailInvalid]. An active member's address is
// [model.ErrorAlreadyMember], and the address of someone who is no longer active is
// [model.ErrorFormerMember]. An actor who may not invite is [model.ErrorNotAllowed],
// [model.ErrorUserInactive], or [model.ErrorUserNotFound].
//
// Panics unless the operation was wired, by [Setup] or by the function of the same name.
func (f *Fat) Invite(ctx context.Context, actor model.UserID, address string) (model.Invitation, error) {
	if f.invite == nil {
		panic("service: Invite not wired; call service.Invite or service.Setup")
	}

	ctx, span := f.tracer.Start(ctx, "service.Invite")
	defer span.End()

	inv, err := f.invite(ctx, actor, address)
	recordError(span, err)
	return inv, err
}

// invitationSender is the store an invitation's link is made in.
type invitationSender interface {
	SetInvitationHash(ctx context.Context, id model.InvitationID, hash string, now model.Time) (model.Invitation, error)
}

// SendInvitation wires [Fat.SendInvitation] to the given store, sender, clock, and randomness, for the
// given group. The randomness must be safe for concurrent use, as [crypto/rand.Reader] is.
//
// Panics if the operation is already wired.
func SendInvitation(f *Fat, db invitationSender, sender emailSender, now func() time.Time, random io.Reader, group model.Group) {
	if f.sendInvitation != nil {
		panic("service: SendInvitation already wired")
	}

	f.sendInvitation = func(ctx context.Context, id model.InvitationID) error {
		span := trace.SpanFromContext(ctx)
		span.SetAttributes(attribute.String("invitation.id", id.String()))

		token, err := model.NewLinkToken(random)
		if err != nil {
			return err
		}

		inv, err := db.SetInvitationHash(ctx, id, token.Hash(), model.Time{T: now()})
		if errors.Is(err, model.ErrorInvitationNotFound) {
			span.SetAttributes(attribute.String("invitation.outcome", "not sent: gone"))
			f.log.InfoContext(ctx, "Sent no invitation, since it was withdrawn, replaced, accepted, or expired", "invitationID", id)
			return nil
		}
		if err != nil {
			return err
		}

		err = sender.SendTransactional(ctx, email.SendOptions{
			Keywords:  model.Keywords{"token": token.String(), "group": group.Name, "inviter": inv.InvitedByName, "lifetime": lifetime(model.InvitationLifetime)},
			Preheader: inv.InvitedByName + " has invited you to join " + group.Name + ". The link works for " + lifetime(model.InvitationLifetime) + ".",
			Subject:   inv.InvitedByName + " invited you to " + group.Name,
			Template:  "invitation",
			To:        inv.Email,
		})
		if err != nil {
			return err
		}

		span.SetAttributes(attribute.String("invitation.outcome", "sent"))
		f.log.InfoContext(ctx, "Sent an invitation", "invitationID", id, "invitedBy", inv.InvitedBy)
		return nil
	}
}

// SendInvitation with the given ID, if it is still waiting and has not expired, with a new link. Only the
// hash of the link's token is stored, and a link sent for it before no longer works. An invitation that
// is gone sends nothing.
//
// Panics unless the operation was wired, by [Setup] or by the function of the same name.
func (f *Fat) SendInvitation(ctx context.Context, id model.InvitationID) error {
	if f.sendInvitation == nil {
		panic("service: SendInvitation not wired; call service.SendInvitation or service.Setup")
	}

	ctx, span := f.tracer.Start(ctx, "service.SendInvitation")
	defer span.End()

	err := f.sendInvitation(ctx, id)
	recordError(span, err)
	return err
}

// invitationsGetter is the store invitations are listed from.
type invitationsGetter interface {
	userGetter
	GetInvitations(ctx context.Context, now model.Time) ([]model.Invitation, error)
}

// GetInvitations wires [Fat.GetInvitations] to the given store and clock.
//
// Panics if the operation is already wired.
func GetInvitations(f *Fat, db invitationsGetter, now func() time.Time) {
	if f.getInvitations != nil {
		panic("service: GetInvitations already wired")
	}

	f.getInvitations = func(ctx context.Context, actor model.UserID) ([]model.Invitation, error) {
		if _, err := authorize(ctx, db, actor, model.ActionSeeInvitations); err != nil {
			return nil, err
		}
		return db.GetInvitations(ctx, model.Time{T: now()})
	}
}

// GetInvitations that are waiting and have not expired, the newest first, with their addresses, for the
// acting user, who must be active and may see them by [model.Allowed].
//
// Panics unless the operation was wired, by [Setup] or by the function of the same name.
func (f *Fat) GetInvitations(ctx context.Context, actor model.UserID) ([]model.Invitation, error) {
	if f.getInvitations == nil {
		panic("service: GetInvitations not wired; call service.GetInvitations or service.Setup")
	}

	ctx, span := f.tracer.Start(ctx, "service.GetInvitations")
	defer span.End()

	invs, err := f.getInvitations(ctx, actor)
	recordError(span, err)
	return invs, err
}

// invitationWithdrawer is the store an invitation is withdrawn in.
type invitationWithdrawer interface {
	userGetter
	DeleteInvitation(ctx context.Context, id model.InvitationID, now model.Time) error
}

// WithdrawInvitation wires [Fat.WithdrawInvitation] to the given store and clock.
//
// Panics if the operation is already wired.
func WithdrawInvitation(f *Fat, db invitationWithdrawer, now func() time.Time) {
	if f.withdrawInvitation != nil {
		panic("service: WithdrawInvitation already wired")
	}

	f.withdrawInvitation = func(ctx context.Context, actor model.UserID, id model.InvitationID) error {
		if _, err := authorize(ctx, db, actor, model.ActionWithdrawInvitation); err != nil {
			return err
		}

		if err := db.DeleteInvitation(ctx, id, model.Time{T: now()}); err != nil {
			return err
		}

		trace.SpanFromContext(ctx).SetAttributes(attribute.String("invitation.id", id.String()))
		f.log.InfoContext(ctx, "Withdrew an invitation", "invitationID", id, "withdrawnBy", actor)
		return nil
	}
}

// WithdrawInvitation with the given ID, as the acting user, who must be active and may withdraw it by
// [model.Allowed], so that its link no longer works. Returns [model.ErrorInvitationNotFound] if there is
// no such invitation, or it has expired.
//
// Panics unless the operation was wired, by [Setup] or by the function of the same name.
func (f *Fat) WithdrawInvitation(ctx context.Context, actor model.UserID, id model.InvitationID) error {
	if f.withdrawInvitation == nil {
		panic("service: WithdrawInvitation not wired; call service.WithdrawInvitation or service.Setup")
	}

	ctx, span := f.tracer.Start(ctx, "service.WithdrawInvitation")
	defer span.End()

	err := f.withdrawInvitation(ctx, actor, id)
	recordError(span, err)
	return err
}

// invitationChecker is the store an invitation is looked at in.
type invitationChecker interface {
	GetInvitationByHash(ctx context.Context, hash string, now model.Time) (model.Invitation, error)
}

// CheckInvitation wires [Fat.CheckInvitation] to the given store and clock.
//
// Panics if the operation is already wired.
func CheckInvitation(f *Fat, db invitationChecker, now func() time.Time) {
	if f.checkInvitation != nil {
		panic("service: CheckInvitation already wired")
	}

	f.checkInvitation = func(ctx context.Context, token string) (model.InvitationCheck, error) {
		if !model.Allowed(nil, model.ActionAcceptInvitation, nil) {
			return model.InvitationCheck{}, model.ErrorNotAllowed
		}

		t, err := model.ParseLinkToken(token)
		if err != nil {
			return model.InvitationCheck{}, err
		}

		inv, err := db.GetInvitationByHash(ctx, t.Hash(), model.Time{T: now()})
		if err != nil {
			return model.InvitationCheck{}, err
		}
		return model.InvitationCheck{InvitedByName: inv.InvitedByName, Expires: inv.Expires}, nil
	}
}

// CheckInvitation with the given token of its link, by anyone: whether accepting it would work now,
// without using it up, and who sent it. An invitation that was accepted, withdrawn, replaced, or has
// expired, and a link that was never made, is [model.ErrorLinkInvalid], whichever it is.
//
// Panics unless the operation was wired, by [Setup] or by the function of the same name.
func (f *Fat) CheckInvitation(ctx context.Context, token string) (model.InvitationCheck, error) {
	if f.checkInvitation == nil {
		panic("service: CheckInvitation not wired; call service.CheckInvitation or service.Setup")
	}

	ctx, span := f.tracer.Start(ctx, "service.CheckInvitation")
	defer span.End()

	check, err := f.checkInvitation(ctx, token)
	recordError(span, err)
	return check, err
}

// invitationAccepter is the store an invitation is accepted in.
type invitationAccepter interface {
	invitationChecker
	AcceptInvitation(ctx context.Context, hash string, now model.Time, name string) (model.User, model.Invitation, error)
}

// AcceptInvitation wires [Fat.AcceptInvitation] to the given store and clock.
//
// Panics if the operation is already wired.
func AcceptInvitation(f *Fat, db invitationAccepter, now func() time.Time) {
	if f.acceptInvitation != nil {
		panic("service: AcceptInvitation already wired")
	}

	f.acceptInvitation = func(ctx context.Context, token, name string) (model.User, error) {
		if !model.Allowed(nil, model.ActionAcceptInvitation, nil) {
			return model.User{}, model.ErrorNotAllowed
		}

		t, err := model.ParseLinkToken(token)
		if err != nil {
			return model.User{}, err
		}

		at := model.Time{T: now()}
		if _, err := db.GetInvitationByHash(ctx, t.Hash(), at); err != nil {
			return model.User{}, err
		}

		cleaned, err := model.CleanName(name)
		if err != nil {
			return model.User{}, err
		}

		u, inv, err := db.AcceptInvitation(ctx, t.Hash(), at, cleaned)
		if err != nil {
			return model.User{}, err
		}

		trace.SpanFromContext(ctx).SetAttributes(attribute.String("user.id", u.ID.String()), attribute.String("invitation.id", inv.ID.String()))
		f.log.InfoContext(ctx, "Accepted an invitation and joined the group", "userID", u.ID, "invitationID", inv.ID, "invitedBy", inv.InvitedBy)
		return u, nil
	}
}

// AcceptInvitation with the given token of its link, by anyone, with the given name, cleaned by
// [model.CleanName], and return who joined. The invitation is used up, and a new active member joins the
// group with the invitation's address and the name, invited by who sent it. They join now, so nothing
// posted or commented before is new to them.
//
// An invitation that does not work, by the rules of [Fat.CheckInvitation], is [model.ErrorLinkInvalid].
// A name that is refused uses up nothing.
//
// Panics unless the operation was wired, by [Setup] or by the function of the same name.
func (f *Fat) AcceptInvitation(ctx context.Context, token, name string) (model.User, error) {
	if f.acceptInvitation == nil {
		panic("service: AcceptInvitation not wired; call service.AcceptInvitation or service.Setup")
	}

	ctx, span := f.tracer.Start(ctx, "service.AcceptInvitation")
	defer span.End()

	u, err := f.acceptInvitation(ctx, token, name)
	recordError(span, err)
	return u, err
}

// membersLister is the store the members are listed from.
type membersLister interface {
	userGetter
	ListMembers(ctx context.Context) ([]model.User, error)
}

// ListMembers wires [Fat.ListMembers] to the given store.
//
// Panics if the operation is already wired.
func ListMembers(f *Fat, db membersLister) {
	if f.listMembers != nil {
		panic("service: ListMembers already wired")
	}

	f.listMembers = func(ctx context.Context, actor model.UserID) ([]model.Member, error) {
		reader, err := authorize(ctx, db, actor, model.ActionRead)
		if err != nil {
			return nil, err
		}

		users, err := db.ListMembers(ctx)
		if err != nil {
			return nil, err
		}

		seeEmails := model.Allowed(&reader, model.ActionSeeEmailAddresses, nil)
		members := make([]model.Member, 0, len(users))
		for _, u := range users {
			m := model.Member{ID: u.ID, Name: u.Name, Role: u.Role, Joined: u.Created}
			if seeEmails {
				m.Email = u.Email
			}
			members = append(members, m)
		}
		return members, nil
	}
}

// ListMembers of the group, who are the active people, in the order they joined, with their names and
// roles, for the acting user, who must be active. Their email addresses are there only if the acting user
// may see them by [model.Allowed], and empty otherwise.
//
// Panics unless the operation was wired, by [Setup] or by the function of the same name.
func (f *Fat) ListMembers(ctx context.Context, actor model.UserID) ([]model.Member, error) {
	if f.listMembers == nil {
		panic("service: ListMembers not wired; call service.ListMembers or service.Setup")
	}

	ctx, span := f.tracer.Start(ctx, "service.ListMembers")
	defer span.End()

	members, err := f.listMembers(ctx, actor)
	recordError(span, err)
	return members, err
}

// roleChanger is the store a role is changed in. It runs the check on the actor and the target in the
// same transaction as the change, as they are there.
type roleChanger interface {
	ChangeRole(ctx context.Context, actor, target model.UserID, role model.Role, check func(actor model.User, target *model.User) error) (model.Role, error)
}

// ChangeRole wires [Fat.ChangeRole] to the given store.
//
// Panics if the operation is already wired.
func ChangeRole(f *Fat, db roleChanger) {
	if f.changeRole != nil {
		panic("service: ChangeRole already wired")
	}

	f.changeRole = func(ctx context.Context, actor, target model.UserID, role string) error {
		// A role that is not one is refused only to someone who may change the target's role.
		r, roleErr := model.ParseRole(role)

		// Two admins changing each other's roles at once cannot leave the group without one, since the
		// check sees the actor as they are when the role changes.
		before, err := db.ChangeRole(ctx, actor, target, r, func(a model.User, t *model.User) error {
			if !a.Active {
				return model.ErrorUserInactive
			}
			probe := t
			if probe == nil {
				probe = &model.User{ID: target}
			}
			if !model.Allowed(&a, model.ActionChangeRole, probe) {
				return model.ErrorNotAllowed
			}
			if t == nil || !t.Active {
				return model.ErrorMemberNotFound
			}
			return roleErr
		})
		if err != nil {
			return err
		}

		trace.SpanFromContext(ctx).SetAttributes(
			attribute.String("role.target_user_id", target.String()),
			attribute.String("role.from", before.String()),
			attribute.String("role.to", r.String()),
		)
		f.log.InfoContext(ctx, "Changed someone's role", "userID", target, "changedBy", actor, "from", before, "to", r)
		return nil
	}
}

// ChangeRole of the target, who must be an active member, to the given role, by its name, as the acting
// user, who must be active and may change the target's role by [model.Allowed]: an admin, and not the
// target. So nobody changes their own role, and the group always has an admin.
//
// An actor who may not is [model.ErrorNotAllowed], [model.ErrorUserInactive], or
// [model.ErrorUserNotFound], whatever the target and the role. Then a target who is not an active member
// is [model.ErrorMemberNotFound], and a role that is not one is [model.ErrorRoleInvalid].
//
// Panics unless the operation was wired, by [Setup] or by the function of the same name.
func (f *Fat) ChangeRole(ctx context.Context, actor, target model.UserID, role string) error {
	if f.changeRole == nil {
		panic("service: ChangeRole not wired; call service.ChangeRole or service.Setup")
	}

	ctx, span := f.tracer.Start(ctx, "service.ChangeRole")
	defer span.End()

	err := f.changeRole(ctx, actor, target, role)
	recordError(span, err)
	return err
}

// lifetime of a link in words, in whole days, hours, or minutes, such as "15 minutes" or "7 days".
func lifetime(d time.Duration) string {
	n, unit := int(d/time.Minute), "minute"
	switch {
	case d%(24*time.Hour) == 0:
		n, unit = int(d/(24*time.Hour)), "day"
	case d%time.Hour == 0:
		n, unit = int(d/time.Hour), "hour"
	}
	if n != 1 {
		unit += "s"
	}
	return fmt.Sprint(n, " ", unit)
}
