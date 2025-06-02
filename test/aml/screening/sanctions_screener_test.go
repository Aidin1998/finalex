package screening_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/Aidin1998/pincex_unified/internal/compliance/aml"
	"github.com/Aidin1998/pincex_unified/internal/compliance/aml/screening"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

func TestSanctionsScreener_ExactMatch(t *testing.T) {
	logger := zap.NewNop().Sugar()
	screener := screening.NewSanctionsScreener(logger)

	// Add a test entry to OFAC list
	ofacList := screener.GetSanctionsList("ofac_sdn")
	testEntry := &screening.SanctionsEntry{
		ID:            "test-001",
		ListID:        "ofac_sdn",
		EntryType:     "individual",
		PrimaryName:   "JOHN SMITH",
		SanctionsType: "sanctions",
		Program:       "TEST",
		IsActive:      true,
		RiskScore:     0.95,
	}
	ofacList.Entries["test-001"] = testEntry

	// Rebuild indexes
	screener.RebuildIndexes()

	// Create test user data
	userData := &aml.AMLUser{
		ID:        uuid.New(),
		UserID:    uuid.New(),
		KYCStatus: "JOHN SMITH", // This should be replaced with actual name field
	}

	// Perform screening
	result, err := screener.ScreenUser(context.Background(), userData.UserID, userData, "onboarding")
	if err != nil {
		t.Fatalf("Screening failed: %v", err)
	}

	// Verify results
	if len(result.Results) == 0 {
		t.Error("Expected to find a match, but got no results")
	}

	if result.Results[0].MatchType != "exact" {
		t.Errorf("Expected exact match, got %s", result.Results[0].MatchType)
	}

	if result.Results[0].MatchScore < 1.0 {
		t.Errorf("Expected score 1.0 for exact match, got %f", result.Results[0].MatchScore)
	}
}

func TestSanctionsScreener_FuzzyMatch(t *testing.T) {
	logger := zap.NewNop().Sugar()
	screener := screening.NewSanctionsScreener(logger)

	// Add a test entry to OFAC list
	ofacList := screener.GetSanctionsList("ofac_sdn")
	testEntry := &screening.SanctionsEntry{
		ID:            "test-002",
		ListID:        "ofac_sdn",
		EntryType:     "individual",
		PrimaryName:   "JOHN SMITH",
		SanctionsType: "sanctions",
		Program:       "TEST",
		IsActive:      true,
		RiskScore:     0.95,
	}
	ofacList.Entries["test-002"] = testEntry

	// Create test user data with similar but not exact name
	userData := &aml.AMLUser{
		ID:        uuid.New(),
		UserID:    uuid.New(),
		KYCStatus: "Jon Smith", // Similar but not exact
	}

	// Perform screening
	result, err := screener.ScreenUser(context.Background(), userData.UserID, userData, "onboarding")
	if err != nil {
		t.Fatalf("Screening failed: %v", err)
	}

	// Verify results - should find fuzzy match
	if len(result.Results) == 0 {
		t.Error("Expected to find a fuzzy match, but got no results")
	}

	if result.Results[0].MatchScore < 0.8 {
		t.Errorf("Expected high fuzzy match score, got %f", result.Results[0].MatchScore)
	}
}

func TestSanctionsScreener_NoMatch(t *testing.T) {
	logger := zap.NewNop().Sugar()
	screener := screening.NewSanctionsScreener(logger)

	// Create test user data with completely different name
	userData := &aml.AMLUser{
		ID:        uuid.New(),
		UserID:    uuid.New(),
		KYCStatus: "Jane Doe",
	}

	// Perform screening
	result, err := screener.ScreenUser(context.Background(), userData.UserID, userData, "onboarding")
	if err != nil {
		t.Fatalf("Screening failed: %v", err)
	}

	// Verify no results
	if len(result.Results) > 0 {
		t.Errorf("Expected no matches, but got %d results", len(result.Results))
	}

	if result.Status != "completed" {
		t.Errorf("Expected status 'completed', got %s", result.Status)
	}
}

func TestSanctionsScreener_MultipleLists(t *testing.T) {
	logger := zap.NewNop().Sugar()
	screener := screening.NewSanctionsScreener(logger)

	// Add test entries to multiple lists
	ofacList := screener.GetSanctionsList("ofac_sdn")
	unList := screener.GetSanctionsList("un_sc")

	ofacEntry := &screening.SanctionsEntry{
		ID:            "ofac-test",
		ListID:        "ofac_sdn",
		EntryType:     "individual",
		PrimaryName:   "JOHN SMITH",
		SanctionsType: "sanctions",
		Program:       "OFAC-TEST",
		IsActive:      true,
		RiskScore:     0.95,
	}
	ofacList.Entries["ofac-test"] = ofacEntry

	unEntry := &screening.SanctionsEntry{
		ID:            "un-test",
		ListID:        "un_sc",
		EntryType:     "individual",
		PrimaryName:   "JOHN SMITH",
		SanctionsType: "sanctions",
		Program:       "UN-TEST",
		IsActive:      true,
		RiskScore:     0.93,
	}
	unList.Entries["un-test"] = unEntry

	// Rebuild indexes
	screener.RebuildIndexes()

	// Create test user data
	userData := &aml.AMLUser{
		ID:        uuid.New(),
		UserID:    uuid.New(),
		KYCStatus: "JOHN SMITH",
	}

	// Perform screening
	result, err := screener.ScreenUser(context.Background(), userData.UserID, userData, "onboarding")
	if err != nil {
		t.Fatalf("Screening failed: %v", err)
	}

	// Verify results from multiple lists
	if len(result.Results) < 2 {
		t.Errorf("Expected matches from multiple lists, got %d results", len(result.Results))
	}

	// Check that we have results from both lists
	listIDs := make(map[string]bool)
	for _, r := range result.Results {
		listIDs[r.ListID] = true
	}

	if !listIDs["ofac_sdn"] || !listIDs["un_sc"] {
		t.Error("Expected results from both OFAC and UN lists")
	}
}

func BenchmarkSanctionsScreener_ScreenUser(b *testing.B) {
	logger := zap.NewNop().Sugar()
	screener := screening.NewSanctionsScreener(logger)

	// Add test entries to make benchmark more realistic
	ofacList := screener.GetSanctionsList("ofac_sdn")
	for i := 0; i < 1000; i++ {
		entry := &screening.SanctionsEntry{
			ID:            fmt.Sprintf("bench-%d", i),
			ListID:        "ofac_sdn",
			EntryType:     "individual",
			PrimaryName:   fmt.Sprintf("PERSON %d", i),
			SanctionsType: "sanctions",
			Program:       "BENCHMARK",
			IsActive:      true,
			RiskScore:     0.95,
		}
		ofacList.Entries[entry.ID] = entry
	}
	screener.RebuildIndexes()

	userData := &aml.AMLUser{
		ID:        uuid.New(),
		UserID:    uuid.New(),
		KYCStatus: "PERSON 500", // Should match one entry
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := screener.ScreenUser(context.Background(), userData.UserID, userData, "benchmark")
		if err != nil {
			b.Fatalf("Screening failed: %v", err)
		}
	}
}
