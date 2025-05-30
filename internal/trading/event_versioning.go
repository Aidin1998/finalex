package trading

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// EventMigrationManager handles event schema migrations and compatibility
type EventMigrationManager struct {
	upgraders map[string]map[int]EventUpgrader // eventType -> version -> upgrader
	logger    *zap.SugaredLogger
}

// NewEventMigrationManager creates a new migration manager
func NewEventMigrationManager(logger *zap.SugaredLogger) *EventMigrationManager {
	return &EventMigrationManager{
		upgraders: make(map[string]map[int]EventUpgrader),
		logger:    logger,
	}
}

// RegisterUpgrader registers an upgrader for a specific event type and version
func (emm *EventMigrationManager) RegisterUpgrader(eventType string, fromVersion int, upgrader EventUpgrader) {
	if emm.upgraders[eventType] == nil {
		emm.upgraders[eventType] = make(map[int]EventUpgrader)
	}
	emm.upgraders[eventType][fromVersion] = upgrader
	emm.logger.Infow("Registered event upgrader",
		"eventType", eventType,
		"fromVersion", fromVersion)
}

// MigrateEvent migrates an event to the latest version
func (emm *EventMigrationManager) MigrateEvent(event *VersionedEvent, targetVersion int) (*VersionedEvent, error) {
	if event.Version >= targetVersion {
		return event, nil // Already at target version or newer
	}

	currentEvent := event
	for currentEvent.Version < targetVersion {
		upgrader, exists := emm.upgraders[event.EventType][currentEvent.Version]
		if !exists {
			return nil, fmt.Errorf("no upgrader found for event type %s version %d",
				event.EventType, currentEvent.Version)
		}

		// Convert to OrderBookEvent for upgrader
		orderBookEvent := &OrderBookEvent{
			Version:   currentEvent.Version,
			Type:      currentEvent.EventType,
			Timestamp: currentEvent.Timestamp.Unix(),
			Payload:   json.RawMessage(mustMarshal(currentEvent.Data)),
		}

		upgradedEvent, err := upgrader(orderBookEvent)
		if err != nil {
			return nil, fmt.Errorf("failed to upgrade event %s from version %d: %w",
				event.EventType, currentEvent.Version, err)
		}

		// Convert back to VersionedEvent
		var data interface{}
		if err := json.Unmarshal(upgradedEvent.Payload, &data); err != nil {
			return nil, fmt.Errorf("failed to unmarshal upgraded event data: %w", err)
		}

		currentEvent = &VersionedEvent{
			EventID:   event.EventID,
			EventType: upgradedEvent.Type,
			Version:   upgradedEvent.Version,
			Timestamp: time.Unix(upgradedEvent.Timestamp, 0),
			Data:      data,
		}

		emm.logger.Debugw("Event migrated",
			"eventID", event.EventID,
			"eventType", event.EventType,
			"fromVersion", event.Version,
			"toVersion", currentEvent.Version)
	}

	return currentEvent, nil
}

// ValidateEventCompatibility checks if an event can be migrated to target version
func (emm *EventMigrationManager) ValidateEventCompatibility(eventType string, fromVersion, toVersion int) error {
	if fromVersion >= toVersion {
		return nil // No migration needed
	}

	// Check that all intermediate upgraders exist
	for version := fromVersion; version < toVersion; version++ {
		if _, exists := emm.upgraders[eventType][version]; !exists {
			return fmt.Errorf("missing upgrader for event type %s version %d", eventType, version)
		}
	}

	return nil
}

// GetSupportedVersions returns all supported versions for an event type
func (emm *EventMigrationManager) GetSupportedVersions(eventType string) []int {
	versions := make([]int, 0)
	if upgraders, exists := emm.upgraders[eventType]; exists {
		for version := range upgraders {
			versions = append(versions, version)
		}
	}
	return versions
}

// EventSchemaRegistry manages event schemas and validation
type EventSchemaRegistry struct {
	schemas map[string]map[int]*EventSchema // eventType -> version -> schema
	logger  *zap.SugaredLogger
}

// EventSchema defines the structure and validation rules for an event
type EventSchema struct {
	EventType   string                 `json:"event_type"`
	Version     int                    `json:"version"`
	Fields      map[string]FieldSchema `json:"fields"`
	Required    []string               `json:"required"`
	Description string                 `json:"description"`
}

// FieldSchema defines validation rules for a field
type FieldSchema struct {
	Type       string                 `json:"type"`   // "string", "int", "float", "bool", "object", "array"
	Format     string                 `json:"format"` // "uuid", "email", "date-time", etc.
	MinLength  *int                   `json:"min_length,omitempty"`
	MaxLength  *int                   `json:"max_length,omitempty"`
	Minimum    *float64               `json:"minimum,omitempty"`
	Maximum    *float64               `json:"maximum,omitempty"`
	Pattern    string                 `json:"pattern,omitempty"`
	Enum       []string               `json:"enum,omitempty"`
	Items      *FieldSchema           `json:"items,omitempty"`      // For array types
	Properties map[string]FieldSchema `json:"properties,omitempty"` // For object types
}

// NewEventSchemaRegistry creates a new schema registry
func NewEventSchemaRegistry(logger *zap.SugaredLogger) *EventSchemaRegistry {
	return &EventSchemaRegistry{
		schemas: make(map[string]map[int]*EventSchema),
		logger:  logger,
	}
}

// RegisterSchema registers a schema for an event type and version
func (esr *EventSchemaRegistry) RegisterSchema(schema *EventSchema) {
	if esr.schemas[schema.EventType] == nil {
		esr.schemas[schema.EventType] = make(map[int]*EventSchema)
	}
	esr.schemas[schema.EventType][schema.Version] = schema
	esr.logger.Infow("Registered event schema",
		"eventType", schema.EventType,
		"version", schema.Version)
}

// ValidateEvent validates an event against its schema
func (esr *EventSchemaRegistry) ValidateEvent(event *VersionedEvent) error {
	schema, exists := esr.schemas[event.EventType][event.Version]
	if !exists {
		return fmt.Errorf("no schema found for event type %s version %d",
			event.EventType, event.Version)
	}

	return esr.validateEventData(event.Data, schema)
}

// validateEventData validates event data against schema
func (esr *EventSchemaRegistry) validateEventData(data interface{}, schema *EventSchema) error {
	dataMap, ok := data.(map[string]interface{})
	if !ok {
		return fmt.Errorf("event data must be an object")
	}

	// Check required fields
	for _, required := range schema.Required {
		if _, exists := dataMap[required]; !exists {
			return fmt.Errorf("required field %s is missing", required)
		}
	}

	// Validate each field
	for fieldName, fieldValue := range dataMap {
		fieldSchema, exists := schema.Fields[fieldName]
		if !exists {
			esr.logger.Warnw("Unknown field in event data",
				"field", fieldName,
				"eventType", schema.EventType,
				"version", schema.Version)
			continue
		}

		if err := esr.validateField(fieldValue, &fieldSchema, fieldName); err != nil {
			return fmt.Errorf("field %s validation failed: %w", fieldName, err)
		}
	}

	return nil
}

// validateField validates a single field against its schema
func (esr *EventSchemaRegistry) validateField(value interface{}, schema *FieldSchema, fieldName string) error {
	if value == nil {
		return nil // Allow nil values, required check is done separately
	}

	switch schema.Type {
	case "string":
		str, ok := value.(string)
		if !ok {
			return fmt.Errorf("expected string, got %T", value)
		}
		return esr.validateStringField(str, schema)
	case "int":
		// Handle both int and float64 (JSON numbers)
		var intVal int64
		switch v := value.(type) {
		case int64:
			intVal = v
		case float64:
			intVal = int64(v)
		case int:
			intVal = int64(v)
		default:
			return fmt.Errorf("expected int, got %T", value)
		}
		return esr.validateNumberField(float64(intVal), schema)
	case "float":
		var floatVal float64
		switch v := value.(type) {
		case float64:
			floatVal = v
		case int64:
			floatVal = float64(v)
		case int:
			floatVal = float64(v)
		default:
			return fmt.Errorf("expected float, got %T", value)
		}
		return esr.validateNumberField(floatVal, schema)
	case "bool":
		_, ok := value.(bool)
		if !ok {
			return fmt.Errorf("expected bool, got %T", value)
		}
	case "array":
		arr, ok := value.([]interface{})
		if !ok {
			return fmt.Errorf("expected array, got %T", value)
		}
		if schema.Items != nil {
			for i, item := range arr {
				if err := esr.validateField(item, schema.Items, fmt.Sprintf("%s[%d]", fieldName, i)); err != nil {
					return err
				}
			}
		}
	case "object":
		obj, ok := value.(map[string]interface{})
		if !ok {
			return fmt.Errorf("expected object, got %T", value)
		}
		if schema.Properties != nil {
			for propName, propValue := range obj {
				if propSchema, exists := schema.Properties[propName]; exists {
					if err := esr.validateField(propValue, &propSchema, fmt.Sprintf("%s.%s", fieldName, propName)); err != nil {
						return err
					}
				}
			}
		}
	default:
		return fmt.Errorf("unsupported field type: %s", schema.Type)
	}

	return nil
}

// validateStringField validates string-specific constraints
func (esr *EventSchemaRegistry) validateStringField(value string, schema *FieldSchema) error {
	if schema.MinLength != nil && len(value) < *schema.MinLength {
		return fmt.Errorf("string too short: %d < %d", len(value), *schema.MinLength)
	}
	if schema.MaxLength != nil && len(value) > *schema.MaxLength {
		return fmt.Errorf("string too long: %d > %d", len(value), *schema.MaxLength)
	}

	if len(schema.Enum) > 0 {
		for _, enum := range schema.Enum {
			if value == enum {
				return nil
			}
		}
		return fmt.Errorf("value %s not in enum %v", value, schema.Enum)
	}

	// Format validation
	switch schema.Format {
	case "uuid":
		if _, err := uuid.Parse(value); err != nil {
			return fmt.Errorf("invalid UUID format: %s", value)
		}
	case "email":
		// Basic email validation
		if !isValidEmail(value) {
			return fmt.Errorf("invalid email format: %s", value)
		}
	}

	return nil
}

// validateNumberField validates number-specific constraints
func (esr *EventSchemaRegistry) validateNumberField(value float64, schema *FieldSchema) error {
	if schema.Minimum != nil && value < *schema.Minimum {
		return fmt.Errorf("number too small: %f < %f", value, *schema.Minimum)
	}
	if schema.Maximum != nil && value > *schema.Maximum {
		return fmt.Errorf("number too large: %f > %f", value, *schema.Maximum)
	}
	return nil
}

// Helper functions
func mustMarshal(v interface{}) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal: %v", err))
	}
	return data
}

func isValidEmail(email string) bool {
	// Very basic email validation - in production use a proper library
	return len(email) > 3 &&
		len(email) < 255 &&
		containsChar(email, '@') &&
		containsChar(email, '.')
}

func containsChar(s string, c rune) bool {
	for _, char := range s {
		if char == c {
			return true
		}
	}
	return false
}

// EventVersionManager manages event versioning across the platform
type EventVersionManager struct {
	migrationManager *EventMigrationManager
	schemaRegistry   *EventSchemaRegistry
	logger           *zap.SugaredLogger
}

// NewEventVersionManager creates a new version manager
func NewEventVersionManager(logger *zap.SugaredLogger) *EventVersionManager {
	return &EventVersionManager{
		migrationManager: NewEventMigrationManager(logger),
		schemaRegistry:   NewEventSchemaRegistry(logger),
		logger:           logger,
	}
}

// ProcessEvent processes an event with validation and migration
func (evm *EventVersionManager) ProcessEvent(event *VersionedEvent, targetVersion int) (*VersionedEvent, error) {
	// First validate the incoming event
	if err := evm.schemaRegistry.ValidateEvent(event); err != nil {
		return nil, fmt.Errorf("event validation failed: %w", err)
	}

	// Migrate if necessary
	if event.Version < targetVersion {
		migratedEvent, err := evm.migrationManager.MigrateEvent(event, targetVersion)
		if err != nil {
			return nil, fmt.Errorf("event migration failed: %w", err)
		}

		// Validate migrated event
		if err := evm.schemaRegistry.ValidateEvent(migratedEvent); err != nil {
			return nil, fmt.Errorf("migrated event validation failed: %w", err)
		}

		return migratedEvent, nil
	}

	return event, nil
}

// RegisterEventType registers schemas and upgraders for an event type
func (evm *EventVersionManager) RegisterEventType(eventType string, schemas []*EventSchema, upgraders map[int]EventUpgrader) error {
	// Register schemas
	for _, schema := range schemas {
		if schema.EventType != eventType {
			return fmt.Errorf("schema event type %s does not match %s", schema.EventType, eventType)
		}
		evm.schemaRegistry.RegisterSchema(schema)
	}

	// Register upgraders
	for version, upgrader := range upgraders {
		evm.migrationManager.RegisterUpgrader(eventType, version, upgrader)
	}

	evm.logger.Infow("Registered event type",
		"eventType", eventType,
		"schemas", len(schemas),
		"upgraders", len(upgraders))

	return nil
}

// GetMigrationManager returns the migration manager
func (evm *EventVersionManager) GetMigrationManager() *EventMigrationManager {
	return evm.migrationManager
}

// GetSchemaRegistry returns the schema registry
func (evm *EventVersionManager) GetSchemaRegistry() *EventSchemaRegistry {
	return evm.schemaRegistry
}
