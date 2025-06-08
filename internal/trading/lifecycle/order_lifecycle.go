package lifecycle

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Aidin1998/finalex/internal/trading/events"
	"github.com/Aidin1998/finalex/internal/trading/model"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// OrderState represents the state of an order in the lifecycle
type OrderState string

const (
	OrderStateSubmitted       OrderState = "SUBMITTED"
	OrderStatePending         OrderState = "PENDING"
	OrderStateValidated       OrderState = "VALIDATED"
	OrderStateAccepted        OrderState = "ACCEPTED"
	OrderStateRejected        OrderState = "REJECTED"
	OrderStatePartiallyFilled OrderState = "PARTIALLY_FILLED"
	OrderStateFilled          OrderState = "FILLED"
	OrderStateCanceling       OrderState = "CANCELING"
	OrderStateCanceled        OrderState = "CANCELED"
	OrderStateExpired         OrderState = "EXPIRED"
	OrderStateFailed          OrderState = "FAILED"
)

// OrderEvent represents an event in the order lifecycle
type OrderEvent struct {
	ID        string    `gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	OrderID   string    `gorm:"type:uuid;not null;index"`
	EventType string    `gorm:"type:varchar(50);not null"`
	EventData string    `gorm:"type:jsonb"`
	Timestamp time.Time `gorm:"not null;default:current_timestamp"`
	UserID    string    `gorm:"type:uuid;not null;index"`
	TraceID   string    `gorm:"type:varchar(100)"`
	Source    string    `gorm:"type:varchar(50);not null"`
	Version   int       `gorm:"not null;default:1"`
}

// OrderStateTransition represents a state transition in the order lifecycle
type OrderStateTransition struct {
	ID        string    `gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	OrderID   string    `gorm:"type:uuid;not null;index"`
	FromState string    `gorm:"type:varchar(50);not null"`
	ToState   string    `gorm:"type:varchar(50);not null"`
	Reason    string    `gorm:"type:text"`
	Timestamp time.Time `gorm:"not null;default:current_timestamp"`
	UserID    string    `gorm:"type:uuid;not null"`
	TraceID   string    `gorm:"type:varchar(100)"`
	Metadata  string    `gorm:"type:jsonb"`
}

// IdempotencyKey represents an idempotency key for order operations
type IdempotencyKey struct {
	ID        string    `gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	Key       string    `gorm:"type:varchar(255);unique;not null"`
	UserID    string    `gorm:"type:uuid;not null;index"`
	OrderID   string    `gorm:"type:uuid"`
	Operation string    `gorm:"type:varchar(50);not null"`
	Response  string    `gorm:"type:jsonb"`
	CreatedAt time.Time `gorm:"not null;default:current_timestamp"`
	ExpiresAt time.Time `gorm:"not null"`
	Completed bool      `gorm:"not null;default:false"`
}

// OrderLifecycleManager manages the complete lifecycle of orders with ACID compliance
type OrderLifecycleManager struct {
	db         *gorm.DB
	logger     *zap.Logger
	eventBus   events.EventBus // Use the unified event bus
	validators []OrderValidator
	hooks      map[OrderState][]StateHook
	metrics    *LifecycleMetrics
	mutex      sync.RWMutex
}

// OrderValidator defines the interface for order validation
type OrderValidator interface {
	ValidateOrder(ctx context.Context, order *model.Order) error
	Name() string
}

// StateHook defines the interface for state transition hooks
type StateHook interface {
	Execute(ctx context.Context, order *model.Order, transition *OrderStateTransition) error
	Name() string
}

// LifecycleMetrics tracks metrics for order lifecycle operations
type LifecycleMetrics struct {
	OrdersSubmitted    int64
	OrdersAccepted     int64
	OrdersRejected     int64
	OrdersFilled       int64
	OrdersCanceled     int64
	StateTransitions   map[string]int64
	ValidationFailures int64
	IdempotencyHits    int64
	mutex              sync.RWMutex
}

// NewOrderLifecycleManager creates a new order lifecycle manager
func NewOrderLifecycleManager(db *gorm.DB, logger *zap.Logger, eventBus events.EventBus) *OrderLifecycleManager {
	manager := &OrderLifecycleManager{
		db:       db,
		logger:   logger,
		eventBus: eventBus,
		hooks:    make(map[OrderState][]StateHook),
		metrics: &LifecycleMetrics{
			StateTransitions: make(map[string]int64),
		},
	}

	// Migrate tables
	manager.migrateTables()

	return manager
}

// migrateTables creates the necessary database tables
func (olm *OrderLifecycleManager) migrateTables() {
	err := olm.db.AutoMigrate(
		&OrderEvent{},
		&OrderStateTransition{},
		&IdempotencyKey{},
	)
	if err != nil {
		olm.logger.Fatal("Failed to migrate order lifecycle tables", zap.Error(err))
	}
}

// AddValidator adds an order validator
func (olm *OrderLifecycleManager) AddValidator(validator OrderValidator) {
	olm.validators = append(olm.validators, validator)
}

// AddStateHook adds a state transition hook
func (olm *OrderLifecycleManager) AddStateHook(state OrderState, hook StateHook) {
	olm.mutex.Lock()
	defer olm.mutex.Unlock()

	olm.hooks[state] = append(olm.hooks[state], hook)
}

// SubmitOrder submits a new order with full lifecycle management
func (olm *OrderLifecycleManager) SubmitOrder(ctx context.Context, order *model.Order, idempotencyKey string) (*model.Order, error) {
	traceID := getTraceID(ctx)

	// Check idempotency
	if idempotencyKey != "" {
		if result, found := olm.checkIdempotency(ctx, idempotencyKey, order.UserID.String(), "submit_order"); found {
			olm.metrics.incrementIdempotencyHits()
			return result, nil
		}
	}

	// Start database transaction
	tx := olm.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	// Set initial state
	order.Status = string(OrderStateSubmitted)
	order.CreatedAt = time.Now()
	order.UpdatedAt = time.Now()

	// Generate order ID if not present
	if order.ID == uuid.Nil {
		order.ID = uuid.New()
	}

	// Record initial event
	event := &OrderEvent{
		OrderID:   order.ID.String(),
		EventType: "ORDER_SUBMITTED",
		EventData: olm.serializeOrder(order),
		UserID:    order.UserID.String(),
		TraceID:   traceID,
		Source:    "lifecycle_manager",
		Timestamp: time.Now(),
	}

	if err := tx.Create(event).Error; err != nil {
		tx.Rollback()
		olm.logger.Error("Failed to record order submission event",
			zap.String("order_id", order.ID.String()),
			zap.Error(err),
		)
		return nil, fmt.Errorf("failed to record order submission: %w", err)
	}

	// Save idempotency key if provided
	if idempotencyKey != "" {
		idempotency := &IdempotencyKey{
			Key:       idempotencyKey,
			UserID:    order.UserID.String(),
			OrderID:   order.ID.String(),
			Operation: "submit_order",
			CreatedAt: time.Now(),
			ExpiresAt: time.Now().Add(24 * time.Hour), // 24-hour expiry
		}

		if err := tx.Create(idempotency).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to save idempotency key: %w", err)
		}
	}

	// Transition to PENDING state
	if err := olm.transitionState(ctx, tx, order, OrderStateSubmitted, OrderStatePending, "Initial submission"); err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to transition to pending state: %w", err)
	}

	// Validate order
	if err := olm.validateOrder(ctx, order); err != nil {
		// Transition to REJECTED state
		olm.transitionState(ctx, tx, order, OrderStatePending, OrderStateRejected, fmt.Sprintf("Validation failed: %s", err.Error()))
		tx.Commit()

		olm.metrics.incrementValidationFailures()
		return nil, fmt.Errorf("order validation failed: %w", err)
	}

	// Transition to VALIDATED state
	if err := olm.transitionState(ctx, tx, order, OrderStatePending, OrderStateValidated, "Validation passed"); err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to transition to validated state: %w", err)
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit order submission transaction: %w", err)
	}

	// Update idempotency key with result
	if idempotencyKey != "" {
		olm.updateIdempotencyResult(idempotencyKey, order)
	}

	// Publish events asynchronously
	go func() {
		if olm.eventBus != nil {
			olm.eventBus.Publish(context.Background(), events.Event{
				Topic:     events.TopicOrder,
				Type:      "ORDER_SUBMITTED",
				Timestamp: time.Now(),
				Payload: events.OrderEvent{
					OrderID:   order.ID.String(),
					UserID:    order.UserID.String(),
					Type:      order.Type,
					Status:    order.Status,
					Pair:      order.Pair,
					Side:      order.Side,
					Price:     order.Price.String(),
					Quantity:  order.Quantity.String(),
					Timestamp: order.CreatedAt,
					Meta:      map[string]interface{}{},
				},
			})
		}
	}()

	olm.metrics.incrementOrdersSubmitted()
	olm.logger.Info("Order submitted successfully",
		zap.String("order_id", order.ID.String()),
		zap.String("user_id", order.UserID.String()),
		zap.String("trace_id", traceID),
	)

	return order, nil
}

// AcceptOrder accepts an order and transitions it to ACCEPTED state
func (olm *OrderLifecycleManager) AcceptOrder(ctx context.Context, orderID string, reason string) error {
	return olm.db.Transaction(func(tx *gorm.DB) error {
		var order model.Order
		if err := tx.Where("id = ?", orderID).First(&order).Error; err != nil {
			return fmt.Errorf("order not found: %w", err)
		}

		if order.Status != string(OrderStateValidated) {
			return fmt.Errorf("order cannot be accepted from state %s", order.Status)
		}

		return olm.transitionState(ctx, tx, &order, OrderStateValidated, OrderStateAccepted, reason)
	})
}

// FillOrder processes an order fill
func (olm *OrderLifecycleManager) FillOrder(ctx context.Context, orderID string, fillQuantity, fillPrice float64, tradeID string) error {
	return olm.db.Transaction(func(tx *gorm.DB) error {
		var order model.Order
		if err := tx.Where("id = ?", orderID).First(&order).Error; err != nil {
			return fmt.Errorf("order not found: %w", err)
		}

		currentState := OrderState(order.Status)
		if currentState != OrderStateAccepted && currentState != OrderStatePartiallyFilled {
			return fmt.Errorf("order cannot be filled from state %s", order.Status)
		}

		// Update order quantities
		fillQtyDec := decimal.NewFromFloat(fillQuantity)
		filledQuantity := order.FilledQuantity.Add(fillQtyDec)
		order.FilledQuantity = filledQuantity
		order.UpdatedAt = time.Now()

		// Determine new state
		var newState OrderState
		var reason string

		remainingQuantity := order.Quantity.Sub(filledQuantity)
		if remainingQuantity.LessThanOrEqual(decimal.NewFromFloat(0.0000001)) { // Consider floating point precision
			newState = OrderStateFilled
			reason = fmt.Sprintf("Order fully filled. Fill: %f @ %f, Trade: %s", fillQuantity, fillPrice, tradeID)
			olm.metrics.incrementOrdersFilled()
		} else {
			newState = OrderStatePartiallyFilled
			reason = fmt.Sprintf("Order partially filled. Fill: %f @ %f, Remaining: %f, Trade: %s", fillQuantity, fillPrice, remainingQuantity.InexactFloat64(), tradeID)
		}

		// Save order updates
		if err := tx.Save(&order).Error; err != nil {
			return fmt.Errorf("failed to update order: %w", err)
		}

		// Record fill event
		fillEvent := &OrderEvent{
			OrderID:   orderID,
			EventType: "ORDER_FILL",
			EventData: fmt.Sprintf(`{"fill_quantity": %f, "fill_price": %f, "trade_id": "%s", "remaining_quantity": %f}`,
				fillQuantity, fillPrice, tradeID, remainingQuantity.InexactFloat64()),
			UserID:    order.UserID.String(),
			TraceID:   getTraceID(ctx),
			Source:    "fill_processor",
			Timestamp: time.Now(),
		}

		if err := tx.Create(fillEvent).Error; err != nil {
			return fmt.Errorf("failed to record fill event: %w", err)
		}

			OrderStateAccepted, OrderStatePartiallyFilled,
		}

		canCancel := false
		for _, state := range cancelableStates {
			if currentState == state {
				canCancel = true
				break
			}
		}

		if !canCancel {
			return fmt.Errorf("order cannot be canceled from state %s", order.Status)
		}

		// Transition through CANCELING to CANCELED
		if err := olm.transitionState(ctx, tx, &order, currentState, OrderStateCanceling, fmt.Sprintf("Cancel requested: %s", reason)); err != nil {
			return err
		}

		return olm.transitionState(ctx, tx, &order, OrderStateCanceling, OrderStateCanceled, "Cancellation completed")
	})
}

// transitionState handles state transitions with proper validation and event recording
func (olm *OrderLifecycleManager) transitionState(ctx context.Context, tx *gorm.DB, order *model.Order,
	fromState, toState OrderState, reason string) error {

	// Validate state transition
	if !olm.isValidTransition(fromState, toState) {
		return fmt.Errorf("invalid state transition from %s to %s", fromState, toState)
	}

	// Update order state
	order.Status = string(toState)
	order.UpdatedAt = time.Now()

	if err := tx.Save(order).Error; err != nil {
		return fmt.Errorf("failed to update order status: %w", err)
	}

	// Record state transition
	transition := &OrderStateTransition{
		OrderID:   order.ID.String(),
		FromState: string(fromState),
		ToState:   string(toState),
		Reason:    reason,
		UserID:    order.UserID.String(),
		TraceID:   getTraceID(ctx),
		Timestamp: time.Now(),
	}

	if err := tx.Create(transition).Error; err != nil {
		return fmt.Errorf("failed to record state transition: %w", err)
	}

	// Execute state hooks
	olm.executeStateHooks(ctx, order, transition)

	// Update metrics
	olm.metrics.incrementStateTransition(fmt.Sprintf("%s->%s", fromState, toState))

	// Publish transition event asynchronously
	go func() {
		if err := olm.eventBus.PublishStateTransition(context.Background(), transition); err != nil {
			olm.logger.Error("Failed to publish state transition", zap.Error(err))
		}
	}()

	return nil
}

// isValidTransition checks if a state transition is valid
func (olm *OrderLifecycleManager) isValidTransition(from, to OrderState) bool {
	validTransitions := map[OrderState][]OrderState{
		OrderStateSubmitted:       {OrderStatePending, OrderStateRejected},
		OrderStatePending:         {OrderStateValidated, OrderStateRejected},
		OrderStateValidated:       {OrderStateAccepted, OrderStateRejected},
		OrderStateAccepted:        {OrderStatePartiallyFilled, OrderStateFilled, OrderStateCanceling, OrderStateExpired},
		OrderStatePartiallyFilled: {OrderStateFilled, OrderStateCanceling, OrderStateExpired},
		OrderStateCanceling:       {OrderStateCanceled},
		OrderStateRejected:        {}, // Terminal state
		OrderStateFilled:          {}, // Terminal state
		OrderStateCanceled:        {}, // Terminal state
		OrderStateExpired:         {}, // Terminal state
		OrderStateFailed:          {}, // Terminal state
	}

	allowedStates, exists := validTransitions[from]
	if !exists {
		return false
	}

	for _, allowedState := range allowedStates {
		if allowedState == to {
			return true
		}
	}

	return false
}

// validateOrder runs all validators on an order
func (olm *OrderLifecycleManager) validateOrder(ctx context.Context, order *model.Order) error {
	for _, validator := range olm.validators {
		if err := validator.ValidateOrder(ctx, order); err != nil {
			olm.logger.Warn("Order validation failed",
				zap.String("validator", validator.Name()),
				zap.String("order_id", order.ID.String()),
				zap.Error(err),
			)
			return fmt.Errorf("validation failed by %s: %w", validator.Name(), err)
		}
	}
	return nil
}

// executeStateHooks executes all hooks for a state transition
func (olm *OrderLifecycleManager) executeStateHooks(ctx context.Context, order *model.Order, transition *OrderStateTransition) {
	olm.mutex.RLock()
	hooks := olm.hooks[OrderState(transition.ToState)]
	olm.mutex.RUnlock()

	for _, hook := range hooks {
		go func(h StateHook) {
			if err := h.Execute(ctx, order, transition); err != nil {
				olm.logger.Error("State hook execution failed",
					zap.String("hook", h.Name()),
					zap.String("order_id", order.ID.String()),
					zap.String("transition", fmt.Sprintf("%s->%s", transition.FromState, transition.ToState)),
					zap.Error(err),
				)
			}
		}(hook)
	}
}

// checkIdempotency checks if an operation with the given idempotency key already exists
func (olm *OrderLifecycleManager) checkIdempotency(ctx context.Context, key, userID, operation string) (*model.Order, bool) {
	var idempotency IdempotencyKey
	err := olm.db.Where("key = ? AND user_id = ? AND operation = ? AND completed = true", key, userID, operation).
		First(&idempotency).Error

	if err != nil {
		return nil, false
	}

	// Check if the key has expired
	if time.Now().After(idempotency.ExpiresAt) {
		// Clean up expired key
		olm.db.Delete(&idempotency)
		return nil, false
	}

	// Deserialize the response
	if idempotency.OrderID != "" {
		var order model.Order
		if err := olm.db.Where("id = ?", idempotency.OrderID).First(&order).Error; err == nil {
			return &order, true
		}
	}

	return nil, true
}

// updateIdempotencyResult updates the idempotency key with the operation result
func (olm *OrderLifecycleManager) updateIdempotencyResult(key string, order *model.Order) {
	olm.db.Model(&IdempotencyKey{}).
		Where("key = ?", key).
		Updates(map[string]interface{}{
			"order_id":  order.ID,
			"response":  olm.serializeOrder(order),
			"completed": true,
		})
}

// serializeOrder serializes an order to JSON string
func (olm *OrderLifecycleManager) serializeOrder(order *model.Order) string {
	// Implement JSON serialization
	// This is a simplified version
	return fmt.Sprintf(`{"id": "%s", "user_id": "%s", "pair": "%s", "status": "%s"}`,
		order.ID, order.UserID, order.Pair, order.Status)
}

// getTraceID extracts or generates a trace ID from context
func getTraceID(ctx context.Context) string {
	if ctx == nil {
		return uuid.New().String()
	}
	if traceID, ok := ctx.Value("trace_id").(string); ok && traceID != "" {
		return traceID
	}
	return uuid.New().String()
}

// Metrics methods
func (lm *LifecycleMetrics) incrementOrdersSubmitted() {
	lm.mutex.Lock()
	defer lm.mutex.Unlock()
	lm.OrdersSubmitted++
}

func (lm *LifecycleMetrics) incrementOrdersAccepted() {
	lm.mutex.Lock()
	defer lm.mutex.Unlock()
	lm.OrdersAccepted++
}

func (lm *LifecycleMetrics) incrementOrdersRejected() {
	lm.mutex.Lock()
	defer lm.mutex.Unlock()
	lm.OrdersRejected++
}

func (lm *LifecycleMetrics) incrementOrdersFilled() {
	lm.mutex.Lock()
	defer lm.mutex.Unlock()
	lm.OrdersFilled++
}

func (lm *LifecycleMetrics) incrementOrdersCanceled() {
	lm.mutex.Lock()
	defer lm.mutex.Unlock()
	lm.OrdersCanceled++
}

func (lm *LifecycleMetrics) incrementStateTransition(transition string) {
	lm.mutex.Lock()
	defer lm.mutex.Unlock()
	lm.StateTransitions[transition]++
}

func (lm *LifecycleMetrics) incrementValidationFailures() {
	lm.mutex.Lock()
	defer lm.mutex.Unlock()
	lm.ValidationFailures++
}

func (lm *LifecycleMetrics) incrementIdempotencyHits() {
	lm.mutex.Lock()
	defer lm.mutex.Unlock()
	lm.IdempotencyHits++
}

// GetMetrics returns a copy of the current metrics
func (olm *OrderLifecycleManager) GetMetrics() *LifecycleMetrics {
	olm.metrics.mutex.RLock()
	defer olm.metrics.mutex.RUnlock()

	// Return a copy to avoid race conditions
	return &LifecycleMetrics{
		OrdersSubmitted:    olm.metrics.OrdersSubmitted,
		OrdersAccepted:     olm.metrics.OrdersAccepted,
		OrdersRejected:     olm.metrics.OrdersRejected,
		OrdersFilled:       olm.metrics.OrdersFilled,
		OrdersCanceled:     olm.metrics.OrdersCanceled,
		StateTransitions:   make(map[string]int64),
		ValidationFailures: olm.metrics.ValidationFailures,
		IdempotencyHits:    olm.metrics.IdempotencyHits,
	}
}
