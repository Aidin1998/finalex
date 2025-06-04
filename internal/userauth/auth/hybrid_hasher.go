package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"time"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
)

// HybridHasher provides multiple hashing algorithms based on security requirements
type HybridHasher struct {
	securityManager *EndpointSecurityManager

	// Bcrypt configuration
	bcryptCost int

	// Argon2 configuration
	argon2Time    uint32
	argon2Memory  uint32
	argon2Threads uint8
	argon2KeyLen  uint32
	argon2SaltLen int
}

// HashedCredential represents a hashed credential with metadata
type HashedCredential struct {
	Hash      string        `json:"hash"`
	Algorithm HashAlgorithm `json:"algorithm"`
	Salt      string        `json:"salt,omitempty"`
	CreatedAt time.Time     `json:"created_at"`
	Cost      int           `json:"cost,omitempty"`    // For bcrypt
	Time      uint32        `json:"time,omitempty"`    // For Argon2
	Memory    uint32        `json:"memory,omitempty"`  // For Argon2
	Threads   uint8         `json:"threads,omitempty"` // For Argon2
	KeyLen    uint32        `json:"key_len,omitempty"` // For Argon2
}

// NewHybridHasher creates a new hybrid hasher
func NewHybridHasher(securityManager *EndpointSecurityManager) *HybridHasher {
	return &HybridHasher{
		securityManager: securityManager,
		bcryptCost:      12, // Secure default
		argon2Time:      1,
		argon2Memory:    64 * 1024, // 64 MB
		argon2Threads:   4,
		argon2KeyLen:    32,
		argon2SaltLen:   16,
	}
}

// HashForEndpoint hashes a credential using the appropriate algorithm for the endpoint
func (hh *HybridHasher) HashForEndpoint(credential, endpoint string) (*HashedCredential, error) {
	algorithm := hh.securityManager.GetRequiredHashAlgorithm(endpoint)
	return hh.Hash(credential, algorithm)
}

// Hash hashes a credential using the specified algorithm
func (hh *HybridHasher) Hash(credential string, algorithm HashAlgorithm) (*HashedCredential, error) {
	switch algorithm {
	case HashSHA256:
		return hh.hashSHA256(credential)
	case HashBcrypt:
		return hh.hashBcrypt(credential)
	case HashArgon2:
		return hh.hashArgon2(credential)
	default:
		return nil, fmt.Errorf("unsupported hash algorithm: %v", algorithm)
	}
}

// VerifyForEndpoint verifies a credential against a hash using endpoint-appropriate algorithm
func (hh *HybridHasher) VerifyForEndpoint(credential, endpoint string, stored *HashedCredential) (bool, error) {
	// For critical trading endpoints, always use the fastest algorithm (SHA256) for verification
	// unless the stored hash explicitly uses a different algorithm
	if hh.securityManager.IsCriticalTradingEndpoint(endpoint) && stored.Algorithm == HashSHA256 {
		return hh.verifySHA256(credential, stored)
	}

	return hh.Verify(credential, stored)
}

// Verify verifies a credential against a stored hash
func (hh *HybridHasher) Verify(credential string, stored *HashedCredential) (bool, error) {
	switch stored.Algorithm {
	case HashSHA256:
		return hh.verifySHA256(credential, stored)
	case HashBcrypt:
		return hh.verifyBcrypt(credential, stored)
	case HashArgon2:
		return hh.verifyArgon2(credential, stored)
	default:
		return false, fmt.Errorf("unsupported hash algorithm: %v", stored.Algorithm)
	}
}

// hashSHA256 provides fast hashing for critical trading operations
func (hh *HybridHasher) hashSHA256(credential string) (*HashedCredential, error) {
	hash := sha256.Sum256([]byte(credential))

	return &HashedCredential{
		Hash:      hex.EncodeToString(hash[:]),
		Algorithm: HashSHA256,
		CreatedAt: time.Now(),
	}, nil
}

// verifySHA256 provides fast verification for critical trading operations
func (hh *HybridHasher) verifySHA256(credential string, stored *HashedCredential) (bool, error) {
	hash := sha256.Sum256([]byte(credential))
	expectedHash := hex.EncodeToString(hash[:])

	// Use constant-time comparison to prevent timing attacks
	return subtle.ConstantTimeCompare([]byte(expectedHash), []byte(stored.Hash)) == 1, nil
}

// hashBcrypt provides secure hashing for user management operations
func (hh *HybridHasher) hashBcrypt(credential string) (*HashedCredential, error) {
	hashBytes, err := bcrypt.GenerateFromPassword([]byte(credential), hh.bcryptCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash with bcrypt: %w", err)
	}

	return &HashedCredential{
		Hash:      string(hashBytes),
		Algorithm: HashBcrypt,
		CreatedAt: time.Now(),
		Cost:      hh.bcryptCost,
	}, nil
}

// verifyBcrypt provides secure verification for user management operations
func (hh *HybridHasher) verifyBcrypt(credential string, stored *HashedCredential) (bool, error) {
	err := bcrypt.CompareHashAndPassword([]byte(stored.Hash), []byte(credential))
	if err != nil {
		if err == bcrypt.ErrMismatchedHashAndPassword {
			return false, nil
		}
		return false, fmt.Errorf("failed to verify bcrypt hash: %w", err)
	}
	return true, nil
}

// hashArgon2 provides maximum security hashing for admin operations
func (hh *HybridHasher) hashArgon2(credential string) (*HashedCredential, error) {
	salt := make([]byte, hh.argon2SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}

	hash := argon2.IDKey(
		[]byte(credential),
		salt,
		hh.argon2Time,
		hh.argon2Memory,
		hh.argon2Threads,
		hh.argon2KeyLen,
	)

	return &HashedCredential{
		Hash:      hex.EncodeToString(hash),
		Algorithm: HashArgon2,
		Salt:      hex.EncodeToString(salt),
		CreatedAt: time.Now(),
		Time:      hh.argon2Time,
		Memory:    hh.argon2Memory,
		Threads:   hh.argon2Threads,
		KeyLen:    hh.argon2KeyLen,
	}, nil
}

// verifyArgon2 provides maximum security verification for admin operations
func (hh *HybridHasher) verifyArgon2(credential string, stored *HashedCredential) (bool, error) {
	salt, err := hex.DecodeString(stored.Salt)
	if err != nil {
		return false, fmt.Errorf("failed to decode salt: %w", err)
	}

	// Use stored parameters for verification
	time := stored.Time
	memory := stored.Memory
	threads := stored.Threads
	keyLen := stored.KeyLen

	// Use defaults if not stored (backward compatibility)
	if time == 0 {
		time = hh.argon2Time
	}
	if memory == 0 {
		memory = hh.argon2Memory
	}
	if threads == 0 {
		threads = hh.argon2Threads
	}
	if keyLen == 0 {
		keyLen = hh.argon2KeyLen
	}

	hash := argon2.IDKey(
		[]byte(credential),
		salt,
		time,
		memory,
		threads,
		keyLen,
	)

	expectedHash := hex.EncodeToString(hash)

	// Use constant-time comparison to prevent timing attacks
	return subtle.ConstantTimeCompare([]byte(expectedHash), []byte(stored.Hash)) == 1, nil
}

// UpgradeHash upgrades a hash to a more secure algorithm if needed
func (hh *HybridHasher) UpgradeHash(credential, endpoint string, stored *HashedCredential) (*HashedCredential, bool, error) {
	requiredAlgorithm := hh.securityManager.GetRequiredHashAlgorithm(endpoint)

	// No upgrade needed if already using the required algorithm
	if stored.Algorithm == requiredAlgorithm {
		return stored, false, nil
	}

	// For critical trading endpoints, never upgrade from SHA256 to maintain performance
	if hh.securityManager.IsCriticalTradingEndpoint(endpoint) && stored.Algorithm == HashSHA256 {
		return stored, false, nil
	}

	// Verify the credential first
	valid, err := hh.Verify(credential, stored)
	if err != nil {
		return nil, false, fmt.Errorf("failed to verify credential for upgrade: %w", err)
	}
	if !valid {
		return nil, false, fmt.Errorf("invalid credential for hash upgrade")
	}

	// Create new hash with required algorithm
	newHash, err := hh.Hash(credential, requiredAlgorithm)
	if err != nil {
		return nil, false, fmt.Errorf("failed to create upgraded hash: %w", err)
	}

	return newHash, true, nil
}

// GetHashingStats returns statistics about hashing performance
func (hh *HybridHasher) GetHashingStats() map[string]interface{} {
	return map[string]interface{}{
		"bcrypt_cost":     hh.bcryptCost,
		"argon2_time":     hh.argon2Time,
		"argon2_memory":   hh.argon2Memory,
		"argon2_threads":  hh.argon2Threads,
		"argon2_key_len":  hh.argon2KeyLen,
		"argon2_salt_len": hh.argon2SaltLen,
	}
}

// SetBcryptCost updates the bcrypt cost factor
func (hh *HybridHasher) SetBcryptCost(cost int) error {
	if cost < 4 || cost > 31 {
		return fmt.Errorf("bcrypt cost must be between 4 and 31")
	}
	hh.bcryptCost = cost
	return nil
}

// SetArgon2Params updates Argon2 parameters
func (hh *HybridHasher) SetArgon2Params(time, memory uint32, threads uint8, keyLen uint32) error {
	if time == 0 || memory == 0 || threads == 0 || keyLen == 0 {
		return fmt.Errorf("argon2 parameters must be greater than 0")
	}

	hh.argon2Time = time
	hh.argon2Memory = memory
	hh.argon2Threads = threads
	hh.argon2KeyLen = keyLen

	return nil
}
