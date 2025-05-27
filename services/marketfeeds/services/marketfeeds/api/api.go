package api

import (
	"net/http"

	"services/marketfeeds/aggregator"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

// HTTP handlers for market-feed endpoints

// RegisterRoutes sets up HTTP endpoints
func RegisterRoutes(r *gin.Engine) {
	r.GET("/health", healthHandler)
	r.GET("/average", averageHandler)
	// admin endpoints
	accounts := gin.Accounts{viper.GetString("ADMIN_USER"): viper.GetString("ADMIN_PASS")}
	admin := r.Group("/admin", gin.BasicAuth(accounts))
	admin.POST("/thresholds", thresholdsHandler)
	admin.POST("/disable-exchange", disableExchangeHandler)
	admin.POST("/enable-exchange", enableExchangeHandler)
}

func healthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func averageHandler(c *gin.Context) {
	symbol := c.Query("symbol")
	avg, prices, err := aggregator.FetchAverageAcrossExchanges(symbol)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"symbol": symbol, "average": avg, "sources": prices})
}

// thresholdsHandler updates thresholds
func thresholdsHandler(c *gin.Context) {
	var req struct {
		OutlierThresholdPercent float64 `json:"outlier_threshold_percent"`
		MarketMakerRangePercent float64 `json:"market_maker_range_percent"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	aggregator.SetOutlierThresholdPercent(req.OutlierThresholdPercent)
	aggregator.SetMarketMakerRangePercent(req.MarketMakerRangePercent)
	c.JSON(http.StatusOK, gin.H{"status": "thresholds updated"})
}

// disableExchangeHandler disables an exchange
func disableExchangeHandler(c *gin.Context) {
	var req struct {
		Exchange string `json:"exchange"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	aggregator.DisableExchange(req.Exchange)
	c.JSON(http.StatusOK, gin.H{"status": "exchange disabled", "exchange": req.Exchange})
}

// enableExchangeHandler enables an exchange
func enableExchangeHandler(c *gin.Context) {
	var req struct {
		Exchange string `json:"exchange"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	aggregator.EnableExchange(req.Exchange)
	c.JSON(http.StatusOK, gin.H{"status": "exchange enabled", "exchange": req.Exchange})
}
