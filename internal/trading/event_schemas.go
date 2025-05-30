package trading

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/trading/model"
	"github.com/google/uuid"
)

// RegisterDefaultEventTypes registers all default event types with their schemas and upgraders
func RegisterDefaultEventTypes(versionManager *EventVersionManager) error {
	// Register ORDER_PLACED event type
	if err := registerOrderPlacedEventType(versionManager); err != nil {
		return fmt.Errorf("failed to register ORDER_PLACED event type: %w", err)
	}

	// Register ORDER_CANCELLED event type
	if err := registerOrderCancelledEventType(versionManager); err != nil {
		return fmt.Errorf("failed to register ORDER_CANCELLED event type: %w", err)
	}

	// Register TRADE_EXECUTED event type
	if err := registerTradeExecutedEventType(versionManager); err != nil {
		return fmt.Errorf("failed to register TRADE_EXECUTED event type: %w", err)
	}

	// Register CHECKPOINT event type
	if err := registerCheckpointEventType(versionManager); err != nil {
		return fmt.Errorf("failed to register CHECKPOINT event type: %w", err)
	}

	// Register financial operation event types
	if err := registerFinancialEventTypes(versionManager); err != nil {
		return fmt.Errorf("failed to register financial event types: %w", err)
	}

	return nil
}

// registerOrderPlacedEventType registers ORDER_PLACED event schemas and upgraders
func registerOrderPlacedEventType(versionManager *EventVersionManager) error {
	eventType := EventTypeOrderPlaced

	// Schema v1
	schemaV1 := &EventSchema{
		EventType:   eventType,
		Version:     1,
		Description: "Order placed event version 1",
		Required:    []string{"order", "timestamp"},
		Fields: map[string]FieldSchema{
			"order": {
				Type: "object",
				Properties: map[string]FieldSchema{
					"id":       {Type: "string", Format: "uuid"},
					"user_id":  {Type: "string", Format: "uuid"},
					"pair":     {Type: "string", MinLength: intPtr(3), MaxLength: intPtr(10)},
					"side":     {Type: "string", Enum: []string{"buy", "sell"}},
					"type":     {Type: "string", Enum: []string{"limit", "market", "fok", "stop_limit"}},
					"quantity": {Type: "string"}, // Decimal as string
					"price":    {Type: "string"}, // Decimal as string
					"status":   {Type: "string", Enum: []string{"new", "open", "filled", "partially_filled", "cancelled", "rejected"}},
				},
			},
			"timestamp": {Type: "string", Format: "date-time"},
			"metadata": {
				Type: "object",
				Properties: map[string]FieldSchema{
					"source":    {Type: "string"},
					"client_ip": {Type: "string"},
				},
			},
		},
	}

	// Schema v2 (example future version with additional fields)
	schemaV2 := &EventSchema{
		EventType:   eventType,
		Version:     2,
		Description: "Order placed event version 2 - added risk metrics",
		Required:    []string{"order", "timestamp", "risk_assessment"},
		Fields: map[string]FieldSchema{
			"order":     schemaV1.Fields["order"], // Same as v1
			"timestamp": schemaV1.Fields["timestamp"],
			"metadata":  schemaV1.Fields["metadata"],
			"risk_assessment": {
				Type: "object",
				Properties: map[string]FieldSchema{
					"risk_score": {Type: "float", Minimum: floatPtr(0), Maximum: floatPtr(100)},
					"checks":     {Type: "array", Items: &FieldSchema{Type: "string"}},
				},
			},
		},
	}

	// Upgrader from v1 to v2
	upgraderV1ToV2 := func(event *OrderBookEvent) (*OrderBookEvent, error) {
		var v1Data map[string]interface{}
		if err := json.Unmarshal(event.Payload, &v1Data); err != nil {
			return nil, fmt.Errorf("failed to unmarshal v1 data: %w", err)
		}

		// Add default risk assessment
		v1Data["risk_assessment"] = map[string]interface{}{
			"risk_score": 50.0, // Default medium risk
			"checks":     []string{"basic_validation"},
		}

		newPayload, err := json.Marshal(v1Data)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal v2 data: %w", err)
		}

		return &OrderBookEvent{
			Version:   2,
			Type:      event.Type,
			Timestamp: event.Timestamp,
			Payload:   newPayload,
		}, nil
	}

	schemas := []*EventSchema{schemaV1, schemaV2}
	upgraders := map[int]EventUpgrader{
		1: upgraderV1ToV2,
	}

	return versionManager.RegisterEventType(eventType, schemas, upgraders)
}

// registerOrderCancelledEventType registers ORDER_CANCELLED event schemas and upgraders
func registerOrderCancelledEventType(versionManager *EventVersionManager) error {
	eventType := EventTypeOrderCancelled

	schemaV1 := &EventSchema{
		EventType:   eventType,
		Version:     1,
		Description: "Order cancelled event version 1",
		Required:    []string{"order_id", "pair", "timestamp", "reason"},
		Fields: map[string]FieldSchema{
			"order_id":  {Type: "string", Format: "uuid"},
			"pair":      {Type: "string", MinLength: intPtr(3), MaxLength: intPtr(10)},
			"timestamp": {Type: "string", Format: "date-time"},
			"reason":    {Type: "string", Enum: []string{"user_request", "system_cancel", "risk_management", "expired"}},
			"metadata": {
				Type: "object",
				Properties: map[string]FieldSchema{
					"cancelled_by":      {Type: "string"},
					"original_quantity": {Type: "string"},
					"filled_quantity":   {Type: "string"},
				},
			},
		},
	}

	schemas := []*EventSchema{schemaV1}
	upgraders := map[int]EventUpgrader{}

	return versionManager.RegisterEventType(eventType, schemas, upgraders)
}

// registerTradeExecutedEventType registers TRADE_EXECUTED event schemas and upgraders
func registerTradeExecutedEventType(versionManager *EventVersionManager) error {
	eventType := EventTypeTradeExecuted

	schemaV1 := &EventSchema{
		EventType:   eventType,
		Version:     1,
		Description: "Trade executed event version 1",
		Required:    []string{"trade", "timestamp"},
		Fields: map[string]FieldSchema{
			"trade": {
				Type: "object",
				Properties: map[string]FieldSchema{
					"id":             {Type: "string", Format: "uuid"},
					"maker_order_id": {Type: "string", Format: "uuid"},
					"taker_order_id": {Type: "string", Format: "uuid"},
					"pair":           {Type: "string"},
					"side":           {Type: "string", Enum: []string{"buy", "sell"}},
					"quantity":       {Type: "string"}, // Decimal as string
					"price":          {Type: "string"}, // Decimal as string
					"maker_fee":      {Type: "string"}, // Decimal as string
					"taker_fee":      {Type: "string"}, // Decimal as string
				},
			},
			"timestamp": {Type: "string", Format: "date-time"},
			"settlement": {
				Type: "object",
				Properties: map[string]FieldSchema{
					"required": {Type: "bool"},
					"status":   {Type: "string", Enum: []string{"pending", "clearing", "settled", "failed"}},
				},
			},
		},
	}

	schemas := []*EventSchema{schemaV1}
	upgraders := map[int]EventUpgrader{}

	return versionManager.RegisterEventType(eventType, schemas, upgraders)
}

// registerCheckpointEventType registers CHECKPOINT event schemas and upgraders
func registerCheckpointEventType(versionManager *EventVersionManager) error {
	eventType := EventTypeCheckpoint

	schemaV1 := &EventSchema{
		EventType:   eventType,
		Version:     1,
		Description: "Checkpoint event version 1",
		Required:    []string{"checkpoint_id", "timestamp", "state_summary"},
		Fields: map[string]FieldSchema{
			"checkpoint_id": {Type: "string"},
			"timestamp":     {Type: "string", Format: "date-time"},
			"state_summary": {
				Type: "object",
				Properties: map[string]FieldSchema{
					"total_orders":  {Type: "int", Minimum: floatPtr(0)},
					"total_trades":  {Type: "int", Minimum: floatPtr(0)},
					"active_pairs":  {Type: "array", Items: &FieldSchema{Type: "string"}},
					"last_sequence": {Type: "int", Minimum: floatPtr(0)},
				},
			},
			"metadata": {
				Type: "object",
				Properties: map[string]FieldSchema{
					"engine_instance": {Type: "string"},
					"memory_usage":    {Type: "int"},
					"uptime_seconds":  {Type: "int"},
				},
			},
		},
	}

	schemas := []*EventSchema{schemaV1}
	upgraders := map[int]EventUpgrader{}

	return versionManager.RegisterEventType(eventType, schemas, upgraders)
}

// Financial operation event types
const (
	EventTypeFundsTransfer     = "FUNDS_TRANSFER"
	EventTypeFundsLocked       = "FUNDS_LOCKED"
	EventTypeFundsUnlocked     = "FUNDS_UNLOCKED"
	EventTypeAccountCreated    = "ACCOUNT_CREATED"
	EventTypeBalanceUpdate     = "BALANCE_UPDATE"
	EventTypeSettlementCleared = "SETTLEMENT_CLEARED"
	EventTypePositionNetted    = "POSITION_NETTED"
)

// registerFinancialEventTypes registers all financial operation event types
func registerFinancialEventTypes(versionManager *EventVersionManager) error {
	// FUNDS_TRANSFER
	fundsTransferSchema := &EventSchema{
		EventType:   EventTypeFundsTransfer,
		Version:     1,
		Description: "Funds transfer event",
		Required:    []string{"transaction_id", "from_user_id", "to_user_id", "currency", "amount", "timestamp"},
		Fields: map[string]FieldSchema{
			"transaction_id": {Type: "string", Format: "uuid"},
			"from_user_id":   {Type: "string", Format: "uuid"},
			"to_user_id":     {Type: "string", Format: "uuid"},
			"currency":       {Type: "string", MinLength: intPtr(3), MaxLength: intPtr(10)},
			"amount":         {Type: "string"}, // Decimal as string
			"fee":            {Type: "string"}, // Decimal as string
			"timestamp":      {Type: "string", Format: "date-time"},
			"reason":         {Type: "string"},
			"metadata": {
				Type: "object",
				Properties: map[string]FieldSchema{
					"transfer_type": {Type: "string", Enum: []string{"internal", "external", "settlement"}},
					"reference":     {Type: "string"},
				},
			},
		},
	}

	// FUNDS_LOCKED
	fundsLockedSchema := &EventSchema{
		EventType:   EventTypeFundsLocked,
		Version:     1,
		Description: "Funds locked event",
		Required:    []string{"user_id", "currency", "amount", "lock_id", "timestamp"},
		Fields: map[string]FieldSchema{
			"user_id":    {Type: "string", Format: "uuid"},
			"currency":   {Type: "string", MinLength: intPtr(3), MaxLength: intPtr(10)},
			"amount":     {Type: "string"}, // Decimal as string
			"lock_id":    {Type: "string", Format: "uuid"},
			"timestamp":  {Type: "string", Format: "date-time"},
			"reason":     {Type: "string", Enum: []string{"order_placement", "settlement", "withdrawal", "compliance"}},
			"expires_at": {Type: "string", Format: "date-time"},
		},
	}

	// FUNDS_UNLOCKED
	fundsUnlockedSchema := &EventSchema{
		EventType:   EventTypeFundsUnlocked,
		Version:     1,
		Description: "Funds unlocked event",
		Required:    []string{"user_id", "currency", "amount", "lock_id", "timestamp"},
		Fields: map[string]FieldSchema{
			"user_id":   {Type: "string", Format: "uuid"},
			"currency":  {Type: "string", MinLength: intPtr(3), MaxLength: intPtr(10)},
			"amount":    {Type: "string"}, // Decimal as string
			"lock_id":   {Type: "string", Format: "uuid"},
			"timestamp": {Type: "string", Format: "date-time"},
			"reason":    {Type: "string", Enum: []string{"order_completion", "order_cancellation", "settlement_completion", "expiration"}},
		},
	}

	// ACCOUNT_CREATED
	accountCreatedSchema := &EventSchema{
		EventType:   EventTypeAccountCreated,
		Version:     1,
		Description: "Account created event",
		Required:    []string{"user_id", "currency", "timestamp"},
		Fields: map[string]FieldSchema{
			"user_id":         {Type: "string", Format: "uuid"},
			"currency":        {Type: "string", MinLength: intPtr(3), MaxLength: intPtr(10)},
			"timestamp":       {Type: "string", Format: "date-time"},
			"initial_balance": {Type: "string"}, // Decimal as string
			"account_type":    {Type: "string", Enum: []string{"trading", "settlement", "fee", "cold_storage"}},
		},
	}

	// BALANCE_UPDATE
	balanceUpdateSchema := &EventSchema{
		EventType:   EventTypeBalanceUpdate,
		Version:     1,
		Description: "Balance update event",
		Required:    []string{"user_id", "currency", "old_balance", "new_balance", "timestamp", "change_type"},
		Fields: map[string]FieldSchema{
			"user_id":       {Type: "string", Format: "uuid"},
			"currency":      {Type: "string", MinLength: intPtr(3), MaxLength: intPtr(10)},
			"old_balance":   {Type: "string"}, // Decimal as string
			"new_balance":   {Type: "string"}, // Decimal as string
			"change_amount": {Type: "string"}, // Decimal as string
			"timestamp":     {Type: "string", Format: "date-time"},
			"change_type":   {Type: "string", Enum: []string{"deposit", "withdrawal", "trade", "fee", "settlement", "transfer"}},
			"reference_id":  {Type: "string"},
		},
	}

	// Register all financial event types
	eventTypes := map[string]*EventSchema{
		EventTypeFundsTransfer:  fundsTransferSchema,
		EventTypeFundsLocked:    fundsLockedSchema,
		EventTypeFundsUnlocked:  fundsUnlockedSchema,
		EventTypeAccountCreated: accountCreatedSchema,
		EventTypeBalanceUpdate:  balanceUpdateSchema,
	}

	for eventType, schema := range eventTypes {
		if err := versionManager.RegisterEventType(eventType, []*EventSchema{schema}, map[int]EventUpgrader{}); err != nil {
			return fmt.Errorf("failed to register %s: %w", eventType, err)
		}
	}

	return nil
}

// Helper functions for schema definition
func intPtr(i int) *int {
	return &i
}

func floatPtr(f float64) *float64 {
	return &f
}

// CreateOrderPlacedEvent creates a properly structured ORDER_PLACED event
func CreateOrderPlacedEvent(order *model.Order, metadata map[string]interface{}) *VersionedEvent {
	return &VersionedEvent{
		EventID:   uuid.New().String(),
		EventType: EventTypeOrderPlaced,
		Version:   1, // Current version
		Timestamp: order.CreatedAt,
		Data: map[string]interface{}{
			"order": map[string]interface{}{
				"id":       order.ID.String(),
				"user_id":  order.UserID.String(),
				"pair":     order.Pair,
				"side":     order.Side,
				"type":     order.Type,
				"quantity": order.Quantity.String(),
				"price":    order.Price.String(),
				"status":   order.Status,
			},
			"timestamp": order.CreatedAt.Format("2006-01-02T15:04:05.000Z"),
			"metadata":  metadata,
		},
	}
}

// CreateOrderCancelledEvent creates a properly structured ORDER_CANCELLED event
func CreateOrderCancelledEvent(orderID uuid.UUID, pair, reason string, metadata map[string]interface{}) *VersionedEvent {
	return &VersionedEvent{
		EventID:   uuid.New().String(),
		EventType: EventTypeOrderCancelled,
		Version:   1,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"order_id":  orderID.String(),
			"pair":      pair,
			"timestamp": time.Now().Format("2006-01-02T15:04:05.000Z"),
			"reason":    reason,
			"metadata":  metadata,
		},
	}
}

// CreateTradeExecutedEvent creates a properly structured TRADE_EXECUTED event
func CreateTradeExecutedEvent(trade *model.Trade, settlementRequired bool) *VersionedEvent {
	return &VersionedEvent{
		EventID:   uuid.New().String(),
		EventType: EventTypeTradeExecuted,
		Version:   1,
		Timestamp: trade.CreatedAt,
		Data: map[string]interface{}{
			"trade": map[string]interface{}{
				"id":             trade.ID.String(),
				"maker_order_id": trade.OrderID.String(), // Note: model.Trade might need MakerOrderID and TakerOrderID
				"taker_order_id": trade.OrderID.String(),
				"pair":           trade.Pair,
				"side":           trade.Side,
				"quantity":       trade.Quantity.String(),
				"price":          trade.Price.String(),
				"maker_fee":      "0", // TODO: Add fee fields to model.Trade
				"taker_fee":      "0",
			},
			"timestamp": trade.CreatedAt.Format("2006-01-02T15:04:05.000Z"),
			"settlement": map[string]interface{}{
				"required": settlementRequired,
				"status":   "pending",
			},
		},
	}
}

// CreateFundsTransferEvent creates a properly structured FUNDS_TRANSFER event
func CreateFundsTransferEvent(transactionID, fromUserID, toUserID uuid.UUID, currency, amount, fee, reason string, metadata map[string]interface{}) *VersionedEvent {
	return &VersionedEvent{
		EventID:   uuid.New().String(),
		EventType: EventTypeFundsTransfer,
		Version:   1,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"transaction_id": transactionID.String(),
			"from_user_id":   fromUserID.String(),
			"to_user_id":     toUserID.String(),
			"currency":       currency,
			"amount":         amount,
			"fee":            fee,
			"timestamp":      time.Now().Format("2006-01-02T15:04:05.000Z"),
			"reason":         reason,
			"metadata":       metadata,
		},
	}
}

// CreateBalanceUpdateEvent creates a properly structured BALANCE_UPDATE event
func CreateBalanceUpdateEvent(userID uuid.UUID, currency, oldBalance, newBalance, changeAmount, changeType, referenceID string) *VersionedEvent {
	return &VersionedEvent{
		EventID:   uuid.New().String(),
		EventType: EventTypeBalanceUpdate,
		Version:   1,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"user_id":       userID.String(),
			"currency":      currency,
			"old_balance":   oldBalance,
			"new_balance":   newBalance,
			"change_amount": changeAmount,
			"timestamp":     time.Now().Format("2006-01-02T15:04:05.000Z"),
			"change_type":   changeType,
			"reference_id":  referenceID,
		},
	}
}
