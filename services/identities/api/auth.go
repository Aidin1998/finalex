package api

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/litebittech/cex/common/errors"
	"github.com/litebittech/cex/services/identities/auth"
)

// SignupRequest represents the request body for user signup
type SignupRequest struct {
	Username  string `json:"username" validate:"required"`
	Password  string `json:"password" validate:"required"`
	Email     string `json:"email" validate:"required,email"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
}

// AuthResponse represents the response for authentication endpoints
type AuthResponse struct {
	IDToken   string       `json:"idToken"`
	ExpiresIn int          `json:"expiresIn"`
	TokenType string       `json:"tokenType"`
	User      UserResponse `json:"user"`
}

// Signup handles the user registration process in auth provide
// @Summary Register a new user
// @Description Register a new user with username, password and email
// @Tags auth
// @Accept json
// @Produce json
// @Param request body SignupRequest true "Signup request"
// @Success 201 {object} AuthResponse
// @Failure 400 {object} errors.Error "Invalid request"
// @Failure 409 {object} errors.Error "Username already exists"
// @Failure 500 {object} errors.Error "Server error"
// @Router /auth/signup [post]
func (e *endpoints) Signup(c echo.Context) error {
	var req SignupRequest
	if err := c.Bind(&req); err != nil {
		c.Error(err)
		return nil
	}

	if err := c.Validate(&req); err != nil {
		c.Error(err)
		return nil
	}

	if e.captchaEnabled {
		// TODO: Implement captcha validation
	}

	if err := e.auth.SignUpInProvider(c.Request().Context(), auth.SignUpIn{
		Username:  req.Username,
		Password:  req.Password,
		Email:     req.Email,
		FirstName: req.FirstName,
		LastName:  req.LastName,
	}); err != nil {
		return err
	}

	if err := e.auth.SignUpInProvider(c.Request().Context(), auth.SignUpIn{
		Username: req.Username, Password: req.Password, Email: req.Email,
	}); err != nil {
		return err
	}

	return c.NoContent(http.StatusCreated)
}

// LoginRequest represents the request body for user login
type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

// Login handles user authentication with email and password
// @Summary Login a user
// @Description Authenticate a user with email and password
// @Tags auth
// @Accept json
// @Produce json
// @Param request body LoginRequest true "Login request"
// @Success 200 {object} AuthResponse
// @Failure 400 {object} errors.Error "Invalid request"
// @Failure 401 {object} errors.Error "Invalid credentials"
// @Failure 500 {object} errors.Error "Server error"
// @Router /auth/login [post]
func (e *endpoints) Login(c echo.Context) error {
	var req LoginRequest
	if err := c.Bind(&req); err != nil {
		c.Error(err)
		return nil
	}

	if err := c.Validate(&req); err != nil {
		c.Error(err)
		return nil
	}

	// Authenticate user
	authResult, err := e.auth.Login(c.Request().Context(), auth.LoginIn{
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		return err
	}

	// Set access token in HTTP-only cookie
	accessCookie := new(http.Cookie)
	accessCookie.Name = "access_token"
	accessCookie.Value = authResult.AccessToken
	accessCookie.HttpOnly = true
	accessCookie.Secure = true
	accessCookie.Path = "/"
	accessCookie.MaxAge = authResult.ExpiresIn
	accessCookie.SameSite = http.SameSiteStrictMode
	c.SetCookie(accessCookie)

	// Set refresh token in HTTP-only cookie (longer expiry)
	refreshCookie := new(http.Cookie)
	refreshCookie.Name = "refresh_token"
	refreshCookie.Value = authResult.RefreshToken
	refreshCookie.HttpOnly = true
	refreshCookie.Secure = true
	refreshCookie.Path = "/"
	refreshCookie.MaxAge = 30 * 24 * 60 * 60 // 30 days in seconds
	refreshCookie.SameSite = http.SameSiteStrictMode
	c.SetCookie(refreshCookie)

	return c.JSON(http.StatusOK, AuthResponse{
		IDToken:   authResult.IDToken,
		ExpiresIn: int(authResult.ExpiresIn),
		TokenType: "Bearer",
		User:      mapUserToResponse(authResult.User),
	})
}

// Authorize handles user token exchanging
// @Summary Authenticate a user
// @Description Exchange authorization code for tokens and set them as HTTP-only cookies
// @Tags auth
// @Accept json
// @Produce json
// @Param code query string true "Authorization code"
// @Success 200 {object} AuthResponse "Authentication successful with user info"
// @Failure 500 {object} errors.Error "Server error"
// @Router /auth/authorize [post]
// @Router /auth/authorize [post]
func (e *endpoints) Authorize(c echo.Context) error {
	code := c.QueryParam("code")
	// state := c.QueryParam("state")

	authResult, err := e.auth.ExchangeAuthCode(c.Request().Context(), code)
	if err != nil {
		return errors.New("Failed to exchange auth code").Wrap(err)
	}

	// Set access token in HTTP-only cookie
	accessCookie := new(http.Cookie)
	accessCookie.Name = "access_token"
	accessCookie.Value = authResult.AccessToken
	accessCookie.HttpOnly = true
	accessCookie.Secure = true
	accessCookie.Path = "/"
	accessCookie.MaxAge = int(authResult.ExpiresIn)
	accessCookie.SameSite = http.SameSiteStrictMode
	c.SetCookie(accessCookie)

	// Set refresh token in HTTP-only cookie (longer expiry)
	refreshCookie := new(http.Cookie)
	refreshCookie.Name = "refresh_token"
	refreshCookie.Value = authResult.RefreshToken
	refreshCookie.HttpOnly = true
	refreshCookie.Secure = true
	refreshCookie.Path = "/"
	refreshCookie.MaxAge = 30 * 24 * 60 * 60 // 30 days in seconds
	refreshCookie.SameSite = http.SameSiteStrictMode
	c.SetCookie(refreshCookie)

	return c.JSON(http.StatusOK, AuthResponse{
		IDToken:   authResult.IDToken,
		ExpiresIn: int(authResult.ExpiresIn),
		TokenType: "Bearer",
		User:      mapUserToResponse(authResult.User),
	})
}
