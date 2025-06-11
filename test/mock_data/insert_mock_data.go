package main

import (
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Account struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey"`
	Email         string
	Username      string
	PasswordHash  string
	FirstName     string
	LastName      string
	Phone         string
	DateOfBirth   time.Time
	Country       string
	Status        string
	KYCLevel      int
	Tier          string
	MFAEnabled    bool
	EmailVerified bool
	PhoneVerified bool
	LastLogin     *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
	Preferences   string
	Roles         string
}

type Reservation struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey"`
	UserID      uuid.UUID
	Currency    string
	Amount      float64
	Type        string
	ReferenceID string
	Status      string
	ExpiresAt   *time.Time
	Version     int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type TransactionJournal struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey"`
	UserID          uuid.UUID
	Currency        string
	Type            string
	Amount          float64
	BalanceBefore   float64
	BalanceAfter    float64
	AvailableBefore float64
	AvailableAfter  float64
	LockedBefore    float64
	LockedAfter     float64
	ReferenceID     string
	Description     string
	Metadata        string
	Status          string
	CreatedAt       time.Time
}

type Order struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey"`
	UserID    uuid.UUID
	Symbol    string
	Side      string
	Price     float64
	Quantity  float64
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Trade struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey"`
	OrderID        uuid.UUID
	CounterOrderID uuid.UUID
	UserID         uuid.UUID
	CounterUserID  uuid.UUID
	Symbol         string
	Side           string
	Price          float64
	Quantity       float64
	Fee            float64
	FeeCurrency    string
	CreatedAt      time.Time
}

type LedgerTransaction struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey"`
	AccountID       uuid.UUID
	TransactionType string
	Amount          float64
	BalanceBefore   float64
	BalanceAfter    float64
	ReferenceID     *uuid.UUID
	ReferenceType   string
	Description     string
	Metadata        string
	CreatedAt       time.Time
	CreatedBy       *uuid.UUID
}

type BalanceSnapshot struct {
	ID               uuid.UUID `gorm:"type:uuid;primaryKey"`
	AccountID        uuid.UUID
	Balance          float64
	AvailableBalance float64
	PendingBalance   float64
	SnapshotAt       time.Time
	SnapshotType     string
	CreatedAt        time.Time
}

type AuditLog struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey"`
	EntityType   string
	EntityID     uuid.UUID
	Action       string
	OldValues    string
	NewValues    string
	ChangedFields string
	UserID       *uuid.UUID
	IPAddress    string
	UserAgent    string
	CreatedAt    time.Time
}

type ComplianceAlert struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey"`
	UserID     string
	Type       string
	Severity   string
	Message    string
	Status     string
	AssignedTo string
	Notes      string
	Metadata   string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type AMLUser struct {
	ID                uuid.UUID `gorm:"type:uuid;primaryKey"`
	UserID            uuid.UUID
	RiskLevel         string
	RiskScore         float64
	KYCStatus         string
	Blacklisted       bool
	Whitelisted       bool
	LastRiskUpdate    time.Time
	CountryCode       string
	HighRiskCountry   bool
	PEPStatus         bool
	SanctionStatus    bool
	CustomerType      string
	BusinessType      string
	RiskFactors       string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func main() {
	dsn := os.Getenv("SUPABASE_DSN")
	if dsn == "" {
		dsn = "postgresql://postgres.aiywrixoivhazskkisjz:Aidin!98@aws-0-ap-southeast-1.pooler.supabase.com:6543/postgres"
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		fmt.Printf("Failed to connect to Supabase: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Connected to Supabase!")

	// Insert multiple mock accounts
	accounts := []Account{
		{ID: uuid.New(), Email: "alice@example.com", Username: "alice", PasswordHash: "pw1", FirstName: "Alice", LastName: "Smith", Phone: "+1234567890", DateOfBirth: time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC), Country: "US", Status: "active", KYCLevel: 2, Tier: "premium", MFAEnabled: true, EmailVerified: true, PhoneVerified: true, CreatedAt: time.Now(), UpdatedAt: time.Now(), Preferences: "{}", Roles: "[\"user\"]"},
		{ID: uuid.New(), Email: "bob@example.com", Username: "bob", PasswordHash: "pw2", FirstName: "Bob", LastName: "Johnson", Phone: "+1987654321", DateOfBirth: time.Date(1985, 5, 15, 0, 0, 0, 0, time.UTC), Country: "GB", Status: "pending_verification", KYCLevel: 1, Tier: "basic", MFAEnabled: false, EmailVerified: false, PhoneVerified: false, CreatedAt: time.Now(), UpdatedAt: time.Now(), Preferences: "{}", Roles: "[\"user\"]"},
		{ID: uuid.New(), Email: "carol@example.com", Username: "carol", PasswordHash: "pw3", FirstName: "Carol", LastName: "Williams", Phone: "+1122334455", DateOfBirth: time.Date(1978, 9, 23, 0, 0, 0, 0, time.UTC), Country: "CA", Status: "suspended", KYCLevel: 3, Tier: "vip", MFAEnabled: true, EmailVerified: true, PhoneVerified: false, CreatedAt: time.Now(), UpdatedAt: time.Now(), Preferences: "{}", Roles: "[\"admin\"]"},