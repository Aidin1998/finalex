package auth

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// GeoIPService provides geographical IP information and location-based security
type GeoIPService struct {
	logger      *zap.Logger
	geoDatabase *GeoDatabase
	config      *GeoIPConfig
	cache       sync.Map // IP -> *CachedGeoInfo
}

// GeoDatabase represents a geographical IP database
type GeoDatabase struct {
	ipRanges    []*IPRange
	countries   map[string]*CountryInfo
	asns        map[int]*ASNInfo
	vpnRanges   []*IPRange
	torNodes    map[string]*TorNode
	proxyRanges []*IPRange
	lastUpdate  time.Time
	mutex       sync.RWMutex
}

// IPRange represents an IP address range with geographical information
type IPRange struct {
	StartIP      net.IP      `json:"start_ip"`
	EndIP        net.IP      `json:"end_ip"`
	CountryCode  string      `json:"country_code"`
	Country      string      `json:"country"`
	Region       string      `json:"region"`
	City         string      `json:"city"`
	Latitude     float64     `json:"latitude"`
	Longitude    float64     `json:"longitude"`
	ASN          int         `json:"asn"`
	ISP          string      `json:"isp"`
	Organization string      `json:"organization"`
	ServiceType  ServiceType `json:"service_type"`
	ThreatLevel  ThreatLevel `json:"threat_level"`
}

// ServiceType defines the type of internet service
type ServiceType int

const (
	ServiceTypeResidential ServiceType = iota
	ServiceTypeHosting
	ServiceTypeMobile
	ServiceTypeEducational
	ServiceTypeGovernment
	ServiceTypeMilitary
	ServiceTypeVPN
	ServiceTypeProxy
	ServiceTypeTor
	ServiceTypeUnknown
)

// ThreatLevel defines the threat level for a region
type ThreatLevel int

const (
	ThreatLevelLow ThreatLevel = iota
	ThreatLevelMedium
	ThreatLevelHigh
	ThreatLevelCritical
)

// CountryInfo provides information about a country
type CountryInfo struct {
	Code            string      `json:"code"`
	Name            string      `json:"name"`
	Continent       string      `json:"continent"`
	ThreatLevel     ThreatLevel `json:"threat_level"`
	ReputationScore float64     `json:"reputation_score"`
	IsHighRisk      bool        `json:"is_high_risk"`
	IsBlocked       bool        `json:"is_blocked"`
	IsAllowed       bool        `json:"is_allowed"`
	RegionalThreats []string    `json:"regional_threats"`
}

// ASNInfo provides information about an Autonomous System Number
type ASNInfo struct {
	ASN               int         `json:"asn"`
	Organization      string      `json:"organization"`
	Country           string      `json:"country"`
	ServiceType       ServiceType `json:"service_type"`
	ReputationScore   float64     `json:"reputation_score"`
	IsHostingProvider bool        `json:"is_hosting_provider"`
	IsResidential     bool        `json:"is_residential"`
	IsMobile          bool        `json:"is_mobile"`
	IsEducational     bool        `json:"is_educational"`
	IsGovernment      bool        `json:"is_government"`
	IsMilitary        bool        `json:"is_military"`
	ThreatLevel       ThreatLevel `json:"threat_level"`
	LastIncident      time.Time   `json:"last_incident"`
}

// TorNode represents a Tor exit node
type TorNode struct {
	IP          string    `json:"ip"`
	Nickname    string    `json:"nickname"`
	ContactInfo string    `json:"contact_info"`
	ExitPolicy  string    `json:"exit_policy"`
	FirstSeen   time.Time `json:"first_seen"`
	LastSeen    time.Time `json:"last_seen"`
	IsActive    bool      `json:"is_active"`
}

// CachedGeoInfo represents cached geographical information
type CachedGeoInfo struct {
	GeoInfo   *GeoInfo  `json:"geo_info"`
	CachedAt  time.Time `json:"cached_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// NewGeoIPService creates a new GeoIP service
func NewGeoIPService() *GeoIPService {
	config := &GeoIPConfig{
		DatabasePath:           "/data/geoip",
		DatabaseUpdateInterval: time.Hour * 24, // Daily updates
		AllowedCountries:       []string{},
		BlockedCountries:       []string{},
		HighRiskCountries:      []string{"CN", "RU", "IR", "KP", "SY"},
		EnableVPNDetection:     true,
		EnableTorDetection:     true,
		EnableProxyDetection:   true,
		VPNBlockScore:          8.0,
	}

	service := &GeoIPService{
		logger:      zap.NewNop(), // Will be set by the caller
		geoDatabase: NewGeoDatabase(),
		config:      config,
	}

	// Initialize with mock data for demonstration
	service.initializeMockData()

	return service
}

// SetLogger sets the logger for the GeoIP service
func (gis *GeoIPService) SetLogger(logger *zap.Logger) {
	gis.logger = logger
}

// GetGeoInfo retrieves geographical information for an IP address
func (gis *GeoIPService) GetGeoInfo(ctx context.Context, ip string) (*GeoInfo, error) {
	// Check cache first
	if cached, ok := gis.cache.Load(ip); ok {
		cachedInfo := cached.(*CachedGeoInfo)
		if time.Now().Before(cachedInfo.ExpiresAt) {
			return cachedInfo.GeoInfo, nil
		}
		// Remove expired cache entry
		gis.cache.Delete(ip)
	}

	// Parse IP address
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return nil, fmt.Errorf("invalid IP address: %s", ip)
	}

	// Look up geographical information
	geoInfo := gis.lookupIP(parsedIP)

	// Enhance with additional threat intelligence
	gis.enhanceGeoInfo(geoInfo, parsedIP)
	// Cache the result
	cachedInfo := &CachedGeoInfo{
		GeoInfo:   geoInfo,
		CachedAt:  time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour), // Default 24 hour cache
	}
	gis.cache.Store(ip, cachedInfo)

	return geoInfo, nil
}

// lookupIP performs the actual IP geographical lookup
func (gis *GeoIPService) lookupIP(ip net.IP) *GeoInfo {
	gis.geoDatabase.mutex.RLock()
	defer gis.geoDatabase.mutex.RUnlock()

	// Find matching IP range
	for _, ipRange := range gis.geoDatabase.ipRanges {
		if gis.isIPInRange(ip, ipRange) {
			return &GeoInfo{
				Country:      ipRange.Country,
				CountryCode:  ipRange.CountryCode,
				Region:       ipRange.Region,
				City:         ipRange.City,
				Latitude:     ipRange.Latitude,
				Longitude:    ipRange.Longitude,
				ASN:          ipRange.ASN,
				ISP:          ipRange.ISP,
				Organization: ipRange.Organization,
				IsVPN:        ipRange.ServiceType == ServiceTypeVPN,
				IsProxy:      ipRange.ServiceType == ServiceTypeProxy,
				IsTor:        ipRange.ServiceType == ServiceTypeTor,
				IsHosting:    ipRange.ServiceType == ServiceTypeHosting,
				ThreatLevel:  ipRange.ThreatLevel.String(),
			}
		}
	}

	// Return default if no match found
	return &GeoInfo{
		Country:      "Unknown",
		CountryCode:  "XX",
		Region:       "Unknown",
		City:         "Unknown",
		Latitude:     0,
		Longitude:    0,
		ASN:          0,
		ISP:          "Unknown",
		Organization: "Unknown",
		IsVPN:        false,
		IsProxy:      false,
		IsTor:        false,
		IsHosting:    false,
		ThreatLevel:  "medium",
	}
}

// enhanceGeoInfo adds additional threat intelligence to geo information
func (gis *GeoIPService) enhanceGeoInfo(geoInfo *GeoInfo, ip net.IP) {
	// Check for VPN/Proxy services
	if gis.config.EnableVPNDetection {
		geoInfo.IsVPN = gis.isVPN(ip)
	}

	if gis.config.EnableProxyDetection {
		geoInfo.IsProxy = gis.isProxy(ip)
	}

	// Check for Tor exit nodes
	if gis.config.EnableTorDetection {
		geoInfo.IsTor = gis.isTorExitNode(ip)
	}

	// Enhance threat level based on country
	if countryInfo, exists := gis.geoDatabase.countries[geoInfo.CountryCode]; exists {
		geoInfo.ThreatLevel = countryInfo.ThreatLevel.String()

		// Check if country is in high-risk list
		for _, riskCountry := range gis.config.HighRiskCountries {
			if strings.EqualFold(geoInfo.CountryCode, riskCountry) {
				if geoInfo.ThreatLevel == "low" || geoInfo.ThreatLevel == "medium" {
					geoInfo.ThreatLevel = "high"
				}
				break
			}
		}
	}
}

// isIPInRange checks if an IP is within a given range
func (gis *GeoIPService) isIPInRange(ip net.IP, ipRange *IPRange) bool {
	return bytes.Compare(ip, ipRange.StartIP) >= 0 && bytes.Compare(ip, ipRange.EndIP) <= 0
}

// isVPN checks if an IP belongs to a VPN service
func (gis *GeoIPService) isVPN(ip net.IP) bool {
	gis.geoDatabase.mutex.RLock()
	defer gis.geoDatabase.mutex.RUnlock()

	for _, vpnRange := range gis.geoDatabase.vpnRanges {
		if gis.isIPInRange(ip, vpnRange) {
			return true
		}
	}
	return false
}

// isProxy checks if an IP belongs to a proxy service
func (gis *GeoIPService) isProxy(ip net.IP) bool {
	gis.geoDatabase.mutex.RLock()
	defer gis.geoDatabase.mutex.RUnlock()

	for _, proxyRange := range gis.geoDatabase.proxyRanges {
		if gis.isIPInRange(ip, proxyRange) {
			return true
		}
	}
	return false
}

// isTorExitNode checks if an IP is a Tor exit node
func (gis *GeoIPService) isTorExitNode(ip net.IP) bool {
	gis.geoDatabase.mutex.RLock()
	defer gis.geoDatabase.mutex.RUnlock()

	ipStr := ip.String()
	if torNode, exists := gis.geoDatabase.torNodes[ipStr]; exists {
		return torNode.IsActive && time.Since(torNode.LastSeen) < time.Hour*24
	}
	return false
}

// CheckGeographicalRestrictions checks if an IP is geographically restricted
func (gis *GeoIPService) CheckGeographicalRestrictions(ctx context.Context, ip string) (bool, string) { // Check if any country restrictions are configured
	if len(gis.config.AllowedCountries) == 0 && len(gis.config.BlockedCountries) == 0 {
		return false, ""
	}

	geoInfo, err := gis.GetGeoInfo(ctx, ip)
	if err != nil {
		gis.logger.Error("Failed to get geo info for IP", zap.String("ip", ip), zap.Error(err))
		return false, ""
	}

	// Check blocked countries
	for _, blocked := range gis.config.BlockedCountries {
		if strings.EqualFold(geoInfo.CountryCode, blocked) {
			return true, fmt.Sprintf("country_blocked:%s", blocked)
		}
	}

	// Check allowed countries (if list is not empty)
	if len(gis.config.AllowedCountries) > 0 {
		allowed := false
		for _, allowedCountry := range gis.config.AllowedCountries {
			if strings.EqualFold(geoInfo.CountryCode, allowedCountry) {
				allowed = true
				break
			}
		}
		if !allowed {
			return true, fmt.Sprintf("country_not_allowed:%s", geoInfo.CountryCode)
		}
	}

	// Check high-risk countries (may require additional verification)
	for _, riskCountry := range gis.config.HighRiskCountries {
		if strings.EqualFold(geoInfo.CountryCode, riskCountry) {
			return false, fmt.Sprintf("high_risk_country:%s", riskCountry)
		}
	}

	return false, ""
}

// IsHighRiskLocation checks if an IP is from a high-risk location
func (gis *GeoIPService) IsHighRiskLocation(ctx context.Context, ip string) bool {
	geoInfo, err := gis.GetGeoInfo(ctx, ip)
	if err != nil {
		return false
	}

	// Check high-risk countries
	for _, riskCountry := range gis.config.HighRiskCountries {
		if strings.EqualFold(geoInfo.CountryCode, riskCountry) {
			return true
		}
	}

	// Check if it's a high-risk service type
	if geoInfo.IsVPN || geoInfo.IsProxy || geoInfo.IsTor {
		return true
	}

	// Check threat level
	return geoInfo.ThreatLevel == "high" || geoInfo.ThreatLevel == "critical"
}

// GetCountryThreatLevel returns the threat level for a country
func (gis *GeoIPService) GetCountryThreatLevel(countryCode string) ThreatLevel {
	gis.geoDatabase.mutex.RLock()
	defer gis.geoDatabase.mutex.RUnlock()

	if countryInfo, exists := gis.geoDatabase.countries[countryCode]; exists {
		return countryInfo.ThreatLevel
	}

	return ThreatLevelMedium
}

// UpdateGeoDatabase updates the geographical database
func (gis *GeoIPService) UpdateGeoDatabase(ctx context.Context) error {
	// In a real implementation, this would download and parse
	// geographical IP databases from services like MaxMind, IP2Location, etc.

	gis.logger.Info("Updating geo IP database")

	// Mock update process
	gis.geoDatabase.mutex.Lock()
	gis.geoDatabase.lastUpdate = time.Now()
	gis.geoDatabase.mutex.Unlock()

	// Clear cache to force fresh lookups
	gis.cache = sync.Map{}

	gis.logger.Info("Geo IP database updated successfully")
	return nil
}

// GetDatabaseStats returns statistics about the geo database
func (gis *GeoIPService) GetDatabaseStats() map[string]interface{} {
	gis.geoDatabase.mutex.RLock()
	defer gis.geoDatabase.mutex.RUnlock()

	return map[string]interface{}{
		"ip_ranges_count":    len(gis.geoDatabase.ipRanges),
		"countries_count":    len(gis.geoDatabase.countries),
		"asns_count":         len(gis.geoDatabase.asns),
		"vpn_ranges_count":   len(gis.geoDatabase.vpnRanges),
		"tor_nodes_count":    len(gis.geoDatabase.torNodes),
		"proxy_ranges_count": len(gis.geoDatabase.proxyRanges),
		"last_update":        gis.geoDatabase.lastUpdate,
		"cache_entries":      gis.getCacheCount(),
	}
}

// getCacheCount returns the number of cached entries
func (gis *GeoIPService) getCacheCount() int {
	count := 0
	gis.cache.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	return count
}

// NewGeoDatabase creates a new geographical database
func NewGeoDatabase() *GeoDatabase {
	return &GeoDatabase{
		ipRanges:    make([]*IPRange, 0),
		countries:   make(map[string]*CountryInfo),
		asns:        make(map[int]*ASNInfo),
		vpnRanges:   make([]*IPRange, 0),
		torNodes:    make(map[string]*TorNode),
		proxyRanges: make([]*IPRange, 0),
		lastUpdate:  time.Now(),
	}
}

// initializeMockData initializes the database with some mock data for demonstration
func (gis *GeoIPService) initializeMockData() {
	gis.geoDatabase.mutex.Lock()
	defer gis.geoDatabase.mutex.Unlock()

	// Add some mock countries
	gis.geoDatabase.countries["US"] = &CountryInfo{
		Code:            "US",
		Name:            "United States",
		Continent:       "North America",
		ThreatLevel:     ThreatLevelLow,
		ReputationScore: 0.8,
		IsHighRisk:      false,
		IsBlocked:       false,
		IsAllowed:       true,
	}

	gis.geoDatabase.countries["CN"] = &CountryInfo{
		Code:            "CN",
		Name:            "China",
		Continent:       "Asia",
		ThreatLevel:     ThreatLevelHigh,
		ReputationScore: 0.3,
		IsHighRisk:      true,
		IsBlocked:       false,
		IsAllowed:       false,
	}

	gis.geoDatabase.countries["RU"] = &CountryInfo{
		Code:            "RU",
		Name:            "Russia",
		Continent:       "Europe",
		ThreatLevel:     ThreatLevelHigh,
		ReputationScore: 0.2,
		IsHighRisk:      true,
		IsBlocked:       false,
		IsAllowed:       false,
	}

	// Add some mock IP ranges
	startIP := net.ParseIP("192.168.1.0")
	endIP := net.ParseIP("192.168.1.255")
	if startIP != nil && endIP != nil {
		gis.geoDatabase.ipRanges = append(gis.geoDatabase.ipRanges, &IPRange{
			StartIP:      startIP,
			EndIP:        endIP,
			CountryCode:  "US",
			Country:      "United States",
			Region:       "California",
			City:         "San Francisco",
			Latitude:     37.7749,
			Longitude:    -122.4194,
			ASN:          13335,
			ISP:          "Cloudflare",
			Organization: "Cloudflare Inc",
			ServiceType:  ServiceTypeHosting,
			ThreatLevel:  ThreatLevelLow,
		})
	}
}

// String methods for enums
func (st ServiceType) String() string {
	switch st {
	case ServiceTypeResidential:
		return "residential"
	case ServiceTypeHosting:
		return "hosting"
	case ServiceTypeMobile:
		return "mobile"
	case ServiceTypeEducational:
		return "educational"
	case ServiceTypeGovernment:
		return "government"
	case ServiceTypeMilitary:
		return "military"
	case ServiceTypeVPN:
		return "vpn"
	case ServiceTypeProxy:
		return "proxy"
	case ServiceTypeTor:
		return "tor"
	default:
		return "unknown"
	}
}

func (tl ThreatLevel) String() string {
	switch tl {
	case ThreatLevelLow:
		return "low"
	case ThreatLevelMedium:
		return "medium"
	case ThreatLevelHigh:
		return "high"
	case ThreatLevelCritical:
		return "critical"
	default:
		return "unknown"
	}
}
