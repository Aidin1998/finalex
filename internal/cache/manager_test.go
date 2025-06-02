package cache

import (
	"testing"
)

func TestCacheManager_CacheLevels(t *testing.T) {
	// Test that our new constant names are working correctly
	if CacheLevelL1 != CacheLevel(0) {
		t.Errorf("Expected CacheLevelL1 to be 0, got %d", CacheLevelL1)
	}

	if CacheLevelL2 != CacheLevel(1) {
		t.Errorf("Expected CacheLevelL2 to be 1, got %d", CacheLevelL2)
	}

	if CacheLevelL3 != CacheLevel(2) {
		t.Errorf("Expected CacheLevelL3 to be 2, got %d", CacheLevelL3)
	}
}
