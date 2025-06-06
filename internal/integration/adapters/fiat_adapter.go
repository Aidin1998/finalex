// Package adapters provides adapter implementations for service contracts
package adapters

import (
	"context"
	"fmt"

	"github.com/Aidin1998/finalex/internal/fiat"
	"github.com/Aidin1998/finalex/internal/integration/contracts"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// FiatServiceAdapter implements contracts.FiatServiceContract
// by adapting the fiat module's native interface
type FiatServiceAdapter struct {
	service fiat.Service
	logger  *zap.Logger
}

// NewFiatServiceAdapter creates a new fiat service adapter
func NewFiatServiceAdapter(service fiat.Service, logger *zap.Logger) *FiatServiceAdapter {
	return &FiatServiceAdapter{
		service: service,
		logger:  logger,
	}
}

// ProcessDeposit processes a fiat deposit request
func (a *FiatServiceAdapter) ProcessDeposit(ctx context.Context, req *contracts.DepositRequest) (*contracts.DepositResponse, error) {
	a.logger.Debug("Processing deposit",
		zap.String("user_id", req.UserID.String()),
		zap.String("currency", req.Currency),
		zap.String("amount", req.Amount.String()))

	// Convert contract request to fiat request
	fiatReq := &fiat.DepositRequest{
		UserID:          req.UserID,
		Currency:        req.Currency,
		Amount:          req.Amount,
		PaymentMethodID: req.PaymentMethodID,
		ReferenceID:     req.ReferenceID,
		Description:     req.Description,
		Metadata:        req.Metadata,
	}

	// Call fiat service to process deposit
	response, err := a.service.ProcessDeposit(ctx, fiatReq)
	if err != nil {
		a.logger.Error("Failed to process deposit",
			zap.String("user_id", req.UserID.String()),
			zap.String("currency", req.Currency),
			zap.Error(err))
		return nil, fmt.Errorf("failed to process deposit: %w", err)
	}

	// Convert fiat response to contract response
	return &contracts.DepositResponse{
		DepositID:     response.DepositID,
		UserID:        response.UserID,
		Currency:      response.Currency,
		Amount:        response.Amount,
		Status:        response.Status,
		PaymentURL:    response.PaymentURL,
		EstimatedTime: response.EstimatedTime,
		CreatedAt:     response.CreatedAt,
	}, nil
}

// ProcessWithdrawal processes a fiat withdrawal request
func (a *FiatServiceAdapter) ProcessWithdrawal(ctx context.Context, req *contracts.WithdrawalRequest) (*contracts.WithdrawalResponse, error) {
	a.logger.Debug("Processing withdrawal",
		zap.String("user_id", req.UserID.String()),
		zap.String("currency", req.Currency),
		zap.String("amount", req.Amount.String()))

	// Convert contract request to fiat request
	fiatReq := &fiat.WithdrawalRequest{
		UserID:        req.UserID,
		Currency:      req.Currency,
		Amount:        req.Amount,
		BankAccountID: req.BankAccountID,
		ReferenceID:   req.ReferenceID,
		Description:   req.Description,
		Metadata:      req.Metadata,
	}

	// Call fiat service to process withdrawal
	response, err := a.service.ProcessWithdrawal(ctx, fiatReq)
	if err != nil {
		a.logger.Error("Failed to process withdrawal",
			zap.String("user_id", req.UserID.String()),
			zap.String("currency", req.Currency),
			zap.Error(err))
		return nil, fmt.Errorf("failed to process withdrawal: %w", err)
	}

	// Convert fiat response to contract response
	return &contracts.WithdrawalResponse{
		WithdrawalID:  response.WithdrawalID,
		UserID:        response.UserID,
		Currency:      response.Currency,
		Amount:        response.Amount,
		Fee:           response.Fee,
		NetAmount:     response.NetAmount,
		Status:        response.Status,
		EstimatedTime: response.EstimatedTime,
		CreatedAt:     response.CreatedAt,
	}, nil
}

// GetPaymentStatus retrieves payment status information
func (a *FiatServiceAdapter) GetPaymentStatus(ctx context.Context, paymentID uuid.UUID) (*contracts.PaymentStatus, error) {
	a.logger.Debug("Getting payment status", zap.String("payment_id", paymentID.String()))

	// Call fiat service to get payment status
	status, err := a.service.GetPaymentStatus(ctx, paymentID)
	if err != nil {
		a.logger.Error("Failed to get payment status",
			zap.String("payment_id", paymentID.String()),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get payment status: %w", err)
	}

	// Convert fiat status to contract status
	return &contracts.PaymentStatus{
		PaymentID:     status.PaymentID,
		Type:          status.Type,
		Status:        status.Status,
		Currency:      status.Currency,
		Amount:        status.Amount,
		ProcessedAt:   status.ProcessedAt,
		CompletedAt:   status.CompletedAt,
		FailureReason: status.FailureReason,
		Metadata:      status.Metadata,
	}, nil
}

// AddBankAccount adds a new bank account
func (a *FiatServiceAdapter) AddBankAccount(ctx context.Context, req *contracts.AddBankAccountRequest) (*contracts.BankAccount, error) {
	a.logger.Debug("Adding bank account",
		zap.String("user_id", req.UserID.String()),
		zap.String("currency", req.Currency))

	// Convert contract request to fiat request
	fiatReq := &fiat.AddBankAccountRequest{
		UserID:         req.UserID,
		AccountNumber:  req.AccountNumber,
		RoutingNumber:  req.RoutingNumber,
		AccountType:    req.AccountType,
		BankName:       req.BankName,
		AccountHolders: req.AccountHolders,
		Currency:       req.Currency,
		Country:        req.Country,
		Metadata:       req.Metadata,
	}

	// Call fiat service to add bank account
	account, err := a.service.AddBankAccount(ctx, fiatReq)
	if err != nil {
		a.logger.Error("Failed to add bank account",
			zap.String("user_id", req.UserID.String()),
			zap.Error(err))
		return nil, fmt.Errorf("failed to add bank account: %w", err)
	}

	// Convert fiat account to contract account
	return &contracts.BankAccount{
		ID:             account.ID,
		UserID:         account.UserID,
		AccountNumber:  account.AccountNumber,
		RoutingNumber:  account.RoutingNumber,
		AccountType:    account.AccountType,
		BankName:       account.BankName,
		AccountHolders: account.AccountHolders,
		Currency:       account.Currency,
		Country:        account.Country,
		Status:         account.Status,
		IsVerified:     account.IsVerified,
		CreatedAt:      account.CreatedAt,
		VerifiedAt:     account.VerifiedAt,
	}, nil
}

// GetBankAccounts retrieves user's bank accounts
func (a *FiatServiceAdapter) GetBankAccounts(ctx context.Context, userID uuid.UUID) ([]*contracts.BankAccount, error) {
	a.logger.Debug("Getting bank accounts", zap.String("user_id", userID.String()))

	// Call fiat service to get bank accounts
	accounts, err := a.service.GetBankAccounts(ctx, userID)
	if err != nil {
		a.logger.Error("Failed to get bank accounts",
			zap.String("user_id", userID.String()),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get bank accounts: %w", err)
	}

	// Convert fiat accounts to contract accounts
	contractAccounts := make([]*contracts.BankAccount, len(accounts))
	for i, account := range accounts {
		contractAccounts[i] = &contracts.BankAccount{
			ID:             account.ID,
			UserID:         account.UserID,
			AccountNumber:  account.AccountNumber,
			RoutingNumber:  account.RoutingNumber,
			AccountType:    account.AccountType,
			BankName:       account.BankName,
			AccountHolders: account.AccountHolders,
			Currency:       account.Currency,
			Country:        account.Country,
			Status:         account.Status,
			IsVerified:     account.IsVerified,
			CreatedAt:      account.CreatedAt,
			VerifiedAt:     account.VerifiedAt,
		}
	}

	return contractAccounts, nil
}

// VerifyBankAccount verifies a bank account
func (a *FiatServiceAdapter) VerifyBankAccount(ctx context.Context, accountID uuid.UUID, verificationData map[string]string) error {
	a.logger.Debug("Verifying bank account", zap.String("account_id", accountID.String()))

	// Call fiat service to verify bank account
	if err := a.service.VerifyBankAccount(ctx, accountID, verificationData); err != nil {
		a.logger.Error("Failed to verify bank account",
			zap.String("account_id", accountID.String()),
			zap.Error(err))
		return fmt.Errorf("failed to verify bank account: %w", err)
	}

	return nil
}

// RemoveBankAccount removes a bank account
func (a *FiatServiceAdapter) RemoveBankAccount(ctx context.Context, accountID uuid.UUID) error {
	a.logger.Debug("Removing bank account", zap.String("account_id", accountID.String()))

	// Call fiat service to remove bank account
	if err := a.service.RemoveBankAccount(ctx, accountID); err != nil {
		a.logger.Error("Failed to remove bank account",
			zap.String("account_id", accountID.String()),
			zap.Error(err))
		return fmt.Errorf("failed to remove bank account: %w", err)
	}

	return nil
}

// GetPaymentMethods retrieves available payment methods
func (a *FiatServiceAdapter) GetPaymentMethods(ctx context.Context, userID uuid.UUID, currency string) ([]*contracts.PaymentMethod, error) {
	a.logger.Debug("Getting payment methods",
		zap.String("user_id", userID.String()),
		zap.String("currency", currency))

	// Call fiat service to get payment methods
	methods, err := a.service.GetPaymentMethods(ctx, userID, currency)
	if err != nil {
		a.logger.Error("Failed to get payment methods",
			zap.String("user_id", userID.String()),
			zap.String("currency", currency),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get payment methods: %w", err)
	}

	// Convert fiat methods to contract methods
	contractMethods := make([]*contracts.PaymentMethod, len(methods))
	for i, method := range methods {
		contractMethods[i] = &contracts.PaymentMethod{
			ID:         method.ID,
			UserID:     method.UserID,
			Type:       method.Type,
			Currency:   method.Currency,
			Provider:   method.Provider,
			Details:    method.Details,
			Status:     method.Status,
			IsDefault:  method.IsDefault,
			IsVerified: method.IsVerified,
			CreatedAt:  method.CreatedAt,
			VerifiedAt: method.VerifiedAt,
		}
	}

	return contractMethods, nil
}

// AddPaymentMethod adds a new payment method
func (a *FiatServiceAdapter) AddPaymentMethod(ctx context.Context, req *contracts.AddPaymentMethodRequest) (*contracts.PaymentMethod, error) {
	a.logger.Debug("Adding payment method",
		zap.String("user_id", req.UserID.String()),
		zap.String("type", req.Type))

	// Convert contract request to fiat request
	fiatReq := &fiat.AddPaymentMethodRequest{
		UserID:    req.UserID,
		Type:      req.Type,
		Currency:  req.Currency,
		Provider:  req.Provider,
		Details:   req.Details,
		IsDefault: req.IsDefault,
		Metadata:  req.Metadata,
	}

	// Call fiat service to add payment method
	method, err := a.service.AddPaymentMethod(ctx, fiatReq)
	if err != nil {
		a.logger.Error("Failed to add payment method",
			zap.String("user_id", req.UserID.String()),
			zap.String("type", req.Type),
			zap.Error(err))
		return nil, fmt.Errorf("failed to add payment method: %w", err)
	}

	// Convert fiat method to contract method
	return &contracts.PaymentMethod{
		ID:         method.ID,
		UserID:     method.UserID,
		Type:       method.Type,
		Currency:   method.Currency,
		Provider:   method.Provider,
		Details:    method.Details,
		Status:     method.Status,
		IsDefault:  method.IsDefault,
		IsVerified: method.IsVerified,
		CreatedAt:  method.CreatedAt,
		VerifiedAt: method.VerifiedAt,
	}, nil
}

// RemovePaymentMethod removes a payment method
func (a *FiatServiceAdapter) RemovePaymentMethod(ctx context.Context, methodID uuid.UUID) error {
	a.logger.Debug("Removing payment method", zap.String("method_id", methodID.String()))

	// Call fiat service to remove payment method
	if err := a.service.RemovePaymentMethod(ctx, methodID); err != nil {
		a.logger.Error("Failed to remove payment method",
			zap.String("method_id", methodID.String()),
			zap.Error(err))
		return fmt.Errorf("failed to remove payment method: %w", err)
	}

	return nil
}

// GetFiatTransactionHistory retrieves fiat transaction history
func (a *FiatServiceAdapter) GetFiatTransactionHistory(ctx context.Context, userID uuid.UUID, filter *contracts.FiatTransactionFilter) ([]*contracts.FiatTransaction, int64, error) {
	a.logger.Debug("Getting fiat transaction history", zap.String("user_id", userID.String()))

	// Convert contract filter to fiat filter
	var fiatFilter *fiat.FiatTransactionFilter
	if filter != nil {
		fiatFilter = &fiat.FiatTransactionFilter{
			Type:      filter.Type,
			Currency:  filter.Currency,
			Status:    filter.Status,
			StartTime: filter.StartTime,
			EndTime:   filter.EndTime,
			Limit:     filter.Limit,
			Offset:    filter.Offset,
		}
	}

	// Call fiat service to get transaction history
	transactions, total, err := a.service.GetFiatTransactionHistory(ctx, userID, fiatFilter)
	if err != nil {
		a.logger.Error("Failed to get fiat transaction history",
			zap.String("user_id", userID.String()),
			zap.Error(err))
		return nil, 0, fmt.Errorf("failed to get fiat transaction history: %w", err)
	}

	// Convert fiat transactions to contract transactions
	contractTxs := make([]*contracts.FiatTransaction, len(transactions))
	for i, tx := range transactions {
		contractTxs[i] = &contracts.FiatTransaction{
			ID:              tx.ID,
			UserID:          tx.UserID,
			Type:            tx.Type,
			Currency:        tx.Currency,
			Amount:          tx.Amount,
			Fee:             tx.Fee,
			NetAmount:       tx.NetAmount,
			Status:          tx.Status,
			PaymentMethodID: tx.PaymentMethodID,
			BankAccountID:   tx.BankAccountID,
			ReferenceID:     tx.ReferenceID,
			Description:     tx.Description,
			Metadata:        tx.Metadata,
			CreatedAt:       tx.CreatedAt,
			ProcessedAt:     tx.ProcessedAt,
			CompletedAt:     tx.CompletedAt,
		}
	}

	return contractTxs, total, nil
}

// GetFiatTransaction retrieves a specific fiat transaction
func (a *FiatServiceAdapter) GetFiatTransaction(ctx context.Context, transactionID uuid.UUID) (*contracts.FiatTransaction, error) {
	a.logger.Debug("Getting fiat transaction", zap.String("transaction_id", transactionID.String()))

	// Call fiat service to get transaction
	tx, err := a.service.GetFiatTransaction(ctx, transactionID)
	if err != nil {
		a.logger.Error("Failed to get fiat transaction",
			zap.String("transaction_id", transactionID.String()),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get fiat transaction: %w", err)
	}

	// Convert fiat transaction to contract transaction
	return &contracts.FiatTransaction{
		ID:              tx.ID,
		UserID:          tx.UserID,
		Type:            tx.Type,
		Currency:        tx.Currency,
		Amount:          tx.Amount,
		Fee:             tx.Fee,
		NetAmount:       tx.NetAmount,
		Status:          tx.Status,
		PaymentMethodID: tx.PaymentMethodID,
		BankAccountID:   tx.BankAccountID,
		ReferenceID:     tx.ReferenceID,
		Description:     tx.Description,
		Metadata:        tx.Metadata,
		CreatedAt:       tx.CreatedAt,
		ProcessedAt:     tx.ProcessedAt,
		CompletedAt:     tx.CompletedAt,
	}, nil
}

// PerformAMLCheck performs AML compliance check
func (a *FiatServiceAdapter) PerformAMLCheck(ctx context.Context, req *contracts.AMLCheckRequest) (*contracts.AMLResult, error) {
	a.logger.Debug("Performing AML check",
		zap.String("user_id", req.UserID.String()),
		zap.String("currency", req.Currency))

	// Convert contract transaction to fiat transaction
	var fiatTransaction *fiat.FiatTransaction
	if req.Transaction != nil {
		fiatTransaction = &fiat.FiatTransaction{
			ID:              req.Transaction.ID,
			UserID:          req.Transaction.UserID,
			Type:            req.Transaction.Type,
			Currency:        req.Transaction.Currency,
			Amount:          req.Transaction.Amount,
			Fee:             req.Transaction.Fee,
			NetAmount:       req.Transaction.NetAmount,
			Status:          req.Transaction.Status,
			PaymentMethodID: req.Transaction.PaymentMethodID,
			BankAccountID:   req.Transaction.BankAccountID,
			ReferenceID:     req.Transaction.ReferenceID,
			Description:     req.Transaction.Description,
			Metadata:        req.Transaction.Metadata,
			CreatedAt:       req.Transaction.CreatedAt,
			ProcessedAt:     req.Transaction.ProcessedAt,
			CompletedAt:     req.Transaction.CompletedAt,
		}
	}

	// Convert contract request to fiat request
	fiatReq := &fiat.AMLCheckRequest{
		UserID:      req.UserID,
		Transaction: fiatTransaction,
		Amount:      req.Amount,
		Currency:    req.Currency,
		Source:      req.Source,
		Destination: req.Destination,
		Metadata:    req.Metadata,
	}

	// Call fiat service to perform AML check
	result, err := a.service.PerformAMLCheck(ctx, fiatReq)
	if err != nil {
		a.logger.Error("Failed to perform AML check",
			zap.String("user_id", req.UserID.String()),
			zap.Error(err))
		return nil, fmt.Errorf("failed to perform AML check: %w", err)
	}

	// Convert fiat result to contract result
	return &contracts.AMLResult{
		CheckID:     result.CheckID,
		Status:      result.Status,
		RiskScore:   result.RiskScore,
		RiskLevel:   result.RiskLevel,
		Flags:       result.Flags,
		Reason:      result.Reason,
		RequiredKYC: result.RequiredKYC,
		ProcessedAt: result.ProcessedAt,
	}, nil
}

// ValidatePayment validates payment request
func (a *FiatServiceAdapter) ValidatePayment(ctx context.Context, req *contracts.PaymentValidationRequest) (*contracts.PaymentValidationResult, error) {
	a.logger.Debug("Validating payment",
		zap.String("user_id", req.UserID.String()),
		zap.String("currency", req.Currency))

	// Convert contract request to fiat request
	fiatReq := &fiat.PaymentValidationRequest{
		UserID:          req.UserID,
		PaymentMethodID: req.PaymentMethodID,
		Amount:          req.Amount,
		Currency:        req.Currency,
		Metadata:        req.Metadata,
	}

	// Call fiat service to validate payment
	result, err := a.service.ValidatePayment(ctx, fiatReq)
	if err != nil {
		a.logger.Error("Failed to validate payment",
			zap.String("user_id", req.UserID.String()),
			zap.Error(err))
		return nil, fmt.Errorf("failed to validate payment: %w", err)
	}

	// Convert fiat result to contract result
	return &contracts.PaymentValidationResult{
		Valid:            result.Valid,
		Errors:           result.Errors,
		Warnings:         result.Warnings,
		EstimatedFee:     result.EstimatedFee,
		ProcessingTime:   result.ProcessingTime,
		RequiresApproval: result.RequiresApproval,
	}, nil
}

// ReportSuspiciousActivity reports suspicious activity
func (a *FiatServiceAdapter) ReportSuspiciousActivity(ctx context.Context, req *contracts.SuspiciousActivityRequest) error {
	a.logger.Debug("Reporting suspicious activity",
		zap.String("user_id", req.UserID.String()),
		zap.String("activity_type", req.ActivityType))

	// Convert contract request to fiat request
	fiatReq := &fiat.SuspiciousActivityRequest{
		UserID:         req.UserID,
		TransactionID:  req.TransactionID,
		ActivityType:   req.ActivityType,
		Description:    req.Description,
		RiskIndicators: req.RiskIndicators,
		Metadata:       req.Metadata,
	}

	// Call fiat service to report suspicious activity
	if err := a.service.ReportSuspiciousActivity(ctx, fiatReq); err != nil {
		a.logger.Error("Failed to report suspicious activity",
			zap.String("user_id", req.UserID.String()),
			zap.Error(err))
		return fmt.Errorf("failed to report suspicious activity: %w", err)
	}

	return nil
}

// GetSupportedCurrencies retrieves supported fiat currencies
func (a *FiatServiceAdapter) GetSupportedCurrencies(ctx context.Context) ([]*contracts.SupportedCurrency, error) {
	a.logger.Debug("Getting supported currencies")

	// Call fiat service to get supported currencies
	currencies, err := a.service.GetSupportedCurrencies(ctx)
	if err != nil {
		a.logger.Error("Failed to get supported currencies", zap.Error(err))
		return nil, fmt.Errorf("failed to get supported currencies: %w", err)
	}

	// Convert fiat currencies to contract currencies
	contractCurrencies := make([]*contracts.SupportedCurrency, len(currencies))
	for i, currency := range currencies {
		contractCurrencies[i] = &contracts.SupportedCurrency{
			Code:             currency.Code,
			Name:             currency.Name,
			Symbol:           currency.Symbol,
			DecimalPlaces:    currency.DecimalPlaces,
			MinDeposit:       currency.MinDeposit,
			MinWithdrawal:    currency.MinWithdrawal,
			MaxDeposit:       currency.MaxDeposit,
			MaxWithdrawal:    currency.MaxWithdrawal,
			DepositFee:       currency.DepositFee,
			WithdrawalFee:    currency.WithdrawalFee,
			IsActive:         currency.IsActive,
			SupportedRegions: currency.SupportedRegions,
		}
	}

	return contractCurrencies, nil
}

// GetExchangeRates retrieves exchange rates
func (a *FiatServiceAdapter) GetExchangeRates(ctx context.Context, baseCurrency string) (map[string]decimal.Decimal, error) {
	a.logger.Debug("Getting exchange rates", zap.String("base_currency", baseCurrency))

	// Call fiat service to get exchange rates
	rates, err := a.service.GetExchangeRates(ctx, baseCurrency)
	if err != nil {
		a.logger.Error("Failed to get exchange rates",
			zap.String("base_currency", baseCurrency),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get exchange rates: %w", err)
	}

	return rates, nil
}

// ConvertCurrency performs currency conversion
func (a *FiatServiceAdapter) ConvertCurrency(ctx context.Context, req *contracts.CurrencyConversionRequest) (*contracts.CurrencyConversionResult, error) {
	a.logger.Debug("Converting currency",
		zap.String("from", req.FromCurrency),
		zap.String("to", req.ToCurrency),
		zap.String("amount", req.Amount.String()))

	// Convert contract request to fiat request
	fiatReq := &fiat.CurrencyConversionRequest{
		FromCurrency: req.FromCurrency,
		ToCurrency:   req.ToCurrency,
		Amount:       req.Amount,
		RateType:     req.RateType,
	}

	// Call fiat service to convert currency
	result, err := a.service.ConvertCurrency(ctx, fiatReq)
	if err != nil {
		a.logger.Error("Failed to convert currency",
			zap.String("from", req.FromCurrency),
			zap.String("to", req.ToCurrency),
			zap.Error(err))
		return nil, fmt.Errorf("failed to convert currency: %w", err)
	}

	// Convert fiat result to contract result
	return &contracts.CurrencyConversionResult{
		FromCurrency: result.FromCurrency,
		ToCurrency:   result.ToCurrency,
		FromAmount:   result.FromAmount,
		ToAmount:     result.ToAmount,
		ExchangeRate: result.ExchangeRate,
		Fee:          result.Fee,
		ProcessedAt:  result.ProcessedAt,
	}, nil
}

// ReconcileFiatTransactions performs fiat transaction reconciliation
func (a *FiatServiceAdapter) ReconcileFiatTransactions(ctx context.Context, req *contracts.FiatReconciliationRequest) (*contracts.FiatReconciliationResult, error) {
	a.logger.Debug("Reconciling fiat transactions")

	// Convert contract request to fiat request
	fiatReq := &fiat.FiatReconciliationRequest{
		Currencies: req.Currencies,
		StartTime:  req.StartTime,
		EndTime:    req.EndTime,
		Force:      req.Force,
	}

	// Call fiat service to reconcile transactions
	result, err := a.service.ReconcileFiatTransactions(ctx, fiatReq)
	if err != nil {
		a.logger.Error("Failed to reconcile fiat transactions", zap.Error(err))
		return nil, fmt.Errorf("failed to reconcile fiat transactions: %w", err)
	}

	// Convert fiat discrepancies to contract discrepancies
	contractDiscrepancies := make([]contracts.FiatDiscrepancy, len(result.Discrepancies))
	for i, discrepancy := range result.Discrepancies {
		contractDiscrepancies[i] = contracts.FiatDiscrepancy{
			TransactionID: discrepancy.TransactionID,
			Type:          discrepancy.Type,
			Currency:      discrepancy.Currency,
			Expected:      discrepancy.Expected,
			Actual:        discrepancy.Actual,
			Difference:    discrepancy.Difference,
			Description:   discrepancy.Description,
			Severity:      discrepancy.Severity,
		}
	}

	// Convert fiat result to contract result
	return &contracts.FiatReconciliationResult{
		TotalTransactions: result.TotalTransactions,
		ReconciledCount:   result.ReconciledCount,
		DiscrepancyCount:  result.DiscrepancyCount,
		Discrepancies:     contractDiscrepancies,
		CurrencyTotals:    result.CurrencyTotals,
		ProcessedAt:       result.ProcessedAt,
	}, nil
}

// GenerateSettlementReport generates settlement report
func (a *FiatServiceAdapter) GenerateSettlementReport(ctx context.Context, req *contracts.SettlementReportRequest) (*contracts.SettlementReport, error) {
	a.logger.Debug("Generating settlement report", zap.String("type", req.ReportType))

	// Convert contract request to fiat request
	fiatReq := &fiat.SettlementReportRequest{
		StartTime:  req.StartTime,
		EndTime:    req.EndTime,
		Currencies: req.Currencies,
		ReportType: req.ReportType,
		Format:     req.Format,
	}

	// Call fiat service to generate settlement report
	report, err := a.service.GenerateSettlementReport(ctx, fiatReq)
	if err != nil {
		a.logger.Error("Failed to generate settlement report",
			zap.String("type", req.ReportType),
			zap.Error(err))
		return nil, fmt.Errorf("failed to generate settlement report: %w", err)
	}

	// Convert fiat currency totals to contract currency totals
	contractCurrencyTotals := make(map[string]contracts.SettlementSummary)
	for currency, summary := range report.CurrencyTotals {
		contractCurrencyTotals[currency] = contracts.SettlementSummary{
			Currency:         summary.Currency,
			TotalDeposits:    summary.TotalDeposits,
			TotalWithdrawals: summary.TotalWithdrawals,
			NetFlow:          summary.NetFlow,
			TransactionCount: summary.TransactionCount,
			TotalFees:        summary.TotalFees,
		}
	}

	// Convert fiat transactions to contract transactions
	contractTransactions := make([]contracts.SettlementTransaction, len(report.Transactions))
	for i, tx := range report.Transactions {
		contractTransactions[i] = contracts.SettlementTransaction{
			ID:          tx.ID,
			Type:        tx.Type,
			Currency:    tx.Currency,
			Amount:      tx.Amount,
			Fee:         tx.Fee,
			Status:      tx.Status,
			ProcessedAt: tx.ProcessedAt,
		}
	}

	// Convert fiat report to contract report
	return &contracts.SettlementReport{
		ReportID:       report.ReportID,
		Type:           report.Type,
		Period:         report.Period,
		CurrencyTotals: contractCurrencyTotals,
		Transactions:   contractTransactions,
		Summary: contracts.SettlementOverallSummary{
			TotalTransactions: report.Summary.TotalTransactions,
			TotalVolume:       report.Summary.TotalVolume,
			TotalFees:         report.Summary.TotalFees,
			NetFlows:          report.Summary.NetFlows,
		},
		GeneratedAt: report.GeneratedAt,
	}, nil
}

// HealthCheck performs health check
func (a *FiatServiceAdapter) HealthCheck(ctx context.Context) (*contracts.HealthStatus, error) {
	a.logger.Debug("Performing health check")

	// Call fiat service health check
	health, err := a.service.HealthCheck(ctx)
	if err != nil {
		a.logger.Error("Health check failed", zap.Error(err))
		return nil, fmt.Errorf("health check failed: %w", err)
	}

	// Convert fiat health status to contract health status
	return &contracts.HealthStatus{
		Status:       health.Status,
		Timestamp:    health.Timestamp,
		Version:      health.Version,
		Uptime:       health.Uptime,
		Metrics:      health.Metrics,
		Dependencies: health.Dependencies,
	}, nil
}
