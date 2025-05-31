package screening

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/compliance/aml"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// SanctionsScreener provides enhanced sanctions screening capabilities
type SanctionsScreener struct {
	mu     sync.RWMutex
	logger *zap.SugaredLogger

	// Sanctions lists
	sanctionsLists map[string]*SanctionsList
	
	// Screening configuration
	config ScreeningConfig

	// Screening history and results
	screeningHistory map[string]*ScreeningHistory
	
	// Watch list management
	watchLists map[string]*WatchList
}

// SanctionsList represents a sanctions list (OFAC, UN, EU, etc.)
type SanctionsList struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	Source       string                 `json:"source"`
	Jurisdiction string                 `json:"jurisdiction"`
	ListType     string                 `json:"list_type"` // "sanctions", "pep", "adverse_media"
	Entries      map[string]*SanctionsEntry `json:"entries"`
	LastUpdated  time.Time              `json:"last_updated"`
	IsActive     bool                   `json:"is_active"`
	UpdateURL    string                 `json:"update_url"`
	Metadata     map[string]interface{} `json:"metadata"`
}

// SanctionsEntry represents an entry in a sanctions list
type SanctionsEntry struct {
	ID              string                 `json:"id"`
	ListID          string                 `json:"list_id"`
	EntryType       string                 `json:"entry_type"` // "individual", "entity", "vessel", "address"
	PrimaryName     string                 `json:"primary_name"`
	AlternateNames  []string               `json:"alternate_names"`
	DateOfBirth     *time.Time             `json:"date_of_birth"`
	PlaceOfBirth    string                 `json:"place_of_birth"`
	Nationality     []string               `json:"nationality"`
	Addresses       []Address              `json:"addresses"`
	Identifiers     []Identifier           `json:"identifiers"`
	SanctionsType   string                 `json:"sanctions_type"`
	Program         string                 `json:"program"`
	EffectiveDate   time.Time              `json:"effective_date"`
	ExpiryDate      *time.Time             `json:"expiry_date"`
	ReasonListed    string                 `json:"reason_listed"`
	Remarks         string                 `json:"remarks"`
	LastUpdated     time.Time              `json:"last_updated"`
	RiskScore       float64                `json:"risk_score"`
	IsActive        bool                   `json:"is_active"`
}

// Address represents an address associated with a sanctions entry
type Address struct {
	AddressType string `json:"address_type"` // "residential", "business", "mailing"
	FullAddress string `json:"full_address"`
	Street      string `json:"street"`
	City        string `json:"city"`
	State       string `json:"state"`
	Country     string `json:"country"`
	PostalCode  string `json:"postal_code"`
}

// Identifier represents identification documents or numbers
type Identifier struct {
	Type        string `json:"type"`        // "passport", "national_id", "tax_id", "business_reg"
	Number      string `json:"number"`
	Country     string `json:"country"`
	ExpiryDate  *time.Time `json:"expiry_date"`
	IssuingAuth string `json:"issuing_authority"`
}

// ScreeningConfig defines screening configuration
type ScreeningConfig struct {
	MatchThreshold       float64           `json:"match_threshold"`
	FuzzyMatchThreshold  float64           `json:"fuzzy_match_threshold"`
	EnableFuzzyMatching  bool              `json:"enable_fuzzy_matching"`
	EnablePhoneticMatch  bool              `json:"enable_phonetic_match"`
	AutoUpdateLists      bool              `json:"auto_update_lists"`
	UpdateInterval       time.Duration     `json:"update_interval"`
	EnabledJurisdictions []string          `json:"enabled_jurisdictions"`
	ScreeningDepth       string            `json:"screening_depth"` // "basic", "enhanced", "comprehensive"
	RetentionPeriod      time.Duration     `json:"retention_period"`
}

// ScreeningHistory tracks screening history for users
type ScreeningHistory struct {
	UserID          uuid.UUID        `json:"user_id"`
	ScreeningEvents []ScreeningEvent `json:"screening_events"`
	LastScreened    time.Time        `json:"last_screened"`
	TotalScreenings int              `json:"total_screenings"`
	HighestRiskMatch float64         `json:"highest_risk_match"`
	CurrentStatus   string           `json:"current_status"` // "clear", "match", "potential_match", "under_review"
}

// ScreeningEvent represents a single screening event
type ScreeningEvent struct {
	ID          uuid.UUID       `json:"id"`
	UserID      uuid.UUID       `json:"user_id"`
	ScreenedAt  time.Time       `json:"screened_at"`
	ScreenType  string          `json:"screen_type"` // "onboarding", "periodic", "transaction_triggered", "manual"
	Results     []ScreeningResult `json:"results"`
	TotalMatches int            `json:"total_matches"`
	HighestScore float64        `json:"highest_score"`
	Status      string          `json:"status"`
	ReviewedBy  *uuid.UUID      `json:"reviewed_by"`
	ReviewNotes string          `json:"review_notes"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// ScreeningResult represents a screening match result
type ScreeningResult struct {
	ID            uuid.UUID              `json:"id"`
	MatchType     string                 `json:"match_type"` // "exact", "fuzzy", "phonetic", "partial"
	MatchScore    float64                `json:"match_score"`
	ListID        string                 `json:"list_id"`
	ListName      string                 `json:"list_name"`
	EntryID       string                 `json:"entry_id"`
	MatchedName   string                 `json:"matched_name"`
	MatchedField  string                 `json:"matched_field"` // "name", "dob", "address", "identifier"
	SanctionsType string                 `json:"sanctions_type"`
	RiskLevel     aml.RiskLevel          `json:"risk_level"`
	IsFalsePositive bool                 `json:"is_false_positive"`
	ReviewStatus  string                 `json:"review_status"` // "pending", "cleared", "confirmed", "escalated"
	Metadata      map[string]interface{} `json:"metadata"`
}

// WatchList represents custom watch lists
type WatchList struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Type        string                 `json:"type"` // "internal", "regulatory", "third_party"
	Entries     map[string]*WatchListEntry `json:"entries"`
	CreatedBy   uuid.UUID              `json:"created_by"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	IsActive    bool                   `json:"is_active"`
	Description string                 `json:"description"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// WatchListEntry represents an entry in a custom watch list
type WatchListEntry struct {
	ID             string                 `json:"id"`
	WatchListID    string                 `json:"watch_list_id"`
	Name           string                 `json:"name"`
	AlternateNames []string               `json:"alternate_names"`
	EntryType      string                 `json:"entry_type"`
	RiskLevel      aml.RiskLevel          `json:"risk_level"`
	Reason         string                 `json:"reason"`
	AddedBy        uuid.UUID              `json:"added_by"`
	AddedAt        time.Time              `json:"added_at"`
	ExpiryDate     *time.Time             `json:"expiry_date"`
	IsActive       bool                   `json:"is_active"`
	Metadata       map[string]interface{} `json:"metadata"`
}

// NewSanctionsScreener creates a new sanctions screener
func NewSanctionsScreener(logger *zap.SugaredLogger) *SanctionsScreener {
	screener := &SanctionsScreener{
		logger:           logger,
		sanctionsLists:   make(map[string]*SanctionsList),
		screeningHistory: make(map[string]*ScreeningHistory),
		watchLists:       make(map[string]*WatchList),
		config: ScreeningConfig{
			MatchThreshold:       0.95,
			FuzzyMatchThreshold:  0.80,
			EnableFuzzyMatching:  true,
			EnablePhoneticMatch:  true,
			AutoUpdateLists:      true,
			UpdateInterval:       24 * time.Hour,
			EnabledJurisdictions: []string{"US", "EU", "UN", "UK"},
			ScreeningDepth:       "enhanced",
			RetentionPeriod:      7 * 365 * 24 * time.Hour, // 7 years
		},
	}

	// Initialize default sanctions lists
	screener.initializeDefaultLists()

	return screener
}

// initializeDefaultLists initializes default sanctions lists
func (ss *SanctionsScreener) initializeDefaultLists() {
	// OFAC SDN List
	ofacList := &SanctionsList{
		ID:           "ofac_sdn",
		Name:         "OFAC Specially Designated Nationals List",
		Source:       "US Treasury OFAC",
		Jurisdiction: "US",
		ListType:     "sanctions",
		Entries:      make(map[string]*SanctionsEntry),
		LastUpdated:  time.Now(),
		IsActive:     true,
		UpdateURL:    "https://www.treasury.gov/ofac/downloads/sdnlist.txt",
	}

	// UN Security Council List
	unList := &SanctionsList{
		ID:           "un_sc",
		Name:         "UN Security Council Consolidated List",
		Source:       "United Nations",
		Jurisdiction: "UN",
		ListType:     "sanctions",
		Entries:      make(map[string]*SanctionsEntry),
		LastUpdated:  time.Now(),
		IsActive:     true,
		UpdateURL:    "https://scsanctions.un.org/resources/xml/en/consolidated.xml",
	}

	// EU Consolidated List
	euList := &SanctionsList{
		ID:           "eu_consolidated",
		Name:         "EU Consolidated List of Sanctions",
		Source:       "European Union",
		Jurisdiction: "EU",
		ListType:     "sanctions",
		Entries:      make(map[string]*SanctionsEntry),
		LastUpdated:  time.Now(),
		IsActive:     true,
		UpdateURL:    "https://webgate.ec.europa.eu/europeaid/fsd/fsf/public/files/xmlFullSanctionsList_1_1/content",
	}

	ss.sanctionsLists["ofac_sdn"] = ofacList
	ss.sanctionsLists["un_sc"] = unList
	ss.sanctionsLists["eu_consolidated"] = euList
}

// ScreenUser performs comprehensive sanctions screening for a user
func (ss *SanctionsScreener) ScreenUser(ctx context.Context, userID uuid.UUID, userData *aml.AMLUser, screenType string) (*ScreeningEvent, error) {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	screeningEvent := &ScreeningEvent{
		ID:         uuid.New(),
		UserID:     userID,
		ScreenedAt: time.Now(),
		ScreenType: screenType,
		Results:    make([]ScreeningResult, 0),
		Status:     "completed",
		Metadata: map[string]interface{}{
			"user_data": userData,
			"config":    ss.config,
		},
	}

	// Screen against all active sanctions lists
	for _, list := range ss.sanctionsLists {
		if !list.IsActive {
			continue
		}

		// Check if jurisdiction is enabled
		if !ss.isJurisdictionEnabled(list.Jurisdiction) {
			continue
		}

		matches := ss.screenAgainstList(userData, list)
		screeningEvent.Results = append(screeningEvent.Results, matches...)
	}

	// Screen against custom watch lists
	watchListMatches := ss.screenAgainstWatchLists(userData)
	screeningEvent.Results = append(screeningEvent.Results, watchListMatches...)

	// Calculate summary statistics
	screeningEvent.TotalMatches = len(screeningEvent.Results)
	screeningEvent.HighestScore = ss.calculateHighestScore(screeningEvent.Results)

	// Determine overall status
	screeningEvent.Status = ss.determineScreeningStatus(screeningEvent.Results)

	// Update screening history
	ss.updateScreeningHistory(userID, screeningEvent)

	ss.logger.Infow("User screening completed",
		"user_id", userID,
		"screen_type", screenType,
		"total_matches", screeningEvent.TotalMatches,
		"highest_score", screeningEvent.HighestScore,
		"status", screeningEvent.Status,
	)

	return screeningEvent, nil
}

// screenAgainstList screens user data against a specific sanctions list
func (ss *SanctionsScreener) screenAgainstList(userData *aml.AMLUser, list *SanctionsList) []ScreeningResult {
	var results []ScreeningResult

	// Screen against each entry in the list
	for _, entry := range list.Entries {
		if !entry.IsActive {
			continue
		}

		// Check name matches
		nameMatches := ss.checkNameMatches(userData, entry, list)
		results = append(results, nameMatches...)

		// Check identifier matches if configured for enhanced screening
		if ss.config.ScreeningDepth == "enhanced" || ss.config.ScreeningDepth == "comprehensive" {
			idMatches := ss.checkIdentifierMatches(userData, entry, list)
			results = append(results, idMatches...)
		}

		// Check address matches for comprehensive screening
		if ss.config.ScreeningDepth == "comprehensive" {
			addressMatches := ss.checkAddressMatches(userData, entry, list)
			results = append(results, addressMatches...)
		}
	}

	return results
}

// checkNameMatches checks for name matches using various algorithms
func (ss *SanctionsScreener) checkNameMatches(userData *aml.AMLUser, entry *SanctionsEntry, list *SanctionsList) []ScreeningResult {
	var results []ScreeningResult

	// Get user names to check (this would come from user profile data)
	userNames := []string{
		// Would extract from userData - for now using placeholder
		"User Full Name", // This would be userData.FullName or similar
	}

	// Check against primary name and alternate names
	namesToCheck := append([]string{entry.PrimaryName}, entry.AlternateNames...)

	for _, userName := range userNames {
		for _, entryName := range namesToCheck {
			// Exact match
			if ss.isExactMatch(userName, entryName) {
				results = append(results, ScreeningResult{
					ID:            uuid.New(),
					MatchType:     "exact",
					MatchScore:    1.0,
					ListID:        list.ID,
					ListName:      list.Name,
					EntryID:       entry.ID,
					MatchedName:   entryName,
					MatchedField:  "name",
					SanctionsType: entry.SanctionsType,
					RiskLevel:     ss.calculateRiskLevel(1.0),
					ReviewStatus:  "pending",
				})
			}

			// Fuzzy match
			if ss.config.EnableFuzzyMatching {
				fuzzyScore := ss.calculateFuzzyMatch(userName, entryName)
				if fuzzyScore >= ss.config.FuzzyMatchThreshold {
					results = append(results, ScreeningResult{
						ID:            uuid.New(),
						MatchType:     "fuzzy",
						MatchScore:    fuzzyScore,
						ListID:        list.ID,
						ListName:      list.Name,
						EntryID:       entry.ID,
						MatchedName:   entryName,
						MatchedField:  "name",
						SanctionsType: entry.SanctionsType,
						RiskLevel:     ss.calculateRiskLevel(fuzzyScore),
						ReviewStatus:  "pending",
					})
				}
			}

			// Phonetic match
			if ss.config.EnablePhoneticMatch {
				phoneticScore := ss.calculatePhoneticMatch(userName, entryName)
				if phoneticScore >= ss.config.FuzzyMatchThreshold {
					results = append(results, ScreeningResult{
						ID:            uuid.New(),
						MatchType:     "phonetic",
						MatchScore:    phoneticScore,
						ListID:        list.ID,
						ListName:      list.Name,
						EntryID:       entry.ID,
						MatchedName:   entryName,
						MatchedField:  "name",
						SanctionsType: entry.SanctionsType,
						RiskLevel:     ss.calculateRiskLevel(phoneticScore),
						ReviewStatus:  "pending",
					})
				}
			}
		}
	}

	return results
}

// checkIdentifierMatches checks for identifier matches
func (ss *SanctionsScreener) checkIdentifierMatches(userData *aml.AMLUser, entry *SanctionsEntry, list *SanctionsList) []ScreeningResult {
	var results []ScreeningResult

	// This would check passport numbers, national IDs, etc.
	// Implementation depends on available user data structure

	return results
}

// checkAddressMatches checks for address matches
func (ss *SanctionsScreener) checkAddressMatches(userData *aml.AMLUser, entry *SanctionsEntry, list *SanctionsList) []ScreeningResult {
	var results []ScreeningResult

	// This would check user addresses against sanctions entry addresses
	// Implementation depends on available user data structure

	return results
}

// screenAgainstWatchLists screens against custom watch lists
func (ss *SanctionsScreener) screenAgainstWatchLists(userData *aml.AMLUser) []ScreeningResult {
	var results []ScreeningResult

	for _, watchList := range ss.watchLists {
		if !watchList.IsActive {
			continue
		}

		// Screen against watch list entries
		for _, entry := range watchList.Entries {
			if !entry.IsActive || (entry.ExpiryDate != nil && entry.ExpiryDate.Before(time.Now())) {
				continue
			}

			// Simple name matching for watch lists
			if ss.isNameMatch(userData, entry.Name, entry.AlternateNames) {
				results = append(results, ScreeningResult{
					ID:           uuid.New(),
					MatchType:    "watchlist",
					MatchScore:   0.95, // High confidence for watch list matches
					ListID:       watchList.ID,
					ListName:     watchList.Name,
					EntryID:      entry.ID,
					MatchedName:  entry.Name,
					MatchedField: "name",
					RiskLevel:    entry.RiskLevel,
					ReviewStatus: "pending",
				})
			}
		}
	}

	return results
}

// Helper functions

func (ss *SanctionsScreener) isJurisdictionEnabled(jurisdiction string) bool {
	for _, enabled := range ss.config.EnabledJurisdictions {
		if enabled == jurisdiction {
			return true
		}
	}
	return false
}

func (ss *SanctionsScreener) isExactMatch(name1, name2 string) bool {
	return strings.EqualFold(strings.TrimSpace(name1), strings.TrimSpace(name2))
}

func (ss *SanctionsScreener) calculateFuzzyMatch(name1, name2 string) float64 {
	// Simplified fuzzy matching - in practice would use more sophisticated algorithms
	// like Jaro-Winkler, Levenshtein distance, etc.
	if strings.Contains(strings.ToLower(name1), strings.ToLower(name2)) ||
		strings.Contains(strings.ToLower(name2), strings.ToLower(name1)) {
		return 0.85
	}
	return 0.0
}

func (ss *SanctionsScreener) calculatePhoneticMatch(name1, name2 string) float64 {
	// Simplified phonetic matching - in practice would use Soundex, Metaphone, etc.
	return 0.0 // Placeholder
}

func (ss *SanctionsScreener) calculateRiskLevel(score float64) aml.RiskLevel {
	if score >= 0.95 {
		return aml.RiskLevelCritical
	} else if score >= 0.85 {
		return aml.RiskLevelHigh
	} else if score >= 0.75 {
		return aml.RiskLevelMedium
	}
	return aml.RiskLevelLow
}

func (ss *SanctionsScreener) calculateHighestScore(results []ScreeningResult) float64 {
	highest := 0.0
	for _, result := range results {
		if result.MatchScore > highest {
			highest = result.MatchScore
		}
	}
	return highest
}

func (ss *SanctionsScreener) determineScreeningStatus(results []ScreeningResult) string {
	if len(results) == 0 {
		return "clear"
	}

	hasHighRisk := false
	for _, result := range results {
		if result.MatchScore >= ss.config.MatchThreshold {
			return "match"
		}
		if result.RiskLevel == aml.RiskLevelHigh || result.RiskLevel == aml.RiskLevelCritical {
			hasHighRisk = true
		}
	}

	if hasHighRisk {
		return "potential_match"
	}

	return "under_review"
}

func (ss *SanctionsScreener) isNameMatch(userData *aml.AMLUser, primaryName string, alternateNames []string) bool {
	// Simplified implementation - would extract actual user names from userData
	userFullName := "User Full Name" // Placeholder

	if ss.isExactMatch(userFullName, primaryName) {
		return true
	}

	for _, altName := range alternateNames {
		if ss.isExactMatch(userFullName, altName) {
			return true
		}
	}

	return false
}

func (ss *SanctionsScreener) updateScreeningHistory(userID uuid.UUID, event *ScreeningEvent) {
	key := userID.String()
	
	history, exists := ss.screeningHistory[key]
	if !exists {
		history = &ScreeningHistory{
			UserID:          userID,
			ScreeningEvents: make([]ScreeningEvent, 0),
			CurrentStatus:   "clear",
		}
		ss.screeningHistory[key] = history
	}

	// Add new event
	history.ScreeningEvents = append(history.ScreeningEvents, *event)
	history.LastScreened = event.ScreenedAt
	history.TotalScreenings++

	// Update highest risk match
	if event.HighestScore > history.HighestRiskMatch {
		history.HighestRiskMatch = event.HighestScore
	}

	// Update current status
	history.CurrentStatus = event.Status

	// Keep only recent events (configurable retention)
	if len(history.ScreeningEvents) > 1000 { // Keep last 1000 events
		history.ScreeningEvents = history.ScreeningEvents[len(history.ScreeningEvents)-1000:]
	}
}

// GetScreeningHistory returns screening history for a user
func (ss *SanctionsScreener) GetScreeningHistory(userID uuid.UUID) (*ScreeningHistory, error) {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	history, exists := ss.screeningHistory[userID.String()]
	if !exists {
		return nil, fmt.Errorf("no screening history found for user: %s", userID.String())
	}

	return history, nil
}

// AddToWatchList adds an entry to a custom watch list
func (ss *SanctionsScreener) AddToWatchList(watchListID string, entry *WatchListEntry) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	watchList, exists := ss.watchLists[watchListID]
	if !exists {
		return fmt.Errorf("watch list not found: %s", watchListID)
	}

	entry.ID = uuid.New().String()
	entry.WatchListID = watchListID
	entry.AddedAt = time.Now()
	entry.IsActive = true

	watchList.Entries[entry.ID] = entry
	watchList.UpdatedAt = time.Now()

	ss.logger.Infow("Entry added to watch list",
		"watch_list_id", watchListID,
		"entry_id", entry.ID,
		"name", entry.Name,
	)

	return nil
}

// UpdateSanctionsList updates a sanctions list with new data
func (ss *SanctionsScreener) UpdateSanctionsList(ctx context.Context, listID string) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	list, exists := ss.sanctionsLists[listID]
	if !exists {
		return fmt.Errorf("sanctions list not found: %s", listID)
	}

	// In practice, this would download and parse the latest list data
	// For now, just update the timestamp
	list.LastUpdated = time.Now()

	ss.logger.Infow("Sanctions list updated",
		"list_id", listID,
		"list_name", list.Name,
		"last_updated", list.LastUpdated,
	)

	return nil
}
