package screening_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/compliance/aml/screening"
	"go.uber.org/zap"
)

func TestFuzzyMatcher_ExactMatch(t *testing.T) {
	logger := zap.NewNop().Sugar()
	matcher := screening.NewFuzzyMatcher(logger)

	result := matcher.MatchNames(context.Background(), "John Smith", "John Smith", []string{})

	if result.MatchType != "exact" {
		t.Errorf("Expected exact match, got %s", result.MatchType)
	}

	if result.OverallScore < 1.0 {
		t.Errorf("Expected score 1.0 for exact match, got %f", result.OverallScore)
	}
}

func TestFuzzyMatcher_FuzzyMatch(t *testing.T) {
	logger := zap.NewNop().Sugar()
	matcher := screening.NewFuzzyMatcher(logger)

	result := matcher.MatchNames(context.Background(), "Jon Smith", "John Smith", []string{})

	if result.OverallScore < 0.8 {
		t.Errorf("Expected high similarity score, got %f", result.OverallScore)
	}

	if result.MatchType == "no_match" {
		t.Errorf("Expected some match for similar names, got no_match")
	}
}

func TestFuzzyMatcher_AlternateNames(t *testing.T) {
	logger := zap.NewNop().Sugar()
	matcher := screening.NewFuzzyMatcher(logger)

	alternateNames := []string{"Johnny Smith", "J. Smith", "John F. Smith"}
	result := matcher.MatchNames(context.Background(), "Johnny Smith", "John Smith", alternateNames)

	if result.MatchType != "exact" {
		t.Errorf("Expected exact match with alternate name, got %s", result.MatchType)
	}

	if result.OverallScore < 1.0 {
		t.Errorf("Expected score 1.0 for exact alternate match, got %f", result.OverallScore)
	}
}

func TestFuzzyMatcher_PhoneticMatch(t *testing.T) {
	logger := zap.NewNop().Sugar()
	matcher := screening.NewFuzzyMatcher(logger)

	// Test phonetic similarity
	result := matcher.MatchNames(context.Background(), "Smith", "Smyth", []string{})

	if result.OverallScore < 0.7 {
		t.Errorf("Expected phonetic similarity, got %f", result.OverallScore)
	}
}

func TestFuzzyMatcher_Performance(t *testing.T) {
	logger := zap.NewNop().Sugar()
	matcher := screening.NewFuzzyMatcher(logger)

	// Test performance with large alternate names list
	alternateNames := make([]string, 100)
	for i := 0; i < 100; i++ {
		alternateNames[i] = fmt.Sprintf("Name Variant %d", i)
	}

	start := time.Now()
	result := matcher.MatchNames(context.Background(), "John Smith", "John Smith", alternateNames)
	elapsed := time.Since(start)

	if elapsed > 100*time.Millisecond {
		t.Errorf("Fuzzy matching took too long: %v", elapsed)
	}

	if result.MatchType != "exact" {
		t.Errorf("Expected exact match, got %s", result.MatchType)
	}
}

func BenchmarkFuzzyMatcher_MatchNames(b *testing.B) {
	logger := zap.NewNop().Sugar()
	matcher := screening.NewFuzzyMatcher(logger)

	alternateNames := []string{"Johnny Smith", "J. Smith", "John F. Smith"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		matcher.MatchNames(context.Background(), "John Smith", "John Smith", alternateNames)
	}
}
