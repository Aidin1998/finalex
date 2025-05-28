package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Aidin1998/pincex_unified/api"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

// Stub implementations of API service interfaces
type stubIdentity struct{}

func (s *stubIdentity) ValidateToken(token string) (string, error) { return "user-id", nil }
func (s *stubIdentity) IsAdmin(userID string) (bool, error)        { return false, nil }

type stubBookkeeper struct{}

func (s *stubBookkeeper) GetAccounts(ctx context.Context, userID string) ([]*models.Account, error) {
	return []*models.Account{}, nil
}

// stub for marketfeeds
type stubMarketfeeds struct{}

func (s *stubMarketfeeds) GetMarketPrices(ctx context.Context) ([]*models.MarketPrice, error) {
	return []*models.MarketPrice{}, nil
}
func (s *stubMarketfeeds) GetCandles(ctx context.Context, symbol, interval string, limit int) ([]*models.Candle, error) {
	return []*models.Candle{}, nil
}

// stub for fiat and trading (empty interfaces)
type stubFiat struct{}
type stubTrading struct{}

// helper to set up router
func setupRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	logger, _ := zap.NewDevelopment()
	srv := api.NewServer(
		logger,
		&stubIdentity{},
		&stubBookkeeper{},
		&stubFiat{},
		&stubMarketfeeds{},
		&stubTrading{},
		nil, // kycProvider
		nil, // walletService
	)
	return srv.Router()
}

func TestHealthCheck(t *testing.T) {
	router := setupRouter()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Equal(t, "ok", resp["status"])
}

func TestGetAccounts_Unauthorized(t *testing.T) {
	router := setupRouter()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/account", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestGetAccounts_Authorized(t *testing.T) {
	router := setupRouter()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/account", nil)
	req.Header.Set("Authorization", "Bearer token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string][]*models.Account
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Contains(t, resp, "accounts")
}
