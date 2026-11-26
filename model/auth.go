package model

import (
	"maragu.dev/glue/model"
)

type AccountID = model.AccountID

type Account struct {
	ID      AccountID
	Created Time
	Updated Time
	Name    string
}

type UserID = model.UserID

// User is a person in the group, or someone who was: a person made inactive is no longer a member, but
// what they wrote stays with their name.
type User struct {
	ID        UserID
	Created   Time
	Updated   Time
	AccountID AccountID `db:"account_id"`
	Name      string
	Email     EmailAddress
	Confirmed bool
	Active    bool
	Role      Role

	// InvitedBy is who invited the person, or nil for the first admin, who was named in the configuration.
	InvitedBy *UserID `db:"invited_by"`
}

// Role of a person in the group. Every person has exactly one: [RoleAdmin], [RoleModerator], or
// [RoleMember].
type Role = model.Role

const (
	RoleAdmin     Role = "admin"
	RoleModerator Role = "moderator"
	RoleMember    Role = "member"
)

// Roles there are, from the one with the most rights to the one with the fewest.
var Roles = []Role{RoleAdmin, RoleModerator, RoleMember}

// ParseRole from its name, or return [ErrorRoleInvalid].
func ParseRole(s string) (Role, error) {
	for _, r := range Roles {
		if string(r) == s {
			return r, nil
		}
	}
	return "", ErrorRoleInvalid
}

// Action that someone may or may not take. [Allowed] says who may take which.
type Action string

const (
	// ActionSignIn is asking for a sign-in link, and using one. Anyone may, signed in or not, since
	// what decides it is the email address and the link.
	ActionSignIn Action = "sign in"

	// ActionAcceptInvitation is opening and accepting an invitation. Anyone may, since what decides it is
	// the link.
	ActionAcceptInvitation Action = "accept an invitation"

	// ActionRead is reading the group: the timeline, comments, and the list of members with their names
	// and roles.
	ActionRead Action = "read the group"

	// ActionPost is posting and commenting.
	ActionPost Action = "post and comment"

	// ActionInvite is inviting an email address to the group.
	ActionInvite Action = "invite"

	// ActionSeeInvitations is listing the invitations that are waiting, with their addresses.
	ActionSeeInvitations Action = "see invitations"

	// ActionWithdrawInvitation is withdrawing an invitation that is waiting.
	ActionWithdrawInvitation Action = "withdraw an invitation"

	// ActionChangeRole is changing another person's role. Nobody may change their own.
	ActionChangeRole Action = "change someone's role"

	// ActionSeeEmailAddresses is seeing members' email addresses.
	ActionSeeEmailAddresses Action = "see members' email addresses"
)

// Actions there are, for going through them all.
var Actions = []Action{
	ActionSignIn, ActionAcceptInvitation, ActionRead, ActionPost, ActionInvite, ActionSeeInvitations,
	ActionWithdrawInvitation, ActionChangeRole, ActionSeeEmailAddresses,
}

// rights of the people in the group: which roles may take which action. An action that is not here is
// one that anyone may take, signed in or not.
var rights = map[Action][]Role{
	ActionRead:               {RoleAdmin, RoleModerator, RoleMember},
	ActionPost:               {RoleAdmin, RoleModerator, RoleMember},
	ActionInvite:             {RoleAdmin, RoleModerator},
	ActionSeeInvitations:     {RoleAdmin, RoleModerator},
	ActionWithdrawInvitation: {RoleAdmin, RoleModerator},
	ActionChangeRole:         {RoleAdmin},
	ActionSeeEmailAddresses:  {RoleAdmin},
}

// public actions, which anyone may take, signed in or not, active or not.
var public = map[Action]bool{
	ActionSignIn:           true,
	ActionAcceptInvitation: true,
}

// Allowed reports whether the actor may take the action, which is the one place the rights of the three
// roles are stated. The actor is nil for someone who is not signed in.
//
//   - Anyone may ask for and use a sign-in link, and accept an invitation. Whether it works is up to
//     the address and the link.
//   - Anything else needs an actor who is active, and whose role has the right.
//   - An action on a person, which is only [ActionChangeRole], needs a target, and nobody may take it on
//     themselves. So an admin cannot stop being one by their own action, and there is always an admin.
func Allowed(actor *User, action Action, target *User) bool {
	if public[action] {
		return true
	}
	if actor == nil || !actor.Active {
		return false
	}
	if action == ActionChangeRole && (target == nil || target.ID == actor.ID) {
		return false
	}
	for _, r := range rights[action] {
		if r == actor.Role {
			return true
		}
	}
	return false
}
