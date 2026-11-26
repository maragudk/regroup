package http

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/alexedwards/scs/v2"
	gluehttp "maragu.dev/glue/http"
	. "maragu.dev/gomponents"
	"maragu.dev/httph"

	"app/html"
	"app/model"
)

// peopleService is the business logic of signing in, of invitations, and of the list of members.
type peopleService interface {
	RequestSignInLink(ctx context.Context, address string) error
	CheckSignInLink(ctx context.Context, token string) (model.SignInLinkCheck, error)
	SignIn(ctx context.Context, token, name string) (model.User, error)
	CheckInvitation(ctx context.Context, token string) (model.InvitationCheck, error)
	AcceptInvitation(ctx context.Context, token, name string) (model.User, error)
	ListMembers(ctx context.Context, actor model.UserID) ([]model.Member, error)
	GetInvitations(ctx context.Context, actor model.UserID) ([]model.Invitation, error)
	Invite(ctx context.Context, actor model.UserID, token, address string) (model.Invitation, error)
	WithdrawInvitation(ctx context.Context, actor model.UserID, id model.InvitationID) error
	ChangeRole(ctx context.Context, actor, target model.UserID, role string) error
}

// people adds the pages for signing in, for invitations, and for the list of members, with what their
// forms do. Every form is a plain form, which works without a script, since a link in an email may open
// in any browser:
//
//   - GET /sign-in is the page that says the group is private, with the field to ask for a sign-in link,
//     as GET / is for someone who is not signed in. Someone who is signed in goes to the group page.
//   - POST /sign-in with an email address asks for a sign-in link, and goes to GET /sign-in/sent, which says
//     that a link is on its way if the address belongs to someone in the group. Every address that is one
//     gets the same answer.
//   - GET /sign-in/link?token=… is the page with the button that signs in with the link, without using it
//     up, and POST /sign-in/link with the token, and a name for the first admin, signs in and goes to the
//     group page.
//   - GET /invitation?token=… is the page with the field for a name and the button that joins, without
//     using the invitation up, and POST /invitation with the token and a name accepts it, signs in, and
//     goes to the group page.
//   - GET /members is the list of members, with what the person may do there.
//   - POST /invitations invites an email address, POST /invitations/withdraw withdraws an invitation, and
//     POST /members/role changes someone's role. Each goes back to the list of members when it is done.
//
// Every answer that a page of the app can lead to is a page with 200, also when it says that a link does
// not work, or why what was sent was refused, since that is the page the person is meant to see. That
// includes a form sent again by going back after signing out or becoming someone else, which can come
// from someone who is not signed in, or who may no longer do what the form does. A request that no page
// sends, such as a form token that is not one, a role that is not one, or a change of one's own role, is
// answered with 400. Someone who is not signed in, or not active, gets the page that says the group is
// private from the pages for members.
func people(r *Router, log *slog.Logger, svc peopleService, opts InjectHTTPRouterOptions, token func() string) {
	group := opts.Group

	// invited in the session says that the person has just invited someone, for the list of members to
	// say so once.
	const invited = "invited"

	emailInvalid := html.Message{Key: "email-invalid", Text: "That is not an email address."}

	// private is the page that says the group is private, with the field to ask for a sign-in link.
	private := func(props html.PageProps, p html.PrivatePageProps) (Node, error) {
		p.PageProps = props
		p.Extra = opts.extra(false)
		return html.PrivatePage(p), nil
	}

	// linkInvalid is the one answer to a sign-in link that does not work, whatever the reason.
	linkInvalid := func(props html.PageProps) (Node, error) {
		return private(props, html.PrivatePageProps{Notice: html.Message{Key: "link-invalid", Text: "This link does not work. Ask for a new one."}})
	}

	// invitationInvalid is the one answer to an invitation that does not work, whatever the reason.
	invitationInvalid := func(props html.PageProps) (Node, error) {
		return html.InvitationInvalidPage(props), nil
	}

	r.Get("/sign-in", func(props html.PageProps) (Node, error) {
		if user := getUserFromContext(props.Ctx); user != nil && user.Active {
			http.Redirect(props.W, props.R, "/", http.StatusSeeOther)
			return nil, nil
		}
		return private(props, html.PrivatePageProps{})
	})

	r.Post("/sign-in", func(props html.PageProps) (Node, error) {
		if page, err := parseForm(props); err != nil {
			return page, err
		}
		email := props.R.PostFormValue("email")

		err := svc.RequestSignInLink(props.Ctx, email)
		switch {
		case errors.Is(err, model.ErrorEmailInvalid):
			return private(props, html.PrivatePageProps{Email: echo(email), Refusal: emailInvalid})
		case err != nil:
			log.ErrorContext(props.Ctx, "Error asking for a sign-in link", "error", err)
			return html.ErrorPage(props), err
		}

		// Going on to another page, so that going back or reloading does not ask again.
		http.Redirect(props.W, props.R, "/sign-in/sent", http.StatusSeeOther)
		return nil, nil
	})

	r.Get("/sign-in/sent", func(props html.PageProps) (Node, error) {
		return private(props, html.PrivatePageProps{Sent: true})
	})

	r.Get("/sign-in/link", func(props html.PageProps) (Node, error) {
		linkToken := props.R.URL.Query().Get("token")
		check, err := svc.CheckSignInLink(props.Ctx, linkToken)
		switch {
		case errors.Is(err, model.ErrorLinkInvalid):
			return linkInvalid(props)
		case err != nil:
			log.ErrorContext(props.Ctx, "Error checking a sign-in link", "error", err)
			return html.ErrorPage(props), err
		}
		return html.SignInLinkPage(html.SignInLinkPageProps{PageProps: props, GroupName: group.Name, Token: linkToken, Joining: check.Joining}), nil
	})

	r.Post("/sign-in/link", func(props html.PageProps) (Node, error) {
		if page, err := parseForm(props); err != nil {
			return page, err
		}
		linkToken := props.R.PostFormValue("token")
		name := props.R.PostFormValue("name")

		u, err := svc.SignIn(props.Ctx, linkToken, name)
		if refusal := nameRefusal(err); refusal.Text != "" {
			// The name is refused only for a link that works, by [service.Fat.SignIn], so the page may name
			// the group.
			return html.SignInLinkPage(html.SignInLinkPageProps{PageProps: props, GroupName: group.Name, Token: echo(linkToken), Joining: true, Name: echo(name), Refusal: refusal}), nil
		}
		if errors.Is(err, model.ErrorLinkInvalid) {
			return linkInvalid(props)
		}
		return signedIn(props, log, r.SM, u, err)
	})

	r.Get("/invitation", func(props html.PageProps) (Node, error) {
		linkToken := props.R.URL.Query().Get("token")
		check, err := svc.CheckInvitation(props.Ctx, linkToken)
		switch {
		case errors.Is(err, model.ErrorLinkInvalid):
			return invitationInvalid(props)
		case err != nil:
			log.ErrorContext(props.Ctx, "Error checking an invitation", "error", err)
			return html.ErrorPage(props), err
		}
		return html.InvitationPage(html.InvitationPageProps{PageProps: props, GroupName: group.Name, Token: linkToken, InvitedByName: check.InvitedByName}), nil
	})

	r.Post("/invitation", func(props html.PageProps) (Node, error) {
		if page, err := parseForm(props); err != nil {
			return page, err
		}
		linkToken := props.R.PostFormValue("token")
		name := props.R.PostFormValue("name")

		u, err := svc.AcceptInvitation(props.Ctx, linkToken, name)
		if refusal := nameRefusal(err); refusal.Text != "" {
			// The name is refused only for an invitation that works, by [service.Fat.AcceptInvitation], and
			// using up nothing. It is checked again for who sent it, and may have stopped working since.
			check, err := svc.CheckInvitation(props.Ctx, linkToken)
			switch {
			case errors.Is(err, model.ErrorLinkInvalid):
				return invitationInvalid(props)
			case err != nil:
				log.ErrorContext(props.Ctx, "Error checking an invitation", "error", err)
				return html.ErrorPage(props), err
			}
			return html.InvitationPage(html.InvitationPageProps{PageProps: props, GroupName: group.Name, Token: echo(linkToken), InvitedByName: check.InvitedByName, Name: echo(name), Refusal: refusal}), nil
		}
		// An address that became a member's in the meantime has no invitation that works.
		if errors.Is(err, model.ErrorLinkInvalid) || errors.Is(err, model.ErrorAlreadyMember) {
			return invitationInvalid(props)
		}
		return signedIn(props, log, r.SM, u, err)
	})

	// members is the list of members as the given person sees it, with the given state of the field to
	// invite with, which gets a new token, and the given notice at the top.
	members := func(props html.PageProps, user *model.User, invite html.InviteFormProps, notice html.Message) (Node, error) {
		list, err := svc.ListMembers(props.Ctx, user.ID)
		if err != nil {
			return membersFailed(props, log, err, private)
		}

		// That someone was invited is said on the list that comes next, if it is asked for, and forgotten
		// by any other answer.
		if r.SM.PopString(props.Ctx, invited) != "" && invite.Notice.Text == "" && notice.Text == "" && props.R.Method == http.MethodGet {
			invite.Notice = html.Message{Key: "invited", Text: "The invitation is on its way."}
		}

		// The person may have stopped being allowed to see the invitations since the request began, and
		// then sees none.
		var invitations []model.Invitation
		if model.Allowed(user, model.ActionSeeInvitations, nil) {
			invitations, err = svc.GetInvitations(props.Ctx, user.ID)
			if err != nil && !errors.Is(err, model.ErrorNotAllowed) {
				return membersFailed(props, log, err, private)
			}
			for i := range invitations {
				invitations[i].Expires.T = invitations[i].Expires.T.In(group.TimeZone)
			}
		}

		invite.Token = token()
		return html.MembersPage(html.MembersPageProps{
			PageProps:   props,
			GroupName:   group.Name,
			Actor:       *user,
			Members:     list,
			Invitations: invitations,
			Now:         opts.Now().In(group.TimeZone),
			Invite:      invite,
			Notice:      notice,
			Extra:       opts.extra(true),
		}), nil
	}

	r.Get("/members", func(props html.PageProps) (Node, error) {
		user := getUserFromContext(props.Ctx)
		if user == nil || !user.Active {
			return private(props, html.PrivatePageProps{})
		}

		return members(props, user, html.InviteFormProps{}, html.Message{})
	})

	r.Post("/invitations", func(props html.PageProps) (Node, error) {
		user := getUserFromContext(props.Ctx)
		if user == nil || !user.Active {
			return private(props, html.PrivatePageProps{})
		}

		if page, err := parseForm(props); err != nil {
			return page, err
		}
		email := props.R.PostFormValue("email")

		inv, err := svc.Invite(props.Ctx, user.ID, props.R.PostFormValue("token"), email)
		var refusal html.Message
		switch {
		case err == nil:
			// A form sent again made nothing, and an invitation from it may no longer be waiting.
			if inv.ID != "" {
				r.SM.Put(props.Ctx, invited, "1")
			}
			http.Redirect(props.W, props.R, "/members", http.StatusSeeOther)
			return nil, nil
		case errors.Is(err, model.ErrorFormTokenInvalid):
			return badRequest(props)
		case errors.Is(err, model.ErrorNotAllowed):
			return members(props, user, html.InviteFormProps{}, html.Message{Key: "not-allowed", Text: "Only moderators and admins can invite."})
		case errors.Is(err, model.ErrorEmailInvalid):
			refusal = emailInvalid
		case errors.Is(err, model.ErrorAlreadyMember):
			refusal = html.Message{Key: "already-member", Text: "That address belongs to someone who is in the group already."}
		case errors.Is(err, model.ErrorFormerMember):
			refusal = html.Message{Key: "former-member", Text: "That address belongs to someone who was in the group. They cannot be invited again for now."}
		case errors.Is(err, model.ErrorFormResentChanged):
			refusal = html.Message{Key: "resent", Text: "Your earlier try did go through, and that invitation is on its way. This address has not been invited."}
		default:
			return membersFailed(props, log, err, private)
		}
		return members(props, user, html.InviteFormProps{Email: echo(email), Refusal: refusal}, html.Message{})
	})

	r.Post("/invitations/withdraw", func(props html.PageProps) (Node, error) {
		user := getUserFromContext(props.Ctx)
		if user == nil || !user.Active {
			return private(props, html.PrivatePageProps{})
		}

		if page, err := parseForm(props); err != nil {
			return page, err
		}

		err := svc.WithdrawInvitation(props.Ctx, user.ID, model.InvitationID(props.R.PostFormValue("invitation")))
		switch {
		case err == nil:
			http.Redirect(props.W, props.R, "/members", http.StatusSeeOther)
			return nil, nil
		case errors.Is(err, model.ErrorNotAllowed):
			return members(props, user, html.InviteFormProps{}, html.Message{Key: "not-allowed", Text: "Only moderators and admins can withdraw invitations."})
		case errors.Is(err, model.ErrorInvitationNotFound):
			return members(props, user, html.InviteFormProps{}, html.Message{Key: "invitation-not-found", Text: "That invitation is not waiting any more. It may have been accepted, withdrawn, or have run out."})
		default:
			return membersFailed(props, log, err, private)
		}
	})

	r.Post("/members/role", func(props html.PageProps) (Node, error) {
		user := getUserFromContext(props.Ctx)
		if user == nil || !user.Active {
			return private(props, html.PrivatePageProps{})
		}

		if page, err := parseForm(props); err != nil {
			return page, err
		}
		target := model.UserID(props.R.PostFormValue("member"))

		err := svc.ChangeRole(props.Ctx, user.ID, target, props.R.PostFormValue("role"))
		var notice html.Message
		code := http.StatusOK
		switch {
		case err == nil:
			http.Redirect(props.W, props.R, "/members", http.StatusSeeOther)
			return nil, nil
		case errors.Is(err, model.ErrorNotAllowed) && target == user.ID:
			notice, code = html.Message{Key: "own-role", Text: "Nobody can change their own role."}, http.StatusBadRequest
		case errors.Is(err, model.ErrorNotAllowed):
			notice = html.Message{Key: "not-allowed", Text: "Only admins can change roles."}
		case errors.Is(err, model.ErrorMemberNotFound):
			notice = html.Message{Key: "member-not-found", Text: "That person is not in the group."}
		case errors.Is(err, model.ErrorRoleInvalid):
			notice, code = html.Message{Key: "role-invalid", Text: "That is not a role."}, http.StatusBadRequest
		default:
			return membersFailed(props, log, err, private)
		}
		page, err := members(props, user, html.InviteFormProps{}, notice)
		if err == nil && code != http.StatusOK {
			return page, httph.HTTPError{Code: code}
		}
		return page, err
	})
}

// nameRefusal in plain words, for a name that was refused. It is empty if the error is not a refusal of
// the name, or is nil.
func nameRefusal(err error) html.Message {
	switch {
	case errors.Is(err, model.ErrorNameEmpty):
		return html.Message{Key: "name-empty", Text: "Write the name the group will see first."}
	case errors.Is(err, model.ErrorNameTooLong):
		return html.Message{Key: "name-too-long", Text: "That is too long. A name can be up to " + fmt.Sprint(model.NameMaxLength) + " characters."}
	case errors.Is(err, model.ErrorNameInvalid):
		return html.Message{Key: "name-invalid", Text: "That name has characters that cannot be used in a name. Remove them and try again."}
	default:
		return html.Message{}
	}
}

// membersFailed is the answer when the business logic returned an error on a page for members that is
// not a refusal. Someone who stopped being active since the request began is not signed in, and gets
// the page that says the group is private.
func membersFailed(props html.PageProps, log *slog.Logger, err error, private func(html.PageProps, html.PrivatePageProps) (Node, error)) (Node, error) {
	if errors.Is(err, model.ErrorUserInactive) || errors.Is(err, model.ErrorUserNotFound) {
		return private(props, html.PrivatePageProps{})
	}
	log.ErrorContext(props.Ctx, "Error on the list of members", "error", err)
	return html.ErrorPage(props), err
}

// signedIn answers signing in, or accepting an invitation, with the given outcome: on success, the
// session is the user's, and the browser goes to the group page.
func signedIn(props html.PageProps, log *slog.Logger, sm *scs.SessionManager, u model.User, err error) (Node, error) {
	if err != nil {
		log.ErrorContext(props.Ctx, "Error signing in", "error", err)
		return html.ErrorPage(props), err
	}

	if err := SignInSession(props.Ctx, sm, u.ID); err != nil {
		log.ErrorContext(props.Ctx, "Error putting the user in the session", "error", err, "userID", u.ID)
		return html.ErrorPage(props), err
	}

	http.Redirect(props.W, props.R, "/", http.StatusSeeOther)
	return nil, nil
}

// SignInSession of the request in the given context as the given user: the session starts afresh with a
// new token and nothing from before in it, so that a session from before cannot be taken over and keeps
// nothing of whoever had it, and it holds the user's ID under [gluehttp.SessionUserIDKey].
func SignInSession(ctx context.Context, sm *scs.SessionManager, id model.UserID) error {
	if err := sm.Clear(ctx); err != nil {
		return err
	}
	if err := sm.RenewToken(ctx); err != nil {
		return err
	}
	sm.Put(ctx, gluehttp.SessionUserIDKey, id.String())
	return nil
}
