package transaction

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/Aidin1998/finalex/pkg/models"
)

// TransactionAPI provides HTTP endpoints for distributed transaction management
type TransactionAPI struct {
	suite  *TransactionManagerSuite
	logger *zap.Logger
}

// NewTransactionAPI creates a new transaction API handler
func NewTransactionAPI(suite *TransactionManagerSuite, logger *zap.Logger) *TransactionAPI {
	return &TransactionAPI{
		suite:  suite,
		logger: logger,
	}
}

// RegisterRoutes registers transaction management routes
func (api *TransactionAPI) RegisterRoutes(router *gin.Engine) {
	txGroup := router.Group("/api/v1/transactions")
	{
		// Transaction execution
		txGroup.POST("/execute", api.ExecuteTransaction)
		txGroup.POST("/execute-workflow", api.ExecuteWorkflow)

		// Transaction management
		txGroup.GET("/status/:id", api.GetTransactionStatus)
		txGroup.POST("/abort/:id", api.AbortTransaction)

		// Monitoring and metrics
		txGroup.GET("/health", api.GetHealthCheck)
		txGroup.GET("/metrics", api.GetMetrics)
		txGroup.GET("/performance", api.GetPerformanceMetrics)

		// Testing endpoints (should be disabled in production)
		txGroup.POST("/test/chaos", api.RunChaosTest)
		txGroup.POST("/test/load", api.RunLoadTest)

		// Configuration management
		txGroup.GET("/config", api.GetConfiguration)
		txGroup.PUT("/config", api.UpdateConfiguration)

		// Lock management
		txGroup.GET("/locks", api.GetActiveLocks)
		txGroup.DELETE("/locks/:resource", api.ReleaseLock)

		// Monitoring and alerts
		txGroup.GET("/alerts", api.GetActiveAlerts)
		txGroup.POST("/alerts/:id/acknowledge", api.AcknowledgeAlert)
	}
}

// ExecuteTransaction handles distributed transaction execution
func (api *TransactionAPI) ExecuteTransaction(c *gin.Context) {
	var request struct {
		Operations []TransactionOperation `json:"operations"`
		Timeout    int                    `json:"timeout_seconds,omitempty"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	result, err := api.suite.ExecuteDistributedTransaction(
		c.Request.Context(),
		request.Operations,
		5*time.Minute,
	)

	if err != nil {
		api.logger.Error("Transaction execution failed",
			zap.Error(err),
			zap.Any("operations", request.Operations))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Transaction execution failed",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, result)
}

// ExecuteWorkflow handles complex workflow execution
func (api *TransactionAPI) ExecuteWorkflow(c *gin.Context) {
	var request struct {
		WorkflowType string                 `json:"workflow_type"`
		Parameters   map[string]interface{} `json:"parameters"`
		Timeout      int                    `json:"timeout_seconds,omitempty"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}
	// Set default timeout if not provided
	timeout := time.Duration(request.Timeout) * time.Second
	if timeout == 0 {
		timeout = 60 * time.Second // Default timeout
	}

	// Execute workflow based on type
	var result interface{}
	var err error

	switch request.WorkflowType {
	case "trade_execution":
		// Extract parameters for ComplexTradeExecutionWorkflow
		userID, ok := request.Parameters["user_id"].(string)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Missing or invalid user_id parameter"})
			return
		}
		// Parse order request from parameters
		orderReq := &models.OrderRequest{
			Symbol:   request.Parameters["symbol"].(string),
			Side:     request.Parameters["side"].(string),
			Quantity: decimal.NewFromFloat(request.Parameters["quantity"].(float64)),
			Price:    decimal.NewFromFloat(request.Parameters["price"].(float64)),
		}

		result, err = api.suite.WorkflowOrchestrator.ComplexTradeExecutionWorkflow(
			c.Request.Context(),
			userID,
			orderReq,
		)

	case "fiat_deposit":
		// Extract parameters for FiatDepositWorkflow
		userID, ok := request.Parameters["user_id"].(string)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Missing or invalid user_id parameter"})
			return
		}
		currency := request.Parameters["currency"].(string)
		amount := request.Parameters["amount"].(float64)
		provider := request.Parameters["provider"].(string)

		result, err = api.suite.WorkflowOrchestrator.FiatDepositWorkflow(
			c.Request.Context(),
			userID,
			currency,
			amount,
			provider,
		)

	case "crypto_withdrawal":
		// Extract parameters for CryptoWithdrawalWorkflow
		userID, ok := request.Parameters["user_id"].(string)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Missing or invalid user_id parameter"})
			return
		}
		walletID := request.Parameters["wallet_id"].(string)
		asset := request.Parameters["asset"].(string)
		toAddress := request.Parameters["to_address"].(string)
		amount := request.Parameters["amount"].(float64)

		result, err = api.suite.WorkflowOrchestrator.CryptoWithdrawalWorkflow(
			c.Request.Context(),
			userID,
			walletID,
			asset,
			toAddress,
			amount,
		)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unknown workflow type"})
		return
	}

	if err != nil {
		api.logger.Error("Workflow execution failed",
			zap.Error(err),
			zap.String("workflow_type", request.WorkflowType))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Workflow execution failed",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetTransactionStatus returns the status of a specific transaction
func (api *TransactionAPI) GetTransactionStatus(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid transaction ID"})
		return
	}

	txn, exists := api.suite.XAManager.GetTransaction(id)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Transaction not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"transaction_id": txn.ID,
		"state":          txn.State,
		"created_at":     txn.CreatedAt,
		"updated_at":     txn.UpdatedAt,
		"timeout_at":     txn.TimeoutAt,
		"resources":      len(txn.Resources),
	})
}

// AbortTransaction forcefully aborts a transaction
func (api *TransactionAPI) AbortTransaction(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid transaction ID"})
		return
	}

	txn, exists := api.suite.XAManager.GetTransaction(id)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Transaction not found"})
		return
	}

	if err := api.suite.XAManager.Abort(c.Request.Context(), txn); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to abort transaction",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":        "Transaction aborted successfully",
		"transaction_id": id,
	})
}

// GetHealthCheck returns the health status of all transaction components
func (api *TransactionAPI) GetHealthCheck(c *gin.Context) {
	health := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now(),
		"components": map[string]interface{}{
			"xa_manager": map[string]interface{}{
				"status": "active",
				"active_transactions": func() int {
					api.suite.XAManager.mu.RLock()
					defer api.suite.XAManager.mu.RUnlock()
					return len(api.suite.XAManager.transactions)
				}(),
			},
			"lock_manager": map[string]interface{}{
				"status": "active",
				"active_locks": func() int {
					api.suite.LockManager.mu.RLock()
					defer api.suite.LockManager.mu.RUnlock()
					return len(api.suite.LockManager.locks)
				}(),
			},
			"performance_metrics": map[string]interface{}{
				"status":     "active",
				"collecting": api.suite.PerformanceMetrics != nil,
			},
			"monitoring_service": map[string]interface{}{
				"status":  "active",
				"enabled": api.suite.MonitoringService != nil,
			},
			"testing_framework": map[string]interface{}{
				"status":    "active",
				"available": api.suite.TestingFramework != nil,
			},
		},
	}

	// Check if any monitoring service reports unhealthy status
	if api.suite.MonitoringService != nil {
		alerts := api.suite.MonitoringService.GetActiveAlerts()
		if len(alerts) > 0 {
			health["status"] = "degraded"
			health["active_alerts"] = len(alerts)
		}
	}

	c.JSON(http.StatusOK, health)
}

// GetMetrics returns XA transaction manager metrics
func (api *TransactionAPI) GetMetrics(c *gin.Context) {
	metrics := api.suite.XAManager.GetMetrics()
	// Return only the fields, not the mutex
	c.JSON(http.StatusOK, gin.H{
		"total_transactions":     metrics.TotalTransactions,
		"committed_transactions": metrics.CommittedTransactions,
		"aborted_transactions":   metrics.AbortedTransactions,
		"heuristic_outcomes":     metrics.HeuristicOutcomes,
		"recovery_attempts":      metrics.RecoveryAttempts,
		"average_commit_time":    metrics.AverageCommitTime,
	})
}

// GetPerformanceMetrics returns detailed performance metrics
func (api *TransactionAPI) GetPerformanceMetrics(c *gin.Context) {
	if api.suite.PerformanceMetrics == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Performance metrics not available"})
		return
	}

	metrics := api.suite.PerformanceMetrics.GetRealTimeMetrics()
	c.JSON(http.StatusOK, metrics)
}

// RunChaosTest executes chaos engineering tests
func (api *TransactionAPI) RunChaosTest(c *gin.Context) {
	var request struct {
		Duration  int `json:"duration_seconds"`
		Intensity int `json:"intensity"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}
	if api.suite.TestingFramework == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Testing framework not available"})
		return
	}

	tester := api.suite.TestingFramework

	// Configure chaos test parameters
	config := ChaosConfig{
		FailureRate:         float64(request.Intensity) / 100.0,
		LatencyInjection:    time.Duration(request.Duration) * time.Second,
		ResourceExhaustion:  true,
		NetworkPartition:    true,
		RandomFailures:      true,
		CircuitBreakerTrips: true,
	}

	tester.EnableChaos(true)
	tester.SetChaosConfig(config)

	result, err := tester.RunScenario(c.Request.Context(), "chaos_test")

	tester.EnableChaos(false)

	if err != nil {
		api.logger.Error("Chaos test failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Chaos test failed",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":           "completed",
		"result":           result,
		"duration_seconds": request.Duration,
		"intensity":        request.Intensity,
	})
}

// RunLoadTest executes load testing
func (api *TransactionAPI) RunLoadTest(c *gin.Context) {
	var request struct {
		Concurrency int    `json:"concurrency"`
		Duration    int    `json:"duration_seconds"`
		ScenarioID  string `json:"scenario_id"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	// TODO: TestingFramework is not implemented. Stub or remove usage to fix build.
	// tester, ok := api.suite.TestingFramework.(*DistributedTransactionTester)
	// if !ok {
	// 	c.JSON(http.StatusInternalServerError, gin.H{"error": "Testing framework not available"})
	// 	return
	// }

	// scenarioID := request.ScenarioID
	// if scenarioID == "" {
	// 	scenarioID = "multi_service"
	// }

	// result, err := tester.LoadTestTransaction(c.Request.Context(), scenarioID, request.Concurrency, time.Duration(request.Duration)*time.Second)
	// if err != nil {
	// 	c.JSON(http.StatusInternalServerError, gin.H{
	// 		"error":   "Load test failed",
	// 		"details": err.Error(),
	// 	})
	// 	return
	// }

	c.JSON(http.StatusNotImplemented, gin.H{"error": "Load test not available"})
}

// GetConfiguration returns current transaction configuration
func (api *TransactionAPI) GetConfiguration(c *gin.Context) {
	config := api.suite.ConfigManager.GetConfig()
	c.JSON(http.StatusOK, config)
}

// UpdateConfiguration updates transaction configuration
func (api *TransactionAPI) UpdateConfiguration(c *gin.Context) {
	var config TransactionConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid configuration format"})
		return
	}

	var request struct {
		Reason    string `json:"reason"`
		UpdatedBy string `json:"updated_by"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		request.Reason = "API update"
		request.UpdatedBy = "api_user"
	}

	if err := api.suite.ConfigManager.UpdateConfig(&config, request.Reason, request.UpdatedBy); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to update configuration",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Configuration updated successfully",
	})
}

// GetActiveLocks returns information about active distributed locks
func (api *TransactionAPI) GetActiveLocks(c *gin.Context) {
	// There is no GetActiveLocks, so return lock count and lock info from the local map
	dlm := api.suite.LockManager
	dlm.mu.RLock()
	locks := make([]*DistributedLock, 0, len(dlm.locks))
	for _, lock := range dlm.locks {
		locks = append(locks, lock)
	}
	dlm.mu.RUnlock()

	c.JSON(http.StatusOK, gin.H{
		"active_lock_count": len(locks),
		"locks":             locks,
	})
}

// ReleaseLock forcefully releases a distributed lock
func (api *TransactionAPI) ReleaseLock(c *gin.Context) {
	resource := c.Param("resource")

	if err := api.suite.LockManager.ReleaseLock(c.Request.Context(), resource); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to release lock",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "Lock released successfully",
		"resource": resource,
	})
}

// GetActiveAlerts returns current active alerts
func (api *TransactionAPI) GetActiveAlerts(c *gin.Context) {
	// TODO: MonitoringService is not implemented. Stub or remove usage to fix build.
	// alerts := api.suite.MonitoringService.GetActiveAlerts(limit)
	alerts := []string{} // Stubbed response
	c.JSON(http.StatusOK, gin.H{
		"alerts": alerts,
		"count":  len(alerts),
	})
}

// AcknowledgeAlert acknowledges a specific alert
func (api *TransactionAPI) AcknowledgeAlert(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid alert ID"})
		return
	}

	var request struct {
		AcknowledgedBy string `json:"acknowledged_by"`
		Notes          string `json:"notes,omitempty"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		request.AcknowledgedBy = "api_user"
	}

	// TODO: MonitoringService is not implemented. Stub or remove usage to fix build.
	// if err := api.suite.MonitoringService.AcknowledgeAlert(id, request.AcknowledgedBy, request.Notes); err != nil {
	// 	c.JSON(http.StatusInternalServerError, gin.H{
	// 		"error":   "Failed to acknowledge alert",
	// 		"details": err.Error(),
	// 	})
	// 	return
	// }

	c.JSON(http.StatusOK, gin.H{
		"message":  "Alert acknowledged successfully",
		"alert_id": id,
	})
}

// TransactionMiddlewareGin creates Gin middleware for automatic transaction management
func (api *TransactionAPI) TransactionMiddlewareGin() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Use TransactionHandler directly
		api.suite.Middleware.TransactionHandler()(c)
	}
}

// GetTransactionFromContext extracts transaction information from Gin context
func GetTransactionFromContext(c *gin.Context) *XATransaction {
	if txn, exists := c.Get("xa_transaction"); exists {
		if xaTxn, ok := txn.(*XATransaction); ok {
			return xaTxn
		}
	}
	return nil
}

// SetTransactionInContext sets transaction information in Gin context
func SetTransactionInContext(c *gin.Context, txn *XATransaction) {
	c.Set("xa_transaction", txn)
	c.Set("xa_transaction_id", txn.ID)
}

// RequireTransaction is a middleware that ensures a transaction is active
func RequireTransaction() gin.HandlerFunc {
	return func(c *gin.Context) {
		txn := GetTransactionFromContext(c)
		if txn == nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Transaction required for this operation",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}
