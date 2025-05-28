// Deprecated: Logic is now in internal/identities/service.go

package api

// Me returns the authenticated user's profile information
// @Summary Get current user
// @Description Returns the profile information of the currently authenticated user
// @Tags users
// @Produce json
// @Success 200 {object} UserResponse
// @Failure 404 {object} errors.Error "User not found"
// @Failure 500 {object} errors.Error "Server error"
// @Router /users/me [get]
func (e *endpoints) Me(c echo.Context) error {
	userId := c.Get("userID").(uuid.UUID)

	user, err := e.users.User(c.Request().Context(), userId)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, mapUserToResponse(user))
}

// UpdateMeRequest represents the request body for updating user names
type UpdateMeRequest struct {
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
}

// UpdateNames handles updating a user's first name and last name
// @Summary Update user names
// @Description Update a user's first name and last name
// @Tags users
// @Accept json
// @Produce json
// @Param request body UpdateNamesRequest true "Update names request"
// @Success 200 {object} UserResponse
// @Failure 400 {object} errors.Error "Invalid request"
// @Failure 404 {object} errors.Error "User not found"
// @Failure 500 {object} errors.Error "Server error"
// @Router /users/me/names [put]
func (e *endpoints) UpdateNames(c echo.Context) error {
	userId := c.Get("userID").(uuid.UUID)

	var req UpdateMeRequest
	if err := c.Bind(&req); err != nil {
		c.Error(err)
		return nil
	}

	if err := c.Validate(&req); err != nil {
		c.Error(err)
		return nil
	}

	user, err := e.users.UpdateNames(c.Request().Context(), userId, req.FirstName, req.LastName)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, mapUserToResponse(user))
}
