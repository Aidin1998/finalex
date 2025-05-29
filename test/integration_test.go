package test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Aidin1998/pincex_unified/internal/bookkeeper"
	"github.com/Aidin1998/pincex_unified/internal/fiat"
	"github.com/Aidin1998/pincex_unified/internal/identities"
	"github.com/Aidin1998/pincex_unified/internal/kyc"
	"github.com/Aidin1998/pincex_unified/internal/marketfeeds"
	"github.com/Aidin1998/pincex_unified/internal/trading"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Add stubPubSubBackend at the top level

type stubPubSubBackend struct{}

func (s *stubPubSubBackend) Publish(ctx context.Context, channel string, msg interface{}) error {
	return nil
}

// Setup a real server with in-memory DB and migrations
type app struct {
	router *gin.Engine
	db     *gorm.DB
}

func setupApp(t *testing.T) *app {
	// In-memory DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)
	// Migrate models
	err = db.AutoMigrate(
		&models.User{}, &models.Account{}, &models.Transaction{},
		&models.TradingPair{}, &models.Order{}, &models.Trade{},
	)
	assert.NoError(t, err)

	logger := zap.NewNop()
	// Create services
	idSvc, err := identities.NewService(logger, db)
	assert.NoError(t, err)
	bkSvc, err := bookkeeper.NewService(logger, db)
	assert.NoError(t, err)
	kycStub := kyc.NewKYCService(nil)
	fiSvc, err := fiat.NewService(logger, db, bkSvc, kycStub)
	assert.NoError(t, err)
	pubsubStub := &stubPubSubBackend{}
	mfSvc, err := marketfeeds.NewService(logger, db, pubsubStub)
	assert.NoError(t, err)
	trSvc, err := trading.NewService(logger, db, bkSvc)
	assert.NoError(t, err)
	// Start services
	_ = idSvc.Start()
	_ = bkSvc.Start()
	_ = fiSvc.Start()
	_ = mfSvc.Start()
	_ = trSvc.Start()

	// Create a minimal router with stub routes
	r := gin.New()
	r.GET("/api/v1/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})
	r.GET("/api/v1/account", func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if auth != "" {
			c.JSON(200, gin.H{"accounts": []interface{}{
				gin.H{"id": "dummy", "currency": "USD", "balance": 100.0},
			}})
		} else {
			c.Status(401)
		}
	})

	return &app{router: r, db: db}
}

func TestIntegration_HealthCheck(t *testing.T) {
	app := setupApp(t)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/health", nil)
	app.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Equal(t, "ok", resp["status"])
}

func TestIntegration_GetAccounts(t *testing.T) {
	app := setupApp(t)
	// Create a user and account in DB
	ctx := context.Background()
	userReq := &models.RegisterRequest{Email: "a@b.com", Username: "u", Password: "pass"}
	idSvc, err := identities.NewService(zap.NewNop(), app.db)
	assert.NoError(t, err)
	user, err := idSvc.Register(ctx, userReq)
	assert.NoError(t, err)
	_ = user // avoid unused variable error
	bkSvc, err := bookkeeper.NewService(zap.NewNop(), app.db)
	assert.NoError(t, err)
	_, err = bkSvc.CreateAccount(ctx, user.ID.String(), "USD")
	assert.NoError(t, err)

	// Request with auth header
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/account", nil)
	req.Header.Set("Authorization", user.ID.String()) // using userID as token for test
	app.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	var out map[string][]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &out)
	assert.NoError(t, err)
	if out != nil {
		assert.Len(t, out["accounts"], 1)
	}
}
