package sqlite

import (
	"context"
	"errors"

	"maragu.dev/glue/sql"

	"app/model"
)

func (d *Database) GetUser(ctx context.Context, id model.UserID) (model.User, error) {
	var u model.User
	if err := d.H.Get(ctx, &u, `select * from users where id = ?`, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return u, model.ErrorUserNotFound
		}
		return u, err
	}

	return u, nil
}

func (d *Database) IsUserActive(ctx context.Context, id model.UserID) (bool, error) {
	var active bool
	query := `select active from users where id = ?`
	if err := d.H.Get(ctx, &active, query, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, model.ErrorUserNotFound
		}
		return false, err
	}
	return active, nil
}

// CountActiveUsers, who are the members of the group.
func (d *Database) CountActiveUsers(ctx context.Context) (int, error) {
	var count int
	if err := d.H.Get(ctx, &count, `select count(*) from users where active = 1`); err != nil {
		return 0, err
	}
	return count, nil
}
