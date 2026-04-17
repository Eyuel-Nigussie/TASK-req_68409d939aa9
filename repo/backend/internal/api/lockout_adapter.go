package api

import (
	"context"
	"errors"

	"github.com/eaglepoint/oops/backend/internal/auth"
	"github.com/eaglepoint/oops/backend/internal/models"
	"github.com/eaglepoint/oops/backend/internal/store"
)

// loginAttemptAdapter bridges the persistent store.LoginAttempts interface
// to the small auth.AttemptStore surface. Keeping this adapter at the api
// layer keeps the auth package free of a dependency on the wider store.
type loginAttemptAdapter struct {
	s store.LoginAttempts
}

func newLoginAttemptAdapter(s store.LoginAttempts) *loginAttemptAdapter {
	return &loginAttemptAdapter{s: s}
}

func (a *loginAttemptAdapter) Get(ctx context.Context, username string) (auth.Attempt, error) {
	row, err := a.s.GetLoginAttempt(ctx, username)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return auth.Attempt{Username: username}, auth.ErrNoAttempt
		}
		return auth.Attempt{}, err
	}
	return auth.Attempt{
		Username:    row.Username,
		Failures:    row.Failures,
		LockedUntil: row.LockedUntil,
	}, nil
}

func (a *loginAttemptAdapter) Upsert(ctx context.Context, at auth.Attempt) error {
	return a.s.UpsertLoginAttempt(ctx, models.LoginAttempt{
		Username:    at.Username,
		Failures:    at.Failures,
		LockedUntil: at.LockedUntil,
	})
}

func (a *loginAttemptAdapter) Clear(ctx context.Context, username string) error {
	if err := a.s.ClearLoginAttempt(ctx, username); err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	return nil
}
