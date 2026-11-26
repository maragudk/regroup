// Package sqlitetest provides testing helpers for the sqlite package.
package sqlitetest

import (
	"crypto/rand"
	"strings"
	"testing"

	"maragu.dev/glue/sqlitetest"

	"app/model"
	"app/sqlite"
)

// NewDatabase for testing, with optional options.
// Options:
// - [WithFixtures] to load fixtures after migration.
func NewDatabase(t *testing.T, opts ...NewDatabaseOption) *sqlite.Database {
	t.Helper()

	return sqlite.NewDatabase(sqlite.NewDatabaseOptions{H: sqlitetest.NewHelper(t, opts...)})
}

type NewDatabaseOption = sqlitetest.NewHelperOption

func WithFixtures(fixtures ...string) sqlitetest.NewHelperOption {
	return sqlitetest.WithFixtures(fixtures...)
}

// CreateUser for testing, active and confirmed, with the given name and an account of its own.
// The user joins the group at the moment it is created.
func CreateUser(t testing.TB, db *sqlite.Database, name string) model.User {
	t.Helper()

	var accountID model.AccountID
	if err := db.H.Get(t.Context(), &accountID, `insert into accounts (name) values (?) returning id`, name); err != nil {
		t.Fatal("error creating account:", err)
	}

	email := strings.ToLower(rand.Text()) + "@example.com"
	var u model.User
	query := `insert into users (account_id, name, email, confirmed, active) values (?, ?, ?, 1, 1) returning *`
	if err := db.H.Get(t.Context(), &u, query, accountID, name, email); err != nil {
		t.Fatal("error creating user:", err)
	}
	return u
}

// DeactivateUser for testing.
func DeactivateUser(t testing.TB, db *sqlite.Database, id model.UserID) {
	t.Helper()

	if err := db.H.Exec(t.Context(), `update users set active = 0 where id = ?`, id); err != nil {
		t.Fatal("error deactivating user:", err)
	}
}
