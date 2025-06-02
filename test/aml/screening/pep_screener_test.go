package screening_test

import (
	"context"
	"testing"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/compliance/aml"
	"github.com/Aidin1998/pincex_unified/internal/compliance/aml/screening"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestNewPEPScreener(t *testing.T) {
	logger := zap.NewNop().Sugar()
	screener := screening.NewPEPScreener(logger)

	assert.NotNil(t, screener)
	config := screener.GetConfig()
	assert.True(t, config.EnablePEPScreening)
	assert.Equal(t, 0.85, config.PEPMatchThreshold)
	assert.True(t, config.IncludeFamily)
	assert.True(t, config.IncludeAssociates)
}

func TestPEPScreener_ScreenUser(t *testing.T) {
	logger := zap.NewNop().Sugar()
	screener := screening.NewPEPScreener(logger)

	// Add test PEP data
	testPEPList := &screening.PEPList{
		ID:      "test-pep-list",
		Name:    "Test PEP List",
		Source:  "test",
		Country: "US",
		Entries: map[string]*screening.PEPEntry{
			"pep-1": {
				ID:             "pep-1",
				ListID:         "test-pep-list",
				Name:           "John Political",
				AlternateNames: []string{"John P. Political", "J. Political"},
				Position:       "Governor",
				Country:        "US",
				PEPType:        "direct",
				RiskLevel:      aml.RiskLevelHigh,
				IsActive:       true,
				Organization:   "State Government",
				Department:     "Executive",
				LastUpdated:    time.Now(),
			},
			"pep-2": {
				ID:             "pep-2",
				ListID:         "test-pep-list",
				Name:           "Jane Spouse Political",
				AlternateNames: []string{"Jane Political"},
				Position:       "Governor's Spouse",
				Country:        "US",
				PEPType:        "family",
				RiskLevel:      aml.RiskLevelMedium,
				IsActive:       true,
				Relationships: []screening.PEPRelationship{
					{
						RelatedID:        "pep-1",
						RelationshipType: "spouse",
						Description:      "Married to John Political",
					},
				},
				LastUpdated: time.Now(),
			},
		},
		LastUpdated: time.Now(),
		IsActive:    true,
	}

	err := screener.LoadPEPList(testPEPList)
	assert.NoError(t, err)

	tests := []struct {
		name          string
		userInfo      aml.UserInfo
		expectedIsPEP bool
		expectedRisk  aml.RiskLevel
		minConfidence float64
	}{
		{
			name: "exact_match_direct_pep",
			userInfo: aml.UserInfo{
				ID:        uuid.New(),
				FirstName: "John",
				LastName:  "Political",
				FullName:  "John Political",
			},
			expectedIsPEP: true,
			expectedRisk:  aml.RiskLevelHigh,
			minConfidence: 0.9,
		},
		{
			name: "exact_match_family_member",
			userInfo: aml.UserInfo{
				ID:        uuid.New(),
				FirstName: "Jane",
				LastName:  "Political",
				FullName:  "Jane Political",
			},
			expectedIsPEP: true,
			expectedRisk:  aml.RiskLevelMedium,
			minConfidence: 0.8,
		},
		{
			name: "fuzzy_match_alternate_name",
			userInfo: aml.UserInfo{
				ID:        uuid.New(),
				FirstName: "John",
				LastName:  "P. Political",
				FullName:  "John P. Political",
			},
			expectedIsPEP: true,
			expectedRisk:  aml.RiskLevelHigh,
			minConfidence: 0.85,
		},
		{
			name: "no_match",
			userInfo: aml.UserInfo{
				ID:        uuid.New(),
				FirstName: "Normal",
				LastName:  "Citizen",
				FullName:  "Normal Citizen",
			},
			expectedIsPEP: false,
			expectedRisk:  aml.RiskLevelLow,
			minConfidence: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			result, err := screener.ScreenUser(ctx, tt.userInfo)

			assert.NoError(t, err)
			assert.NotNil(t, result)
			assert.Equal(t, tt.expectedIsPEP, result.IsPEP)
			assert.Equal(t, tt.expectedRisk, result.RiskLevel)

			if tt.expectedIsPEP {
				assert.True(t, len(result.PEPMatches) > 0)
				assert.True(t, result.Confidence >= tt.minConfidence)
			}
		})
	}
}

func TestPEPScreener_LoadPEPList(t *testing.T) {
	logger := zap.NewNop().Sugar()
	screener := screening.NewPEPScreener(logger)

	pepList := &screening.PEPList{
		ID:      "test-list",
		Name:    "Test List",
		Source:  "test",
		Country: "US",
		Entries: map[string]*screening.PEPEntry{
			"entry-1": {
				ID:          "entry-1",
				ListID:      "test-list",
				Name:        "Test Person",
				Position:    "Official",
				Country:     "US",
				PEPType:     "direct",
				RiskLevel:   aml.RiskLevelHigh,
				IsActive:    true,
				LastUpdated: time.Now(),
			},
		},
		LastUpdated: time.Now(),
		IsActive:    true,
	}

	err := screener.LoadPEPList(pepList)
	assert.NoError(t, err)

	// Verify list was loaded
	status := screener.GetPEPListStatus()
	assert.Equal(t, 1, len(status))
	assert.Equal(t, "test-list", status[0].ID)
}

func TestPEPScreener_RemovePEPList(t *testing.T) {
	logger := zap.NewNop().Sugar()
	screener := screening.NewPEPScreener(logger)

	// Add a list first
	pepList := &screening.PEPList{
		ID:      "test-list",
		Name:    "Test List",
		Source:  "test",
		Country: "US",
		Entries: make(map[string]*screening.PEPEntry),
	}

	err := screener.LoadPEPList(pepList)
	assert.NoError(t, err)

	// Verify it exists
	status := screener.GetPEPListStatus()
	assert.Equal(t, 1, len(status))

	// Remove it
	err = screener.RemovePEPList("test-list")
	assert.NoError(t, err)

	// Verify it's gone
	status = screener.GetPEPListStatus()
	assert.Equal(t, 0, len(status))
}

func BenchmarkPEPScreener_ScreenUser(b *testing.B) {
	logger := zap.NewNop().Sugar()
	screener := screening.NewPEPScreener(logger)

	// Create a large test dataset
	entries := make(map[string]*screening.PEPEntry)
	for i := 0; i < 1000; i++ {
		entryID := uuid.New().String()
		entries[entryID] = &screening.PEPEntry{
			ID:          entryID,
			ListID:      "benchmark-list",
			Name:        "Test Person " + entryID[:8],
			Position:    "Official",
			Country:     "US",
			PEPType:     "direct",
			RiskLevel:   aml.RiskLevelMedium,
			IsActive:    true,
			LastUpdated: time.Now(),
		}
	}

	testList := &screening.PEPList{
		ID:          "benchmark-list",
		Name:        "Benchmark List",
		Source:      "test",
		Country:     "US",
		Entries:     entries,
		LastUpdated: time.Now(),
		IsActive:    true,
	}

	screener.LoadPEPList(testList)

	userInfo := aml.UserInfo{
		ID:        uuid.New(),
		FirstName: "Test",
		LastName:  "User",
		FullName:  "Test User",
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := screener.ScreenUser(ctx, userInfo)
		if err != nil {
			b.Fatal(err)
		}
	}
}
