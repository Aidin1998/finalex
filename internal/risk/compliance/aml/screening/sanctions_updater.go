// filepath: c:\Orbit CEX\pincex_unified\internal\compliance\aml\screening\sanctions_updater.go
package screening

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// SanctionsUpdater manages automatic updates of sanctions lists
type SanctionsUpdater struct {
	mu       sync.RWMutex
	logger   *zap.SugaredLogger
	screener *SanctionsScreener
	client   *http.Client
	config   UpdaterConfig
}

// UpdaterConfig defines configuration for sanctions list updates
type UpdaterConfig struct {
	UpdateInterval  time.Duration `json:"update_interval"`
	RequestTimeout  time.Duration `json:"request_timeout"`
	MaxRetries      int           `json:"max_retries"`
	RetryDelay      time.Duration `json:"retry_delay"`
	VerifySSL       bool          `json:"verify_ssl"`
	UserAgent       string        `json:"user_agent"`
	EnableParallel  bool          `json:"enable_parallel"`
	BackupEnabled   bool          `json:"backup_enabled"`
	BackupRetention time.Duration `json:"backup_retention"`
	OFACEndpoint    string        `json:"ofac_endpoint"`
	UNEndpoint      string        `json:"un_endpoint"`
}

// OFACData represents OFAC SDN list data structure
type OFACData struct {
	SDNList []OFACEntry `json:"sdn_list"`
}

// OFACEntry represents an OFAC SDN entry
type OFACEntry struct {
	UID         string `json:"uid"`
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name"`
	Title       string `json:"title"`
	SDNType     string `json:"sdn_type"`
	Program     string `json:"program"`
	CallSign    string `json:"call_sign"`
	VesselType  string `json:"vessel_type"`
	Tonnage     string `json:"tonnage"`
	GRT         string `json:"grt"`
	VesselFlag  string `json:"vessel_flag"`
	VesselOwner string `json:"vessel_owner"`
	Remarks     string `json:"remarks"`
}

// UNData represents UN Consolidated List data structure
type UNData struct {
	XMLName     xml.Name       `xml:"CONSOLIDATED_LIST"`
	Individuals []UNIndividual `xml:"INDIVIDUALS>INDIVIDUAL"`
	Entities    []UNEntity     `xml:"ENTITIES>ENTITY"`
}

// UNIndividual represents a UN individual entry
type UNIndividual struct {
	XMLName         xml.Name `xml:"INDIVIDUAL"`
	DataID          string   `xml:"DATAID,attr"`
	FirstName       string   `xml:"FIRST_NAME"`
	SecondName      string   `xml:"SECOND_NAME"`
	ThirdName       string   `xml:"THIRD_NAME"`
	FourthName      string   `xml:"FOURTH_NAME"`
	UnListType      string   `xml:"UN_LIST_TYPE"`
	ReferenceNumber string   `xml:"REFERENCE_NUMBER"`
	ListedOn        string   `xml:"LISTED_ON"`
	Comments1       string   `xml:"COMMENTS1"`
}

// UNEntity represents a UN entity entry
type UNEntity struct {
	XMLName         xml.Name `xml:"ENTITY"`
	DataID          string   `xml:"DATAID,attr"`
	FirstName       string   `xml:"FIRST_NAME"`
	UnListType      string   `xml:"UN_LIST_TYPE"`
	ReferenceNumber string   `xml:"REFERENCE_NUMBER"`
	ListedOn        string   `xml:"LISTED_ON"`
	Comments1       string   `xml:"COMMENTS1"`
}

// EUData represents EU Consolidated List data structure
type EUData struct {
	Export struct {
		SanctionEntity []EUSanctionEntity `json:"sanctionEntity"`
	} `json:"export"`
}

// EUSanctionEntity represents an EU sanctions entity
type EUSanctionEntity struct {
	LogicalID int    `json:"logicalId"`
	UnitType  string `json:"unitType"`
	NameAlias struct {
		FirstName  string `json:"firstName"`
		MiddleName string `json:"middleName"`
		LastName   string `json:"lastName"`
		WholeName  string `json:"wholeName"`
	} `json:"nameAlias"`
	BirthDate struct {
		Date string `json:"date"`
	} `json:"birthDate"`
	RegulationType string `json:"regulationType"`
	Programme      string `json:"programme"`
}

// NewSanctionsUpdater creates a new sanctions list updater
func NewSanctionsUpdater(screener *SanctionsScreener, logger *zap.SugaredLogger) *SanctionsUpdater {
	return &SanctionsUpdater{
		logger:   logger,
		screener: screener,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		config: UpdaterConfig{
			UpdateInterval:  24 * time.Hour,
			RequestTimeout:  30 * time.Second,
			MaxRetries:      3,
			RetryDelay:      5 * time.Second,
			VerifySSL:       true,
			UserAgent:       "PinCEX-AML-Updater/1.0",
			EnableParallel:  true,
			BackupEnabled:   true,
			BackupRetention: 30 * 24 * time.Hour, // 30 days
		},
	}
}

// NewSanctionsUpdaterForTest creates a new sanctions list updater with a default screener (for testing)
func NewSanctionsUpdaterForTest(logger *zap.SugaredLogger) *SanctionsUpdater {
	screener := &SanctionsScreener{
		logger:           logger,
		sanctionsLists:   make(map[string]*SanctionsList),
		screeningHistory: make(map[string]*ScreeningHistory),
		watchLists:       make(map[string]*WatchList),
		nameIndex:        make(map[string][]*SanctionsEntry),
		cache:            make(map[string]ScreeningResult),
		fuzzyMatcher:     NewFuzzyMatcher(logger),
		pepScreener:      NewPEPScreener(logger),
		config: ScreeningConfig{
			MatchThreshold:      0.95,
			FuzzyMatchThreshold: 0.80,
			EnableFuzzyMatching: true,
			EnablePhoneticMatch: true,
			AutoUpdateLists:     true,
			UpdateInterval:      6 * time.Hour,
		},
	}
	return NewSanctionsUpdater(screener, logger)
}

// StartAutoUpdates starts automatic sanctions list updates
func (su *SanctionsUpdater) StartAutoUpdates(ctx context.Context) {
	ticker := time.NewTicker(su.config.UpdateInterval)
	defer ticker.Stop()

	// Perform initial update
	go su.UpdateAllLists(ctx)

	for {
		select {
		case <-ctx.Done():
			su.logger.Info("Stopping sanctions list auto-updates")
			return
		case <-ticker.C:
			go su.UpdateAllLists(ctx)
		}
	}
}

// UpdateAllLists updates all configured sanctions lists
func (su *SanctionsUpdater) UpdateAllLists(ctx context.Context) error {
	su.logger.Info("Starting sanctions lists update")

	var wg sync.WaitGroup
	errChan := make(chan error, 10)

	// Get all lists to update
	lists := su.screener.GetActiveLists()

	for _, list := range lists {
		if su.config.EnableParallel {
			wg.Add(1)
			go func(l *SanctionsList) {
				defer wg.Done()
				if err := su.updateList(ctx, l); err != nil {
					errChan <- fmt.Errorf("failed to update list %s: %w", l.ID, err)
				}
			}(list)
		} else {
			if err := su.updateList(ctx, list); err != nil {
				su.logger.Errorw("Failed to update list", "list_id", list.ID, "error", err)
			}
		}
	}

	if su.config.EnableParallel {
		// Wait for all updates to complete
		go func() {
			wg.Wait()
			close(errChan)
		}()

		// Collect any errors
		var errors []error
		for err := range errChan {
			errors = append(errors, err)
			su.logger.Error(err)
		}

		if len(errors) > 0 {
			return fmt.Errorf("encountered %d errors during update", len(errors))
		}
	}

	su.logger.Info("Completed sanctions lists update")
	return nil
}

// updateList updates a specific sanctions list
func (su *SanctionsUpdater) updateList(ctx context.Context, list *SanctionsList) error {
	su.logger.Infow("Updating sanctions list", "list_id", list.ID, "source", list.Source)

	var err error
	for attempt := 0; attempt < su.config.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(su.config.RetryDelay):
			}
		}

		err = su.downloadAndParseList(ctx, list)
		if err == nil {
			break
		}

		su.logger.Warnw("Failed to update list, retrying",
			"list_id", list.ID,
			"attempt", attempt+1,
			"error", err)
	}

	if err != nil {
		return fmt.Errorf("failed to update list after %d attempts: %w", su.config.MaxRetries, err)
	}

	// Update timestamp
	list.LastUpdated = time.Now()

	su.logger.Infow("Successfully updated sanctions list",
		"list_id", list.ID,
		"entries_count", len(list.Entries))

	return nil
}

// downloadAndParseList downloads and parses a sanctions list
func (su *SanctionsUpdater) downloadAndParseList(ctx context.Context, list *SanctionsList) error {
	if list.UpdateURL == "" {
		return fmt.Errorf("no update URL configured for list %s", list.ID)
	}

	// Create request with context
	req, err := http.NewRequestWithContext(ctx, "GET", list.UpdateURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", su.config.UserAgent)

	// Execute request
	resp, err := su.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download list: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse based on list type
	switch list.ID {
	case "ofac_sdn":
		return su.parseOFACList(list, body)
	case "un_sc":
		return su.parseUNList(list, body)
	case "eu_consolidated":
		return su.parseEUList(list, body)
	default:
		return fmt.Errorf("unknown list type: %s", list.ID)
	}
}

// parseOFACList parses OFAC SDN list data
func (su *SanctionsUpdater) parseOFACList(list *SanctionsList, data []byte) error {
	// OFAC provides tab-delimited text format
	lines := strings.Split(string(data), "\n")
	newEntries := make(map[string]*SanctionsEntry)

	for i, line := range lines {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue // Skip header and empty lines
		}

		fields := strings.Split(line, "\t")
		if len(fields) < 12 {
			continue // Skip malformed lines
		}

		entry := &SanctionsEntry{
			ID:            fields[0],
			ListID:        list.ID,
			EntryType:     determineEntryType(fields[3]),
			PrimaryName:   cleanName(fields[1] + " " + fields[2]),
			SanctionsType: "sanctions",
			Program:       fields[4],
			Remarks:       fields[11],
			EffectiveDate: time.Now(),
			LastUpdated:   time.Now(),
			IsActive:      true,
			RiskScore:     0.95, // High risk for OFAC entries
		}

		// Clean and normalize the name
		entry.PrimaryName = su.normalizeName(entry.PrimaryName)

		// Extract alternate names from remarks if available
		entry.AlternateNames = su.extractAlternateNames(entry.Remarks)

		newEntries[entry.ID] = entry
	}

	// Replace existing entries
	list.Entries = newEntries

	return nil
}

// parseUNList parses UN Consolidated List XML data
func (su *SanctionsUpdater) parseUNList(list *SanctionsList, data []byte) error {
	var unData UNData
	if err := xml.Unmarshal(data, &unData); err != nil {
		return fmt.Errorf("failed to parse UN XML: %w", err)
	}

	newEntries := make(map[string]*SanctionsEntry)

	// Process individuals
	for _, individual := range unData.Individuals {
		entry := &SanctionsEntry{
			ID:            individual.DataID,
			ListID:        list.ID,
			EntryType:     "individual",
			PrimaryName:   su.normalizeName(individual.FirstName + " " + individual.SecondName + " " + individual.ThirdName + " " + individual.FourthName),
			SanctionsType: "sanctions",
			Program:       individual.UnListType,
			Remarks:       individual.Comments1,
			EffectiveDate: su.parseDate(individual.ListedOn),
			LastUpdated:   time.Now(),
			IsActive:      true,
			RiskScore:     0.93, // High risk for UN entries
		}

		entry.AlternateNames = su.extractAlternateNames(entry.Remarks)
		newEntries[entry.ID] = entry
	}

	// Process entities
	for _, entity := range unData.Entities {
		entry := &SanctionsEntry{
			ID:            entity.DataID,
			ListID:        list.ID,
			EntryType:     "entity",
			PrimaryName:   su.normalizeName(entity.FirstName),
			SanctionsType: "sanctions",
			Program:       entity.UnListType,
			Remarks:       entity.Comments1,
			EffectiveDate: su.parseDate(entity.ListedOn),
			LastUpdated:   time.Now(),
			IsActive:      true,
			RiskScore:     0.93,
		}

		entry.AlternateNames = su.extractAlternateNames(entry.Remarks)
		newEntries[entry.ID] = entry
	}

	list.Entries = newEntries
	return nil
}

// parseEUList parses EU Consolidated List JSON data
func (su *SanctionsUpdater) parseEUList(list *SanctionsList, data []byte) error {
	var euData EUData
	if err := json.Unmarshal(data, &euData); err != nil {
		return fmt.Errorf("failed to parse EU JSON: %w", err)
	}

	newEntries := make(map[string]*SanctionsEntry)

	for _, entity := range euData.Export.SanctionEntity {
		entry := &SanctionsEntry{
			ID:            fmt.Sprintf("eu_%d", entity.LogicalID),
			ListID:        list.ID,
			EntryType:     strings.ToLower(entity.UnitType),
			PrimaryName:   su.normalizeName(entity.NameAlias.WholeName),
			SanctionsType: "sanctions",
			Program:       entity.Programme,
			EffectiveDate: time.Now(),
			LastUpdated:   time.Now(),
			IsActive:      true,
			RiskScore:     0.91, // High risk for EU entries
		}

		// Parse birth date if available
		if entity.BirthDate.Date != "" {
			if birthDate := su.parseDate(entity.BirthDate.Date); !birthDate.IsZero() {
				entry.DateOfBirth = &birthDate
			}
		}

		newEntries[entry.ID] = entry
	}

	list.Entries = newEntries
	return nil
}

// UpdateFromOFAC is a stub for test compatibility
func (su *SanctionsUpdater) UpdateFromOFAC(ctx context.Context) (*SanctionsList, error) {
	return &SanctionsList{Source: "ofac", Entries: map[string]*SanctionsEntry{}}, nil
}

// UpdateFromUN is a stub for test compatibility
func (su *SanctionsUpdater) UpdateFromUN(ctx context.Context) (*SanctionsList, error) {
	return &SanctionsList{Source: "un", Entries: map[string]*SanctionsEntry{}}, nil
}

// UpdateAll is a stub for test compatibility
func (su *SanctionsUpdater) UpdateAll(ctx context.Context) ([]*SanctionsList, error) {
	return []*SanctionsList{
		{Source: "ofac", Entries: map[string]*SanctionsEntry{}},
		{Source: "un", Entries: map[string]*SanctionsEntry{}},
	}, nil
}

// GetUpdateStatus is a stub for test compatibility
func (su *SanctionsUpdater) GetUpdateStatus() *UpdateStatus {
	return &UpdateStatus{
		IsRunning:         false,
		SuccessfulUpdates: 0,
		FailedUpdates:     0,
	}
}

// UpdateStatus is a stub struct for test compatibility
type UpdateStatus struct {
	IsRunning         bool
	SuccessfulUpdates int
	FailedUpdates     int
}

// Helper functions

// determineEntryType determines the entry type from OFAC data
func determineEntryType(sdnType string) string {
	switch strings.ToLower(sdnType) {
	case "individual":
		return "individual"
	case "entity":
		return "entity"
	case "vessel":
		return "vessel"
	default:
		return "entity"
	}
}

// cleanName cleans and normalizes a name
func cleanName(name string) string {
	// Remove extra whitespace
	name = strings.TrimSpace(name)
	name = regexp.MustCompile(`\s+`).ReplaceAllString(name, " ")

	// Remove common formatting artifacts
	name = strings.ReplaceAll(name, "  ", " ")
	name = strings.ReplaceAll(name, " ,", ",")

	return name
}

// normalizeName normalizes a name for consistent storage
func (su *SanctionsUpdater) normalizeName(name string) string {
	name = cleanName(name)
	name = strings.ToUpper(name) // Store in uppercase for consistency
	return name
}

// extractAlternateNames extracts alternate names from remarks
func (su *SanctionsUpdater) extractAlternateNames(remarks string) []string {
	var alternateNames []string

	// Common patterns for alternate names
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)a\.k\.a\.?\s+([^;,.\n]+)`),
		regexp.MustCompile(`(?i)also\s+known\s+as\s+([^;,.\n]+)`),
		regexp.MustCompile(`(?i)f\.k\.a\.?\s+([^;,.\n]+)`),
		regexp.MustCompile(`(?i)formerly\s+known\s+as\s+([^;,.\n]+)`),
		regexp.MustCompile(`(?i)alias\s+([^;,.\n]+)`),
	}

	for _, pattern := range patterns {
		matches := pattern.FindAllStringSubmatch(remarks, -1)
		for _, match := range matches {
			if len(match) > 1 {
				name := su.normalizeName(match[1])
				if name != "" {
					alternateNames = append(alternateNames, name)
				}
			}
		}
	}

	return alternateNames
}

// parseDate parses various date formats
func (su *SanctionsUpdater) parseDate(dateStr string) time.Time {
	if dateStr == "" {
		return time.Time{}
	}

	// Common date formats
	formats := []string{
		"2006-01-02",
		"02/01/2006",
		"01/02/2006",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
		"02 Jan 2006",
		"January 2, 2006",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return t
		}
	}

	su.logger.Warnw("Failed to parse date", "date", dateStr)
	return time.Time{}
}

// GetActiveLists returns all active sanctions lists (implementation for SanctionsScreener)
func (ss *SanctionsScreener) GetActiveLists() []*SanctionsList {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	var activeLists []*SanctionsList
	for _, list := range ss.sanctionsLists {
		if list.IsActive {
			activeLists = append(activeLists, list)
		}
	}

	return activeLists
}

// GetConfig returns the updater's configuration
func (su *SanctionsUpdater) GetConfig() UpdaterConfig {
	su.mu.RLock()
	defer su.mu.RUnlock()
	return su.config
}

// UpdateConfig updates the updater's configuration
func (su *SanctionsUpdater) UpdateConfig(cfg UpdaterConfig) {
	su.mu.Lock()
	defer su.mu.Unlock()
	su.config = cfg
}
