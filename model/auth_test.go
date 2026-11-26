package model_test

import (
	"fmt"
	"testing"

	"maragu.dev/is"

	"app/model"
)

func TestAllowed(t *testing.T) {
	admin := &model.User{ID: "u_admin", Active: true, Role: model.RoleAdmin}
	moderator := &model.User{ID: "u_moderator", Active: true, Role: model.RoleModerator}
	member := &model.User{ID: "u_member", Active: true, Role: model.RoleMember}
	other := &model.User{ID: "u_other", Active: true, Role: model.RoleMember}

	// Who may do what, written out by hand from docs/todo/people.md, not generated: one row for each
	// action, and one column for each kind of actor. Someone made inactive may do what someone not
	// signed in may, whatever their role.
	tests := []struct {
		action                                 model.Action
		admin, moderator, member, inactive, nobody bool
	}{
		{action: model.ActionSignIn, admin: true, moderator: true, member: true, inactive: true, nobody: true},
		{action: model.ActionAcceptInvitation, admin: true, moderator: true, member: true, inactive: true, nobody: true},
		{action: model.ActionRead, admin: true, moderator: true, member: true},
		{action: model.ActionPost, admin: true, moderator: true, member: true},
		{action: model.ActionInvite, admin: true, moderator: true},
		{action: model.ActionSeeInvitations, admin: true, moderator: true},
		{action: model.ActionWithdrawInvitation, admin: true, moderator: true},
		{action: model.ActionChangeRole, admin: true},
		{action: model.ActionSeeEmailAddresses, admin: true},
	}

	t.Run("covers every action there is", func(t *testing.T) {
		is.Equal(t, len(model.Actions), len(tests))
		for i, a := range model.Actions {
			is.Equal(t, a, tests[i].action)
		}
	})

	for _, test := range tests {
		t.Run(fmt.Sprintf("lets the roles %v as the table says", test.action), func(t *testing.T) {
			for _, role := range model.Roles {
				for _, active := range []bool{true, false} {
					actor := &model.User{ID: "u_actor", Active: active, Role: role}
					expected := test.inactive
					if active {
						expected = map[model.Role]bool{model.RoleAdmin: test.admin, model.RoleModerator: test.moderator, model.RoleMember: test.member}[role]
					}
					is.Equal(t, expected, model.Allowed(actor, test.action, other), fmt.Sprint(role, " active ", active))
				}
			}
			is.Equal(t, test.nobody, model.Allowed(nil, test.action, other), "not signed in")
		})
	}

	t.Run("lets nobody change their own role, not even an admin", func(t *testing.T) {
		for _, actor := range []*model.User{admin, moderator, member} {
			is.True(t, !model.Allowed(actor, model.ActionChangeRole, actor), string(actor.Role))
			is.True(t, !model.Allowed(actor, model.ActionChangeRole, &model.User{ID: actor.ID}), string(actor.Role))
		}
	})

	t.Run("lets an admin change another admin's role, and needs someone to change it of", func(t *testing.T) {
		is.True(t, model.Allowed(admin, model.ActionChangeRole, &model.User{ID: "u_admin2", Active: true, Role: model.RoleAdmin}))
		is.True(t, !model.Allowed(admin, model.ActionChangeRole, nil))
	})

	t.Run("lets someone with a role that is not one do nothing but the public actions", func(t *testing.T) {
		actor := &model.User{ID: "u_actor", Active: true, Role: "owner"}
		for _, a := range model.Actions {
			is.Equal(t, a == model.ActionSignIn || a == model.ActionAcceptInvitation, model.Allowed(actor, a, other), string(a))
		}
	})
}

func TestParseRole(t *testing.T) {
	t.Run("parses each role by its name", func(t *testing.T) {
		for _, r := range model.Roles {
			parsed, err := model.ParseRole(r.String())
			is.NotError(t, err)
			is.Equal(t, r, parsed)
		}
	})

	t.Run("refuses anything else, in any spelling", func(t *testing.T) {
		for _, s := range []string{"", "Admin", " admin", "owner", "members"} {
			_, err := model.ParseRole(s)
			is.Error(t, model.ErrorRoleInvalid, err, s)
		}
	})
}
