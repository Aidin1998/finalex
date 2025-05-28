package identities_test

import (
	"context"
	"testing"

	"github.com/Aidin1998/pincex_unified/internal/identities"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)
	assert.NoError(t, db.AutoMigrate(&models.User{}))
	return db
}

func TestRegisterAndLogin(t *testing.T) {
	db := setupTestDB(t)
	logger := zap.NewNop()
	svc, err := identities.NewService(logger, db)
	assert.NoError(t, err)

	ctx := context.Background()
	req := &models.RegisterRequest{
		Email:     "test@example.com",
		Username:  "testuser",
		Password:  "password123",
		FirstName: "Test",
		LastName:  "User",
	}

	user, err := svc.Register(ctx, req)
	assert.NoError(t, err)
	assert.Equal(t, req.Email, user.Email)
	assert.Equal(t, req.Username, user.Username)

	loginReq := &models.LoginRequest{
		Login:    req.Email,
		Password: req.Password,
	}
	resp, err := svc.Login(ctx, loginReq)
	assert.NoError(t, err)
	assert.NotEmpty(t, resp.Token)

	userID, err := svc.ValidateToken(resp.Token)
	assert.NoError(t, err)
	assert.Equal(t, user.ID.String(), userID)

	isAdmin, err := svc.IsAdmin(userID)
	assert.NoError(t, err)
	assert.False(t, isAdmin)
}
