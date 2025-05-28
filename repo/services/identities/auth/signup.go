package auth

import (
	"context"

	"github.com/auth0/go-auth0/management"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/litebittech/cex/common/errors"
	"github.com/litebittech/cex/services/identities/models"
	"github.com/litebittech/cex/services/identities/users"
)

type CustomClaims struct {
	jwt.MapClaims
}

func (c *CustomClaims) UserID() string {
	var (
		ok     bool
		raw    interface{}
		userID string
	)
	raw, ok = c.MapClaims["https://litebit.tech/userid"]
	if !ok {
		return ""
	}

	userID, ok = raw.(string)
	if !ok {
		return ""
	}

	return userID
}

func (c *CustomClaims) Onboarded() bool {
	var (
		ok        bool
		raw       interface{}
		onboarded bool
	)

	raw, ok = c.MapClaims["https://litebit.tech/onboarded"]
	if !ok {
		return false
	}

	onboarded, ok = raw.(bool)
	if !ok {
		return false
	}

	return onboarded
}

func (a *Auth) ensureUserIsOnboarded(ctx context.Context, idToken string) (*models.User, error) {
	claims, err := a.parseIdToken(idToken)
	if err != nil {
		return nil, err
	}

	providerID, err := claims.GetSubject()
	if err != nil {
		return nil, errors.New("failed to get id token claim").Wrap(err).Trace()
	}

	if !claims.Onboarded() {
		return a.onboardUser(ctx, providerID)
	}

	return a.users.UserByProviderID(ctx, providerID)
}

func (a *Auth) onboardUser(ctx context.Context, id string) (*models.User, error) {
	providerUser, err := a.management.User.Read(ctx, id)
	if err != nil {
		return nil, errors.New("failed to get user from provider").Wrap(err).Trace()
	}

	user, err := a.users.UserByProviderID(ctx, providerUser.GetID())
	if errors.Is(err, errors.NotFound) {
		user, err = a.users.Create(ctx, users.CreateIn{
			ID:         uuid.New(),
			Username:   providerUser.GetUsername(),
			FirstName:  providerUser.GetGivenName(),
			LastName:   providerUser.GetFamilyName(),
			ProviderID: providerUser.GetID(),
			Email:      providerUser.GetEmail(),
		})
		if err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}

	if err := a.markUserAsOnboarded(ctx, providerUser.GetID(), user.ID); err != nil {
		return nil, errors.New("failed to mark user as onboarded").Wrap(err)
	}

	return user, nil
}

func (a *Auth) markUserAsOnboarded(ctx context.Context, providerId string, userId uuid.UUID) error {
	err := a.management.User.Update(ctx, providerId, &management.User{UserMetadata: &map[string]interface{}{
		"userId":    userId.String(),
		"onboarded": true,
	}})
	if err != nil {
		return errors.New("failed to update user metadata").Wrap(err).Trace()
	}
	return nil
}

func (a *Auth) parseIdToken(idToken string) (*CustomClaims, error) {
	mapClaims := jwt.MapClaims{}

	_, _, err := new(jwt.Parser).ParseUnverified(idToken, mapClaims)
	if err != nil {
		return nil, errors.New("failed to parse id token").Wrap(err).Trace()
	}

	return &CustomClaims{mapClaims}, nil
}
