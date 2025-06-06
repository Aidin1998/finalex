// Package adapters provides adapter implementations for service contracts
package adapters

import (
	"context"
	"fmt"

	"github.com/Aidin1998/finalex/internal/integration/contracts"
	"github.com/Aidin1998/finalex/internal/trading"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// TradingServiceAdapter implements contracts.TradingServiceContract
// by adapting the trading module's native interface
type TradingServiceAdapter struct {
	service trading.Service
	logger  *zap.Logger
}

// NewTradingServiceAdapter creates a new trading service adapter
func NewTradingServiceAdapter(service trading.Service, logger *zap.Logger) *TradingServiceAdapter {
	return &TradingServiceAdapter{
		service: service,
		logger:  logger,
	}
}

// PlaceOrder places a new trading order
func (a *TradingServiceAdapter) PlaceOrder(ctx context.Context, req *contracts.PlaceOrderRequest) (*contracts.OrderResponse, error) {
	a.logger.Debug("Placing order",
		zap.String("user_id", req.UserID.String()),
		zap.String("symbol", req.Symbol),
		zap.String("side", req.Side),
		zap.String("type", req.Type))

	// Convert contract request to trading request
	tradingReq := &trading.PlaceOrderRequest{
		UserID:        req.UserID,
		Symbol:        req.Symbol,
		Side:          req.Side,
		Type:          req.Type,
		Quantity:      req.Quantity,
		Price:         req.Price,
		StopPrice:     req.StopPrice,
		TimeInForce:   req.TimeInForce,
		PostOnly:      req.PostOnly,
		ReduceOnly:    req.ReduceOnly,
		ClientOrderID: req.ClientOrderID,
		Metadata:      req.Metadata,
	}

	// Call trading service to place order
	response, err := a.service.PlaceOrder(ctx, tradingReq)
	if err != nil {
		a.logger.Error("Failed to place order",
			zap.String("user_id", req.UserID.String()),
			zap.String("symbol", req.Symbol),
			zap.Error(err))
		return nil, fmt.Errorf("failed to place order: %w", err)
	}

	// Convert trading trades to contract trades
	contractTrades := make([]*contracts.Trade, len(response.Trades))
	for i, trade := range response.Trades {
		contractTrades[i] = &contracts.Trade{
			ID:         trade.ID,
			Symbol:     trade.Symbol,
			OrderID:    trade.OrderID,
			BuyerID:    trade.BuyerID,
			SellerID:   trade.SellerID,
			Price:      trade.Price,
			Quantity:   trade.Quantity,
			Commission: trade.Commission,
			Role:       trade.Role,
			Status:     trade.Status,
			ExecutedAt: trade.ExecutedAt,
			SettledAt:  trade.SettledAt,
		}
	}

	// Convert trading response to contract response
	return &contracts.OrderResponse{
		OrderID:       response.OrderID,
		ClientOrderID: response.ClientOrderID,
		Symbol:        response.Symbol,
		Side:          response.Side,
		Type:          response.Type,
		Quantity:      response.Quantity,
		Price:         response.Price,
		Status:        response.Status,
		FilledQty:     response.FilledQty,
		RemainingQty:  response.RemainingQty,
		AvgPrice:      response.AvgPrice,
		CreatedAt:     response.CreatedAt,
		UpdatedAt:     response.UpdatedAt,
		Trades:        contractTrades,
	}, nil
}

// CancelOrder cancels an existing order
func (a *TradingServiceAdapter) CancelOrder(ctx context.Context, req *contracts.CancelOrderRequest) error {
	a.logger.Debug("Cancelling order",
		zap.String("user_id", req.UserID.String()))

	// Convert contract request to trading request
	tradingReq := &trading.CancelOrderRequest{
		OrderID:       req.OrderID,
		ClientOrderID: req.ClientOrderID,
		UserID:        req.UserID,
		Symbol:        req.Symbol,
	}

	// Call trading service to cancel order
	if err := a.service.CancelOrder(ctx, tradingReq); err != nil {
		a.logger.Error("Failed to cancel order",
			zap.String("user_id", req.UserID.String()),
			zap.Error(err))
		return fmt.Errorf("failed to cancel order: %w", err)
	}

	return nil
}

// ModifyOrder modifies an existing order
func (a *TradingServiceAdapter) ModifyOrder(ctx context.Context, req *contracts.ModifyOrderRequest) (*contracts.OrderResponse, error) {
	a.logger.Debug("Modifying order",
		zap.String("order_id", req.OrderID.String()),
		zap.String("user_id", req.UserID.String()))

	// Convert contract request to trading request
	tradingReq := &trading.ModifyOrderRequest{
		OrderID:     req.OrderID,
		UserID:      req.UserID,
		NewQuantity: req.NewQuantity,
		NewPrice:    req.NewPrice,
	}

	// Call trading service to modify order
	response, err := a.service.ModifyOrder(ctx, tradingReq)
	if err != nil {
		a.logger.Error("Failed to modify order",
			zap.String("order_id", req.OrderID.String()),
			zap.String("user_id", req.UserID.String()),
			zap.Error(err))
		return nil, fmt.Errorf("failed to modify order: %w", err)
	}

	// Convert trading trades to contract trades
	contractTrades := make([]*contracts.Trade, len(response.Trades))
	for i, trade := range response.Trades {
		contractTrades[i] = &contracts.Trade{
			ID:         trade.ID,
			Symbol:     trade.Symbol,
			OrderID:    trade.OrderID,
			BuyerID:    trade.BuyerID,
			SellerID:   trade.SellerID,
			Price:      trade.Price,
			Quantity:   trade.Quantity,
			Commission: trade.Commission,
			Role:       trade.Role,
			Status:     trade.Status,
			ExecutedAt: trade.ExecutedAt,
			SettledAt:  trade.SettledAt,
		}
	}

	// Convert trading response to contract response
	return &contracts.OrderResponse{
		OrderID:       response.OrderID,
		ClientOrderID: response.ClientOrderID,
		Symbol:        response.Symbol,
		Side:          response.Side,
		Type:          response.Type,
		Quantity:      response.Quantity,
		Price:         response.Price,
		Status:        response.Status,
		FilledQty:     response.FilledQty,
		RemainingQty:  response.RemainingQty,
		AvgPrice:      response.AvgPrice,
		CreatedAt:     response.CreatedAt,
		UpdatedAt:     response.UpdatedAt,
		Trades:        contractTrades,
	}, nil
}

// GetOrder retrieves order information
func (a *TradingServiceAdapter) GetOrder(ctx context.Context, orderID uuid.UUID) (*contracts.Order, error) {
	a.logger.Debug("Getting order", zap.String("order_id", orderID.String()))

	// Call trading service to get order
	order, err := a.service.GetOrder(ctx, orderID)
	if err != nil {
		a.logger.Error("Failed to get order",
			zap.String("order_id", orderID.String()),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get order: %w", err)
	}

	// Convert trading order to contract order
	return &contracts.Order{
		ID:            order.ID,
		UserID:        order.UserID,
		Symbol:        order.Symbol,
		Side:          order.Side,
		Type:          order.Type,
		Quantity:      order.Quantity,
		Price:         order.Price,
		StopPrice:     order.StopPrice,
		TimeInForce:   order.TimeInForce,
		Status:        order.Status,
		FilledQty:     order.FilledQty,
		RemainingQty:  order.RemainingQty,
		AvgPrice:      order.AvgPrice,
		Commission:    order.Commission,
		ClientOrderID: order.ClientOrderID,
		CreatedAt:     order.CreatedAt,
		UpdatedAt:     order.UpdatedAt,
		ExpiresAt:     order.ExpiresAt,
	}, nil
}

// GetUserOrders retrieves user orders with filtering
func (a *TradingServiceAdapter) GetUserOrders(ctx context.Context, userID uuid.UUID, filter *contracts.OrderFilter) ([]*contracts.Order, int64, error) {
	a.logger.Debug("Getting user orders", zap.String("user_id", userID.String()))

	// Convert contract filter to trading filter
	var tradingFilter *trading.OrderFilter
	if filter != nil {
		tradingFilter = &trading.OrderFilter{
			Symbol:    filter.Symbol,
			Status:    filter.Status,
			Side:      filter.Side,
			Type:      filter.Type,
			StartTime: filter.StartTime,
			EndTime:   filter.EndTime,
			Limit:     filter.Limit,
			Offset:    filter.Offset,
		}
	}

	// Call trading service to get user orders
	orders, total, err := a.service.GetUserOrders(ctx, userID, tradingFilter)
	if err != nil {
		a.logger.Error("Failed to get user orders",
			zap.String("user_id", userID.String()),
			zap.Error(err))
		return nil, 0, fmt.Errorf("failed to get user orders: %w", err)
	}

	// Convert trading orders to contract orders
	contractOrders := make([]*contracts.Order, len(orders))
	for i, order := range orders {
		contractOrders[i] = &contracts.Order{
			ID:            order.ID,
			UserID:        order.UserID,
			Symbol:        order.Symbol,
			Side:          order.Side,
			Type:          order.Type,
			Quantity:      order.Quantity,
			Price:         order.Price,
			StopPrice:     order.StopPrice,
			TimeInForce:   order.TimeInForce,
			Status:        order.Status,
			FilledQty:     order.FilledQty,
			RemainingQty:  order.RemainingQty,
			AvgPrice:      order.AvgPrice,
			Commission:    order.Commission,
			ClientOrderID: order.ClientOrderID,
			CreatedAt:     order.CreatedAt,
			UpdatedAt:     order.UpdatedAt,
			ExpiresAt:     order.ExpiresAt,
		}
	}

	return contractOrders, total, nil
}

// GetOrderBook retrieves order book data
func (a *TradingServiceAdapter) GetOrderBook(ctx context.Context, symbol string, depth int) (*contracts.OrderBook, error) {
	a.logger.Debug("Getting order book",
		zap.String("symbol", symbol),
		zap.Int("depth", depth))

	// Call trading service to get order book
	orderBook, err := a.service.GetOrderBook(ctx, symbol, depth)
	if err != nil {
		a.logger.Error("Failed to get order book",
			zap.String("symbol", symbol),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get order book: %w", err)
	}

	// Convert trading order book levels to contract levels
	contractBids := make([]contracts.OrderBookLevel, len(orderBook.Bids))
	for i, bid := range orderBook.Bids {
		contractBids[i] = contracts.OrderBookLevel{
			Price:    bid.Price,
			Quantity: bid.Quantity,
			Orders:   bid.Orders,
		}
	}

	contractAsks := make([]contracts.OrderBookLevel, len(orderBook.Asks))
	for i, ask := range orderBook.Asks {
		contractAsks[i] = contracts.OrderBookLevel{
			Price:    ask.Price,
			Quantity: ask.Quantity,
			Orders:   ask.Orders,
		}
	}

	// Convert trading order book to contract order book
	return &contracts.OrderBook{
		Symbol:    orderBook.Symbol,
		Sequence:  orderBook.Sequence,
		Timestamp: orderBook.Timestamp,
		Bids:      contractBids,
		Asks:      contractAsks,
	}, nil
}

// GetTicker retrieves ticker data
func (a *TradingServiceAdapter) GetTicker(ctx context.Context, symbol string) (*contracts.Ticker, error) {
	a.logger.Debug("Getting ticker", zap.String("symbol", symbol))

	// Call trading service to get ticker
	ticker, err := a.service.GetTicker(ctx, symbol)
	if err != nil {
		a.logger.Error("Failed to get ticker",
			zap.String("symbol", symbol),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get ticker: %w", err)
	}

	// Convert trading ticker to contract ticker
	return &contracts.Ticker{
		Symbol:     ticker.Symbol,
		LastPrice:  ticker.LastPrice,
		BestBid:    ticker.BestBid,
		BestAsk:    ticker.BestAsk,
		Volume24h:  ticker.Volume24h,
		Change24h:  ticker.Change24h,
		ChangePerc: ticker.ChangePerc,
		High24h:    ticker.High24h,
		Low24h:     ticker.Low24h,
		Timestamp:  ticker.Timestamp,
	}, nil
}

// GetTrades retrieves recent trades
func (a *TradingServiceAdapter) GetTrades(ctx context.Context, symbol string, limit int) ([]*contracts.Trade, error) {
	a.logger.Debug("Getting trades",
		zap.String("symbol", symbol),
		zap.Int("limit", limit))

	// Call trading service to get trades
	trades, err := a.service.GetTrades(ctx, symbol, limit)
	if err != nil {
		a.logger.Error("Failed to get trades",
			zap.String("symbol", symbol),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get trades: %w", err)
	}

	// Convert trading trades to contract trades
	contractTrades := make([]*contracts.Trade, len(trades))
	for i, trade := range trades {
		contractTrades[i] = &contracts.Trade{
			ID:         trade.ID,
			Symbol:     trade.Symbol,
			OrderID:    trade.OrderID,
			BuyerID:    trade.BuyerID,
			SellerID:   trade.SellerID,
			Price:      trade.Price,
			Quantity:   trade.Quantity,
			Commission: trade.Commission,
			Role:       trade.Role,
			Status:     trade.Status,
			ExecutedAt: trade.ExecutedAt,
			SettledAt:  trade.SettledAt,
		}
	}

	return contractTrades, nil
}

// GetUserTrades retrieves user trades with filtering
func (a *TradingServiceAdapter) GetUserTrades(ctx context.Context, userID uuid.UUID, filter *contracts.TradeFilter) ([]*contracts.Trade, int64, error) {
	a.logger.Debug("Getting user trades", zap.String("user_id", userID.String()))

	// Convert contract filter to trading filter
	var tradingFilter *trading.TradeFilter
	if filter != nil {
		tradingFilter = &trading.TradeFilter{
			Symbol:    filter.Symbol,
			StartTime: filter.StartTime,
			EndTime:   filter.EndTime,
			Limit:     filter.Limit,
			Offset:    filter.Offset,
		}
	}

	// Call trading service to get user trades
	trades, total, err := a.service.GetUserTrades(ctx, userID, tradingFilter)
	if err != nil {
		a.logger.Error("Failed to get user trades",
			zap.String("user_id", userID.String()),
			zap.Error(err))
		return nil, 0, fmt.Errorf("failed to get user trades: %w", err)
	}

	// Convert trading trades to contract trades
	contractTrades := make([]*contracts.Trade, len(trades))
	for i, trade := range trades {
		contractTrades[i] = &contracts.Trade{
			ID:         trade.ID,
			Symbol:     trade.Symbol,
			OrderID:    trade.OrderID,
			BuyerID:    trade.BuyerID,
			SellerID:   trade.SellerID,
			Price:      trade.Price,
			Quantity:   trade.Quantity,
			Commission: trade.Commission,
			Role:       trade.Role,
			Status:     trade.Status,
			ExecutedAt: trade.ExecutedAt,
			SettledAt:  trade.SettledAt,
		}
	}

	return contractTrades, total, nil
}

// GetMarkets retrieves all available markets
func (a *TradingServiceAdapter) GetMarkets(ctx context.Context) ([]*contracts.Market, error) {
	a.logger.Debug("Getting markets")

	// Call trading service to get markets
	markets, err := a.service.GetMarkets(ctx)
	if err != nil {
		a.logger.Error("Failed to get markets", zap.Error(err))
		return nil, fmt.Errorf("failed to get markets: %w", err)
	}

	// Convert trading markets to contract markets
	contractMarkets := make([]*contracts.Market, len(markets))
	for i, market := range markets {
		contractMarkets[i] = &contracts.Market{
			Symbol:        market.Symbol,
			BaseCurrency:  market.BaseCurrency,
			QuoteCurrency: market.QuoteCurrency,
			Status:        market.Status,
			MinOrderSize:  market.MinOrderSize,
			MaxOrderSize:  market.MaxOrderSize,
			PriceStep:     market.PriceStep,
			QuantityStep:  market.QuantityStep,
			MakerFee:      market.MakerFee,
			TakerFee:      market.TakerFee,
			CreatedAt:     market.CreatedAt,
		}
	}

	return contractMarkets, nil
}

// GetMarket retrieves specific market information
func (a *TradingServiceAdapter) GetMarket(ctx context.Context, symbol string) (*contracts.Market, error) {
	a.logger.Debug("Getting market", zap.String("symbol", symbol))

	// Call trading service to get market
	market, err := a.service.GetMarket(ctx, symbol)
	if err != nil {
		a.logger.Error("Failed to get market",
			zap.String("symbol", symbol),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get market: %w", err)
	}

	// Convert trading market to contract market
	return &contracts.Market{
		Symbol:        market.Symbol,
		BaseCurrency:  market.BaseCurrency,
		QuoteCurrency: market.QuoteCurrency,
		Status:        market.Status,
		MinOrderSize:  market.MinOrderSize,
		MaxOrderSize:  market.MaxOrderSize,
		PriceStep:     market.PriceStep,
		QuantityStep:  market.QuantityStep,
		MakerFee:      market.MakerFee,
		TakerFee:      market.TakerFee,
		CreatedAt:     market.CreatedAt,
	}, nil
}

// GetMarketStats retrieves market statistics
func (a *TradingServiceAdapter) GetMarketStats(ctx context.Context, symbol string) (*contracts.MarketStats, error) {
	a.logger.Debug("Getting market stats", zap.String("symbol", symbol))

	// Call trading service to get market stats
	stats, err := a.service.GetMarketStats(ctx, symbol)
	if err != nil {
		a.logger.Error("Failed to get market stats",
			zap.String("symbol", symbol),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get market stats: %w", err)
	}

	// Convert trading stats to contract stats
	return &contracts.MarketStats{
		Symbol:        stats.Symbol,
		Volume24h:     stats.Volume24h,
		High24h:       stats.High24h,
		Low24h:        stats.Low24h,
		Change24h:     stats.Change24h,
		ChangePerc24h: stats.ChangePerc24h,
		TradeCount24h: stats.TradeCount24h,
		LastPrice:     stats.LastPrice,
		UpdatedAt:     stats.UpdatedAt,
	}, nil
}

// ValidateOrder validates order before placement
func (a *TradingServiceAdapter) ValidateOrder(ctx context.Context, req *contracts.PlaceOrderRequest) (*contracts.OrderValidation, error) {
	a.logger.Debug("Validating order",
		zap.String("user_id", req.UserID.String()),
		zap.String("symbol", req.Symbol))

	// Convert contract request to trading request
	tradingReq := &trading.PlaceOrderRequest{
		UserID:        req.UserID,
		Symbol:        req.Symbol,
		Side:          req.Side,
		Type:          req.Type,
		Quantity:      req.Quantity,
		Price:         req.Price,
		StopPrice:     req.StopPrice,
		TimeInForce:   req.TimeInForce,
		PostOnly:      req.PostOnly,
		ReduceOnly:    req.ReduceOnly,
		ClientOrderID: req.ClientOrderID,
		Metadata:      req.Metadata,
	}

	// Call trading service to validate order
	validation, err := a.service.ValidateOrder(ctx, tradingReq)
	if err != nil {
		a.logger.Error("Failed to validate order",
			zap.String("user_id", req.UserID.String()),
			zap.String("symbol", req.Symbol),
			zap.Error(err))
		return nil, fmt.Errorf("failed to validate order: %w", err)
	}

	// Convert trading validation to contract validation
	return &contracts.OrderValidation{
		Valid:           validation.Valid,
		Errors:          validation.Errors,
		Warnings:        validation.Warnings,
		EstimatedFee:    validation.EstimatedFee,
		RequiredBalance: validation.RequiredBalance,
	}, nil
}

// GetUserRiskProfile retrieves user risk profile
func (a *TradingServiceAdapter) GetUserRiskProfile(ctx context.Context, userID uuid.UUID) (*contracts.RiskProfile, error) {
	a.logger.Debug("Getting user risk profile", zap.String("user_id", userID.String()))

	// Call trading service to get user risk profile
	profile, err := a.service.GetUserRiskProfile(ctx, userID)
	if err != nil {
		a.logger.Error("Failed to get user risk profile",
			zap.String("user_id", userID.String()),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get user risk profile: %w", err)
	}

	// Convert trading profile to contract profile
	return &contracts.RiskProfile{
		UserID:            profile.UserID,
		RiskLevel:         profile.RiskLevel,
		MaxOrderSize:      profile.MaxOrderSize,
		MaxDailyVolume:    profile.MaxDailyVolume,
		MaxOpenOrders:     profile.MaxOpenOrders,
		AllowedSymbols:    profile.AllowedSymbols,
		RestrictedSymbols: profile.RestrictedSymbols,
		UpdatedAt:         profile.UpdatedAt,
	}, nil
}

// CheckTradeCompliance checks trade compliance
func (a *TradingServiceAdapter) CheckTradeCompliance(ctx context.Context, req *contracts.ComplianceCheckRequest) (*contracts.ComplianceResult, error) {
	a.logger.Debug("Checking trade compliance",
		zap.String("user_id", req.UserID.String()),
		zap.String("symbol", req.Symbol))

	// Convert contract request to trading request
	tradingReq := &trading.ComplianceCheckRequest{
		UserID:    req.UserID,
		Symbol:    req.Symbol,
		Amount:    req.Amount,
		Operation: req.Operation,
		Metadata:  req.Metadata,
	}

	// Call trading service to check compliance
	result, err := a.service.CheckTradeCompliance(ctx, tradingReq)
	if err != nil {
		a.logger.Error("Failed to check trade compliance",
			zap.String("user_id", req.UserID.String()),
			zap.String("symbol", req.Symbol),
			zap.Error(err))
		return nil, fmt.Errorf("failed to check trade compliance: %w", err)
	}

	// Convert trading result to contract result
	return &contracts.ComplianceResult{
		Approved: result.Approved,
		Reason:   result.Reason,
		Warnings: result.Warnings,
		Flags:    result.Flags,
	}, nil
}

// SettleTrade settles a trade
func (a *TradingServiceAdapter) SettleTrade(ctx context.Context, tradeID uuid.UUID) error {
	a.logger.Debug("Settling trade", zap.String("trade_id", tradeID.String()))

	// Call trading service to settle trade
	if err := a.service.SettleTrade(ctx, tradeID); err != nil {
		a.logger.Error("Failed to settle trade",
			zap.String("trade_id", tradeID.String()),
			zap.Error(err))
		return fmt.Errorf("failed to settle trade: %w", err)
	}

	return nil
}

// GetPendingSettlements retrieves pending settlements
func (a *TradingServiceAdapter) GetPendingSettlements(ctx context.Context) ([]*contracts.PendingSettlement, error) {
	a.logger.Debug("Getting pending settlements")

	// Call trading service to get pending settlements
	settlements, err := a.service.GetPendingSettlements(ctx)
	if err != nil {
		a.logger.Error("Failed to get pending settlements", zap.Error(err))
		return nil, fmt.Errorf("failed to get pending settlements: %w", err)
	}

	// Convert trading settlements to contract settlements
	contractSettlements := make([]*contracts.PendingSettlement, len(settlements))
	for i, settlement := range settlements {
		contractSettlements[i] = &contracts.PendingSettlement{
			TradeID:     settlement.TradeID,
			Symbol:      settlement.Symbol,
			BuyerID:     settlement.BuyerID,
			SellerID:    settlement.SellerID,
			Amount:      settlement.Amount,
			Status:      settlement.Status,
			CreatedAt:   settlement.CreatedAt,
			RetryCount:  settlement.RetryCount,
			LastAttempt: settlement.LastAttempt,
		}
	}

	return contractSettlements, nil
}

// ReconcileTrades performs trade reconciliation
func (a *TradingServiceAdapter) ReconcileTrades(ctx context.Context, req *contracts.TradeReconciliationRequest) (*contracts.TradeReconciliationResult, error) {
	a.logger.Debug("Reconciling trades")

	// Convert contract request to trading request
	tradingReq := &trading.TradeReconciliationRequest{
		StartTime: req.StartTime,
		EndTime:   req.EndTime,
		Symbols:   req.Symbols,
		Force:     req.Force,
	}

	// Call trading service to reconcile trades
	result, err := a.service.ReconcileTrades(ctx, tradingReq)
	if err != nil {
		a.logger.Error("Failed to reconcile trades", zap.Error(err))
		return nil, fmt.Errorf("failed to reconcile trades: %w", err)
	}

	// Convert trading discrepancies to contract discrepancies
	contractDiscrepancies := make([]contracts.TradeDiscrepancy, len(result.Discrepancies))
	for i, discrepancy := range result.Discrepancies {
		contractDiscrepancies[i] = contracts.TradeDiscrepancy{
			TradeID:     discrepancy.TradeID,
			Type:        discrepancy.Type,
			Expected:    discrepancy.Expected,
			Actual:      discrepancy.Actual,
			Difference:  discrepancy.Difference,
			Description: discrepancy.Description,
		}
	}

	// Convert trading result to contract result
	return &contracts.TradeReconciliationResult{
		ProcessedTrades:  result.ProcessedTrades,
		ReconciledTrades: result.ReconciledTrades,
		Discrepancies:    contractDiscrepancies,
		Summary:          result.Summary,
		ProcessedAt:      result.ProcessedAt,
	}, nil
}

// GetTradingMetrics retrieves trading performance metrics
func (a *TradingServiceAdapter) GetTradingMetrics(ctx context.Context, req *contracts.MetricsRequest) (*contracts.TradingMetrics, error) {
	a.logger.Debug("Getting trading metrics")

	// Convert contract request to trading request
	tradingReq := &trading.MetricsRequest{
		StartTime: req.StartTime,
		EndTime:   req.EndTime,
		Symbols:   req.Symbols,
		UserID:    req.UserID,
	}

	// Call trading service to get trading metrics
	metrics, err := a.service.GetTradingMetrics(ctx, tradingReq)
	if err != nil {
		a.logger.Error("Failed to get trading metrics", zap.Error(err))
		return nil, fmt.Errorf("failed to get trading metrics: %w", err)
	}

	// Convert trading symbol metrics to contract symbol metrics
	contractSymbolMetrics := make(map[string]contracts.SymbolMetrics)
	for symbol, symbolMetrics := range metrics.SymbolMetrics {
		contractSymbolMetrics[symbol] = contracts.SymbolMetrics{
			Symbol:      symbolMetrics.Symbol,
			Volume:      symbolMetrics.Volume,
			TradeCount:  symbolMetrics.TradeCount,
			OrderCount:  symbolMetrics.OrderCount,
			AvgSpread:   symbolMetrics.AvgSpread,
			PriceChange: symbolMetrics.PriceChange,
		}
	}

	// Convert trading metrics to contract metrics
	return &contracts.TradingMetrics{
		TotalOrders:      metrics.TotalOrders,
		FilledOrders:     metrics.FilledOrders,
		TotalTrades:      metrics.TotalTrades,
		TotalVolume:      metrics.TotalVolume,
		AverageOrderSize: metrics.AverageOrderSize,
		FillRate:         metrics.FillRate,
		SymbolMetrics:    contractSymbolMetrics,
		GeneratedAt:      metrics.GeneratedAt,
	}, nil
}

// GetLatencyMetrics retrieves system latency metrics
func (a *TradingServiceAdapter) GetLatencyMetrics(ctx context.Context) (*contracts.LatencyMetrics, error) {
	a.logger.Debug("Getting latency metrics")

	// Call trading service to get latency metrics
	metrics, err := a.service.GetLatencyMetrics(ctx)
	if err != nil {
		a.logger.Error("Failed to get latency metrics", zap.Error(err))
		return nil, fmt.Errorf("failed to get latency metrics: %w", err)
	}

	// Convert trading latency metrics to contract latency metrics
	return &contracts.LatencyMetrics{
		OrderProcessing: contracts.LatencyStats{
			Mean:   metrics.OrderProcessing.Mean,
			Median: metrics.OrderProcessing.Median,
			P95:    metrics.OrderProcessing.P95,
			P99:    metrics.OrderProcessing.P99,
			Max:    metrics.OrderProcessing.Max,
			Min:    metrics.OrderProcessing.Min,
		},
		TradeExecution: contracts.LatencyStats{
			Mean:   metrics.TradeExecution.Mean,
			Median: metrics.TradeExecution.Median,
			P95:    metrics.TradeExecution.P95,
			P99:    metrics.TradeExecution.P99,
			Max:    metrics.TradeExecution.Max,
			Min:    metrics.TradeExecution.Min,
		},
		OrderBookUpdate: contracts.LatencyStats{
			Mean:   metrics.OrderBookUpdate.Mean,
			Median: metrics.OrderBookUpdate.Median,
			P95:    metrics.OrderBookUpdate.P95,
			P99:    metrics.OrderBookUpdate.P99,
			Max:    metrics.OrderBookUpdate.Max,
			Min:    metrics.OrderBookUpdate.Min,
		},
		DatabaseWrite: contracts.LatencyStats{
			Mean:   metrics.DatabaseWrite.Mean,
			Median: metrics.DatabaseWrite.Median,
			P95:    metrics.DatabaseWrite.P95,
			P99:    metrics.DatabaseWrite.P99,
			Max:    metrics.DatabaseWrite.Max,
			Min:    metrics.DatabaseWrite.Min,
		},
		GeneratedAt: metrics.GeneratedAt,
	}, nil
}

// HealthCheck performs health check
func (a *TradingServiceAdapter) HealthCheck(ctx context.Context) (*contracts.HealthStatus, error) {
	a.logger.Debug("Performing health check")

	// Call trading service health check
	health, err := a.service.HealthCheck(ctx)
	if err != nil {
		a.logger.Error("Health check failed", zap.Error(err))
		return nil, fmt.Errorf("health check failed: %w", err)
	}

	// Convert trading health status to contract health status
	return &contracts.HealthStatus{
		Status:       health.Status,
		Timestamp:    health.Timestamp,
		Version:      health.Version,
		Uptime:       health.Uptime,
		Metrics:      health.Metrics,
		Dependencies: health.Dependencies,
	}, nil
}
