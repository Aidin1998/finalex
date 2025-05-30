// =============================
// Trigger Monitor Worker Functions
// =============================
// This file contains the worker functions for the trigger monitoring service

package trigger

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	metricsapi "github.com/Orbit-CEX/pincex_unified/internal/analytics/metrics"
	"github.com/Orbit-CEX/pincex_unified/internal/trading/model"
)

// monitoringWorker continuously monitors trigger conditions
func (tm *TriggerMonitor) monitoringWorker(ctx context.Context, workerID int) {
	defer tm.workerWg.Done()

	ticker := time.NewTicker(tm.monitorInterval)
	defer ticker.Stop()

	tm.logger.Debugw("Starting monitoring worker", "worker_id", workerID)

	for {
		select {
		case <-ctx.Done():
			return
		case <-tm.stopChan:
			return
		case <-ticker.C:
			tm.processConditions(workerID)
		}
	}
}

// processConditions processes all trigger conditions
func (tm *TriggerMonitor) processConditions(workerID int) {
	startTime := time.Now()

	// Get snapshot of conditions to avoid holding lock for too long
	tm.conditionsMu.RLock()
	conditions := make([]*TriggerCondition, 0, len(tm.conditions))
	for _, condition := range tm.conditions {
		conditions = append(conditions, condition)
	}
	tm.conditionsMu.RUnlock()

	// Process conditions in batches
	batchSize := (len(conditions) / 4) + 1
	startIdx := workerID * batchSize
	endIdx := startIdx + batchSize

	if startIdx >= len(conditions) {
		return
	}
	if endIdx > len(conditions) {
		endIdx = len(conditions)
	}

	for i := startIdx; i < endIdx; i++ {
		condition := conditions[i]
		if tm.checkTriggerCondition(condition) {
			tm.executeTrigger(condition)
		}
	}

	// Update performance metrics
	processingTime := time.Since(startTime)
	atomic.AddInt64(&tm.avgLatencyNs, processingTime.Nanoseconds())

	maxLatency := atomic.LoadInt64(&tm.maxLatencyNs)
	if processingTime.Nanoseconds() > maxLatency {
		atomic.StoreInt64(&tm.maxLatencyNs, processingTime.Nanoseconds())
	}
}

// checkTriggerCondition checks if a trigger condition is met
func (tm *TriggerMonitor) checkTriggerCondition(condition *TriggerCondition) bool {
	currentPrice := tm.getCurrentPrice(condition.Pair)
	if currentPrice.IsZero() {
		return false
	}

	// Update last price
	tm.pricesMu.Lock()
	tm.lastPrices[condition.Pair] = currentPrice
	tm.pricesMu.Unlock()

	switch condition.Type {
	case TriggerTypeStopLoss, TriggerTypeTakeProfit:
		return tm.checkPriceTrigger(condition, currentPrice)
	case TriggerTypeTrailingStop:
		return tm.checkTrailingStopTrigger(condition, currentPrice)
	default:
		return false
	}
}

// checkPriceTrigger checks simple price trigger conditions
func (tm *TriggerMonitor) checkPriceTrigger(condition *TriggerCondition, currentPrice decimal.Decimal) bool {
	switch condition.Direction {
	case "above":
		return currentPrice.GreaterThanOrEqual(condition.TriggerPrice)
	case "below":
		return currentPrice.LessThanOrEqual(condition.TriggerPrice)
	default:
		return false
	}
}

// checkTrailingStopTrigger checks trailing stop trigger conditions
func (tm *TriggerMonitor) checkTrailingStopTrigger(condition *TriggerCondition, currentPrice decimal.Decimal) bool {
	// Update trailing price if beneficial
	shouldUpdate := false
	newTriggerPrice := condition.TriggerPrice

	if condition.Direction == "below" {
		// Sell trailing stop: update if price goes higher
		if currentPrice.GreaterThan(condition.LastTrailingPrice) {
			newTriggerPrice = currentPrice.Sub(condition.TrailingOffset)
			shouldUpdate = true
		}
	} else {
		// Buy trailing stop: update if price goes lower
		if currentPrice.LessThan(condition.LastTrailingPrice) {
			newTriggerPrice = currentPrice.Add(condition.TrailingOffset)
			shouldUpdate = true
		}
	}

	if shouldUpdate {
		tm.conditionsMu.Lock()
		if cond, exists := tm.conditions[condition.ID]; exists {
			cond.TriggerPrice = newTriggerPrice
			cond.LastTrailingPrice = currentPrice
			cond.UpdatedAt = time.Now()
		}
		tm.conditionsMu.Unlock()
	}

	// Check if trigger condition is met
	return tm.checkPriceTrigger(condition, currentPrice)
}

// executeTrigger executes a triggered condition
func (tm *TriggerMonitor) executeTrigger(condition *TriggerCondition) {
	startTime := time.Now()
	defer func() {
		atomic.AddInt64(&tm.triggersProcessed, 1)

		// Record trigger latency
		latency := time.Since(startTime)
		if metricsapi.BusinessMetricsInstance != nil {
			metricsapi.BusinessMetricsInstance.RecordTriggerLatency(condition.Pair, latency)
		}
	}()

	currentPrice := tm.getCurrentPrice(condition.Pair)

	tm.logger.Infow("Executing trigger",
		"condition_id", condition.ID,
		"order_id", condition.OrderID,
		"type", condition.Type,
		"trigger_price", condition.TriggerPrice,
		"current_price", currentPrice)

	// Execute the trigger callback
	if condition.OnTrigger != nil {
		if err := condition.OnTrigger(condition, currentPrice); err != nil {
			tm.logger.Errorw("Failed to execute trigger",
				"condition_id", condition.ID,
				"error", err)

			// Record failed trigger
			if metricsapi.AlertingServiceInstance != nil {
				metricsapi.AlertingServiceInstance.Raise(metricsapi.Alert{
					Type:      metricsapi.AlertTriggerFailure,
					Market:    condition.Pair,
					User:      condition.UserID.String(),
					OrderType: "trigger",
					Value:     0,
					Threshold: 0,
					Details:   err.Error(),
					Timestamp: time.Now(),
				})
			}
			return
		}
	}

	// Remove the triggered condition
	tm.conditionsMu.Lock()
	delete(tm.conditions, condition.ID)
	tm.conditionsMu.Unlock()
}

// triggerStopLoss handles stop-loss order triggering
func (tm *TriggerMonitor) triggerStopLoss(condition *TriggerCondition, currentPrice decimal.Decimal) error {
	// Get the original order
	order, err := tm.orderRepo.GetOrder(context.Background(), condition.OrderID)
	if err != nil {
		return err
	}

	// Create market order for stop-loss execution
	marketOrder := &model.Order{
		ID:               uuid.New(),
		UserID:           order.UserID,
		Pair:             order.Pair,
		Type:             model.OrderTypeMarket,
		Side:             order.Side,
		Quantity:         order.Quantity.Sub(order.FilledQuantity),
		Price:            decimal.Zero, // Market order
		Status:           model.OrderStatusNew,
		TimeInForce:      model.TimeInForceIOC,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
		ParentOrderID:    &order.ID,
		TriggerPrice:     condition.TriggerPrice,
		IsTriggeredOrder: true,
	}

	// Update original order status
	order.Status = model.OrderStatusTriggered
	order.UpdatedAt = time.Now()

	// Save orders
	if err := tm.orderRepo.UpdateOrder(context.Background(), order); err != nil {
		return err
	}

	if err := tm.orderRepo.CreateOrder(context.Background(), marketOrder); err != nil {
		return err
	}

	// Execute the market order if callback is set
	if tm.onOrderTrigger != nil {
		return tm.onOrderTrigger(marketOrder)
	}

	return nil
}

// triggerTakeProfit handles take-profit order triggering
func (tm *TriggerMonitor) triggerTakeProfit(condition *TriggerCondition, currentPrice decimal.Decimal) error {
	// Get the original order
	order, err := tm.orderRepo.GetOrder(context.Background(), condition.OrderID)
	if err != nil {
		return err
	}

	// Create limit order for take-profit execution
	profitOrder := &model.Order{
		ID:               uuid.New(),
		UserID:           order.UserID,
		Pair:             order.Pair,
		Type:             model.OrderTypeLimit,
		Side:             order.Side,
		Quantity:         order.Quantity.Sub(order.FilledQuantity),
		Price:            condition.TriggerPrice, // Use trigger price as limit price
		Status:           model.OrderStatusNew,
		TimeInForce:      model.TimeInForceGTC,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
		ParentOrderID:    &order.ID,
		TriggerPrice:     condition.TriggerPrice,
		IsTriggeredOrder: true,
	}

	// Update original order status
	order.Status = model.OrderStatusTriggered
	order.UpdatedAt = time.Now()

	// Save orders
	if err := tm.orderRepo.UpdateOrder(context.Background(), order); err != nil {
		return err
	}

	if err := tm.orderRepo.CreateOrder(context.Background(), profitOrder); err != nil {
		return err
	}

	// Execute the limit order if callback is set
	if tm.onOrderTrigger != nil {
		return tm.onOrderTrigger(profitOrder)
	}

	return nil
}

// triggerTrailingStop handles trailing stop order triggering
func (tm *TriggerMonitor) triggerTrailingStop(condition *TriggerCondition, currentPrice decimal.Decimal) error {
	// Get the original order
	order, err := tm.orderRepo.GetOrder(context.Background(), condition.OrderID)
	if err != nil {
		return err
	}

	// Create market order for trailing stop execution
	stopOrder := &model.Order{
		ID:               uuid.New(),
		UserID:           order.UserID,
		Pair:             order.Pair,
		Type:             model.OrderTypeMarket,
		Side:             order.Side,
		Quantity:         order.Quantity.Sub(order.FilledQuantity),
		Price:            decimal.Zero, // Market order
		Status:           model.OrderStatusNew,
		TimeInForce:      model.TimeInForceIOC,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
		ParentOrderID:    &order.ID,
		TriggerPrice:     condition.TriggerPrice,
		TrailingOffset:   condition.TrailingOffset,
		IsTriggeredOrder: true,
	}

	// Update original order status
	order.Status = model.OrderStatusTriggered
	order.UpdatedAt = time.Now()

	// Save orders
	if err := tm.orderRepo.UpdateOrder(context.Background(), order); err != nil {
		return err
	}

	if err := tm.orderRepo.CreateOrder(context.Background(), stopOrder); err != nil {
		return err
	}

	// Execute the market order if callback is set
	if tm.onOrderTrigger != nil {
		return tm.onOrderTrigger(stopOrder)
	}

	return nil
}

// getCurrentPrice gets the current market price for a trading pair
func (tm *TriggerMonitor) getCurrentPrice(pair string) decimal.Decimal {
	tm.orderBooksMu.RLock()
	ob, exists := tm.orderBooks[pair]
	tm.orderBooksMu.RUnlock()

	if !exists {
		return decimal.Zero
	}

	// Get mid price from order book
	snapshot, _ := ob.GetSnapshot(1)
	if len(snapshot[0]) == 0 || len(snapshot[1]) == 0 {
		return decimal.Zero
	}

	// Parse best bid and ask
	bestBidStr := snapshot[0][0] // bids[0][0] = price
	bestAskStr := snapshot[1][0] // asks[0][0] = price

	bestBid, err1 := decimal.NewFromString(bestBidStr)
	bestAsk, err2 := decimal.NewFromString(bestAskStr)

	if err1 != nil || err2 != nil {
		return decimal.Zero
	}

	// Return mid price
	return bestBid.Add(bestAsk).Div(decimal.NewFromInt(2))
}

// icebergWorker manages iceberg order slice creation and monitoring
func (tm *TriggerMonitor) icebergWorker(ctx context.Context) {
	defer tm.workerWg.Done()

	ticker := time.NewTicker(time.Second) // Check every second for iceberg refills
	defer ticker.Stop()

	tm.logger.Debug("Starting iceberg worker")

	for {
		select {
		case <-ctx.Done():
			return
		case <-tm.stopChan:
			return
		case <-ticker.C:
			tm.processIcebergOrders()
		}
	}
}

// processIcebergOrders processes all active iceberg orders
func (tm *TriggerMonitor) processIcebergOrders() {
	tm.icebergMu.RLock()
	states := make([]*IcebergOrderState, 0, len(tm.icebergOrders))
	for _, state := range tm.icebergOrders {
		if state.Status == "active" {
			states = append(states, state)
		}
	}
	tm.icebergMu.RUnlock()

	for _, state := range states {
		// Check if current slice needs monitoring
		tm.monitorIcebergSlice(state)
	}
}

// monitorIcebergSlice monitors a specific iceberg slice
func (tm *TriggerMonitor) monitorIcebergSlice(state *IcebergOrderState) {
	// Get current slice order from order book or repository
	currentOrder, err := tm.orderRepo.GetOrder(context.Background(), state.CurrentSliceID)
	if err != nil {
		tm.logger.Errorw("Failed to get iceberg slice order",
			"slice_id", state.CurrentSliceID,
			"error", err)
		return
	}

	// Check if slice is fully filled
	if currentOrder.Status == model.OrderStatusFilled {
		tm.icebergMu.Lock()
		state.FilledQuantity = state.FilledQuantity.Add(currentOrder.Quantity)
		state.RemainingQuantity = state.TotalQuantity.Sub(state.FilledQuantity)
		tm.icebergMu.Unlock()

		// Create new slice if more quantity remains
		if state.RemainingQuantity.GreaterThan(decimal.Zero) {
			if err := tm.createIcebergSlice(state); err != nil {
				tm.logger.Errorw("Failed to create new iceberg slice",
					"order_id", state.OrderID,
					"error", err)
			}
		} else {
			// Iceberg order is complete
			tm.icebergMu.Lock()
			state.Status = "filled"
			delete(tm.icebergOrders, state.OrderID)
			tm.icebergMu.Unlock()

			tm.logger.Infow("Iceberg order completed",
				"order_id", state.OrderID,
				"total_filled", state.FilledQuantity)
		}
	}
}

// createIcebergSlice creates a new slice for an iceberg order
func (tm *TriggerMonitor) createIcebergSlice(state *IcebergOrderState) error {
	// Calculate slice size
	sliceSize := state.DisplayQuantity

	// Apply randomization if enabled
	if state.RandomizeSlices {
		// Randomize between min and max slice size
		minSize := state.MinSliceSize
		maxSize := state.MaxSliceSize
		if maxSize.LessThan(minSize) {
			maxSize = minSize
		}

		// Simple randomization - in production, use crypto/rand
		variation := maxSize.Sub(minSize)
		if !variation.IsZero() {
			// Use slice count as seed for deterministic but varied sizes
			factor := decimal.NewFromFloat(float64(state.SliceCount%100) / 100.0)
			sliceSize = minSize.Add(variation.Mul(factor))
		}
	}

	// Ensure slice doesn't exceed remaining quantity
	if sliceSize.GreaterThan(state.RemainingQuantity) {
		sliceSize = state.RemainingQuantity
	}

	// Create new slice order
	sliceOrder := &model.Order{
		ID:              uuid.New(),
		UserID:          state.UserID,
		Pair:            state.Pair,
		Type:            model.OrderTypeLimit,
		Side:            state.Side,
		Quantity:        sliceSize,
		Price:           state.Price,
		Status:          model.OrderStatusNew,
		TimeInForce:     model.TimeInForceGTC,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
		ParentOrderID:   &state.OrderID,
		DisplayQuantity: sliceSize, // For tracking
		IsIcebergSlice:  true,
	}

	// Update iceberg state
	state.CurrentSliceID = sliceOrder.ID
	state.SliceCount++
	state.LastRefill = time.Now()

	// Save slice order
	if err := tm.orderRepo.CreateOrder(context.Background(), sliceOrder); err != nil {
		return err
	}

	tm.logger.Debugw("Created iceberg slice",
		"order_id", state.OrderID,
		"slice_id", sliceOrder.ID,
		"slice_size", sliceSize,
		"slice_count", state.SliceCount)

	// Execute slice order if callback is set
	if tm.onIcebergSlice != nil {
		return tm.onIcebergSlice(state, sliceOrder)
	}

	return nil
}

// metricsWorker collects and reports metrics
func (tm *TriggerMonitor) metricsWorker(ctx context.Context) {
	defer tm.workerWg.Done()

	ticker := time.NewTicker(10 * time.Second) // Report metrics every 10 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-tm.stopChan:
			return
		case <-ticker.C:
			tm.reportMetrics()
		}
	}
}

// reportMetrics reports trigger monitoring metrics
func (tm *TriggerMonitor) reportMetrics() {
	stats := tm.GetTriggerStats()

	// Report to business metrics if available
	if metricsapi.BusinessMetricsInstance != nil {
		metricsapi.BusinessMetricsInstance.RecordTriggerMetrics(stats)
	}

	tm.logger.Debugw("Trigger monitor metrics", "stats", stats)
}
