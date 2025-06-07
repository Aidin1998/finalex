// Package wallet provides background workers for the wallet module
package wallet

import (
	"context"
	"fmt"
	"time"

	"your-project/internal/wallet/config"
	"your-project/internal/wallet/interfaces"
	"your-project/internal/wallet/state"
	"your-project/pkg/logger"
)

// ConfirmationWorker monitors transaction confirmations
type ConfirmationWorker struct {
	repository   interfaces.WalletRepository
	fireblocks   interfaces.FireblocksClient
	stateMachine *state.TransactionStateMachine
	config       *config.WalletConfig
	log          logger.Logger
	interval     time.Duration
	stopCh       chan struct{}
}

// Name returns the worker name
func (w *ConfirmationWorker) Name() string {
	return "confirmation-worker"
}

// Start starts the confirmation worker
func (w *ConfirmationWorker) Start(ctx context.Context) error {
	w.stopCh = make(chan struct{})

	go w.run(ctx)
	return nil
}

// Stop stops the confirmation worker
func (w *ConfirmationWorker) Stop(ctx context.Context) error {
	if w.stopCh != nil {
		close(w.stopCh)
	}
	return nil
}

// run executes the worker loop
func (w *ConfirmationWorker) run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopCh:
			return
		case <-ticker.C:
			if err := w.processConfirmations(ctx); err != nil {
				w.log.Error("failed to process confirmations", "error", err)
			}
		}
	}
}

// processConfirmations processes pending transactions for confirmations
func (w *ConfirmationWorker) processConfirmations(ctx context.Context) error {
	// Get pending transactions
	transactions, err := w.repository.GetTransactionsByStatus(ctx, []interfaces.TxStatus{
		interfaces.TxStatusPending,
		interfaces.TxStatusConfirming,
	})
	if err != nil {
		return fmt.Errorf("failed to get pending transactions: %w", err)
	}

	for _, tx := range transactions {
		if err := w.processTransaction(ctx, tx); err != nil {
			w.log.Error("failed to process transaction confirmation",
				"tx_id", tx.ID,
				"error", err,
			)
		}
	}

	return nil
}

// processTransaction processes a single transaction for confirmations
func (w *ConfirmationWorker) processTransaction(ctx context.Context, tx *interfaces.WalletTransaction) error {
	if tx.FireblocksID == "" {
		return nil // Skip transactions without Fireblocks ID
	}

	// Get transaction status from Fireblocks
	fbTx, err := w.fireblocks.GetTransaction(ctx, tx.FireblocksID)
	if err != nil {
		return fmt.Errorf("failed to get Fireblocks transaction: %w", err)
	}

	// Update confirmations
	newConfirmations := fbTx.NumOfConfirmations
	if newConfirmations != tx.Confirmations {
		if err := w.repository.UpdateTransactionConfirmations(ctx, tx.ID, newConfirmations); err != nil {
			return fmt.Errorf("failed to update confirmations: %w", err)
		}
		tx.Confirmations = newConfirmations
	}

	// Update transaction hash if available
	if fbTx.TxHash != "" && fbTx.TxHash != tx.TxHash {
		if err := w.repository.UpdateTransactionHash(ctx, tx.ID, fbTx.TxHash); err != nil {
			w.log.Warn("failed to update transaction hash", "tx_id", tx.ID, "error", err)
		}
		tx.TxHash = fbTx.TxHash
	}

	// Check for state transitions
	return w.checkStateTransition(ctx, tx, fbTx)
}

// checkStateTransition checks if transaction should transition state
func (w *ConfirmationWorker) checkStateTransition(ctx context.Context, tx *interfaces.WalletTransaction, fbTx *interfaces.FireblocksTransaction) error {
	var newState interfaces.TxStatus
	var metadata map[string]interface{}

	switch fbTx.Status {
	case "SUBMITTED":
		if tx.Status == interfaces.TxStatusInitiated {
			newState = interfaces.TxStatusPending
		}
	case "PENDING_SIGNATURE", "PENDING_AUTHORIZATION", "QUEUED", "PENDING_3RD_PARTY_MANUAL_APPROVAL":
		if tx.Status == interfaces.TxStatusInitiated {
			newState = interfaces.TxStatusPending
		}
	case "BROADCASTING":
		if tx.Status == interfaces.TxStatusPending {
			newState = interfaces.TxStatusConfirming
		}
	case "CONFIRMING":
		if tx.Status == interfaces.TxStatusPending {
			newState = interfaces.TxStatusConfirming
		}
	case "CONFIRMED":
		if tx.Confirmations >= tx.RequiredConf {
			if tx.Status == interfaces.TxStatusConfirming {
				newState = interfaces.TxStatusConfirmed
			} else if tx.Status == interfaces.TxStatusConfirmed {
				newState = interfaces.TxStatusCompleted
			}
		}
	case "COMPLETED":
		if tx.Status != interfaces.TxStatusCompleted {
			newState = interfaces.TxStatusCompleted
		}
	case "CANCELLED", "REJECTED":
		if tx.Status != interfaces.TxStatusRejected {
			newState = interfaces.TxStatusRejected
			metadata = map[string]interface{}{
				"reason": fbTx.SubStatus,
			}
		}
	case "FAILED":
		if tx.Status != interfaces.TxStatusFailed {
			newState = interfaces.TxStatusFailed
			metadata = map[string]interface{}{
				"error": fbTx.SubStatus,
			}
		}
	}

	// Execute state transition if needed
	if newState != "" && newState != tx.Status {
		if metadata == nil {
			metadata = make(map[string]interface{})
		}
		metadata["confirmations"] = tx.Confirmations
		metadata["tx_hash"] = fbTx.TxHash
		metadata["fireblocks_status"] = fbTx.Status
		metadata["fireblocks_substatus"] = fbTx.SubStatus

		if _, err := w.stateMachine.TransitionTo(ctx, tx.ID, newState, metadata); err != nil {
			return fmt.Errorf("failed to transition state: %w", err)
		}
	}

	return nil
}

// CleanupWorker cleans up expired locks and old data
type CleanupWorker struct {
	repository      interfaces.WalletRepository
	fundLockService interfaces.FundLockService
	log             logger.Logger
	interval        time.Duration
	stopCh          chan struct{}
}

// Name returns the worker name
func (w *CleanupWorker) Name() string {
	return "cleanup-worker"
}

// Start starts the cleanup worker
func (w *CleanupWorker) Start(ctx context.Context) error {
	w.stopCh = make(chan struct{})

	go w.run(ctx)
	return nil
}

// Stop stops the cleanup worker
func (w *CleanupWorker) Stop(ctx context.Context) error {
	if w.stopCh != nil {
		close(w.stopCh)
	}
	return nil
}

// run executes the worker loop
func (w *CleanupWorker) run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopCh:
			return
		case <-ticker.C:
			if err := w.performCleanup(ctx); err != nil {
				w.log.Error("failed to perform cleanup", "error", err)
			}
		}
	}
}

// performCleanup performs cleanup operations
func (w *CleanupWorker) performCleanup(ctx context.Context) error {
	// Clean up expired fund locks
	if err := w.cleanupExpiredLocks(ctx); err != nil {
		w.log.Error("failed to cleanup expired locks", "error", err)
	}

	// Clean up old transaction data (optional)
	if err := w.cleanupOldTransactions(ctx); err != nil {
		w.log.Error("failed to cleanup old transactions", "error", err)
	}

	return nil
}

// cleanupExpiredLocks removes expired fund locks
func (w *CleanupWorker) cleanupExpiredLocks(ctx context.Context) error {
	count, err := w.repository.CleanupExpiredLocks(ctx)
	if err != nil {
		return fmt.Errorf("failed to cleanup expired locks: %w", err)
	}

	if count > 0 {
		w.log.Info("cleaned up expired fund locks", "count", count)
	}

	return nil
}

// cleanupOldTransactions archives or removes old transaction data
func (w *CleanupWorker) cleanupOldTransactions(ctx context.Context) error {
	// Archive transactions older than 1 year
	cutoff := time.Now().AddDate(-1, 0, 0)

	count, err := w.repository.ArchiveOldTransactions(ctx, cutoff)
	if err != nil {
		return fmt.Errorf("failed to archive old transactions: %w", err)
	}

	if count > 0 {
		w.log.Info("archived old transactions", "count", count, "cutoff", cutoff)
	}

	return nil
}

// BalanceSyncWorker synchronizes balances with Fireblocks
type BalanceSyncWorker struct {
	repository     interfaces.WalletRepository
	fireblocks     interfaces.FireblocksClient
	balanceManager interfaces.BalanceManager
	config         *config.WalletConfig
	log            logger.Logger
	interval       time.Duration
	stopCh         chan struct{}
}

// Name returns the worker name
func (w *BalanceSyncWorker) Name() string {
	return "balance-sync-worker"
}

// Start starts the balance sync worker
func (w *BalanceSyncWorker) Start(ctx context.Context) error {
	w.stopCh = make(chan struct{})

	go w.run(ctx)
	return nil
}

// Stop stops the balance sync worker
func (w *BalanceSyncWorker) Stop(ctx context.Context) error {
	if w.stopCh != nil {
		close(w.stopCh)
	}
	return nil
}

// run executes the worker loop
func (w *BalanceSyncWorker) run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopCh:
			return
		case <-ticker.C:
			if err := w.synchronizeBalances(ctx); err != nil {
				w.log.Error("failed to synchronize balances", "error", err)
			}
		}
	}
}

// synchronizeBalances synchronizes balances with Fireblocks
func (w *BalanceSyncWorker) synchronizeBalances(ctx context.Context) error {
	// Get balances from Fireblocks
	fbBalances, err := w.fireblocks.GetVaultAccountBalances(ctx, w.config.Fireblocks.VaultAccountID)
	if err != nil {
		return fmt.Errorf("failed to get Fireblocks balances: %w", err)
	}

	// Process each asset balance
	for _, fbBalance := range fbBalances {
		assetConfig, exists := w.config.GetAssetConfig(fbBalance.AssetID)
		if !exists {
			continue // Skip unconfigured assets
		}

		if err := w.syncAssetBalance(ctx, assetConfig.Symbol, fbBalance); err != nil {
			w.log.Error("failed to sync asset balance",
				"asset", assetConfig.Symbol,
				"error", err,
			)
		}
	}

	return nil
}

// syncAssetBalance synchronizes balance for a specific asset
func (w *BalanceSyncWorker) syncAssetBalance(ctx context.Context, asset string, fbBalance *interfaces.VaultAccountBalance) error {
	// Get total user balances from database
	totalUserBalance, err := w.repository.GetTotalUserBalance(ctx, asset)
	if err != nil {
		return fmt.Errorf("failed to get total user balance: %w", err)
	}

	// Compare with Fireblocks balance
	fbTotal := fbBalance.Available.Add(fbBalance.Pending)

	if !totalUserBalance.Equal(fbTotal) {
		w.log.Warn("balance mismatch detected",
			"asset", asset,
			"user_total", totalUserBalance,
			"fireblocks_total", fbTotal,
			"difference", fbTotal.Sub(totalUserBalance),
		)

		// TODO: Implement balance reconciliation logic
		// This could involve:
		// 1. Checking for missing deposits
		// 2. Verifying withdrawal completions
		// 3. Alerting administrators
		// 4. Creating reconciliation transactions
	}

	return nil
}

// WebhookWorker processes Fireblocks webhooks
type WebhookWorker struct {
	repository   interfaces.WalletRepository
	stateMachine *state.TransactionStateMachine
	config       *config.WalletConfig
	log          logger.Logger
	webhookCh    chan interfaces.FireblocksWebhook
	stopCh       chan struct{}
}

// Name returns the worker name
func (w *WebhookWorker) Name() string {
	return "webhook-worker"
}

// Start starts the webhook worker
func (w *WebhookWorker) Start(ctx context.Context) error {
	w.webhookCh = make(chan interfaces.FireblocksWebhook, 100)
	w.stopCh = make(chan struct{})

	go w.run(ctx)
	return nil
}

// Stop stops the webhook worker
func (w *WebhookWorker) Stop(ctx context.Context) error {
	if w.stopCh != nil {
		close(w.stopCh)
	}
	if w.webhookCh != nil {
		close(w.webhookCh)
	}
	return nil
}

// ProcessWebhook queues a webhook for processing
func (w *WebhookWorker) ProcessWebhook(webhook interfaces.FireblocksWebhook) {
	select {
	case w.webhookCh <- webhook:
	default:
		w.log.Warn("webhook queue full, dropping webhook", "webhook_id", webhook.Data.ID)
	}
}

// run executes the worker loop
func (w *WebhookWorker) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopCh:
			return
		case webhook := <-w.webhookCh:
			if err := w.processWebhook(ctx, webhook); err != nil {
				w.log.Error("failed to process webhook",
					"webhook_id", webhook.Data.ID,
					"error", err,
				)
			}
		}
	}
}

// processWebhook processes a single webhook
func (w *WebhookWorker) processWebhook(ctx context.Context, webhook interfaces.FireblocksWebhook) error {
	switch webhook.Type {
	case "TRANSACTION_STATUS_UPDATED":
		return w.processTransactionUpdate(ctx, webhook.Data)
	case "TRANSACTION_CREATED":
		return w.processTransactionCreated(ctx, webhook.Data)
	default:
		w.log.Debug("ignoring webhook type", "type", webhook.Type)
		return nil
	}
}

// processTransactionUpdate processes transaction status updates
func (w *WebhookWorker) processTransactionUpdate(ctx context.Context, data interfaces.FireblocksData) error {
	// Find transaction by Fireblocks ID
	tx, err := w.repository.GetTransactionByFireblocksID(ctx, data.ID)
	if err != nil {
		// Transaction might not exist in our system (external transaction)
		w.log.Debug("transaction not found for webhook", "fireblocks_id", data.ID)
		return nil
	}

	// Update transaction confirmations
	if data.NumOfConfirmations != tx.Confirmations {
		if err := w.repository.UpdateTransactionConfirmations(ctx, tx.ID, data.NumOfConfirmations); err != nil {
			return fmt.Errorf("failed to update confirmations: %w", err)
		}
	}

	// Update transaction hash
	if data.TxHash != "" && data.TxHash != tx.TxHash {
		if err := w.repository.UpdateTransactionHash(ctx, tx.ID, data.TxHash); err != nil {
			w.log.Warn("failed to update transaction hash", "tx_id", tx.ID, "error", err)
		}
	}

	// Process state transition based on webhook status
	return w.processWebhookStateTransition(ctx, tx, data)
}

// processTransactionCreated processes new transaction creation webhooks
func (w *WebhookWorker) processTransactionCreated(ctx context.Context, data interfaces.FireblocksData) error {
	// Check if this is a deposit to our vault
	if data.Destination.Type != "VAULT_ACCOUNT" || data.Destination.ID != w.config.Fireblocks.VaultAccountID {
		return nil
	}

	// Check if we already have this transaction
	if _, err := w.repository.GetTransactionByFireblocksID(ctx, data.ID); err == nil {
		return nil // Already exists
	}

	// Create new deposit transaction
	// This handles external deposits not initiated through our system
	return w.createExternalDeposit(ctx, data)
}

// processWebhookStateTransition processes state transitions from webhooks
func (w *WebhookWorker) processWebhookStateTransition(ctx context.Context, tx *interfaces.WalletTransaction, data interfaces.FireblocksData) error {
	var newState interfaces.TxStatus
	var metadata map[string]interface{}

	// Map Fireblocks status to our internal status
	switch data.Status {
	case "CONFIRMING":
		if tx.Status == interfaces.TxStatusPending {
			newState = interfaces.TxStatusConfirming
		}
	case "CONFIRMED":
		if data.NumOfConfirmations >= tx.RequiredConf {
			if tx.Status == interfaces.TxStatusConfirming {
				newState = interfaces.TxStatusConfirmed
			} else if tx.Status == interfaces.TxStatusConfirmed {
				newState = interfaces.TxStatusCompleted
			}
		}
	case "COMPLETED":
		if tx.Status != interfaces.TxStatusCompleted {
			newState = interfaces.TxStatusCompleted
		}
	case "FAILED":
		if tx.Status != interfaces.TxStatusFailed {
			newState = interfaces.TxStatusFailed
			metadata = map[string]interface{}{
				"error": data.SubStatus,
			}
		}
	case "CANCELLED", "REJECTED":
		if tx.Status != interfaces.TxStatusRejected {
			newState = interfaces.TxStatusRejected
			metadata = map[string]interface{}{
				"reason": data.SubStatus,
			}
		}
	}

	// Execute state transition
	if newState != "" && newState != tx.Status {
		if metadata == nil {
			metadata = make(map[string]interface{})
		}
		metadata["webhook_trigger"] = true
		metadata["fireblocks_status"] = data.Status
		metadata["fireblocks_substatus"] = data.SubStatus

		if _, err := w.stateMachine.TransitionTo(ctx, tx.ID, newState, metadata); err != nil {
			return fmt.Errorf("failed to transition state from webhook: %w", err)
		}
	}

	return nil
}

// createExternalDeposit creates a transaction for external deposits
func (w *WebhookWorker) createExternalDeposit(ctx context.Context, data interfaces.FireblocksData) error {
	// TODO: Implement external deposit handling
	// This would involve:
	// 1. Parsing the destination address to find the user
	// 2. Creating a new deposit transaction
	// 3. Crediting the user's balance
	// 4. Sending notifications

	w.log.Info("external deposit detected",
		"fireblocks_id", data.ID,
		"asset", data.AssetID,
		"amount", data.Amount,
		"destination", data.Destination.ID,
	)

	return nil
}
