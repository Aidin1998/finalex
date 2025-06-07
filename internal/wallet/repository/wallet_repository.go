// Package repository provides data access layer for wallet module
package repository

import (
	"context"
	"time"

	"github.com/Aidin1998/finalex/internal/wallet/interfaces"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// WalletRepository implements the wallet repository interface
type WalletRepository struct {
	db     *gorm.DB
	logger *zap.Logger
}

// NewWalletRepository creates a new wallet repository
func NewWalletRepository(db *gorm.DB, logger *zap.Logger) *WalletRepository {
	return &WalletRepository{
		db:     db,
		logger: logger,
	}
}

// Transaction operations

// CreateTransaction creates a new wallet transaction
func (wr *WalletRepository) CreateTransaction(ctx context.Context, tx *interfaces.WalletTransaction) error {
	return wr.db.WithContext(ctx).Create(tx).Error
}

// GetTransaction retrieves a transaction by ID
func (wr *WalletRepository) GetTransaction(ctx context.Context, txID uuid.UUID) (*interfaces.WalletTransaction, error) {
	var tx interfaces.WalletTransaction
	err := wr.db.WithContext(ctx).Where("id = ?", txID).First(&tx).Error
	if err != nil {
		return nil, err
	}
	return &tx, nil
}

// UpdateTransaction updates a transaction
func (wr *WalletRepository) UpdateTransaction(ctx context.Context, tx *interfaces.WalletTransaction) error {
	return wr.db.WithContext(ctx).Save(tx).Error
}

// UpdateTransactionInTx updates a transaction within a database transaction
func (wr *WalletRepository) UpdateTransactionInTx(ctx context.Context, dbTx *gorm.DB, tx *interfaces.WalletTransaction) error {
	return dbTx.WithContext(ctx).Save(tx).Error
}

// GetTransactionByFireblocksID retrieves a transaction by Fireblocks ID
func (wr *WalletRepository) GetTransactionByFireblocksID(ctx context.Context, fireblocksID string) (*interfaces.WalletTransaction, error) {
	var tx interfaces.WalletTransaction
	err := wr.db.WithContext(ctx).Where("fireblocks_id = ?", fireblocksID).First(&tx).Error
	if err != nil {
		return nil, err
	}
	return &tx, nil
}

// GetTransactionByTxHash retrieves a transaction by transaction hash
func (wr *WalletRepository) GetTransactionByTxHash(ctx context.Context, txHash string) (*interfaces.WalletTransaction, error) {
	var tx interfaces.WalletTransaction
	err := wr.db.WithContext(ctx).Where("tx_hash = ?", txHash).First(&tx).Error
	if err != nil {
		return nil, err
	}
	return &tx, nil
}

// GetUserTransactions retrieves transactions for a user
func (wr *WalletRepository) GetUserTransactions(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*interfaces.WalletTransaction, error) {
	var transactions []*interfaces.WalletTransaction
	err := wr.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&transactions).Error
	return transactions, err
}

// GetUserTransactionsByDirection retrieves transactions for a user by direction
func (wr *WalletRepository) GetUserTransactionsByDirection(ctx context.Context, userID uuid.UUID, direction interfaces.Direction, limit, offset int) ([]*interfaces.WalletTransaction, error) {
	var transactions []*interfaces.WalletTransaction
	err := wr.db.WithContext(ctx).
		Where("user_id = ? AND direction = ?", userID, direction).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&transactions).Error
	return transactions, err
}

// GetUserTransactionsByAsset retrieves transactions for a user by asset
func (wr *WalletRepository) GetUserTransactionsByAsset(ctx context.Context, userID uuid.UUID, asset string, limit, offset int) ([]*interfaces.WalletTransaction, error) {
	var transactions []*interfaces.WalletTransaction
	err := wr.db.WithContext(ctx).
		Where("user_id = ? AND asset = ?", userID, asset).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&transactions).Error
	return transactions, err
}

// CountPendingDeposits counts pending deposits for a user
func (wr *WalletRepository) CountPendingDeposits(ctx context.Context, userID uuid.UUID) (int, error) {
	var count int64
	err := wr.db.WithContext(ctx).
		Model(&interfaces.WalletTransaction{}).
		Where("user_id = ? AND direction = ? AND status IN ?",
			userID, interfaces.DirectionDeposit,
			[]interfaces.TxStatus{interfaces.TxStatusInitiated, interfaces.TxStatusPending, interfaces.TxStatusConfirming}).
		Count(&count).Error
	return int(count), err
}

// GetDailyWithdrawalTotal gets total withdrawal amount for a user on a specific day
func (wr *WalletRepository) GetDailyWithdrawalTotal(ctx context.Context, userID uuid.UUID, asset string, date time.Time) (decimal.Decimal, error) {
	var total decimal.Decimal
	endDate := date.Add(24 * time.Hour)

	err := wr.db.WithContext(ctx).
		Model(&interfaces.WalletTransaction{}).
		Select("COALESCE(SUM(amount), 0)").
		Where("user_id = ? AND asset = ? AND direction = ? AND status = ? AND created_at >= ? AND created_at < ?",
			userID, asset, interfaces.DirectionWithdrawal, interfaces.TxStatusCompleted, date, endDate).
		Scan(&total).Error

	return total, err
}

// Balance operations

// CreateBalance creates a new balance record
func (wr *WalletRepository) CreateBalance(ctx context.Context, balance *interfaces.WalletBalance) error {
	return wr.db.WithContext(ctx).Create(balance).Error
}

// CreateBalanceInTx creates a new balance record within a transaction
func (wr *WalletRepository) CreateBalanceInTx(ctx context.Context, dbTx *gorm.DB, balance *interfaces.WalletBalance) error {
	return dbTx.WithContext(ctx).Create(balance).Error
}

// GetBalance retrieves balance for a user and asset
func (wr *WalletRepository) GetBalance(ctx context.Context, userID uuid.UUID, asset string) (*interfaces.WalletBalance, error) {
	var balance interfaces.WalletBalance
	err := wr.db.WithContext(ctx).
		Where("user_id = ? AND asset = ?", userID, asset).
		First(&balance).Error
	if err != nil {
		return nil, err
	}
	return &balance, nil
}

// GetBalanceForUpdate retrieves balance with row lock
func (wr *WalletRepository) GetBalanceForUpdate(ctx context.Context, userID uuid.UUID, asset string) (*interfaces.WalletBalance, error) {
	return wr.GetBalanceForUpdateInTx(ctx, wr.db, userID, asset)
}

// GetBalanceForUpdateInTx retrieves balance with row lock within a transaction
func (wr *WalletRepository) GetBalanceForUpdateInTx(ctx context.Context, dbTx *gorm.DB, userID uuid.UUID, asset string) (*interfaces.WalletBalance, error) {
	var balance interfaces.WalletBalance
	err := dbTx.WithContext(ctx).
		Set("gorm:query_option", "FOR UPDATE").
		Where("user_id = ? AND asset = ?", userID, asset).
		First(&balance).Error
	if err != nil {
		return nil, err
	}
	return &balance, nil
}

// UpdateBalance updates a balance record
func (wr *WalletRepository) UpdateBalance(ctx context.Context, balance *interfaces.WalletBalance) error {
	return wr.db.WithContext(ctx).Save(balance).Error
}

// UpdateBalanceInTx updates a balance record within a transaction
func (wr *WalletRepository) UpdateBalanceInTx(ctx context.Context, dbTx *gorm.DB, balance *interfaces.WalletBalance) error {
	return dbTx.WithContext(ctx).Save(balance).Error
}

// GetUserBalances retrieves all balances for a user
func (wr *WalletRepository) GetUserBalances(ctx context.Context, userID uuid.UUID) ([]*interfaces.WalletBalance, error) {
	var balances []*interfaces.WalletBalance
	err := wr.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Find(&balances).Error
	return balances, err
}

// Fund lock operations

// CreateFundLock creates a new fund lock
func (wr *WalletRepository) CreateFundLock(ctx context.Context, lock *interfaces.FundLock) error {
	return wr.db.WithContext(ctx).Create(lock).Error
}

// CreateFundLockInTx creates a new fund lock within a transaction
func (wr *WalletRepository) CreateFundLockInTx(ctx context.Context, dbTx *gorm.DB, lock *interfaces.FundLock) error {
	return dbTx.WithContext(ctx).Create(lock).Error
}

// GetFundLock retrieves a fund lock by ID
func (wr *WalletRepository) GetFundLock(ctx context.Context, lockID uuid.UUID) (*interfaces.FundLock, error) {
	var lock interfaces.FundLock
	err := wr.db.WithContext(ctx).Where("id = ?", lockID).First(&lock).Error
	if err != nil {
		return nil, err
	}
	return &lock, nil
}

// GetFundLockForUpdate retrieves a fund lock with row lock
func (wr *WalletRepository) GetFundLockForUpdate(ctx context.Context, lockID uuid.UUID) (*interfaces.FundLock, error) {
	return wr.GetFundLockForUpdateInTx(ctx, wr.db, lockID)
}

// GetFundLockForUpdateInTx retrieves a fund lock with row lock within a transaction
func (wr *WalletRepository) GetFundLockForUpdateInTx(ctx context.Context, dbTx *gorm.DB, lockID uuid.UUID) (*interfaces.FundLock, error) {
	var lock interfaces.FundLock
	err := dbTx.WithContext(ctx).
		Set("gorm:query_option", "FOR UPDATE").
		Where("id = ?", lockID).
		First(&lock).Error
	if err != nil {
		return nil, err
	}
	return &lock, nil
}

// UpdateFundLock updates a fund lock
func (wr *WalletRepository) UpdateFundLock(ctx context.Context, lock *interfaces.FundLock) error {
	return wr.db.WithContext(ctx).Save(lock).Error
}

// DeleteFundLock deletes a fund lock
func (wr *WalletRepository) DeleteFundLock(ctx context.Context, lockID uuid.UUID) error {
	return wr.db.WithContext(ctx).Delete(&interfaces.FundLock{}, lockID).Error
}

// DeleteFundLockInTx deletes a fund lock within a transaction
func (wr *WalletRepository) DeleteFundLockInTx(ctx context.Context, dbTx *gorm.DB, lockID uuid.UUID) error {
	return dbTx.WithContext(ctx).Delete(&interfaces.FundLock{}, lockID).Error
}

// GetUserFundLocks retrieves all fund locks for a user
func (wr *WalletRepository) GetUserFundLocks(ctx context.Context, userID uuid.UUID) ([]*interfaces.FundLock, error) {
	var locks []*interfaces.FundLock
	err := wr.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&locks).Error
	return locks, err
}

// CountUserLocks counts active locks for a user
func (wr *WalletRepository) CountUserLocks(ctx context.Context, userID uuid.UUID) (int, error) {
	var count int64
	err := wr.db.WithContext(ctx).
		Model(&interfaces.FundLock{}).
		Where("user_id = ?", userID).
		Count(&count).Error
	return int(count), err
}

// GetExpiredFundLocks retrieves expired fund locks
func (wr *WalletRepository) GetExpiredFundLocks(ctx context.Context, before time.Time) ([]*interfaces.FundLock, error) {
	var locks []*interfaces.FundLock
	err := wr.db.WithContext(ctx).
		Where("expires IS NOT NULL AND expires < ?", before).
		Find(&locks).Error
	return locks, err
}

// Address operations

// CreateAddress creates a new deposit address
func (wr *WalletRepository) CreateAddress(ctx context.Context, address *interfaces.DepositAddress) error {
	return wr.db.WithContext(ctx).Create(address).Error
}

// GetAddress retrieves an address by ID
func (wr *WalletRepository) GetAddress(ctx context.Context, addressID uuid.UUID) (*interfaces.DepositAddress, error) {
	var address interfaces.DepositAddress
	err := wr.db.WithContext(ctx).Where("id = ?", addressID).First(&address).Error
	if err != nil {
		return nil, err
	}
	return &address, nil
}

// GetAddressByValue retrieves an address by its value
func (wr *WalletRepository) GetAddressByValue(ctx context.Context, address string) (*interfaces.DepositAddress, error) {
	var addr interfaces.DepositAddress
	err := wr.db.WithContext(ctx).Where("address = ?", address).First(&addr).Error
	if err != nil {
		return nil, err
	}
	return &addr, nil
}

// UpdateAddress updates an address
func (wr *WalletRepository) UpdateAddress(ctx context.Context, address *interfaces.DepositAddress) error {
	return wr.db.WithContext(ctx).Save(address).Error
}

// GetUserAddresses retrieves all addresses for a user
func (wr *WalletRepository) GetAllUserAddresses(ctx context.Context, userID uuid.UUID) ([]*interfaces.DepositAddress, error) {
	var addresses []*interfaces.DepositAddress
	err := wr.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&addresses).Error
	return addresses, err
}

// GetUserAddressesByAsset retrieves addresses for a user and asset
func (wr *WalletRepository) GetUserAddressesByAsset(ctx context.Context, userID uuid.UUID, asset string) ([]*interfaces.DepositAddress, error) {
	var addresses []*interfaces.DepositAddress
	err := wr.db.WithContext(ctx).
		Where("user_id = ? AND asset = ?", userID, asset).
		Order("created_at DESC").
		Find(&addresses).Error
	return addresses, err
}

// Health check operations

// HealthCheck performs a health check on the database
func (wr *WalletRepository) HealthCheck(ctx context.Context) error {
	var result int
	return wr.db.WithContext(ctx).Raw("SELECT 1").Scan(&result).Error
}

// Migration and setup operations

// AutoMigrate creates/updates database tables
func (wr *WalletRepository) AutoMigrate() error {
	return wr.db.AutoMigrate(
		&interfaces.WalletTransaction{},
		&interfaces.WalletBalance{},
		&interfaces.FundLock{},
		&interfaces.DepositAddress{},
	)
}

// CreateIndexes creates database indexes for performance
func (wr *WalletRepository) CreateIndexes() error {
	// Transaction indexes
	if err := wr.db.Exec("CREATE INDEX IF NOT EXISTS idx_wallet_transactions_user_id ON wallet_transactions(user_id)").Error; err != nil {
		return err
	}
	if err := wr.db.Exec("CREATE INDEX IF NOT EXISTS idx_wallet_transactions_status ON wallet_transactions(status)").Error; err != nil {
		return err
	}
	if err := wr.db.Exec("CREATE INDEX IF NOT EXISTS idx_wallet_transactions_fireblocks_id ON wallet_transactions(fireblocks_id)").Error; err != nil {
		return err
	}
	if err := wr.db.Exec("CREATE INDEX IF NOT EXISTS idx_wallet_transactions_tx_hash ON wallet_transactions(tx_hash)").Error; err != nil {
		return err
	}
	if err := wr.db.Exec("CREATE INDEX IF NOT EXISTS idx_wallet_transactions_user_direction ON wallet_transactions(user_id, direction)").Error; err != nil {
		return err
	}
	if err := wr.db.Exec("CREATE INDEX IF NOT EXISTS idx_wallet_transactions_user_asset ON wallet_transactions(user_id, asset)").Error; err != nil {
		return err
	}

	// Balance indexes
	if err := wr.db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_wallet_balances_user_asset ON wallet_balances(user_id, asset)").Error; err != nil {
		return err
	}

	// Fund lock indexes
	if err := wr.db.Exec("CREATE INDEX IF NOT EXISTS idx_fund_locks_user_id ON fund_locks(user_id)").Error; err != nil {
		return err
	}
	if err := wr.db.Exec("CREATE INDEX IF NOT EXISTS idx_fund_locks_expires ON fund_locks(expires)").Error; err != nil {
		return err
	}

	// Address indexes
	if err := wr.db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_deposit_addresses_address ON deposit_addresses(address)").Error; err != nil {
		return err
	}
	if err := wr.db.Exec("CREATE INDEX IF NOT EXISTS idx_deposit_addresses_user_asset ON deposit_addresses(user_id, asset)").Error; err != nil {
		return err
	}

	return nil
}

// Statistics and reporting operations

// GetTransactionStats retrieves transaction statistics
func (wr *WalletRepository) GetTransactionStats(ctx context.Context, userID uuid.UUID, from, to time.Time) (*interfaces.TransactionStats, error) {
	stats := &interfaces.TransactionStats{
		UserID:   userID,
		From:     from,
		To:       to,
		ByAsset:  make(map[string]*interfaces.AssetStats),
		ByStatus: make(map[interfaces.TxStatus]int64),
	}

	// Get transaction counts by status
	var statusCounts []struct {
		Status string
		Count  int
	}
	err := wr.db.WithContext(ctx).
		Model(&interfaces.WalletTransaction{}).
		Select("status, COUNT(*) as count").
		Where("user_id = ? AND created_at >= ? AND created_at <= ?", userID, from, to).
		Group("status").
		Scan(&statusCounts).Error
	if err != nil {
		return nil, err
	}
	for _, sc := range statusCounts {
		stats.ByStatus[interfaces.TxStatus(sc.Status)] = int64(sc.Count)
		stats.TotalTransactions += int64(sc.Count)
	}

	// Get asset statistics
	var assetStats []struct {
		Asset       string
		Direction   string
		Count       int
		TotalAmount decimal.Decimal
	}
	err = wr.db.WithContext(ctx).
		Model(&interfaces.WalletTransaction{}).
		Select("asset, direction, COUNT(*) as count, COALESCE(SUM(amount), 0) as total_amount").
		Where("user_id = ? AND created_at >= ? AND created_at <= ?", userID, from, to).
		Group("asset, direction").
		Scan(&assetStats).Error
	if err != nil {
		return nil, err
	}

	for _, as := range assetStats {
		if stats.ByAsset[as.Asset] == nil {
			stats.ByAsset[as.Asset] = &interfaces.AssetStats{
				Asset: as.Asset,
			}
		}
		if as.Direction == string(interfaces.DirectionDeposit) {
			stats.ByAsset[as.Asset].DepositCount = int64(as.Count)
			stats.ByAsset[as.Asset].DepositAmount = as.TotalAmount
		} else {
			stats.ByAsset[as.Asset].WithdrawalCount = int64(as.Count)
			stats.ByAsset[as.Asset].WithdrawalAmount = as.TotalAmount
		}
	}

	return stats, nil
}
