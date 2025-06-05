package compliance

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/userauth/models"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ComplianceService provides enterprise-grade compliance management
type ComplianceService struct {
	db           *gorm.DB
	logger       *zap.Logger
	geoService   GeolocationService
	sanctionList SanctionCheckService
	kycProvider  KYCProvider
}

// NewComplianceService creates a new compliance service
func NewComplianceService(
	db *gorm.DB,
	logger *zap.Logger,
	geoService GeolocationService,
	sanctionList SanctionCheckService,
	kycProvider KYCProvider,
) *ComplianceService {
	return &ComplianceService{
		db:           db,
		logger:       logger,
		geoService:   geoService,
		sanctionList: sanctionList,
		kycProvider:  kycProvider,
	}
}

// ComplianceRequest represents a compliance check request
type ComplianceRequest struct {
	Email           string                 `json:"email"`
	FirstName       string                 `json:"first_name"`
	LastName        string                 `json:"last_name"`
	Country         string                 `json:"country"`
	IPAddress       string                 `json:"ip_address"`
	DateOfBirth     *time.Time             `json:"date_of_birth"`
	PhoneNumber     string                 `json:"phone_number"`
	GeolocationData map[string]interface{} `json:"geolocation_data"`
}

// ComplianceResult represents the result of compliance checks
type ComplianceResult struct {
	Blocked                 bool              `json:"blocked"`
	Reason                  string            `json:"reason,omitempty"`
	KYCRequired             bool              `json:"kyc_required"`
	RequiredKYCLevel        string            `json:"required_kyc_level"`
	InitialTier             string            `json:"initial_tier"`
	Flags                   []string          `json:"flags"`
	RiskScore               float64           `json:"risk_score"`
	RestrictedJurisdictions []string          `json:"restricted_jurisdictions"`
	RequiredDocuments       []string          `json:"required_documents"`
	ComplianceRequirements  map[string]string `json:"compliance_requirements"`
	RetentionPeriod         time.Duration     `json:"retention_period"`
	DataProcessingConsent   bool              `json:"data_processing_consent"`
}

// PerformRegistrationChecks performs comprehensive compliance checks for user registration
func (cs *ComplianceService) PerformRegistrationChecks(ctx context.Context, req *ComplianceRequest) (*ComplianceResult, error) {
	result := &ComplianceResult{
		Flags:                  []string{},
		ComplianceRequirements: make(map[string]string),
		RetentionPeriod:        7 * 365 * 24 * time.Hour, // Default 7 years
		DataProcessingConsent:  true,
	}

	// 1. Geolocation and Jurisdiction Check
	if err := cs.performJurisdictionCheck(ctx, req, result); err != nil {
		return nil, fmt.Errorf("jurisdiction check failed: %w", err)
	}

	// 2. Sanctions and Watchlist Check
	if err := cs.performSanctionsCheck(ctx, req, result); err != nil {
		return nil, fmt.Errorf("sanctions check failed: %w", err)
	}

	// 3. Age Verification and Legal Capacity
	if err := cs.performAgeVerification(req, result); err != nil {
		return nil, fmt.Errorf("age verification failed: %w", err)
	}

	// 4. Risk Assessment
	if err := cs.performRiskAssessment(ctx, req, result); err != nil {
		return nil, fmt.Errorf("risk assessment failed: %w", err)
	}

	// 5. KYC Requirements Determination
	cs.determineKYCRequirements(req, result)

	// 6. Data Privacy Compliance (GDPR, CCPA, etc.)
	cs.setDataPrivacyRequirements(req, result)

	// 7. AML/CTF Compliance
	cs.setAMLRequirements(req, result)

	// 8. Final Risk Score Calculation
	cs.calculateFinalRiskScore(result)

	// Log compliance check
	cs.logComplianceCheck(ctx, req, result)

	return result, nil
}

// performJurisdictionCheck checks if user's jurisdiction is allowed
func (cs *ComplianceService) performJurisdictionCheck(ctx context.Context, req *ComplianceRequest, result *ComplianceResult) error {
	// Get geolocation from IP
	geoInfo, err := cs.geoService.GetLocationInfo(req.IPAddress)
	if err != nil {
		cs.logger.Warn("Geolocation lookup failed", zap.Error(err))
		result.Flags = append(result.Flags, "geolocation_failed")
	}

	// Check restricted jurisdictions
	restrictedCountries := cs.getRestrictedJurisdictions()

	// Check declared country
	if cs.isRestrictedJurisdiction(req.Country, restrictedCountries) {
		result.Blocked = true
		result.Reason = fmt.Sprintf("Services not available in %s", req.Country)
		result.RestrictedJurisdictions = append(result.RestrictedJurisdictions, req.Country)
		return nil
	}

	// Check IP geolocation country (if available)
	if geoInfo != nil && geoInfo.Country != "" {
		if cs.isRestrictedJurisdiction(geoInfo.Country, restrictedCountries) {
			result.Blocked = true
			result.Reason = fmt.Sprintf("Access from %s is not permitted", geoInfo.Country)
			result.RestrictedJurisdictions = append(result.RestrictedJurisdictions, geoInfo.Country)
			return nil
		}

		// Flag if declared country doesn't match IP country
		if geoInfo.Country != req.Country {
			result.Flags = append(result.Flags, "country_mismatch")
			result.RiskScore += 20.0
		}
	}

	// Set jurisdiction-specific requirements
	cs.setJurisdictionRequirements(req.Country, result)

	return nil
}

// performSanctionsCheck checks against sanctions and watchlists
func (cs *ComplianceService) performSanctionsCheck(ctx context.Context, req *ComplianceRequest, result *ComplianceResult) error {
	sanctionResult, err := cs.sanctionList.CheckPerson(ctx, &SanctionCheckRequest{
		FirstName:   req.FirstName,
		LastName:    req.LastName,
		DateOfBirth: req.DateOfBirth,
		Country:     req.Country,
	})
	if err != nil {
		cs.logger.Error("Sanctions check failed", zap.Error(err))
		result.Flags = append(result.Flags, "sanctions_check_failed")
		return nil // Don't block on technical failure, but flag it
	}

	if sanctionResult.IsMatch {
		result.Blocked = true
		result.Reason = "User appears on sanctions or watchlist"
		result.Flags = append(result.Flags, "sanctions_match")
		result.RiskScore = 100.0
		return nil
	}

	if sanctionResult.IsPossibleMatch {
		result.Flags = append(result.Flags, "sanctions_possible_match")
		result.RiskScore += 50.0
		result.KYCRequired = true
		result.RequiredKYCLevel = "enhanced"
	}

	return nil
}

// performAgeVerification verifies user meets minimum age requirements
func (cs *ComplianceService) performAgeVerification(req *ComplianceRequest, result *ComplianceResult) error {
	if req.DateOfBirth == nil {
		result.Flags = append(result.Flags, "missing_date_of_birth")
		return fmt.Errorf("date of birth is required")
	}

	age := time.Since(*req.DateOfBirth).Hours() / 24 / 365.25
	minAge := cs.getMinimumAge(req.Country)

	if age < minAge {
		result.Blocked = true
		result.Reason = fmt.Sprintf("Minimum age requirement (%d years) not met", int(minAge))
		return nil
	}

	// Flag users close to minimum age for enhanced verification
	if age < minAge+2 {
		result.Flags = append(result.Flags, "young_user")
		result.KYCRequired = true
	}

	return nil
}

// performRiskAssessment performs initial risk assessment
func (cs *ComplianceService) performRiskAssessment(ctx context.Context, req *ComplianceRequest, result *ComplianceResult) error {
	riskScore := 0.0

	// Country risk
	countryRisk := cs.getCountryRiskScore(req.Country)
	riskScore += countryRisk

	// Email domain risk
	emailDomain := strings.Split(req.Email, "@")[1]
	if cs.isHighRiskEmailDomain(emailDomain) {
		riskScore += 15.0
		result.Flags = append(result.Flags, "high_risk_email_domain")
	}

	// Disposable email check
	if cs.isDisposableEmail(emailDomain) {
		riskScore += 25.0
		result.Flags = append(result.Flags, "disposable_email")
	}

	// VPN/Proxy detection
	if req.GeolocationData != nil {
		if vpn, ok := req.GeolocationData["is_vpn"].(bool); ok && vpn {
			riskScore += 20.0
			result.Flags = append(result.Flags, "vpn_detected")
		}
		if proxy, ok := req.GeolocationData["is_proxy"].(bool); ok && proxy {
			riskScore += 20.0
			result.Flags = append(result.Flags, "proxy_detected")
		}
	}

	result.RiskScore = riskScore

	return nil
}

// determineKYCRequirements determines KYC requirements based on risk and jurisdiction
func (cs *ComplianceService) determineKYCRequirements(req *ComplianceRequest, result *ComplianceResult) {
	kycLevel := "basic"

	// Enhanced KYC for high-risk countries
	if cs.isHighRiskCountry(req.Country) {
		kycLevel = "enhanced"
		result.KYCRequired = true
	}

	// Enhanced KYC for high risk scores
	if result.RiskScore >= 50.0 {
		kycLevel = "enhanced"
		result.KYCRequired = true
	}

	// US customers require enhanced KYC
	if req.Country == "USA" {
		kycLevel = "enhanced"
		result.KYCRequired = true
	}

	// EU customers require GDPR-compliant KYC
	if cs.isEUCountry(req.Country) {
		result.KYCRequired = true
		result.ComplianceRequirements["gdpr"] = "full_compliance"
	}

	result.RequiredKYCLevel = kycLevel

	// Set required documents based on KYC level
	result.RequiredDocuments = cs.getRequiredDocuments(kycLevel, req.Country)

	// Set initial tier based on KYC requirements
	if result.KYCRequired {
		result.InitialTier = "basic"
	} else {
		result.InitialTier = "premium"
	}
}

// setDataPrivacyRequirements sets data privacy compliance requirements
func (cs *ComplianceService) setDataPrivacyRequirements(req *ComplianceRequest, result *ComplianceResult) {
	// GDPR requirements for EU residents
	if cs.isEUCountry(req.Country) {
		result.ComplianceRequirements["gdpr"] = "full_compliance"
		result.ComplianceRequirements["data_subject_rights"] = "enabled"
		result.ComplianceRequirements["consent_management"] = "required"
		result.RetentionPeriod = 6 * 365 * 24 * time.Hour // 6 years for EU
	}

	// CCPA requirements for California residents
	if req.Country == "USA" {
		result.ComplianceRequirements["ccpa"] = "applicable"
		result.ComplianceRequirements["data_sale_opt_out"] = "enabled"
	}

	// Other US state privacy laws
	if req.Country == "USA" {
		result.ComplianceRequirements["privacy_rights"] = "us_applicable"
	}

	// Canada PIPEDA
	if req.Country == "CAN" {
		result.ComplianceRequirements["pipeda"] = "applicable"
	}
}

// setAMLRequirements sets Anti-Money Laundering requirements
func (cs *ComplianceService) setAMLRequirements(req *ComplianceRequest, result *ComplianceResult) {
	// Enhanced due diligence for high-risk countries
	if cs.isHighRiskCountry(req.Country) {
		result.ComplianceRequirements["enhanced_due_diligence"] = "required"
		result.ComplianceRequirements["ongoing_monitoring"] = "enhanced"
	}

	// PEP screening for all users
	result.ComplianceRequirements["pep_screening"] = "required"

	// Transaction monitoring thresholds
	if cs.isHighRiskCountry(req.Country) {
		result.ComplianceRequirements["transaction_threshold"] = "low"
	} else {
		result.ComplianceRequirements["transaction_threshold"] = "standard"
	}
}

// calculateFinalRiskScore calculates the final risk score
func (cs *ComplianceService) calculateFinalRiskScore(result *ComplianceResult) {
	// Cap risk score at 100
	if result.RiskScore > 100.0 {
		result.RiskScore = 100.0
	}

	// Block users with extremely high risk scores
	if result.RiskScore >= 90.0 && !result.Blocked {
		result.Blocked = true
		result.Reason = "Risk score too high for onboarding"
	}
}

// Helper functions

func (cs *ComplianceService) getRestrictedJurisdictions() []string {
	// OFAC and other restricted jurisdictions
	return []string{
		"IRN", "PRK", "SYR", "CUB", // OFAC primary sanctions
		"AFG", "MMR", "BLR", // Additional restricted
	}
}

func (cs *ComplianceService) isRestrictedJurisdiction(country string, restricted []string) bool {
	for _, r := range restricted {
		if country == r {
			return true
		}
	}
	return false
}

func (cs *ComplianceService) getMinimumAge(country string) float64 {
	// Age requirements by country
	switch country {
	case "USA", "GBR", "CAN", "AUS":
		return 18.0
	case "KOR":
		return 19.0
	case "JPN":
		return 20.0
	default:
		return 18.0
	}
}

func (cs *ComplianceService) getCountryRiskScore(country string) float64 {
	// FATF and other risk assessments
	highRiskCountries := map[string]float64{
		"AFG": 50.0, "IRN": 50.0, "PRK": 50.0,
		"PAK": 30.0, "TUR": 25.0, "RUS": 40.0,
		"CHN": 20.0, "VEN": 35.0,
	}

	if score, exists := highRiskCountries[country]; exists {
		return score
	}
	return 0.0
}

func (cs *ComplianceService) isHighRiskCountry(country string) bool {
	return cs.getCountryRiskScore(country) >= 25.0
}

func (cs *ComplianceService) isEUCountry(country string) bool {
	euCountries := []string{
		"AUT", "BEL", "BGR", "HRV", "CYP", "CZE", "DNK", "EST",
		"FIN", "FRA", "DEU", "GRC", "HUN", "IRL", "ITA", "LVA",
		"LTU", "LUX", "MLT", "NLD", "POL", "PRT", "ROU", "SVK",
		"SVN", "ESP", "SWE",
	}

	for _, eu := range euCountries {
		if country == eu {
			return true
		}
	}
	return false
}

func (cs *ComplianceService) isHighRiskEmailDomain(domain string) bool {
	highRiskDomains := []string{
		"guerrillamail.com", "10minutemail.com", "tempmail.org",
	}

	for _, risky := range highRiskDomains {
		if domain == risky {
			return true
		}
	}
	return false
}

func (cs *ComplianceService) isDisposableEmail(domain string) bool {
	// This would typically check against a comprehensive list
	disposableDomains := []string{
		"10minutemail.com", "guerrillamail.com", "tempmail.org",
		"mailinator.com", "yopmail.com",
	}

	for _, disposable := range disposableDomains {
		if domain == disposable {
			return true
		}
	}
	return false
}

func (cs *ComplianceService) getRequiredDocuments(kycLevel, country string) []string {
	basic := []string{"government_id", "proof_of_address"}
	enhanced := []string{"government_id", "proof_of_address", "selfie_with_id", "bank_statement"}

	if kycLevel == "enhanced" {
		return enhanced
	}
	return basic
}

func (cs *ComplianceService) setJurisdictionRequirements(country string, result *ComplianceResult) {
	switch country {
	case "USA":
		result.ComplianceRequirements["patriot_act"] = "required"
		result.ComplianceRequirements["bsa"] = "applicable"
		result.ComplianceRequirements["state_licensing"] = "check_required"
	case "GBR":
		result.ComplianceRequirements["fca_compliance"] = "required"
		result.ComplianceRequirements["mlr"] = "applicable"
	case "DEU":
		result.ComplianceRequirements["bafin"] = "required"
		result.ComplianceRequirements["geldwäschegesetz"] = "applicable"
	}
}

func (cs *ComplianceService) logComplianceCheck(ctx context.Context, req *ComplianceRequest, result *ComplianceResult) {
	auditData := map[string]interface{}{
		"email":                   req.Email,
		"country":                 req.Country,
		"risk_score":              result.RiskScore,
		"blocked":                 result.Blocked,
		"kyc_required":            result.KYCRequired,
		"flags":                   result.Flags,
		"compliance_requirements": result.ComplianceRequirements,
	}

	eventData, _ := json.Marshal(auditData)

	auditLog := &models.UserAuditLog{
		ID:            uuid.New(),
		EventType:     "compliance_check",
		EventCategory: "compliance",
		EventSeverity: "info",
		IPAddress:     req.IPAddress,
		EventData:     string(eventData),
		ProcessedBy:   "compliance_service",
		CorrelationID: uuid.New().String(),
		CreatedAt:     time.Now(),
	}

	if result.Blocked {
		auditLog.EventSeverity = "warning"
	}

	if err := cs.db.Create(auditLog).Error; err != nil {
		cs.logger.Error("Failed to log compliance check", zap.Error(err))
	}
}

// External service interfaces

type GeolocationService interface {
	GetLocationInfo(ip string) (*GeolocationInfo, error)
}

type GeolocationInfo struct {
	Country   string  `json:"country"`
	Region    string  `json:"region"`
	City      string  `json:"city"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	ISP       string  `json:"isp"`
	VPN       bool    `json:"vpn"`
	Proxy     bool    `json:"proxy"`
}

type SanctionCheckService interface {
	CheckPerson(ctx context.Context, req *SanctionCheckRequest) (*SanctionCheckResult, error)
}

type SanctionCheckRequest struct {
	FirstName   string     `json:"first_name"`
	LastName    string     `json:"last_name"`
	DateOfBirth *time.Time `json:"date_of_birth"`
	Country     string     `json:"country"`
}

type SanctionCheckResult struct {
	IsMatch         bool     `json:"is_match"`
	IsPossibleMatch bool     `json:"is_possible_match"`
	MatchDetails    string   `json:"match_details"`
	ListSources     []string `json:"list_sources"`
}

type KYCProvider interface {
	GetKYCRequirements(country string) (*KYCRequirements, error)
}

type KYCRequirements struct {
	Level             string   `json:"level"`
	RequiredDocuments []string `json:"required_documents"`
	VerificationSteps []string `json:"verification_steps"`
}

// Data Export and Deletion (GDPR/CCPA Compliance)

// RequestDataExport creates a data export request for GDPR/CCPA compliance
func (cs *ComplianceService) RequestDataExport(ctx context.Context, userID uuid.UUID, requestType string, requestedBy uuid.UUID) (*models.UserDataExport, error) {
	export := &models.UserDataExport{
		ID:             uuid.New(),
		UserID:         userID,
		RequestType:    requestType,
		Status:         "pending",
		RequestedBy:    requestedBy,
		ExportFormat:   "json",
		IncludeDeleted: false,
		DataCategories: `["profile", "transactions", "login_history", "kyc_documents"]`,
		LegalBasis:     "gdpr_article_20", // Right to data portability
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	if err := cs.db.Create(export).Error; err != nil {
		return nil, fmt.Errorf("failed to create data export request: %w", err)
	}

	// Start async processing
	go cs.processDataExport(context.Background(), export)

	return export, nil
}

// processDataExport processes a data export request asynchronously
func (cs *ComplianceService) processDataExport(ctx context.Context, export *models.UserDataExport) {
	// Update status to processing
	export.Status = "processing"
	cs.db.Save(export)

	// This would collect all user data according to the request
	// Implementation would involve querying all relevant tables and creating a comprehensive export

	cs.logger.Info("Processing data export request",
		zap.String("export_id", export.ID.String()),
		zap.String("user_id", export.UserID.String()),
		zap.String("request_type", export.RequestType))

	// Mark as completed (simplified for this example)
	export.Status = "completed"
	completedAt := time.Now()
	export.CompletedAt = &completedAt
	cs.db.Save(export)
}
