package users

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/litebittech/cex/internal/identities/models"
	"github.com/litebittech/cex/internal/identities/users/store"
	"github.com/litebittech/cex/pkg/errors"
)

type CreateIn = store.CreateIn

type Users struct {
	log   *slog.Logger
	store store.Store
}

func New(log *slog.Logger, store store.Store) *Users {
	return &Users{
		log:   log,
		store: store,
	}
}

func (u *Users) Create(ctx context.Context, in CreateIn) (*models.User, error) {
	err := u.store.Create(ctx, in)
	if err != nil {
		return nil, errors.New("failed to create user").Wrap(err)
	}

	user, err := u.store.User(ctx, in.ID)
	if err != nil {
		return nil, errors.New("failed to fetch newly created user").Wrap(err)
	}

	return user, nil
}

func (u *Users) User(ctx context.Context, id uuid.UUID) (*models.User, error) {
	user, err := u.store.User(ctx, id)
	if err != nil {
		return nil, errors.New("failed to get user").Wrap(err)
	}

	return user, nil
}

func (u *Users) UserByProviderID(ctx context.Context, providerID string) (*models.User, error) {
	user, err := u.store.UserByProviderID(ctx, providerID)
	if err != nil {
		return nil, errors.New("failed to get user by provider ID").Wrap(err)
	}

	return user, nil
}

func (u *Users) UserByUsername(ctx context.Context, username string) (*models.User, error) {
	user, err := u.store.UserByUsername(ctx, username)
	if err != nil {
		return nil, errors.New("failed to get user by username").Wrap(err)
	}

	return user, nil
}

func (u *Users) List(ctx context.Context, filter models.UserFilter) ([]models.User, error) {
	users, err := u.store.List(ctx, filter)
	if err != nil {
		return nil, errors.New("failed to list users").Wrap(err)
	}

	return users, nil
}

func (u *Users) UpdateNames(ctx context.Context, id uuid.UUID, firstName, lastName string) (*models.User, error) {
	// Update the user's names
	err := u.store.Update(ctx, id, store.UpdateIn{FirstName: firstName, LastName: lastName})
	if err != nil {
		return nil, errors.New("failed to update user names").Wrap(err)
	}

	// Fetch the updated user
	updatedUser, err := u.User(ctx, id)
	if err != nil {
		return nil, errors.New("failed to fetch updated user").Wrap(err)
	}

	return updatedUser, nil
}
