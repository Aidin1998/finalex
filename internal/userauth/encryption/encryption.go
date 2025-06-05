package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"golang.org/x/crypto/pbkdf2"
)

// PIIEncryptionService provides enterprise-grade encryption for PII data
type PIIEncryptionService struct {
	masterKey     []byte
	keyDerivation KeyDerivationService
}

// NewPIIEncryptionService creates a new PII encryption service
func NewPIIEncryptionService(masterKey string) *PIIEncryptionService {
	// In production, this should come from a secure key management service
	hashedKey := sha256.Sum256([]byte(masterKey))
	return &PIIEncryptionService{
		masterKey:     hashedKey[:],
		keyDerivation: NewPBKDF2KeyDerivation(),
	}
}

// EncryptedData represents encrypted data with metadata
type EncryptedData struct {
	Data      string `json:"data"`
	Nonce     string `json:"nonce"`
	Salt      string `json:"salt"`
	Algorithm string `json:"algorithm"`
	KeyID     string `json:"key_id"`
}

// EncryptPII encrypts PII data using AES-GCM with unique keys per user
func (pes *PIIEncryptionService) EncryptPII(data map[string]interface{}) (encrypted, keyRef string, err error) {
	// Generate unique salt for this data
	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		return "", "", fmt.Errorf("salt generation failed: %w", err)
	}

	// Derive unique encryption key using PBKDF2
	dataKey := pbkdf2.Key(pes.masterKey, salt, 100000, 32, sha256.New)

	// Create AES-GCM cipher
	block, err := aes.NewCipher(dataKey)
	if err != nil {
		return "", "", fmt.Errorf("cipher creation failed: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", "", fmt.Errorf("GCM creation failed: %w", err)
	}

	// Generate nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", "", fmt.Errorf("nonce generation failed: %w", err)
	}

	// Serialize data to JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", "", fmt.Errorf("JSON serialization failed: %w", err)
	}

	// Encrypt data
	ciphertext := gcm.Seal(nil, nonce, jsonData, nil)

	// Create encrypted data structure
	encData := EncryptedData{
		Data:      base64.StdEncoding.EncodeToString(ciphertext),
		Nonce:     base64.StdEncoding.EncodeToString(nonce),
		Salt:      base64.StdEncoding.EncodeToString(salt),
		Algorithm: "AES-256-GCM",
		KeyID:     generateKeyID(salt),
	}

	// Serialize encrypted data
	encryptedJSON, err := json.Marshal(encData)
	if err != nil {
		return "", "", fmt.Errorf("encrypted data serialization failed: %w", err)
	}

	encryptedB64 := base64.StdEncoding.EncodeToString(encryptedJSON)
	return encryptedB64, encData.KeyID, nil
}

// DecryptPII decrypts PII data
func (pes *PIIEncryptionService) DecryptPII(encryptedData string) (map[string]interface{}, error) {
	// Decode base64
	encryptedJSON, err := base64.StdEncoding.DecodeString(encryptedData)
	if err != nil {
		return nil, fmt.Errorf("base64 decode failed: %w", err)
	}

	// Deserialize encrypted data
	var encData EncryptedData
	if err := json.Unmarshal(encryptedJSON, &encData); err != nil {
		return nil, fmt.Errorf("encrypted data deserialization failed: %w", err)
	}

	// Decode components
	ciphertext, err := base64.StdEncoding.DecodeString(encData.Data)
	if err != nil {
		return nil, fmt.Errorf("ciphertext decode failed: %w", err)
	}

	nonce, err := base64.StdEncoding.DecodeString(encData.Nonce)
	if err != nil {
		return nil, fmt.Errorf("nonce decode failed: %w", err)
	}

	salt, err := base64.StdEncoding.DecodeString(encData.Salt)
	if err != nil {
		return nil, fmt.Errorf("salt decode failed: %w", err)
	}

	// Derive the same key
	dataKey := pbkdf2.Key(pes.masterKey, salt, 100000, 32, sha256.New)

	// Create cipher
	block, err := aes.NewCipher(dataKey)
	if err != nil {
		return nil, fmt.Errorf("cipher creation failed: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("GCM creation failed: %w", err)
	}

	// Decrypt
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	// Deserialize data
	var data map[string]interface{}
	if err := json.Unmarshal(plaintext, &data); err != nil {
		return nil, fmt.Errorf("data deserialization failed: %w", err)
	}

	return data, nil
}

// Encrypt encrypts simple string data
func (pes *PIIEncryptionService) Encrypt(data string) (string, error) {
	// Generate salt
	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("salt generation failed: %w", err)
	}

	// Derive key
	dataKey := pbkdf2.Key(pes.masterKey, salt, 100000, 32, sha256.New)

	// Create cipher
	block, err := aes.NewCipher(dataKey)
	if err != nil {
		return "", fmt.Errorf("cipher creation failed: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("GCM creation failed: %w", err)
	}

	// Generate nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("nonce generation failed: %w", err)
	}

	// Encrypt
	ciphertext := gcm.Seal(nil, nonce, []byte(data), nil)

	// Create encrypted data structure
	encData := EncryptedData{
		Data:      base64.StdEncoding.EncodeToString(ciphertext),
		Nonce:     base64.StdEncoding.EncodeToString(nonce),
		Salt:      base64.StdEncoding.EncodeToString(salt),
		Algorithm: "AES-256-GCM",
		KeyID:     generateKeyID(salt),
	}

	// Serialize and encode
	encryptedJSON, err := json.Marshal(encData)
	if err != nil {
		return "", fmt.Errorf("encrypted data serialization failed: %w", err)
	}

	return base64.StdEncoding.EncodeToString(encryptedJSON), nil
}

// Decrypt decrypts simple string data
func (pes *PIIEncryptionService) Decrypt(encryptedData string) (string, error) {
	// Decode base64
	encryptedJSON, err := base64.StdEncoding.DecodeString(encryptedData)
	if err != nil {
		return "", fmt.Errorf("base64 decode failed: %w", err)
	}

	// Deserialize
	var encData EncryptedData
	if err := json.Unmarshal(encryptedJSON, &encData); err != nil {
		return "", fmt.Errorf("encrypted data deserialization failed: %w", err)
	}

	// Decode components
	ciphertext, err := base64.StdEncoding.DecodeString(encData.Data)
	if err != nil {
		return "", fmt.Errorf("ciphertext decode failed: %w", err)
	}

	nonce, err := base64.StdEncoding.DecodeString(encData.Nonce)
	if err != nil {
		return "", fmt.Errorf("nonce decode failed: %w", err)
	}

	salt, err := base64.StdEncoding.DecodeString(encData.Salt)
	if err != nil {
		return "", fmt.Errorf("salt decode failed: %w", err)
	}

	// Derive key
	dataKey := pbkdf2.Key(pes.masterKey, salt, 100000, 32, sha256.New)

	// Create cipher
	block, err := aes.NewCipher(dataKey)
	if err != nil {
		return "", fmt.Errorf("cipher creation failed: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("GCM creation failed: %w", err)
	}

	// Decrypt
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decryption failed: %w", err)
	}

	return string(plaintext), nil
}

// EncryptFieldsInPlace encrypts specific fields in a struct in place
func (pes *PIIEncryptionService) EncryptFieldsInPlace(data map[string]interface{}, fields []string) error {
	for _, field := range fields {
		if value, exists := data[field]; exists {
			if strValue, ok := value.(string); ok && strValue != "" {
				encrypted, err := pes.Encrypt(strValue)
				if err != nil {
					return fmt.Errorf("failed to encrypt field %s: %w", field, err)
				}
				data[field] = encrypted
			}
		}
	}
	return nil
}

// DecryptFieldsInPlace decrypts specific fields in a struct in place
func (pes *PIIEncryptionService) DecryptFieldsInPlace(data map[string]interface{}, fields []string) error {
	for _, field := range fields {
		if value, exists := data[field]; exists {
			if strValue, ok := value.(string); ok && strValue != "" {
				decrypted, err := pes.Decrypt(strValue)
				if err != nil {
					return fmt.Errorf("failed to decrypt field %s: %w", field, err)
				}
				data[field] = decrypted
			}
		}
	}
	return nil
}

// KeyDerivationService interface for key derivation
type KeyDerivationService interface {
	DeriveKey(password, salt []byte, iterations, keyLen int) []byte
}

// PBKDF2KeyDerivation implements PBKDF2 key derivation
type PBKDF2KeyDerivation struct{}

// NewPBKDF2KeyDerivation creates a new PBKDF2 key derivation service
func NewPBKDF2KeyDerivation() *PBKDF2KeyDerivation {
	return &PBKDF2KeyDerivation{}
}

// DeriveKey derives a key using PBKDF2
func (kd *PBKDF2KeyDerivation) DeriveKey(password, salt []byte, iterations, keyLen int) []byte {
	return pbkdf2.Key(password, salt, iterations, keyLen, sha256.New)
}

// generateKeyID generates a unique key ID from salt
func generateKeyID(salt []byte) string {
	hash := sha256.Sum256(salt)
	return fmt.Sprintf("key_%x", hash[:8])
}

// RotateEncryption re-encrypts data with a new key (for key rotation)
func (pes *PIIEncryptionService) RotateEncryption(oldEncryptedData string, newMasterKey []byte) (string, error) {
	// Decrypt with old key
	oldMasterKey := pes.masterKey
	decrypted, err := pes.DecryptPII(oldEncryptedData)
	if err != nil {
		return "", fmt.Errorf("decryption with old key failed: %w", err)
	}

	// Set new master key
	pes.masterKey = newMasterKey

	// Re-encrypt with new key
	newEncrypted, _, err := pes.EncryptPII(decrypted)
	if err != nil {
		// Restore old key on failure
		pes.masterKey = oldMasterKey
		return "", fmt.Errorf("encryption with new key failed: %w", err)
	}

	return newEncrypted, nil
}

// GetEncryptionMetadata extracts metadata from encrypted data without decrypting
func (pes *PIIEncryptionService) GetEncryptionMetadata(encryptedData string) (*EncryptionMetadata, error) {
	encryptedJSON, err := base64.StdEncoding.DecodeString(encryptedData)
	if err != nil {
		return nil, fmt.Errorf("base64 decode failed: %w", err)
	}

	var encData EncryptedData
	if err := json.Unmarshal(encryptedJSON, &encData); err != nil {
		return nil, fmt.Errorf("encrypted data deserialization failed: %w", err)
	}

	return &EncryptionMetadata{
		Algorithm: encData.Algorithm,
		KeyID:     encData.KeyID,
	}, nil
}

// EncryptionMetadata represents metadata about encrypted data
type EncryptionMetadata struct {
	Algorithm string `json:"algorithm"`
	KeyID     string `json:"key_id"`
}

// SecureWipe securely wipes sensitive data from memory
func SecureWipe(data []byte) {
	for i := range data {
		data[i] = 0
	}
}

// ValidateEncryptedData validates that encrypted data is properly formatted
func (pes *PIIEncryptionService) ValidateEncryptedData(encryptedData string) error {
	_, err := pes.GetEncryptionMetadata(encryptedData)
	return err
}
