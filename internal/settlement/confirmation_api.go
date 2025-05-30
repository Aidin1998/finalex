// Package settlement provides the confirmation API for settlement status queries.
package settlement

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// SettlementConfirmation is reused from settlement_processor.go
// type SettlementConfirmation struct { ... }

// ConfirmationStore is a simple in-memory store for demo (replace with DB in prod)
type ConfirmationStore struct {
	data map[string]SettlementConfirmation
}

func NewConfirmationStore() *ConfirmationStore {
	return &ConfirmationStore{data: make(map[string]SettlementConfirmation)}
}

func (cs *ConfirmationStore) Save(conf SettlementConfirmation) {
	cs.data[conf.SettlementID] = conf
}

func (cs *ConfirmationStore) Get(settlementID string) (SettlementConfirmation, bool) {
	conf, ok := cs.data[settlementID]
	return conf, ok
}

// RegisterConfirmationAPI registers the confirmation status API endpoints.
func RegisterConfirmationAPI(router *gin.Engine, store *ConfirmationStore) {
	router.GET("/api/v1/settlement/confirmation/:id", func(c *gin.Context) {
		settlementID := c.Param("id")
		conf, ok := store.Get(settlementID)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusOK, conf)
	})

	router.GET("/api/v1/settlement/receipt/:id", func(c *gin.Context) {
		settlementID := c.Param("id")
		conf, ok := store.Get(settlementID)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"settlement_id": conf.SettlementID,
			"trade_id":      conf.TradeID,
			"status":        conf.Status,
			"receipt":       conf.Receipt,
			"confirmed_at":  conf.ConfirmedAt.Format(time.RFC3339),
		})
	})
}
