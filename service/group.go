package service

import (
	"context"

	"app/model"
)

// membersCounter is the store the members of the group are counted in.
type membersCounter interface {
	GetUser(ctx context.Context, id model.UserID) (model.User, error)
	CountActiveUsers(ctx context.Context) (int, error)
}

// GetMembers wires [Fat.GetMembers] to the given store.
//
// Panics if the operation is already wired.
func GetMembers(f *Fat, db membersCounter) {
	if f.getMembers != nil {
		panic("service: GetMembers already wired")
	}

	f.getMembers = func(ctx context.Context, actor model.UserID) (int, error) {
		if _, err := authorize(ctx, db, actor, model.ActionRead); err != nil {
			return 0, err
		}

		return db.CountActiveUsers(ctx)
	}
}

// GetMembers returns how many people are in the group, for the acting user, who must be active.
// The members are the active users.
//
// Panics unless the operation was wired, by [Setup] or by the function of the same name.
func (f *Fat) GetMembers(ctx context.Context, actor model.UserID) (int, error) {
	if f.getMembers == nil {
		panic("service: GetMembers not wired; call service.GetMembers or service.Setup")
	}

	ctx, span := f.tracer.Start(ctx, "service.GetMembers")
	defer span.End()

	members, err := f.getMembers(ctx, actor)
	recordError(span, err)
	return members, err
}
