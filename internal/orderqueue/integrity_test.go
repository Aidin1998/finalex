package orderqueue

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// DataIntegrityTest verifies that no data is lost or corrupted during various operations
type DataIntegrityTest struct {
	Name            string
	OrderCount      int
	ConcurrentOps   int
	ValidationLevel ValidationLevel
}

type ValidationLevel int

const (
	BasicValidation ValidationLevel = iota
	ChecksumValidation
	FullContentValidation
)

// OrderChecksum represents a cryptographic checksum of an order
type OrderChecksum struct {
	OrderID   string
	Checksum  string
	Timestamp time.Time
}

func TestDataIntegrity_BasicConsistency(t *testing.T) {
	test := DataIntegrityTest{
		Name:            "Basic Consistency",
		OrderCount:      1000,
		ConcurrentOps:   5,
		ValidationLevel: BasicValidation,
	}
	
	runDataIntegrityTest(t, test)
}

func TestDataIntegrity_ChecksumValidation(t *testing.T) {
	test := DataIntegrityTest{
		Name:            "Checksum Validation", 
		OrderCount:      5000,
		ConcurrentOps:   10,
		ValidationLevel: ChecksumValidation,
	}
	
	runDataIntegrityTest(t, test)
}

func TestDataIntegrity_FullContentValidation(t *testing.T) {
	test := DataIntegrityTest{
		Name:            "Full Content Validation",
		OrderCount:      2000,
		ConcurrentOps:   8,
		ValidationLevel: FullContentValidation,
	}
	
	runDataIntegrityTest(t, test)
}

func runDataIntegrityTest(t *testing.T, test DataIntegrityTest) {
	tempDir := t.TempDir()
	
	// Phase 1: Generate reference dataset with checksums
	referenceOrders := generateReferenceOrders(test.OrderCount)
	referenceChecksums := calculateOrderChecksums(referenceOrders)
	
	t.Logf("Generated %d reference orders with checksums", len(referenceOrders))
	
	// Phase 2: Store orders in queue with concurrent operations
	queue, err := NewBadgerQueue(tempDir)
	require.NoError(t, err)
	
	// Concurrent enqueue operations
	var wg sync.WaitGroup
	orderChan := make(chan *Order, len(referenceOrders))
	
	// Fill channel with orders
	for _, order := range referenceOrders {
		orderChan <- order
	}
	close(orderChan)
	
	// Start concurrent workers
	enqueueErrors := make(chan error, test.ConcurrentOps)
	for i := 0; i < test.ConcurrentOps; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for order := range orderChan {
				if err := queue.Enqueue(order); err != nil {
					enqueueErrors <- fmt.Errorf("worker %d: %w", workerID, err)
				}
			}
		}(i)
	}
	
	wg.Wait()
	close(enqueueErrors)
	
	// Check for enqueue errors
	for err := range enqueueErrors {
		t.Errorf("Enqueue error: %v", err)
	}
	
	// Phase 3: Randomly process some orders
	processedOrders := make(map[string]*Order)
	pendingOrderIDs := make(map[string]bool)
	
	// Dequeue and randomly acknowledge orders
	for i := 0; i < test.OrderCount; i++ {
		order, err := queue.Dequeue()
		require.NoError(t, err)
		
		if order == nil {
			break // No more orders
		}
		
		pendingOrderIDs[order.ID] = true
		
		// Randomly acknowledge orders (80% chance)
		if rand.Float32() < 0.8 {
			err = queue.Acknowledge(order.ID)
			require.NoError(t, err)
			processedOrders[order.ID] = order
			delete(pendingOrderIDs, order.ID)
		}
	}
	
	t.Logf("Processed %d orders, %d pending", len(processedOrders), len(pendingOrderIDs))
	
	// Phase 4: Simulate shutdown and recovery
	queue.Shutdown()
	
	recoveredQueue, err := NewBadgerQueue(tempDir)
	require.NoError(t, err)
	defer recoveredQueue.Shutdown()
	
	// Replay pending orders
	replayedOrders, err := recoveredQueue.ReplayPending()
	require.NoError(t, err)
	
	// Phase 5: Data integrity validation
	validateDataIntegrity(t, test.ValidationLevel, referenceOrders, referenceChecksums, 
		processedOrders, replayedOrders, pendingOrderIDs)
}

func generateReferenceOrders(count int) []*Order {
	orders := make([]*Order, count)
	
	for i := 0; i < count; i++ {
		// Create deterministic but varied data
		dataContent := fmt.Sprintf(`{
			"orderID": "ref-order-%d",
			"symbol": "%s",
			"side": "%s",
			"amount": %.8f,
			"price": %.8f,
			"timestamp": %d,
			"nonce": %d
		}`, i, 
			[]string{"BTC-USD", "ETH-USD", "ADA-USD", "DOT-USD"}[i%4],
			[]string{"buy", "sell"}[i%2],
			float64(100+i%1000)/100.0,
			float64(50000+i*10)/100.0,
			time.Now().UnixNano(),
			i*12345)
		
		orders[i] = &Order{
			ID:        fmt.Sprintf("ref-order-%d", i),
			Priority:  Priority(i % 3),
			Data:      []byte(dataContent),
			Timestamp: time.Now().Add(time.Duration(i) * time.Microsecond),
		}
	}
	
	return orders
}

func calculateOrderChecksums(orders []*Order) map[string]OrderChecksum {
	checksums := make(map[string]OrderChecksum)
	
	for _, order := range orders {
		// Create checksum from all order fields
		hasher := sha256.New()
		hasher.Write([]byte(order.ID))
		hasher.Write([]byte(fmt.Sprintf("%d", order.Priority)))
		hasher.Write(order.Data)
		hasher.Write([]byte(order.Timestamp.Format(time.RFC3339Nano)))
		
		checksum := hex.EncodeToString(hasher.Sum(nil))
		
		checksums[order.ID] = OrderChecksum{
			OrderID:   order.ID,
			Checksum:  checksum,
			Timestamp: time.Now(),
		}
	}
	
	return checksums
}

func validateDataIntegrity(t *testing.T, level ValidationLevel, 
	referenceOrders []*Order, referenceChecksums map[string]OrderChecksum,
	processedOrders map[string]*Order, replayedOrders []*Order, 
	expectedPendingIDs map[string]bool) {
	
	t.Logf("Starting data integrity validation (level: %d)", level)
	
	// Create maps for easier lookup
	referenceMap := make(map[string]*Order)
	for _, order := range referenceOrders {
		referenceMap[order.ID] = order
	}
	
	replayedMap := make(map[string]*Order)
	for _, order := range replayedOrders {
		replayedMap[order.ID] = order
	}
	
	// Validation 1: No orders should be completely lost
	totalAccountedOrders := len(processedOrders) + len(replayedOrders)
	assert.Equal(t, len(referenceOrders), totalAccountedOrders, 
		"Total accounted orders should equal reference orders")
	
	// Validation 2: No duplicate processing
	duplicates := findDuplicates(processedOrders, replayedMap)
	assert.Empty(t, duplicates, "No orders should be both processed and replayed")
	
	// Validation 3: All expected pending orders are replayed
	for expectedID := range expectedPendingIDs {
		assert.Contains(t, replayedMap, expectedID, 
			"Expected pending order %s should be replayed", expectedID)
	}
	
	// Validation 4: No unexpected orders in replay
	for replayedID := range replayedMap {
		assert.Contains(t, expectedPendingIDs, replayedID,
			"Replayed order %s should be in expected pending list", replayedID)
	}
	
	switch level {
	case BasicValidation:
		t.Log("Basic validation completed successfully")
		
	case ChecksumValidation:
		validateChecksums(t, referenceChecksums, processedOrders, replayedOrders)
		
	case FullContentValidation:
		validateFullContent(t, referenceMap, processedOrders, replayedOrders)
	}
}

func findDuplicates(processedOrders map[string]*Order, replayedMap map[string]*Order) []string {
	var duplicates []string
	
	for processedID := range processedOrders {
		if _, exists := replayedMap[processedID]; exists {
			duplicates = append(duplicates, processedID)
		}
	}
	
	return duplicates
}

func validateChecksums(t *testing.T, referenceChecksums map[string]OrderChecksum, 
	processedOrders map[string]*Order, replayedOrders []*Order) {
	
	t.Log("Performing checksum validation...")
	
	// Validate processed orders
	for _, order := range processedOrders {
		refChecksum, exists := referenceChecksums[order.ID]
		require.True(t, exists, "Reference checksum should exist for order %s", order.ID)
		
		actualChecksum := calculateSingleOrderChecksum(order)
		assert.Equal(t, refChecksum.Checksum, actualChecksum,
			"Processed order %s checksum mismatch", order.ID)
	}
	
	// Validate replayed orders
	for _, order := range replayedOrders {
		refChecksum, exists := referenceChecksums[order.ID]
		require.True(t, exists, "Reference checksum should exist for replayed order %s", order.ID)
		
		actualChecksum := calculateSingleOrderChecksum(order)
		assert.Equal(t, refChecksum.Checksum, actualChecksum,
			"Replayed order %s checksum mismatch", order.ID)
	}
	
	t.Log("Checksum validation completed successfully")
}

func validateFullContent(t *testing.T, referenceMap map[string]*Order,
	processedOrders map[string]*Order, replayedOrders []*Order) {
	
	t.Log("Performing full content validation...")
	
	// Validate processed orders
	for _, order := range processedOrders {
		refOrder, exists := referenceMap[order.ID]
		require.True(t, exists, "Reference order should exist for %s", order.ID)
		
		validateOrderEquality(t, refOrder, order, "processed")
	}
	
	// Validate replayed orders
	for _, order := range replayedOrders {
		refOrder, exists := referenceMap[order.ID]
		require.True(t, exists, "Reference order should exist for replayed %s", order.ID)
		
		validateOrderEquality(t, refOrder, order, "replayed")
	}
	
	t.Log("Full content validation completed successfully")
}

func validateOrderEquality(t *testing.T, expected, actual *Order, context string) {
	assert.Equal(t, expected.ID, actual.ID, "%s order ID mismatch", context)
	assert.Equal(t, expected.Priority, actual.Priority, "%s order priority mismatch", context)
	assert.Equal(t, expected.Data, actual.Data, "%s order data mismatch", context)
	
	// Timestamp comparison with tolerance for serialization
	timeDiff := expected.Timestamp.Sub(actual.Timestamp)
	if timeDiff < 0 {
		timeDiff = -timeDiff
	}
	assert.Less(t, timeDiff, time.Millisecond, "%s order timestamp mismatch", context)
}

func calculateSingleOrderChecksum(order *Order) string {
	hasher := sha256.New()
	hasher.Write([]byte(order.ID))
	hasher.Write([]byte(fmt.Sprintf("%d", order.Priority)))
	hasher.Write(order.Data)
	hasher.Write([]byte(order.Timestamp.Format(time.RFC3339Nano)))
	
	return hex.EncodeToString(hasher.Sum(nil))
}

// Test priority preservation during recovery
func TestDataIntegrity_PriorityPreservation(t *testing.T) {
	tempDir := t.TempDir()
	queue, err := NewBadgerQueue(tempDir)
	require.NoError(t, err)
	
	// Create orders with specific priority patterns
	orders := []*Order{
		{ID: "low-1", Priority: LowPriority, Data: []byte("low1"), Timestamp: time.Now()},
		{ID: "high-1", Priority: HighPriority, Data: []byte("high1"), Timestamp: time.Now().Add(time.Microsecond)},
		{ID: "medium-1", Priority: MediumPriority, Data: []byte("medium1"), Timestamp: time.Now().Add(2 * time.Microsecond)},
		{ID: "high-2", Priority: HighPriority, Data: []byte("high2"), Timestamp: time.Now().Add(3 * time.Microsecond)},
		{ID: "low-2", Priority: LowPriority, Data: []byte("low2"), Timestamp: time.Now().Add(4 * time.Microsecond)},
		{ID: "medium-2", Priority: MediumPriority, Data: []byte("medium2"), Timestamp: time.Now().Add(5 * time.Microsecond)},
	}
	
	// Enqueue all orders
	for _, order := range orders {
		err := queue.Enqueue(order)
		require.NoError(t, err)
	}
	
	// Dequeue first order but don't acknowledge (to leave it pending)
	firstOrder, err := queue.Dequeue()
	require.NoError(t, err)
	assert.Equal(t, "high-1", firstOrder.ID) // Should be highest priority first
	
	queue.Shutdown()
	
	// Recover and validate priority ordering
	recoveredQueue, err := NewBadgerQueue(tempDir)
	require.NoError(t, err)
	defer recoveredQueue.Shutdown()
	
	replayedOrders, err := recoveredQueue.ReplayPending()
	require.NoError(t, err)
	
	// All orders should be replayed since none were acknowledged
	assert.Len(t, replayedOrders, len(orders))
	
	// Validate priority ordering is preserved
	expectedOrder := []string{"high-1", "high-2", "medium-1", "medium-2", "low-1", "low-2"}
	
	for i, expectedID := range expectedOrder {
		assert.Equal(t, expectedID, replayedOrders[i].ID,
			"Priority order not preserved at position %d", i)
	}
}

// Test data consistency across multiple recovery cycles
func TestDataIntegrity_MultipleRecoveryCycles(t *testing.T) {
	tempDir := t.TempDir()
	
	const totalOrders = 1000
	const recoveryCycles = 5
	
	originalOrders := generateReferenceOrders(totalOrders)
	originalChecksums := calculateOrderChecksums(originalOrders)
	
	// Track what should be pending after each cycle
	var allProcessedIDs []string
	
	for cycle := 0; cycle < recoveryCycles; cycle++ {
		t.Logf("Starting recovery cycle %d", cycle+1)
		
		queue, err := NewBadgerQueue(tempDir)
		require.NoError(t, err)
		
		if cycle == 0 {
			// First cycle: enqueue all orders
			for _, order := range originalOrders {
				err := queue.Enqueue(order)
				require.NoError(t, err)
			}
		} else {
			// Subsequent cycles: replay pending orders
			replayedOrders, err := queue.ReplayPending()
			require.NoError(t, err)
			t.Logf("Cycle %d: Replayed %d orders", cycle+1, len(replayedOrders))
		}
		
		// Process some orders randomly
		cycleProcessed := 0
		maxToProcess := (totalOrders - len(allProcessedIDs)) / (recoveryCycles - cycle)
		
		for i := 0; i < maxToProcess; i++ {
			order, err := queue.Dequeue()
			if err != nil || order == nil {
				break
			}
			
			// Acknowledge order
			err = queue.Acknowledge(order.ID)
			require.NoError(t, err)
			
			allProcessedIDs = append(allProcessedIDs, order.ID)
			cycleProcessed++
			
			// Validate order integrity
			refChecksum, exists := originalChecksums[order.ID]
			require.True(t, exists, "Reference checksum missing for %s", order.ID)
			
			actualChecksum := calculateSingleOrderChecksum(order)
			assert.Equal(t, refChecksum.Checksum, actualChecksum,
				"Checksum mismatch in cycle %d for order %s", cycle+1, order.ID)
		}
		
		t.Logf("Cycle %d: Processed %d orders, total processed: %d", 
			cycle+1, cycleProcessed, len(allProcessedIDs))
		
		queue.Shutdown()
	}
	
	// Final validation
	assert.Equal(t, totalOrders, len(allProcessedIDs), 
		"All orders should be processed after all cycles")
	
	// Ensure no duplicates were processed
	processedSet := make(map[string]bool)
	for _, id := range allProcessedIDs {
		assert.False(t, processedSet[id], "Duplicate processing detected for order %s", id)
		processedSet[id] = true
	}
}
