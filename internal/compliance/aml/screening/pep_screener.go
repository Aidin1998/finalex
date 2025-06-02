package screening

import (
	"context"
	"strings"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/compliance/aml"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// PEPScreener provides Politically Exposed Person (PEP) screening capabilities
type PEPScreener struct {
	logger *zap.SugaredLogger
	config PEPConfig
	lists  map[string]*PEPList
}

// PEPConfig defines configuration for PEP screening
type PEPConfig struct {
	EnablePEPScreening bool          `json:"enable_pep_screening"`
	PEPMatchThreshold  float64       `json:"pep_match_threshold"`
	IncludeFamily      bool          `json:"include_family"`
	IncludeAssociates  bool          `json:"include_associates"`
	MinRiskLevel       aml.RiskLevel `json:"min_risk_level"`
	DataSources        []string      `json:"data_sources"`
	UpdateInterval     time.Duration `json:"update_interval"`
}

// PEPList represents a list of Politically Exposed Persons
type PEPList struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Source      string                 `json:"source"`
	Country     string                 `json:"country"`
	Entries     map[string]*PEPEntry   `json:"entries"`
	LastUpdated time.Time              `json:"last_updated"`
	IsActive    bool                   `json:"is_active"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// PEPEntry represents a PEP entry
type PEPEntry struct {
	ID             string                 `json:"id"`
	ListID         string                 `json:"list_id"`
	Name           string                 `json:"name"`
	AlternateNames []string               `json:"alternate_names"`
	Position       string                 `json:"position"`
	Country        string                 `json:"country"`
	PEPType        string                 `json:"pep_type"` // "direct", "family", "associate"
	RiskLevel      aml.RiskLevel          `json:"risk_level"`
	IsActive       bool                   `json:"is_active"`
	StartDate      *time.Time             `json:"start_date"`
	EndDate        *time.Time             `json:"end_date"`
	Organization   string                 `json:"organization"`
	Department     string                 `json:"department"`
	Relationships  []PEPRelationship      `json:"relationships"`
	Metadata       map[string]interface{} `json:"metadata"`
	LastUpdated    time.Time              `json:"last_updated"`
}

// PEPRelationship represents relationships between PEP entries
type PEPRelationship struct {
	RelatedID        string `json:"related_id"`
	RelationshipType string `json:"relationship_type"` // "spouse", "child", "parent", "associate", "business_partner"
	Description      string `json:"description"`
}

// PEPScreeningResult represents a PEP screening result
type PEPScreeningResult struct {
	ID         uuid.UUID              `json:"id"`
	UserID     uuid.UUID              `json:"user_id"`
	ScreenedAt time.Time              `json:"screened_at"`
	IsPEP      bool                   `json:"is_pep"`
	PEPMatches []PEPMatch             `json:"pep_matches"`
	RiskLevel  aml.RiskLevel          `json:"risk_level"`
	Confidence float64                `json:"confidence"`
	Metadata   map[string]interface{} `json:"metadata"`
}

// PEPMatch represents a match with a PEP entry
type PEPMatch struct {
	EntryID      string        `json:"entry_id"`
	ListID       string        `json:"list_id"`
	MatchScore   float64       `json:"match_score"`
	MatchType    string        `json:"match_type"`
	PEPType      string        `json:"pep_type"`
	Position     string        `json:"position"`
	Country      string        `json:"country"`
	RiskLevel    aml.RiskLevel `json:"risk_level"`
	IsCurrentPEP bool          `json:"is_current_pep"`
}

// NewPEPScreener creates a new PEP screener
func NewPEPScreener(logger *zap.SugaredLogger) *PEPScreener {
	return &PEPScreener{
		logger: logger,
		config: PEPConfig{
			EnablePEPScreening: true,
			PEPMatchThreshold:  0.85,
			IncludeFamily:      true,
			IncludeAssociates:  true,
			MinRiskLevel:       aml.RiskLevelMedium,
			DataSources:        []string{"world_bank", "ofac", "eu_sanctions"},
			UpdateInterval:     24 * time.Hour,
		},
		lists: make(map[string]*PEPList),
	}
}

// ScreenForPEP screens a user for PEP status
func (ps *PEPScreener) ScreenForPEP(ctx context.Context, userID uuid.UUID, userData *aml.AMLUser) (*PEPScreeningResult, error) {
	if !ps.config.EnablePEPScreening {
		return &PEPScreeningResult{
			ID:         uuid.New(),
			UserID:     userID,
			ScreenedAt: time.Now(),
			IsPEP:      false,
			PEPMatches: []PEPMatch{},
			RiskLevel:  aml.RiskLevelLow,
			Confidence: 1.0,
		}, nil
	}

	result := &PEPScreeningResult{
		ID:         uuid.New(),
		UserID:     userID,
		ScreenedAt: time.Now(),
		PEPMatches: make([]PEPMatch, 0),
		Metadata:   make(map[string]interface{}),
	}

	// Screen against all active PEP lists
	for _, list := range ps.lists {
		if !list.IsActive {
			continue
		}

		matches := ps.screenAgainstPEPList(userData, list)
		result.PEPMatches = append(result.PEPMatches, matches...)
	}

	// Determine overall PEP status and risk level
	result.IsPEP = len(result.PEPMatches) > 0
	result.RiskLevel = ps.calculatePEPRiskLevel(result.PEPMatches)
	result.Confidence = ps.calculatePEPConfidence(result.PEPMatches)

	ps.logger.Infow("PEP screening completed",
		"user_id", userID,
		"is_pep", result.IsPEP,
		"matches", len(result.PEPMatches),
		"risk_level", result.RiskLevel)

	return result, nil
}

// screenAgainstPEPList screens user data against a specific PEP list
func (ps *PEPScreener) screenAgainstPEPList(userData *aml.AMLUser, list *PEPList) []PEPMatch {
	var matches []PEPMatch

	// This would typically involve more sophisticated name matching
	// For now, we'll use a simplified approach
	userName := userData.KYCStatus // This should be the actual user name from KYC data

	for _, entry := range list.Entries {
		if ps.isPEPMatch(userName, entry) {
			match := PEPMatch{
				EntryID:      entry.ID,
				ListID:       entry.ListID,
				MatchScore:   0.9, // Simplified scoring
				MatchType:    "fuzzy",
				PEPType:      entry.PEPType,
				Position:     entry.Position,
				Country:      entry.Country,
				RiskLevel:    entry.RiskLevel,
				IsCurrentPEP: ps.isCurrentPEP(entry),
			}
			matches = append(matches, match)
		}
	}

	return matches
}

// isPEPMatch determines if a user name matches a PEP entry
func (ps *PEPScreener) isPEPMatch(userName string, entry *PEPEntry) bool {
	// Simplified matching logic - in practice, this would use the fuzzy matcher
	normalizedUserName := strings.ToLower(strings.TrimSpace(userName))
	normalizedPEPName := strings.ToLower(strings.TrimSpace(entry.Name))

	// Check primary name
	if strings.Contains(normalizedPEPName, normalizedUserName) ||
		strings.Contains(normalizedUserName, normalizedPEPName) {
		return true
	}

	// Check alternate names
	for _, altName := range entry.AlternateNames {
		normalizedAltName := strings.ToLower(strings.TrimSpace(altName))
		if strings.Contains(normalizedAltName, normalizedUserName) ||
			strings.Contains(normalizedUserName, normalizedAltName) {
			return true
		}
	}

	return false
}

// isCurrentPEP determines if a PEP entry represents a current PEP
func (ps *PEPScreener) isCurrentPEP(entry *PEPEntry) bool {
	if entry.EndDate == nil {
		return true // No end date means currently active
	}
	return entry.EndDate.After(time.Now())
}

// calculatePEPRiskLevel calculates the overall risk level based on PEP matches
func (ps *PEPScreener) calculatePEPRiskLevel(matches []PEPMatch) aml.RiskLevel {
	if len(matches) == 0 {
		return aml.RiskLevelLow
	}

	highestRisk := aml.RiskLevelLow
	for _, match := range matches {
		if match.RiskLevel == aml.RiskLevelCritical {
			return aml.RiskLevelCritical
		}
		if match.RiskLevel == aml.RiskLevelHigh && highestRisk != aml.RiskLevelCritical {
			highestRisk = aml.RiskLevelHigh
		}
		if match.RiskLevel == aml.RiskLevelMedium && highestRisk == aml.RiskLevelLow {
			highestRisk = aml.RiskLevelMedium
		}
	}

	return highestRisk
}

// calculatePEPConfidence calculates confidence in the PEP screening result
func (ps *PEPScreener) calculatePEPConfidence(matches []PEPMatch) float64 {
	if len(matches) == 0 {
		return 1.0 // High confidence in negative result if no matches
	}

	totalScore := 0.0
	for _, match := range matches {
		totalScore += match.MatchScore
	}

	return totalScore / float64(len(matches))
}
