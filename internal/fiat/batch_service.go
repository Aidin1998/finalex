package fiat

import (
	"context"
	"fmt"
	"time"

	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// BatchFiatOperationResult holds the results of a batch fiat operation
type BatchFiatOperationResult struct {
	SuccessCount  int                            `json:"success_count"`
	FailedItems   map[string]error               `json:"failed_items"`
	Duration      time.Duration                  `json:"duration"`
	ProcessedData map[string]*models.Transaction `json:"processed_data,omitempty"`
}

// BatchTransactionRequest represents a batch transaction request
type BatchTransactionRequest struct {
	UserID      string                 `json:"user_id"`
	Type        string                 `json:"type"` // "deposit" or "withdrawal"
	Currency    string                 `json:"currency"`
	Amount      float64                `json:"amount"`
	Reference   string                 `json:"reference"`
	Description string                 `json:"description"`
	BankDetails map[string]interface{} `json:"bank_details,omitempty"`
	Provider    string                 `json:"provider,omitempty"`
}

// UserTransactionQuery represents a query for user transactions
type UserTransactionQuery struct {
	UserID    string `json:"user_id"`
	Currency  string `json:"currency,omitempty"`
	Type      string `json:"type"` // "deposit" or "withdrawal"
	Limit     int    `json:"limit"`
	Offset    int    `json:"offset"`
	RequestID string `json:"request_id"` // For tracking individual requests
}

// BatchTransactionQueryResult holds results for a batch transaction query
type BatchTransactionQueryResult struct {
	UserID       string                `json:"user_id"`
	RequestID    string                `json:"request_id"`
	Transactions []*models.Transaction `json:"transactions"`
	Total        int64                 `json:"total"`
	Error        error                 `json:"error,omitempty"`
}

// BatchGetTransactions retrieves transactions for multiple users efficiently (resolves N+1 queries)
func (s *Service) BatchGetTransactions(ctx context.Context, queries []UserTransactionQuery) (map[string]*BatchTransactionQueryResult, error) {
	if len(queries) == 0 {
		return make(map[string]*BatchTransactionQueryResult), nil
	}

	start := time.Now()
	defer func() {
		s.logger.Debug("BatchGetTransactions completed",
			zap.Int("query_count", len(queries)),
			zap.Duration("duration", time.Since(start)))
	}()

	results := make(map[string]*BatchTransactionQueryResult)

	// Group queries by type for optimization
	depositQueries := make([]UserTransactionQuery, 0)
	withdrawalQueries := make([]UserTransactionQuery, 0)

	for _, query := range queries {
		switch query.Type {
		case "deposit":
			depositQueries = append(depositQueries, query)
		case "withdrawal":
			withdrawalQueries = append(withdrawalQueries, query)
		default:
			results[query.RequestID] = &BatchTransactionQueryResult{
				UserID:    query.UserID,
				RequestID: query.RequestID,
				Error:     fmt.Errorf("invalid transaction type: %s", query.Type),
			}
		}
	}

	// Process deposits in batch
	if len(depositQueries) > 0 {
		depositResults, err := s.batchGetTransactionsByType(ctx, depositQueries, "deposit")
		if err != nil {
			s.logger.Error("Failed to batch get deposits", zap.Error(err))
		} else {
			for requestID, result := range depositResults {
				results[requestID] = result
			}
		}
	}

	// Process withdrawals in batch
	if len(withdrawalQueries) > 0 {
		withdrawalResults, err := s.batchGetTransactionsByType(ctx, withdrawalQueries, "withdrawal")
		if err != nil {
			s.logger.Error("Failed to batch get withdrawals", zap.Error(err))
		} else {
			for requestID, result := range withdrawalResults {
				results[requestID] = result
			}
		}
	}

	return results, nil
}

// batchGetTransactionsByType processes transaction queries for a specific type
func (s *Service) batchGetTransactionsByType(ctx context.Context, queries []UserTransactionQuery, transactionType string) (map[string]*BatchTransactionQueryResult, error) {
	results := make(map[string]*BatchTransactionQueryResult)

	if len(queries) == 0 {
		return results, nil
	}

	// Extract unique user IDs and currencies
	userIDs := make([]string, 0, len(queries))
	currencies := make(map[string]bool)
	userIDSet := make(map[string]bool)

	for _, query := range queries {
		if !userIDSet[query.UserID] {
			userIDs = append(userIDs, query.UserID)
			userIDSet[query.UserID] = true
		}
		if query.Currency != "" {
			currencies[query.Currency] = true
		}
	}

	// Build base query
	baseQuery := s.db.WithContext(ctx).Model(&models.Transaction{}).
		Where("user_id IN ? AND type = ?", userIDs, transactionType)

	// Add currency filter if specified for any query
	if len(currencies) > 0 {
		currencyList := make([]string, 0, len(currencies))
		for currency := range currencies {
			currencyList = append(currencyList, currency)
		}
		baseQuery = baseQuery.Where("currency IN ?", currencyList)
	}

	// Get all transactions for batch processing
	var transactions []models.Transaction
	if err := baseQuery.Order("user_id, created_at DESC").Find(&transactions).Error; err != nil {
		return nil, fmt.Errorf("failed to batch get %s transactions: %w", transactionType, err)
	}

	// Group transactions by user ID
	userTransactions := make(map[string][]*models.Transaction)
	for i := range transactions {
		userID := transactions[i].UserID.String()
		userTransactions[userID] = append(userTransactions[userID], &transactions[i])
	}

	// Process each query
	for _, query := range queries {
		result := &BatchTransactionQueryResult{
			UserID:    query.UserID,
			RequestID: query.RequestID,
		}

		userTxs, exists := userTransactions[query.UserID]
		if !exists {
			result.Transactions = []*models.Transaction{}
			result.Total = 0
		} else {
			// Filter by currency if specified
			filteredTxs := make([]*models.Transaction, 0)
			if query.Currency != "" {
				for _, tx := range userTxs {
					if tx.Currency == query.Currency {
						filteredTxs = append(filteredTxs, tx)
					}
				}
			} else {
				filteredTxs = userTxs
			}

			result.Total = int64(len(filteredTxs))

			// Apply pagination
			start := query.Offset
			end := query.Offset + query.Limit
			if start >= len(filteredTxs) {
				result.Transactions = []*models.Transaction{}
			} else {
				if end > len(filteredTxs) {
					end = len(filteredTxs)
				}
				result.Transactions = filteredTxs[start:end]
			}
		}

		results[query.RequestID] = result
	}

	return results, nil
}

// BatchGetDeposits retrieves deposits for multiple users efficiently
func (s *Service) BatchGetDeposits(ctx context.Context, userIDs []string, currency string, limit, offset int) (map[string]*BatchTransactionQueryResult, error) {
	queries := make([]UserTransactionQuery, len(userIDs))
	for i, userID := range userIDs {
		queries[i] = UserTransactionQuery{
			UserID:    userID,
			Currency:  currency,
			Type:      "deposit",
			Limit:     limit,
			Offset:    offset,
			RequestID: fmt.Sprintf("deposit_%s_%d", userID, i),
		}
	}

	return s.BatchGetTransactions(ctx, queries)
}

// BatchGetWithdrawals retrieves withdrawals for multiple users efficiently
func (s *Service) BatchGetWithdrawals(ctx context.Context, userIDs []string, currency string, limit, offset int) (map[string]*BatchTransactionQueryResult, error) {
	queries := make([]UserTransactionQuery, len(userIDs))
	for i, userID := range userIDs {
		queries[i] = UserTransactionQuery{
			UserID:    userID,
			Currency:  currency,
			Type:      "withdrawal",
			Limit:     limit,
			Offset:    offset,
			RequestID: fmt.Sprintf("withdrawal_%s_%d", userID, i),
		}
	}

	return s.BatchGetTransactions(ctx, queries)
}

// BatchCreateTransactions creates multiple fiat transactions efficiently
func (s *Service) BatchCreateTransactions(ctx context.Context, requests []BatchTransactionRequest) (*BatchFiatOperationResult, error) {
	start := time.Now()
	result := &BatchFiatOperationResult{
		FailedItems:   make(map[string]error),
		ProcessedData: make(map[string]*models.Transaction),
		Duration:      0,
	}

	defer func() {
		result.Duration = time.Since(start)
		s.logger.Debug("BatchCreateTransactions completed",
			zap.Int("total_requests", len(requests)),
			zap.Int("success_count", result.SuccessCount),
			zap.Int("failed_count", len(result.FailedItems)),
			zap.Duration("duration", result.Duration))
	}()

	if len(requests) == 0 {
		return result, nil
	}

	// Process transactions in batches of 50 to avoid memory issues
	batchSize := 50
	for i := 0; i < len(requests); i += batchSize {
		end := i + batchSize
		if end > len(requests) {
			end = len(requests)
		}

		batchRequests := requests[i:end]
		if err := s.processBatchCreateTransactions(ctx, batchRequests, result); err != nil {
			s.logger.Error("Failed to process batch create transactions",
				zap.Error(err),
				zap.Int("batch_start", i),
				zap.Int("batch_end", end))
		}
	}

	return result, nil
}

// processBatchCreateTransactions processes a batch of transaction creation requests
func (s *Service) processBatchCreateTransactions(ctx context.Context, requests []BatchTransactionRequest, result *BatchFiatOperationResult) error {
	tx := s.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	for _, req := range requests {
		reqKey := fmt.Sprintf("%s-%s-%s", req.UserID, req.Type, req.Currency)

		// Validate request
		if err := s.validateBatchTransactionRequest(req); err != nil {
			result.FailedItems[reqKey] = fmt.Errorf("validation failed: %w", err)
			continue
		}

		// Create transaction record
		transaction := &models.Transaction{
			ID:          uuid.New(),
			UserID:      uuid.MustParse(req.UserID),
			Type:        req.Type,
			Amount:      req.Amount,
			Currency:    req.Currency,
			Status:      "pending",
			Reference:   req.Reference,
			Description: req.Description,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		if err := tx.Create(transaction).Error; err != nil {
			result.FailedItems[reqKey] = fmt.Errorf("failed to create transaction: %w", err)
			continue
		}

		result.ProcessedData[reqKey] = transaction
		result.SuccessCount++
	}

	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit batch transaction: %w", err)
	}

	return nil
}

// validateBatchTransactionRequest validates a batch transaction request
func (s *Service) validateBatchTransactionRequest(req BatchTransactionRequest) error {
	// Validate user ID
	if _, err := uuid.Parse(req.UserID); err != nil {
		return fmt.Errorf("invalid user ID: %w", err)
	}

	// Validate transaction type
	if req.Type != "deposit" && req.Type != "withdrawal" {
		return fmt.Errorf("invalid transaction type: %s", req.Type)
	}

	// Validate currency
	if !isSupportedFiatCurrency(req.Currency) {
		return fmt.Errorf("unsupported currency: %s", req.Currency)
	}

	// Validate amount
	if req.Amount <= 0 {
		return fmt.Errorf("invalid amount: %f", req.Amount)
	}

	// Validate provider for deposits
	if req.Type == "deposit" && req.Provider != "" && !isSupportedProvider(req.Provider) {
		return fmt.Errorf("unsupported provider: %s", req.Provider)
	}

	// Validate bank details for withdrawals
	if req.Type == "withdrawal" && len(req.BankDetails) > 0 {
		if err := validateBankDetails(req.BankDetails); err != nil {
			return fmt.Errorf("invalid bank details: %w", err)
		}
	}

	return nil
}

// BatchCompleteTransactions completes multiple transactions efficiently
func (s *Service) BatchCompleteTransactions(ctx context.Context, transactionIDs []string) (*BatchFiatOperationResult, error) {
	start := time.Now()
	result := &BatchFiatOperationResult{
		FailedItems: make(map[string]error),
		Duration:    0,
	}

	defer func() {
		result.Duration = time.Since(start)
		s.logger.Debug("BatchCompleteTransactions completed",
			zap.Int("total_transactions", len(transactionIDs)),
			zap.Int("success_count", result.SuccessCount),
			zap.Int("failed_count", len(result.FailedItems)),
			zap.Duration("duration", result.Duration))
	}()

	if len(transactionIDs) == 0 {
		return result, nil
	}

	// Validate all transaction IDs first
	validIDs := make([]string, 0, len(transactionIDs))
	for _, id := range transactionIDs {
		if _, err := uuid.Parse(id); err != nil {
			result.FailedItems[id] = fmt.Errorf("invalid transaction ID: %w", err)
			continue
		}
		validIDs = append(validIDs, id)
	}

	if len(validIDs) == 0 {
		return result, nil
	}

	// Get all transactions to validate they exist and are pending
	var transactions []models.Transaction
	if err := s.db.WithContext(ctx).Where("id IN ? AND status = ?", validIDs, "pending").Find(&transactions).Error; err != nil {
		return nil, fmt.Errorf("failed to get transactions: %w", err)
	}

	// Map found transactions
	foundTxs := make(map[string]*models.Transaction)
	for i := range transactions {
		foundTxs[transactions[i].ID.String()] = &transactions[i]
	}

	// Mark missing/invalid transactions as failed
	for _, id := range validIDs {
		if _, exists := foundTxs[id]; !exists {
			result.FailedItems[id] = fmt.Errorf("transaction not found or not pending")
		}
	}

	// Update all valid transactions to completed status
	if len(foundTxs) > 0 {
		foundIDs := make([]string, 0, len(foundTxs))
		for id := range foundTxs {
			foundIDs = append(foundIDs, id)
		}

		updateResult := s.db.WithContext(ctx).Model(&models.Transaction{}).
			Where("id IN ?", foundIDs).
			Updates(map[string]interface{}{
				"status":     "completed",
				"updated_at": time.Now(),
			})

		if updateResult.Error != nil {
			return nil, fmt.Errorf("failed to batch complete transactions: %w", updateResult.Error)
		}

		result.SuccessCount = int(updateResult.RowsAffected)
	}

	return result, nil
}

// BatchGetTransactionsByStatus retrieves transactions by status for multiple users
func (s *Service) BatchGetTransactionsByStatus(ctx context.Context, userIDs []string, status string, transactionType string, limit int) (map[string][]*models.Transaction, error) {
	if len(userIDs) == 0 {
		return make(map[string][]*models.Transaction), nil
	}

	start := time.Now()
	defer func() {
		s.logger.Debug("BatchGetTransactionsByStatus completed",
			zap.Int("user_count", len(userIDs)),
			zap.String("status", status),
			zap.String("type", transactionType),
			zap.Duration("duration", time.Since(start)))
	}()

	query := s.db.WithContext(ctx).Model(&models.Transaction{}).
		Where("user_id IN ? AND status = ?", userIDs, status)

	if transactionType != "" {
		query = query.Where("type = ?", transactionType)
	}

	var transactions []models.Transaction
	if err := query.Order("user_id, created_at DESC").Limit(limit * len(userIDs)).Find(&transactions).Error; err != nil {
		return nil, fmt.Errorf("failed to batch get transactions by status: %w", err)
	}

	// Group by user ID
	results := make(map[string][]*models.Transaction)
	for i := range transactions {
		userID := transactions[i].UserID.String()
		if results[userID] == nil {
			results[userID] = make([]*models.Transaction, 0)
		}
		if len(results[userID]) < limit {
			results[userID] = append(results[userID], &transactions[i])
		}
	}

	// Ensure all users are represented in results
	for _, userID := range userIDs {
		if results[userID] == nil {
			results[userID] = []*models.Transaction{}
		}
	}

	return results, nil
}

// GetUserTransactionSummary provides transaction summary for a user
func (s *Service) GetUserTransactionSummary(ctx context.Context, userID string, currency string) (*UserTransactionSummary, error) {
	summary := &UserTransactionSummary{
		UserID:   userID,
		Currency: currency,
	}

	// Build base query
	query := s.db.WithContext(ctx).Model(&models.Transaction{}).Where("user_id = ?", userID)
	if currency != "" {
		query = query.Where("currency = ?", currency)
	}

	// Get deposit summary
	var depositStats struct {
		TotalDeposits   int64   `gorm:"column:total_deposits"`
		DepositAmount   float64 `gorm:"column:deposit_amount"`
		PendingDeposits int64   `gorm:"column:pending_deposits"`
	}

	depositQuery := query.Select(`
		COUNT(CASE WHEN type = 'deposit' THEN 1 END) as total_deposits,
		COALESCE(SUM(CASE WHEN type = 'deposit' AND status = 'completed' THEN amount END), 0) as deposit_amount,
		COUNT(CASE WHEN type = 'deposit' AND status = 'pending' THEN 1 END) as pending_deposits
	`)

	if err := depositQuery.Scan(&depositStats).Error; err != nil {
		return nil, fmt.Errorf("failed to get deposit summary: %w", err)
	}

	// Get withdrawal summary
	var withdrawalStats struct {
		TotalWithdrawals   int64   `gorm:"column:total_withdrawals"`
		WithdrawalAmount   float64 `gorm:"column:withdrawal_amount"`
		PendingWithdrawals int64   `gorm:"column:pending_withdrawals"`
	}

	withdrawalQuery := query.Select(`
		COUNT(CASE WHEN type = 'withdrawal' THEN 1 END) as total_withdrawals,
		COALESCE(SUM(CASE WHEN type = 'withdrawal' AND status = 'completed' THEN amount END), 0) as withdrawal_amount,
		COUNT(CASE WHEN type = 'withdrawal' AND status = 'pending' THEN 1 END) as pending_withdrawals
	`)

	if err := withdrawalQuery.Scan(&withdrawalStats).Error; err != nil {
		return nil, fmt.Errorf("failed to get withdrawal summary: %w", err)
	}

	summary.TotalDeposits = depositStats.TotalDeposits
	summary.TotalDepositAmount = depositStats.DepositAmount
	summary.PendingDeposits = depositStats.PendingDeposits
	summary.TotalWithdrawals = withdrawalStats.TotalWithdrawals
	summary.TotalWithdrawalAmount = withdrawalStats.WithdrawalAmount
	summary.PendingWithdrawals = withdrawalStats.PendingWithdrawals

	return summary, nil
}

// UserTransactionSummary holds transaction summary for a user
type UserTransactionSummary struct {
	UserID                string  `json:"user_id"`
	Currency              string  `json:"currency"`
	TotalDeposits         int64   `json:"total_deposits"`
	TotalDepositAmount    float64 `json:"total_deposit_amount"`
	PendingDeposits       int64   `json:"pending_deposits"`
	TotalWithdrawals      int64   `json:"total_withdrawals"`
	TotalWithdrawalAmount float64 `json:"total_withdrawal_amount"`
	PendingWithdrawals    int64   `json:"pending_withdrawals"`
}
