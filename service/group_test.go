package service_test

import (
	"testing"

	"maragu.dev/is"

	"app/model"
	"app/service"
	"app/servicetest"
	"app/sqlitetest"
)

func TestFat_GetMembers(t *testing.T) {
	t.Run("counts the people who are active", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		sqlitetest.CreateUser(t, db, "Thomas")
		amira := sqlitetest.CreateUser(t, db, "Amira")
		sqlitetest.DeactivateUser(t, db, amira.ID)
		fat := servicetest.NewFat(t)
		service.GetMembers(fat, db)

		members, err := fat.GetMembers(t.Context(), signe.ID)
		is.NotError(t, err)
		is.Equal(t, 2, members)
	})

	t.Run("refuses an inactive user and a user that does not exist", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		signe := sqlitetest.CreateUser(t, db, "Signe")
		sqlitetest.DeactivateUser(t, db, signe.ID)
		fat := servicetest.NewFat(t)
		service.GetMembers(fat, db)

		_, err := fat.GetMembers(t.Context(), signe.ID)
		is.Error(t, model.ErrorUserInactive, err)

		_, err = fat.GetMembers(t.Context(), "u_nope")
		is.Error(t, model.ErrorUserNotFound, err)
	})

	t.Run("panics when wired twice, and when called without being wired", func(t *testing.T) {
		db := sqlitetest.NewDatabase(t)
		fat := servicetest.NewFat(t)

		is.True(t, panics(func() { _, _ = fat.GetMembers(t.Context(), "u_nope") }), "expected a panic when not wired")

		service.GetMembers(fat, db)
		is.True(t, panics(func() { service.GetMembers(fat, db) }), "expected a panic when wired twice")
	})
}

// panics reports whether the given function does.
func panics(f func()) (panicked bool) {
	defer func() {
		panicked = recover() != nil
	}()
	f()
	return false
}
