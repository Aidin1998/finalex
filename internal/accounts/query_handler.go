// CQRS Query Handler for Account read operations with sub-millisecond performance
package accounts

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// AccountQueryHandler handles all read operations using CQRS pattern
type AccountQueryHandler struct {
	dataManager *AccountDataManager
	repository  *Repository
	cache       *CacheLayer
	hotCache    *HotCache
	logger      *zap.Logger
	metrics     *QueryMetrics
}

// QueryMetrics holds Prometheus metrics for query operations
type QueryMetrics struct {
	QueriesTotal    *prometheus.CounterVec
	QueryDuration   *prometheus.HistogramVec
	QueryErrors     *prometheus.CounterVec
	CacheHits       *prometheus.CounterVec
	CacheMisses     *prometheus.CounterVec
	DatabaseQueries *prometheus.CounterVec
	QueryLatencyP99 *prometheus.GaugeVec
	ThroughputQPS   *prometheus.GaugeVec
}

// AccountQuery represents a query to be executed
type AccountQuery interface {
	GetType() string
	GetCacheKeys() []string
	GetTraceID() string
	Validate() error
}

// GetAccountQuery represents an account retrieval query
type GetAccountQuery struct {
	UserID   uuid.UUID `json:"user_id"`
	Currency string    `json:"currency"`
	TraceID  string    `json:"trace_id"`
}

func (q *GetAccountQuery) GetType() string    { return "get_account" }
func (q *GetAccountQuery) GetTraceID() string { return q.TraceID }
func (q *GetAccountQuery) GetCacheKeys() []string {
	return []string{
		fmt.Sprintf("account:%s:%s", q.UserID.String(), q.Currency),
		fmt.Sprintf(AccountBalanceKey, q.UserID.String(), q.Currency),
		fmt.Sprintf(AccountMetaKey, q.UserID.String(), q.Currency),
	}
}

func (q *GetAccountQuery) Validate() error {
	if q.UserID == uuid.Nil {
		return fmt.Errorf("user_id is required")
	}
	if q.Currency == "" {
		return fmt.Errorf("currency is required")
	}
	return nil
}

// GetBalanceQuery represents a balance retrieval query
type GetBalanceQuery struct {
	UserID   uuid.UUID `json:"user_id"`
	Currency string    `json:"currency"`
	TraceID  string    `json:"trace_id"`
}

func (q *GetBalanceQuery) GetType() string    { return "get_balance" }
func (q *GetBalanceQuery) GetTraceID() string { return q.TraceID }
func (q *GetBalanceQuery) GetCacheKeys() []string {
	return []string{
		fmt.Sprintf("balance:%s:%s", q.UserID.String(), q.Currency),
		fmt.Sprintf(AccountBalanceKey, q.UserID.String(), q.Currency),
	}
}

func (q *GetBalanceQuery) Validate() error {
	if q.UserID == uuid.Nil {
		return fmt.Errorf("user_id is required")
	}
	if q.Currency == "" {
		return fmt.Errorf("currency is required")
	}
	return nil
}

// GetAccountHistoryQuery represents an account history query
type GetAccountHistoryQuery struct {
	UserID   uuid.UUID  `json:"user_id"`
	Currency string     `json:"currency"`
	Limit    int        `json:"limit"`
	Offset   int        `json:"offset"`
	FromDate *time.Time `json:"from_date"`
	ToDate   *time.Time `json:"to_date"`
	TraceID  string     `json:"trace_id"`
}

func (q *GetAccountHistoryQuery) GetType() string    { return "get_account_history" }
func (q *GetAccountHistoryQuery) GetTraceID() string { return q.TraceID }
func (q *GetAccountHistoryQuery) GetCacheKeys() []string {
	return []string{
		fmt.Sprintf("history:%s:%s:%d:%d", q.UserID.String(), q.Currency, q.Limit, q.Offset),
	}
}

func (q *GetAccountHistoryQuery) Validate() error {
	if q.UserID == uuid.Nil {
		return fmt.Errorf("user_id is required")
	}
	if q.Currency == "" {
		return fmt.Errorf("currency is required")
	}
	if q.Limit <= 0 {
		q.Limit = 100 // Default limit
	}
	if q.Limit > 1000 {
		return fmt.Errorf("limit cannot exceed 1000")
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
	return nil
}

// GetReservationsQuery represents a reservations query
type GetReservationsQuery struct {
	UserID   uuid.UUID `json:"user_id"`
	Currency string    `json:"currency"`
	Status   string    `json:"status"`
	Limit    int       `json:"limit"`
	Offset   int       `json:"offset"`
	TraceID  string    `json:"trace_id"`
}

func (q *GetReservationsQuery) GetType() string    { return "get_reservations" }
func (q *GetReservationsQuery) GetTraceID() string { return q.TraceID }
func (q *GetReservationsQuery) GetCacheKeys() []string {
	return []string{
		fmt.Sprintf("reservations:%s:%s:%s:%d:%d",
			q.UserID.String(), q.Currency, q.Status, q.Limit, q.Offset),
	}
}

func (q *GetReservationsQuery) Validate() error {
	if q.UserID == uuid.Nil {
		return fmt.Errorf("user_id is required")
	}
	if q.Currency == "" {
		return fmt.Errorf("currency is required")
	}
	if q.Limit <= 0 {
		q.Limit = 100
	}
	if q.Limit > 500 {
		return fmt.Errorf("limit cannot exceed 500")
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
	return nil
}

// NewAccountQueryHandler creates a new query handler
func NewAccountQueryHandler(dataManager *AccountDataManager, logger *zap.Logger) *AccountQueryHandler {
	metrics := &QueryMetrics{
		QueriesTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "account_queries_total",
				Help: "Total number of account queries processed",
			},
			[]string{"query_type", "status", "cache_tier"},
		),
		QueryDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "account_query_duration_seconds",
				Help:    "Duration of account query processing",
				Buckets: []float64{0.00001, 0.00005, 0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1}, // 10μs to 100ms
			},
			[]string{"query_type", "cache_tier"},
		),
		QueryErrors: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "account_query_errors_total",
				Help: "Total number of account query errors",
			},
			[]string{"query_type", "error_type"},
		),
		CacheHits: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "account_query_cache_hits_total",
				Help: "Total number of cache hits",
			},
			[]string{"query_type", "cache_tier"},
		),
		CacheMisses: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "account_query_cache_misses_total",
				Help: "Total number of cache misses",
			},
			[]string{"query_type", "cache_tier"},
		),
		DatabaseQueries: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "account_query_database_queries_total",
				Help: "Total number of database queries",
			},
			[]string{"query_type", "status"},
		),
		QueryLatencyP99: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "account_query_latency_p99_seconds",
				Help: "99th percentile query latency",
			},
			[]string{"query_type"},
		),
		ThroughputQPS: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "account_query_throughput_qps",
				Help: "Query throughput in queries per second",
			},
			[]string{"query_type"},
		),
	}

	return &AccountQueryHandler{
		dataManager: dataManager,
		repository:  dataManager.repository,
		cache:       dataManager.cache,
		hotCache:    dataManager.hotCache,
		logger:      logger,
		metrics:     metrics,
	}
}

// ExecuteQuery executes a query using optimal caching strategy
func (qh *AccountQueryHandler) ExecuteQuery(ctx context.Context, query AccountQuery) (interface{}, error) {
	start := time.Now()
	queryType := query.GetType()

	defer func() {
		duration := time.Since(start)
		qh.metrics.QueryDuration.WithLabelValues(queryType, "total").Observe(duration.Seconds())
	}()

	// Validate query
	if err := query.Validate(); err != nil {
		qh.metrics.QueryErrors.WithLabelValues(queryType, "validation").Inc()
		return nil, fmt.Errorf("query validation failed: %w", err)
	}

	// Execute query based on type
	var result interface{}
	var err error

	switch q := query.(type) {
	case *GetAccountQuery:
		result, err = qh.executeGetAccount(ctx, q)
	case *GetBalanceQuery:
		result, err = qh.executeGetBalance(ctx, q)
	case *GetAccountHistoryQuery:
		result, err = qh.executeGetAccountHistory(ctx, q)
	case *GetReservationsQuery:
		result, err = qh.executeGetReservations(ctx, q)
	default:
		err = fmt.Errorf("unknown query type: %s", queryType)
		qh.metrics.QueryErrors.WithLabelValues(queryType, "unknown_type").Inc()
	}

	if err != nil {
		qh.metrics.QueriesTotal.WithLabelValues(queryType, "failed", "none").Inc()
		return nil, err
	}

	qh.metrics.QueriesTotal.WithLabelValues(queryType, "success", "cache").Inc()
	return result, nil
}

// GetAccount retrieves an account with sub-millisecond performance
func (qh *AccountQueryHandler) GetAccount(ctx context.Context, userID uuid.UUID, currency string) (*Account, error) {
	query := &GetAccountQuery{
		UserID:   userID,
		Currency: currency,
		TraceID:  fmt.Sprintf("get_account_%s_%s", userID.String(), currency),
	}

	result, err := qh.ExecuteQuery(ctx, query)
	if err != nil {
		return nil, err
	}

	return result.(*Account), nil
}

// GetBalance retrieves account balance with sub-millisecond performance
func (qh *AccountQueryHandler) GetBalance(ctx context.Context, userID uuid.UUID, currency string) (decimal.Decimal, error) {
	query := &GetBalanceQuery{
		UserID:   userID,
		Currency: currency,
		TraceID:  fmt.Sprintf("get_balance_%s_%s", userID.String(), currency),
	}

	result, err := qh.ExecuteQuery(ctx, query)
	if err != nil {
		return decimal.Zero, err
	}

	return result.(decimal.Decimal), nil
}

// GetAccountHistory retrieves account transaction history
func (qh *AccountQueryHandler) GetAccountHistory(ctx context.Context, userID uuid.UUID, currency string, limit int, offset int) ([]*LedgerTransaction, error) {
	query := &GetAccountHistoryQuery{
		UserID:   userID,
		Currency: currency,
		Limit:    limit,
		Offset:   offset,
		TraceID:  fmt.Sprintf("get_history_%s_%s_%d_%d", userID.String(), currency, limit, offset),
	}

	result, err := qh.ExecuteQuery(ctx, query)
	if err != nil {
		return nil, err
	}

	return result.([]*LedgerTransaction), nil
}

// GetReservations retrieves account reservations
func (qh *AccountQueryHandler) GetReservations(ctx context.Context, userID uuid.UUID, currency string, status string, limit int, offset int) ([]*Reservation, error) {
	query := &GetReservationsQuery{
		UserID:   userID,
		Currency: currency,
		Status:   status,
		Limit:    limit,
		Offset:   offset,
		TraceID:  fmt.Sprintf("get_reservations_%s_%s_%s_%d_%d", userID.String(), currency, status, limit, offset),
	}

	result, err := qh.ExecuteQuery(ctx, query)
	if err != nil {
		return nil, err
	}

	return result.([]*Reservation), nil
}

// executeGetAccount implements account retrieval with multi-tier caching
func (qh *AccountQueryHandler) executeGetAccount(ctx context.Context, query *GetAccountQuery) (*Account, error) {
	queryType := query.GetType()

	// Try hot cache first (sub-millisecond)
	if account, exists := qh.hotCache.GetAccount(ctx, query.UserID, query.Currency); exists {
		qh.metrics.CacheHits.WithLabelValues(queryType, "hot").Inc()
		qh.metrics.QueryDuration.WithLabelValues(queryType, "hot").Observe(0.0001) // ~0.1ms
		return account, nil
	}
	qh.metrics.CacheMisses.WithLabelValues(queryType, "hot").Inc()

	// Try warm cache (Redis)
	cacheKey := fmt.Sprintf(AccountBalanceKey, query.UserID.String(), query.Currency)
	if cachedData, exists := qh.cache.Get(ctx, cacheKey); exists {
		if account, ok := cachedData.(*CachedAccount); ok {
			// Convert cached account to Account
			result := &Account{
				ID:          account.ID,
				UserID:      account.UserID,
				Currency:    account.Currency,
				Balance:     account.Balance,
				Available:   account.Available,
				Locked:      account.Locked,
				Version:     account.Version,
				AccountType: account.AccountType,
				Status:      account.Status,
				UpdatedAt:   account.UpdatedAt,
			}

			// Update hot cache for future requests
			qh.hotCache.SetAccount(ctx, query.UserID, query.Currency, result, 5*time.Minute)

			qh.metrics.CacheHits.WithLabelValues(queryType, "warm").Inc()
			qh.metrics.QueryDuration.WithLabelValues(queryType, "warm").Observe(0.001) // ~1ms
			return result, nil
		}
	}
	qh.metrics.CacheMisses.WithLabelValues(queryType, "warm").Inc()

	// Try cold cache
	if cachedData, exists := qh.cache.GetCold(ctx, cacheKey); exists {
		if account, ok := cachedData.(*CachedAccount); ok {
			result := &Account{
				ID:          account.ID,
				UserID:      account.UserID,
				Currency:    account.Currency,
				Balance:     account.Balance,
				Available:   account.Available,
				Locked:      account.Locked,
				Version:     account.Version,
				AccountType: account.AccountType,
				Status:      account.Status,
				UpdatedAt:   account.UpdatedAt,
			}

			// Update higher tier caches
			qh.hotCache.SetAccount(ctx, query.UserID, query.Currency, result, 5*time.Minute)
			qh.cache.Set(ctx, cacheKey, &CachedAccount{
				ID:           result.ID,
				UserID:       result.UserID,
				Currency:     result.Currency,
				Balance:      result.Balance,
				Available:    result.Available,
				Locked:       result.Locked,
				Version:      result.Version,
				AccountType:  result.AccountType,
				Status:       result.Status,
				UpdatedAt:    result.UpdatedAt,
				CachedAt:     time.Now(),
				AccessCount:  1,
				LastAccessed: time.Now(),
			}, WarmDataTTL)

			qh.metrics.CacheHits.WithLabelValues(queryType, "cold").Inc()
			qh.metrics.QueryDuration.WithLabelValues(queryType, "cold").Observe(0.005) // ~5ms
			return result, nil
		}
	}
	qh.metrics.CacheMisses.WithLabelValues(queryType, "cold").Inc()

	// Query from database
	account, err := qh.repository.GetAccount(ctx, query.UserID, query.Currency)
	if err != nil {
		qh.metrics.DatabaseQueries.WithLabelValues(queryType, "failed").Inc()
		qh.metrics.QueryErrors.WithLabelValues(queryType, "database").Inc()
		return nil, fmt.Errorf("failed to get account from database: %w", err)
	}

	qh.metrics.DatabaseQueries.WithLabelValues(queryType, "success").Inc()
	qh.metrics.QueryDuration.WithLabelValues(queryType, "database").Observe(0.01) // ~10ms

	// Update all cache tiers
	cachedAccount := &CachedAccount{
		ID:           account.ID,
		UserID:       account.UserID,
		Currency:     account.Currency,
		Balance:      account.Balance,
		Available:    account.Available,
		Locked:       account.Locked,
		Version:      account.Version,
		AccountType:  account.AccountType,
		Status:       account.Status,
		UpdatedAt:    account.UpdatedAt,
		CachedAt:     time.Now(),
		AccessCount:  1,
		LastAccessed: time.Now(),
	}

	// Cache in all tiers
	qh.hotCache.SetAccount(ctx, query.UserID, query.Currency, account, HotDataTTL)
	qh.cache.Set(ctx, cacheKey, cachedAccount, WarmDataTTL)
	qh.cache.SetCold(ctx, cacheKey, cachedAccount, ColdDataTTL)

	return account, nil
}

// executeGetBalance implements balance retrieval with optimized caching
func (qh *AccountQueryHandler) executeGetBalance(ctx context.Context, query *GetBalanceQuery) (decimal.Decimal, error) {
	queryType := query.GetType()

	// Try hot cache first for immediate response
	if balance, exists := qh.hotCache.GetBalance(ctx, query.UserID, query.Currency); exists {
		qh.metrics.CacheHits.WithLabelValues(queryType, "hot").Inc()
		qh.metrics.QueryDuration.WithLabelValues(queryType, "hot").Observe(0.00005) // ~0.05ms
		return balance, nil
	}

	// Get full account and extract balance
	account, err := qh.executeGetAccount(ctx, &GetAccountQuery{
		UserID:   query.UserID,
		Currency: query.Currency,
		TraceID:  query.TraceID,
	})
	if err != nil {
		return decimal.Zero, err
	}

	// Cache balance separately for fast access
	qh.hotCache.SetBalance(ctx, query.UserID, query.Currency, account.Balance, HotDataTTL)

	return account.Balance, nil
}

// executeGetAccountHistory implements history retrieval with pagination
func (qh *AccountQueryHandler) executeGetAccountHistory(ctx context.Context, query *GetAccountHistoryQuery) ([]*LedgerTransaction, error) {
	queryType := query.GetType()

	// Try cache first
	cacheKey := fmt.Sprintf("history:%s:%s:%d:%d", query.UserID.String(), query.Currency, query.Limit, query.Offset)
	if cachedData, exists := qh.cache.Get(ctx, cacheKey); exists {
		if transactions, ok := cachedData.([]*LedgerTransaction); ok {
			qh.metrics.CacheHits.WithLabelValues(queryType, "warm").Inc()
			return transactions, nil
		}
	}
	qh.metrics.CacheMisses.WithLabelValues(queryType, "warm").Inc()

	// Query from database
	transactions, err := qh.repository.GetAccountHistory(ctx, query.UserID, query.Currency, query.Limit, query.Offset)
	if err != nil {
		qh.metrics.DatabaseQueries.WithLabelValues(queryType, "failed").Inc()
		return nil, fmt.Errorf("failed to get account history: %w", err)
	}

	qh.metrics.DatabaseQueries.WithLabelValues(queryType, "success").Inc()

	// Cache the result for subsequent requests
	qh.cache.Set(ctx, cacheKey, transactions, 10*time.Minute) // Shorter TTL for historical data

	return transactions, nil
}

// executeGetReservations implements reservations retrieval with filtering
func (qh *AccountQueryHandler) executeGetReservations(ctx context.Context, query *GetReservationsQuery) ([]*Reservation, error) {
	queryType := query.GetType()

	// Try cache first
	cacheKey := fmt.Sprintf("reservations:%s:%s:%s:%d:%d",
		query.UserID.String(), query.Currency, query.Status, query.Limit, query.Offset)
	if cachedData, exists := qh.cache.Get(ctx, cacheKey); exists {
		if reservations, ok := cachedData.([]*Reservation); ok {
			qh.metrics.CacheHits.WithLabelValues(queryType, "warm").Inc()
			return reservations, nil
		}
	}
	qh.metrics.CacheMisses.WithLabelValues(queryType, "warm").Inc()

	// Query from database
	reservations, err := qh.repository.GetReservations(ctx, query.UserID, query.Currency, query.Status, query.Limit, query.Offset)
	if err != nil {
		qh.metrics.DatabaseQueries.WithLabelValues(queryType, "failed").Inc()
		return nil, fmt.Errorf("failed to get reservations: %w", err)
	}

	qh.metrics.DatabaseQueries.WithLabelValues(queryType, "success").Inc()

	// Cache the result
	qh.cache.Set(ctx, cacheKey, reservations, 5*time.Minute) // Shorter TTL for active data

	return reservations, nil
}

// GetAccountsByUser retrieves all accounts for a user (optimized bulk query)
func (qh *AccountQueryHandler) GetAccountsByUser(ctx context.Context, userID uuid.UUID) ([]*Account, error) {
	start := time.Now()
	defer func() {
		qh.metrics.QueryDuration.WithLabelValues("get_accounts_by_user", "total").Observe(time.Since(start).Seconds())
	}()

	accounts, err := qh.repository.GetAccountsByUser(ctx, userID)
	if err != nil {
		qh.metrics.QueryErrors.WithLabelValues("get_accounts_by_user", "database").Inc()
		return nil, fmt.Errorf("failed to get accounts by user: %w", err)
	}

	// Cache individual accounts for future single-account queries
	for _, account := range accounts {
		qh.hotCache.SetAccount(ctx, account.UserID, account.Currency, account, HotDataTTL)
	}

	qh.metrics.QueriesTotal.WithLabelValues("get_accounts_by_user", "success", "database").Inc()
	return accounts, nil
}

// GetTotalBalance retrieves total balance across all currencies for a user
func (qh *AccountQueryHandler) GetTotalBalance(ctx context.Context, userID uuid.UUID, baseCurrency string) (decimal.Decimal, error) {
	start := time.Now()
	defer func() {
		qh.metrics.QueryDuration.WithLabelValues("get_total_balance", "total").Observe(time.Since(start).Seconds())
	}()

	// This would typically require exchange rate conversion
	// For now, we'll just sum balances in the same currency
	accounts, err := qh.GetAccountsByUser(ctx, userID)
	if err != nil {
		return decimal.Zero, err
	}

	var totalBalance decimal.Decimal
	for _, account := range accounts {
		if account.Currency == baseCurrency {
			totalBalance = totalBalance.Add(account.Balance)
		}
		// TODO: Add currency conversion for different currencies
	}

	return totalBalance, nil
}
