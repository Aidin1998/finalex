package auth

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/auth0/go-auth0/authentication"
	"github.com/auth0/go-auth0/authentication/database"
	"github.com/auth0/go-auth0/authentication/oauth"
	"github.com/auth0/go-auth0/management"
	"github.com/litebittech/cex/internal/identities/models"
	"github.com/litebittech/cex/internal/identities/users"
	"github.com/litebittech/cex/pkg/errors"
)

type Auth0Config struct {
	Domain       string
	ClientID     string
	ClientSecret string
	RedirectURI  string
}

type Auth struct {
	log         *slog.Logger
	users       *users.Users
	redirectUri string

	management     *management.Management
	authentication *authentication.Authentication
}

func New(log *slog.Logger, auth0Config Auth0Config, users *users.Users) (*Auth, error) {
	authentication, err := authentication.New(
		context.Background(),
		auth0Config.Domain,
		authentication.WithClientID(auth0Config.ClientID),
		authentication.WithClientSecret(auth0Config.ClientSecret), // Optional depending on the grants used
	)

	management, err := management.New(
		auth0Config.Domain,
		management.WithClientCredentials(context.Background(), auth0Config.ClientID, auth0Config.ClientSecret))
	if err != nil {
		panic(fmt.Sprintf("failed to initialize the auth0 authentication API client: %+v", err))
	}

	return &Auth{
		log:            log,
		users:          users,
		redirectUri:    auth0Config.RedirectURI,
		authentication: authentication,
		management:     management,
	}, nil
}

type Result struct {
	AccessToken  string
	RefreshToken string
	IDToken      string
	ExpiresIn    int
	User         *models.User
}

type SignUpIn struct {
	Email     string
	Username  string
	Password  string
	FirstName string
	LastName  string
}

type LoginIn struct {
	Email    string
	Password string
}

func (a *Auth) SignUpInProvider(ctx context.Context, in SignUpIn) error {
	user := database.SignupRequest{
		Connection: "Username-Password-Authentication",
		Username:   in.Username,
		Password:   in.Password,
		Email:      in.Email,
		GivenName:  in.FirstName,
		FamilyName: in.LastName,
	}

	if _, err := a.authentication.Database.Signup(ctx, user); err != nil {
		var authErr *authentication.Error
		if errors.As(err, &authErr) {
			if authErr.Err == "user_exists" {
				return errors.Conflict.Explain("User already exists")
			}
		}

		return errors.New("Failed to create new user in provider").Wrap(err)
	}

	return nil
}

func (a *Auth) Login(ctx context.Context, in LoginIn) (*Result, error) {
	tokenSet, err := a.authentication.OAuth.LoginWithPassword(ctx, oauth.LoginWithPasswordRequest{
		Username: in.Email,
		Password: in.Password,
		Audience: "http://localhost:3001",
		Scope:    "openid offline_access",
	}, oauth.IDTokenValidationOptions{})
	if err != nil {
		return nil, errors.New("Failed to fetch tokens with password").Wrap(err)
	}

	user, err := a.ensureUserIsOnboarded(ctx, tokenSet.IDToken)
	if err != nil {
		return nil, err
	}

	return &Result{
		AccessToken:  tokenSet.AccessToken,
		RefreshToken: tokenSet.RefreshToken,
		IDToken:      tokenSet.IDToken,
		ExpiresIn:    int(tokenSet.ExpiresIn),
		User:         user,
	}, nil
}

func (a *Auth) ExchangeAuthCode(ctx context.Context, code string) (*Result, error) {
	tokenSet, err := a.authentication.OAuth.LoginWithAuthCode(ctx, oauth.LoginWithAuthCodeRequest{
		Code:        code,
		RedirectURI: a.redirectUri,
	}, oauth.IDTokenValidationOptions{})
	if err != nil {
		return nil, errors.New("Failed to fetch tokens with authorization code flow").Wrap(err)
	}

	user, err := a.ensureUserIsOnboarded(ctx, tokenSet.IDToken)
	if err != nil {
		return nil, err
	}

	return &Result{
		AccessToken:  tokenSet.AccessToken,
		RefreshToken: tokenSet.RefreshToken,
		IDToken:      tokenSet.IDToken,
		ExpiresIn:    int(tokenSet.ExpiresIn),
		User:         user,
	}, nil
}
