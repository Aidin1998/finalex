package crosspair

import (
	"context"
	"fmt"
	"time"

	"github.com/Aidin1998/finalex/internal/trading/balance"
	"github.com/Aidin1998/finalex/internal/trading/coordination"
	"github.com/Aidin1998/finalex/internal/trading/model"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// AtomicExecutor handles atomic execution of cross-pair trades
type AtomicExecutor struct {
	logger          *zap.Logger
	balanceService  *BalanceService
	coordinator     *TradeCoordinator
	rateCalculator  RateCalculator
	matchingEngines map[string]MatchingEngine
	tradeStore      CrossPairTradeStore
}

// AtomicExecutionResult represents the result of an atomic execution
type AtomicExecutionResult struct {
	Trade         *CrossPairTrade
	ActualRate    decimal.Decimal
	Slippage      decimal.Decimal
	TotalFees     []CrossPairFee
	ExecutionTime time.Duration
}

// NewAtomicExecutor creates a new atomic executor
func NewAtomicExecutor(
	logger *zap.Logger,
	balanceService *BalanceService,
	coordinator *TradeCoordinator,
	rateCalculator RateCalculator,
	matchingEngines map[string]MatchingEngine,
	tradeStore CrossPairTradeStore,
) *AtomicExecutor {
	return &AtomicExecutor{
		logger:          logger.Named("atomic-executor"),
		balanceService:  balanceService,
		coordinator:     coordinator,
		rateCalculator:  rateCalculator,
		matchingEngines: matchingEngines,
		tradeStore:      tradeStore,
	}
}

// ExecuteOrder executes a cross-pair order atomically
func (e *AtomicExecutor) ExecuteOrder(ctx context.Context, order *CrossPairOrder) (*AtomicExecutionResult, error) {
	startTime := time.Now()

	// Validate order is ready for execution
	if !order.CanExecute() {
		return nil, NewCrossPairError("ORDER_NOT_EXECUTABLE", "order is not in executable state")
	}

	// Get fresh rate calculation
	rateResult, err := e.rateCalculator.CalculateRate(order.FromAsset, order.ToAsset, order.Quantity)
	if err != nil {
		return nil, fmt.Errorf("failed to get current rate: %w", err)
	}

	// Check slippage tolerance
	if order.Type == CrossPairLimitOrder && order.Price != nil {
		priceSlippage := rateResult.Route.SyntheticRate.Sub(*order.Price).Div(*order.Price).Abs()
		if priceSlippage.GreaterThan(order.MaxSlippage) {
			return nil, NewCrossPairError("SLIPPAGE_EXCEEDED",
				fmt.Sprintf("price slippage %.4f%% exceeds maximum %.4f%%",
					priceSlippage.Mul(decimal.NewFromInt(100)).InexactFloat64(),
					order.MaxSlippage.Mul(decimal.NewFromInt(100)).InexactFloat64()))
		}
	}

	// Execute the two-leg trade atomically
	result, err := e.executeAtomicTrade(ctx, order, rateResult.Route)
	if err != nil {
		return nil, fmt.Errorf("atomic execution failed: %w", err)
	}

	result.ExecutionTime = time.Since(startTime)

	// Store the trade
	if err := e.tradeStore.Create(ctx, result.Trade); err != nil {
		e.logger.Error("failed to store cross-pair trade",
			zap.String("trade_id", result.Trade.ID.String()),
			zap.Error(err))
		// Don't fail the execution, just log the error
	}

	return result, nil
}

// executeAtomicTrade executes the atomic two-leg trade
func (e *AtomicExecutor) executeAtomicTrade(ctx context.Context, order *CrossPairOrder, route *CrossPairRoute) (*AtomicExecutionResult, error) { // Create coordination context for atomic execution
	coordCtx := &coordination.ExecutionContext{
		TransactionID:     uuid.New(),
		InitiatedAt:       time.Now(),
		ExpiresAt:         time.Now().Add(30 * time.Second),
		Operations:        make([]coordination.Operation, 0),
		Metadata:          make(map[string]interface{}),
		Priority:          1,
		RequiresConsensus: false,
	} // Prepare balance transfers for atomic execution
	transfers := []balance.MultiAssetTransfer{
		{
			FromUserID:  order.UserID.String(),
			ToUserID:    uuid.Nil.String(), // System account for intermediate holding
			Currency:    order.FromAsset,
			Amount:      order.Quantity,
			Reference:   "crosspair_execution",
			Description: "Cross-pair trade execution",
		},
	}

	// Execute using AtomicMultiAssetTransfer to ensure consistency
	err := error(nil)
	if svc, ok := interface{}(e.balanceService).(interface {
		AtomicMultiAssetTransfer(context.Context, *balance.AtomicMultiAssetTransferRequest) (*balance.AtomicMultiAssetTransferResponse, error)
	}); ok {
		req := &balance.AtomicMultiAssetTransferRequest{
			Transfers:   transfers,
			Reference:   coordCtx.TransactionID.String(),
			Idempotency: coordCtx.TransactionID.String(),
		}
		_, err = svc.AtomicMultiAssetTransfer(ctx, req)
	} else {
		err = fmt.Errorf("AtomicMultiAssetTransfer not implemented on balanceService")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to initiate atomic transfer: %w", err)
	}

	// Defer rollback in case of failure
	defer func() {
		if err != nil {
			if abort, ok := interface{}(e.coordinator).(interface {
				Abort(ctx context.Context, ctx2 *coordination.ExecutionContext) error
			}); ok {
				_ = abort.Abort(ctx, coordCtx)
			}
		}
	}()

	// Execute first leg: FromAsset -> BaseAsset
	firstLeg, err := e.executeFirstLeg(ctx, order, route, coordCtx)
	if err != nil {
		return nil, fmt.Errorf("first leg execution failed: %w", err)
	}

	// Execute second leg: BaseAsset -> ToAsset
	secondLeg, err := e.executeSecondLeg(ctx, order, route, firstLeg.Quantity, coordCtx)
	if err != nil {
		return nil, fmt.Errorf("second leg execution failed: %w", err)
	}

	// Calculate actual execution rate and slippage
	actualRate := secondLeg.Quantity.Div(order.Quantity)
	expectedRate := route.SyntheticRate
	slippage := actualRate.Sub(expectedRate).Div(expectedRate).Abs()

	// Aggregate fees
	totalFees := append(firstLeg.Fees, secondLeg.Fees...)

	// Create cross-pair trade record
	trade := &CrossPairTrade{
		ID:              uuid.New(),
		OrderID:         order.ID,
		UserID:          order.UserID,
		FromAsset:       order.FromAsset,
		ToAsset:         order.ToAsset,
		Quantity:        order.Quantity,
		ExecutedRate:    actualRate,
		Legs:            []CrossPairTradeLeg{*firstLeg, *secondLeg},
		Fees:            totalFees,
		TotalFeeUSD:     e.calculateTotalFeeUSD(totalFees),
		Slippage:        slippage,
		ExecutionTimeMs: 0, // Will be set by caller
		CreatedAt:       time.Now(),
	}

	// Commit the transaction
	if commit, ok := interface{}(e.coordinator).(interface {
		Commit(ctx context.Context, ctx2 *coordination.ExecutionContext) error
	}); ok {
		if err := commit.Commit(ctx, coordCtx); err != nil {
			return nil, fmt.Errorf("failed to commit transaction: %w", err)
		}
	}

	// Update order execution status
	order.ExecutedQuantity = order.ExecutedQuantity.Add(order.Quantity)
	order.ExecutedRate = &actualRate
	return &AtomicExecutionResult{
		Trade:      trade,
		ActualRate: actualRate,
		Slippage:   slippage,
		TotalFees:  totalFees,
	}, nil
}

// executeFirstLeg executes the first leg of the cross-pair trade
func (e *AtomicExecutor) executeFirstLeg(ctx context.Context, order *CrossPairOrder, route *CrossPairRoute, coordCtx *coordination.ExecutionContext) (*CrossPairTradeLeg, error) {
	// Get matching engine for first pair
	engine, exists := e.matchingEngines[route.FirstPair]
	if !exists {
		return nil, NewCrossPairError("NO_MATCHING_ENGINE", fmt.Sprintf("no matching engine for pair %s", route.FirstPair))
	}

	// Create spot order for first leg
	spotOrder := &model.Order{
		ID:        uuid.New(),
		UserID:    order.UserID,
		Pair:      route.FirstPair,
		Side:      model.OrderSideSell, // Always sell the from asset
		Type:      model.OrderTypeMarket,
		Quantity:  order.Quantity,
		Status:    OrderStatusPending,
		CreatedAt: time.Now(),
	}

	// Execute the order
	spotTrade, err := engine.ExecuteMarketOrder(ctx, spotOrder)
	if err != nil {
		return nil, fmt.Errorf("failed to execute first leg order: %w", err)
	}

	// Capture orderbook snapshot for audit
	orderbook, _ := engine.GetOrderbook(ctx)
	var snapshot *OrderBookSnapshot
	if orderbook != nil {
		snapshot = e.convertOrderbookSnapshot(orderbook, route.FirstPair)
	}

	// Create cross-pair trade leg
	leg := &CrossPairTradeLeg{
		ID:                uuid.New(),
		Pair:              route.FirstPair,
		Side:              "SELL",
		Quantity:          spotTrade.Quantity,
		Price:             spotTrade.Price,
		Commission:        spotTrade.Commission,
		CommissionAsset:   spotTrade.CommissionAsset,
		TradeID:           spotTrade.ID,
		ExecutedAt:        spotTrade.ExecutedAt,
		OrderBookSnapshot: snapshot,
	}

	// Calculate fees for this leg
	leg.Fees = []CrossPairFee{
		{
			Asset:   spotTrade.CommissionAsset,
			Amount:  spotTrade.Commission,
			FeeType: "TRADING",
			Pair:    route.FirstPair,
		},
	}

	e.logger.Info("first leg executed",
		zap.String("order_id", order.ID.String()),
		zap.String("pair", route.FirstPair),
		zap.String("quantity", spotTrade.Quantity.String()),
		zap.String("price", spotTrade.Price.String()))

	return leg, nil
}

// executeSecondLeg executes the second leg of the cross-pair trade
func (e *AtomicExecutor) executeSecondLeg(ctx context.Context, order *CrossPairOrder, route *CrossPairRoute, baseQuantity decimal.Decimal, coordCtx *coordination.ExecutionContext) (*CrossPairTradeLeg, error) {
	// Get matching engine for second pair
	engine, exists := e.matchingEngines[route.SecondPair]
	if !exists {
		return nil, NewCrossPairError("NO_MATCHING_ENGINE", fmt.Sprintf("no matching engine for pair %s", route.SecondPair))
	}

	// Create spot order for second leg
	spotOrder := &model.Order{
		ID:        uuid.New(),
		UserID:    order.UserID,
		Pair:      route.SecondPair,
		Side:      model.OrderSideBuy, // Buy the to asset with base asset
		Type:      model.OrderTypeMarket,
		Quantity:  baseQuantity, // Use the base asset quantity from first leg
		Status:    OrderStatusPending,
		CreatedAt: time.Now(),
	}

	// Execute the order
	spotTrade, err := engine.ExecuteMarketOrder(ctx, spotOrder)
	if err != nil {
		return nil, fmt.Errorf("failed to execute second leg order: %w", err)
	}

	// Capture orderbook snapshot for audit
	orderbook, _ := engine.GetOrderbook(ctx)
	var snapshot *OrderBookSnapshot
	if orderbook != nil {
		snapshot = e.convertOrderbookSnapshot(orderbook, route.SecondPair)
	}

	// Create cross-pair trade leg
	leg := &CrossPairTradeLeg{
		ID:                uuid.New(),
		Pair:              route.SecondPair,
		Side:              "BUY",
		Quantity:          spotTrade.Quantity,
		Price:             spotTrade.Price,
		Commission:        spotTrade.Commission,
		CommissionAsset:   spotTrade.CommissionAsset,
		TradeID:           spotTrade.ID,
		ExecutedAt:        spotTrade.ExecutedAt,
		OrderBookSnapshot: snapshot,
	}

	// Calculate fees for this leg
	leg.Fees = []CrossPairFee{
		{
			Asset:   spotTrade.CommissionAsset,
			Amount:  spotTrade.Commission,
			FeeType: "TRADING",
			Pair:    route.SecondPair,
		},
	}

	e.logger.Info("second leg executed",
		zap.String("order_id", order.ID.String()),
		zap.String("pair", route.SecondPair),
		zap.String("quantity", spotTrade.Quantity.String()),
		zap.String("price", spotTrade.Price.String()))

	return leg, nil
}

// convertOrderbookSnapshot converts a spot orderbook to cross-pair snapshot format
func (e *AtomicExecutor) convertOrderbookSnapshot(orderbook *OrderBookSnapshot, pair string) *OrderBookSnapshot {
	// Already correct type, just return as is
	return orderbook
}

// calculateTotalFeeUSD calculates the total fees in USD equivalent
func (e *AtomicExecutor) calculateTotalFeeUSD(fees []CrossPairFee) decimal.Decimal {
	// This is a simplified implementation
	// In practice, you'd need to convert each fee to USD using current rates
	totalUSD := decimal.Zero

	for _, fee := range fees {
		// For now, assume 1:1 ratio - this should be enhanced with real price conversion
		if fee.Asset == "USDT" || fee.Asset == "USD" || fee.Asset == "USDC" {
			totalUSD = totalUSD.Add(fee.Amount)
		}
		// For other assets, you'd need to convert using current market rates
	}

	return totalUSD
}

// EstimateExecution provides an estimate of execution without actually executing
func (e *AtomicExecutor) EstimateExecution(ctx context.Context, order *CrossPairOrder) (*AtomicEstimate, error) {
	// Get current rate
	rateResult, err := e.rateCalculator.CalculateRate(order.FromAsset, order.ToAsset, order.Quantity)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate rate: %w", err)
	}

	// Get orderbook depths for more accurate estimation
	firstEngine, exists := e.matchingEngines[rateResult.Route.FirstPair]
	if !exists {
		return nil, NewCrossPairError("NO_MATCHING_ENGINE", fmt.Sprintf("no matching engine for pair %s", rateResult.Route.FirstPair))
	}

	secondEngine, exists := e.matchingEngines[rateResult.Route.SecondPair]
	if !exists {
		return nil, NewCrossPairError("NO_MATCHING_ENGINE", fmt.Sprintf("no matching engine for pair %s", rateResult.Route.SecondPair))
	}

	firstOrderbook, err := firstEngine.GetOrderbook(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get first orderbook: %w", err)
	}

	secondOrderbook, err := secondEngine.GetOrderbook(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get second orderbook: %w", err)
	}

	// Estimate execution based on orderbook depth
	firstLegEstimate := e.estimateLegExecution(firstOrderbook.Asks, order.Quantity, true)
	secondLegEstimate := e.estimateLegExecution(secondOrderbook.Bids, firstLegEstimate.OutputQuantity, false)

	actualRate := secondLegEstimate.OutputQuantity.Div(order.Quantity)
	slippage := actualRate.Sub(rateResult.Route.SyntheticRate).Div(rateResult.Route.SyntheticRate).Abs()
	return &AtomicEstimate{
		EstimatedRate:     actualRate,
		EstimatedOutput:   secondLegEstimate.OutputQuantity,
		EstimatedSlippage: slippage,
		Confidence:        rateResult.Confidence,
		EstimatedFees:     []CrossPairFee{}, // Simplified for now
		FirstLeg:          firstLegEstimate,
		SecondLeg:         secondLegEstimate,
	}, nil
}

// AtomicEstimate represents an estimated execution result from AtomicExecutor
type AtomicEstimate struct {
	EstimatedRate     decimal.Decimal `json:"estimated_rate"`
	EstimatedOutput   decimal.Decimal `json:"estimated_output"`
	EstimatedSlippage decimal.Decimal `json:"estimated_slippage"`
	Confidence        decimal.Decimal `json:"confidence"`
	EstimatedFees     []CrossPairFee  `json:"estimated_fees"`
	FirstLeg          *LegEstimate    `json:"first_leg"`
	SecondLeg         *LegEstimate    `json:"second_leg"`
}

// LegEstimate represents the estimate for one leg of execution
type LegEstimate struct {
	Pair           string          `json:"pair"`
	InputQuantity  decimal.Decimal `json:"input_quantity"`
	OutputQuantity decimal.Decimal `json:"output_quantity"`
	AveragePrice   decimal.Decimal `json:"average_price"`
	Slippage       decimal.Decimal `json:"slippage"`
}

// estimateLegExecution estimates the execution for one leg
func (e *AtomicExecutor) estimateLegExecution(levels []OrderBookLevel, quantity decimal.Decimal, isSell bool) *LegEstimate {
	var totalCost decimal.Decimal
	var totalQuantity decimal.Decimal
	remaining := quantity

	bestPrice := decimal.Zero
	if len(levels) > 0 {
		bestPrice = levels[0].Price
	}

	for _, level := range levels {
		if remaining.LessThanOrEqual(decimal.Zero) {
			break
		}

		availableQuantity := decimal.Min(level.Quantity, remaining)
		cost := availableQuantity.Mul(level.Price)

		totalCost = totalCost.Add(cost)
		totalQuantity = totalQuantity.Add(availableQuantity)
		remaining = remaining.Sub(availableQuantity)
	}

	var averagePrice decimal.Decimal
	var outputQuantity decimal.Decimal
	var slippage decimal.Decimal

	if totalQuantity.GreaterThan(decimal.Zero) {
		if isSell {
			averagePrice = totalCost.Div(totalQuantity)
			outputQuantity = totalCost
		} else {
			averagePrice = totalCost.Div(totalQuantity)
			outputQuantity = totalQuantity
		}

		if bestPrice.GreaterThan(decimal.Zero) {
			slippage = averagePrice.Sub(bestPrice).Div(bestPrice).Abs()
		}
	}

	return &LegEstimate{
		InputQuantity:  quantity,
		OutputQuantity: outputQuantity,
		AveragePrice:   averagePrice,
		Slippage:       slippage,
	}
}

// ValidateExecutionConditions validates that execution conditions are met
func (e *AtomicExecutor) ValidateExecutionConditions(ctx context.Context, order *CrossPairOrder) error {
	// Check if matching engines are available
	route := order.Route
	if route == nil {
		return NewCrossPairError("NO_ROUTE", "order has no execution route")
	}

	if _, exists := e.matchingEngines[route.FirstPair]; !exists {
		return NewCrossPairError("NO_MATCHING_ENGINE", fmt.Sprintf("no matching engine for pair %s", route.FirstPair))
	}

	if _, exists := e.matchingEngines[route.SecondPair]; !exists {
		return NewCrossPairError("NO_MATCHING_ENGINE", fmt.Sprintf("no matching engine for pair %s", route.SecondPair))
	}

	// Check user balance
	balInfo, err := e.balanceService.GetBalance(ctx, order.UserID, order.FromAsset)
	if err != nil {
		return fmt.Errorf("failed to get user balance: %w", err)
	}

	if balInfo.LessThan(order.Quantity) {
		return NewCrossPairError("INSUFFICIENT_BALANCE", "insufficient balance for execution")
	}

	// Check orderbook liquidity
	rateResult, err := e.rateCalculator.CalculateRate(order.FromAsset, order.ToAsset, order.Quantity)
	if err != nil {
		return fmt.Errorf("failed to validate rate: %w", err)
	}

	if rateResult.Confidence.LessThan(decimal.NewFromFloat(0.3)) {
		return NewCrossPairError("INSUFFICIENT_CONFIDENCE", "insufficient confidence in execution rate")
	}

	return nil
}
