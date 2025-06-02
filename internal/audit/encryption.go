package audit

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/pbkdf2"
)

// EncryptionService provides encryption/decryption capabilities for audit data
type EncryptionService struct {
	masterKey []byte
	gcm       cipher.AEAD
}

// NewEncryptionService creates a new encryption service
func NewEncryptionService(masterKey string) (*EncryptionService, error) {
	// Derive a 32-byte key from the master key using PBKDF2
	salt := []byte("pincex_audit_salt") // In production, use a random salt per installation
	key := pbkdf2.Key([]byte(masterKey), salt, 10000, 32, sha256.New)

	// Create AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	return &EncryptionService{
		masterKey: key,
		gcm:       gcm,
	}, nil
}

// EncryptedData represents encrypted data with metadata
type EncryptedData struct {
	Data      string `json:"data"`      // Base64 encoded encrypted data
	Nonce     string `json:"nonce"`     // Base64 encoded nonce
	Algorithm string `json:"algorithm"` // Encryption algorithm used
	Version   int    `json:"version"`   // Encryption version for key rotation
}

// Encrypt encrypts the given data and returns encrypted data structure
func (e *EncryptionService) Encrypt(data []byte) (*EncryptedData, error) {
	// Generate a random nonce
	nonce := make([]byte, e.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt the data
	ciphertext := e.gcm.Seal(nil, nonce, data, nil)

	return &EncryptedData{
		Data:      base64.StdEncoding.EncodeToString(ciphertext),
		Nonce:     base64.StdEncoding.EncodeToString(nonce),
		Algorithm: "AES-256-GCM",
		Version:   1,
	}, nil
}

// Decrypt decrypts the given encrypted data
func (e *EncryptionService) Decrypt(encData *EncryptedData) ([]byte, error) {
	// Decode the base64 data
	ciphertext, err := base64.StdEncoding.DecodeString(encData.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode ciphertext: %w", err)
	}

	nonce, err := base64.StdEncoding.DecodeString(encData.Nonce)
	if err != nil {
		return nil, fmt.Errorf("failed to decode nonce: %w", err)
	}

	// Verify algorithm and version
	if encData.Algorithm != "AES-256-GCM" {
		return nil, fmt.Errorf("unsupported encryption algorithm: %s", encData.Algorithm)
	}

	if encData.Version != 1 {
		return nil, fmt.Errorf("unsupported encryption version: %d", encData.Version)
	}

	// Decrypt the data
	plaintext, err := e.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt data: %w", err)
	}

	return plaintext, nil
}

// EncryptJSON encrypts a JSON-serializable object
func (e *EncryptionService) EncryptJSON(obj interface{}) (*EncryptedData, error) {
	data, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal object: %w", err)
	}

	return e.Encrypt(data)
}

// DecryptJSON decrypts data and unmarshals it into the given object
func (e *EncryptionService) DecryptJSON(encData *EncryptedData, obj interface{}) error {
	data, err := e.Decrypt(encData)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(data, obj); err != nil {
		return fmt.Errorf("failed to unmarshal object: %w", err)
	}

	return nil
}

// EncryptString encrypts a string and returns base64 encoded result
func (e *EncryptionService) EncryptString(text string) (string, error) {
	encData, err := e.Encrypt([]byte(text))
	if err != nil {
		return "", err
	}

	// Serialize the encrypted data structure
	serialized, err := json.Marshal(encData)
	if err != nil {
		return "", fmt.Errorf("failed to serialize encrypted data: %w", err)
	}

	return base64.StdEncoding.EncodeToString(serialized), nil
}

// DecryptString decrypts a base64 encoded encrypted string
func (e *EncryptionService) DecryptString(encryptedText string) (string, error) {
	// Decode the base64 serialized data
	serialized, err := base64.StdEncoding.DecodeString(encryptedText)
	if err != nil {
		return "", fmt.Errorf("failed to decode encrypted text: %w", err)
	}

	// Deserialize the encrypted data structure
	var encData EncryptedData
	if err := json.Unmarshal(serialized, &encData); err != nil {
		return "", fmt.Errorf("failed to deserialize encrypted data: %w", err)
	}

	// Decrypt the data
	data, err := e.Decrypt(&encData)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

// SensitiveDataProcessor handles encryption/decryption of sensitive audit data
type SensitiveDataProcessor struct {
	encService *EncryptionService
}

// NewSensitiveDataProcessor creates a new sensitive data processor
func NewSensitiveDataProcessor(encService *EncryptionService) *SensitiveDataProcessor {
	return &SensitiveDataProcessor{
		encService: encService,
	}
}

// ProcessRequestData sanitizes and encrypts sensitive request data
func (p *SensitiveDataProcessor) ProcessRequestData(data map[string]interface{}) (map[string]interface{}, error) {
	// Clone the data to avoid modifying the original
	processed := make(map[string]interface{})
	for k, v := range data {
		processed[k] = v
	}

	// List of sensitive fields to encrypt
	sensitiveFields := map[string]bool{
		"password":        true,
		"new_password":    true,
		"old_password":    true,
		"private_key":     true,
		"secret_key":      true,
		"api_secret":      true,
		"seed_phrase":     true,
		"mnemonic":        true,
		"pin":             true,
		"otp":             true,
		"totp":            true,
		"backup_code":     true,
		"recovery_code":   true,
		"ssn":             true,
		"tax_id":          true,
		"passport":        true,
		"drivers_license": true,
		"credit_card":     true,
		"bank_account":    true,
		"routing_number":  true,
		"iban":            true,
		"swift_code":      true,
	}

	// Fields to completely remove (too sensitive to store even encrypted)
	removeFields := map[string]bool{
		"authorization": true,
		"cookie":        true,
		"x-auth-token":  true,
		"bearer":        true,
	}

	// Process each field
	for key, value := range processed {
		lowerKey := strings.ToLower(key)

		if removeFields[lowerKey] {
			// Remove completely sensitive fields
			processed[key] = "[REDACTED]"
		} else if sensitiveFields[lowerKey] {
			// Encrypt sensitive fields
			if strVal, ok := value.(string); ok && strVal != "" {
				encrypted, err := p.encService.EncryptString(strVal)
				if err != nil {
					return nil, fmt.Errorf("failed to encrypt field %s: %w", key, err)
				}
				processed[key] = map[string]interface{}{
					"encrypted": true,
					"data":      encrypted,
				}
			}
		} else if strings.Contains(lowerKey, "password") || strings.Contains(lowerKey, "secret") || strings.Contains(lowerKey, "key") {
			// Catch any password/secret/key fields not explicitly listed
			processed[key] = "[REDACTED]"
		}
	}

	return processed, nil
}

// ProcessResponseData sanitizes response data for audit logging
func (p *SensitiveDataProcessor) ProcessResponseData(data map[string]interface{}) map[string]interface{} {
	// Clone the data
	processed := make(map[string]interface{})
	for k, v := range data {
		processed[k] = v
	}

	// Fields to remove from response data
	removeFields := map[string]bool{
		"password":       true,
		"private_key":    true,
		"secret_key":     true,
		"api_secret":     true,
		"access_token":   true,
		"refresh_token":  true,
		"session_id":     true,
		"csrf_token":     true,
		"backup_codes":   true,
		"recovery_codes": true,
		"seed_phrase":    true,
		"mnemonic":       true,
	}

	// Process each field
	for key, value := range processed {
		lowerKey := strings.ToLower(key)

		if removeFields[lowerKey] {
			processed[key] = "[REDACTED]"
		} else if strings.Contains(lowerKey, "token") || strings.Contains(lowerKey, "secret") || strings.Contains(lowerKey, "key") {
			// Catch any token/secret/key fields not explicitly listed
			processed[key] = "[REDACTED]"
		}
	}

	return processed
}

// DecryptAuditData decrypts encrypted fields in audit data for authorized access
func (p *SensitiveDataProcessor) DecryptAuditData(data map[string]interface{}) (map[string]interface{}, error) {
	decrypted := make(map[string]interface{})

	for key, value := range data {
		if encField, ok := value.(map[string]interface{}); ok {
			if encrypted, exists := encField["encrypted"]; exists && encrypted == true {
				if encData, ok := encField["data"].(string); ok {
					decryptedValue, err := p.encService.DecryptString(encData)
					if err != nil {
						return nil, fmt.Errorf("failed to decrypt field %s: %w", key, err)
					}
					decrypted[key] = decryptedValue
				} else {
					decrypted[key] = value
				}
			} else {
				decrypted[key] = value
			}
		} else {
			decrypted[key] = value
		}
	}

	return decrypted, nil
}

// KeyRotation handles encryption key rotation for enhanced security
type KeyRotation struct {
	currentService *EncryptionService
	oldServices    []*EncryptionService
}

// NewKeyRotation creates a new key rotation manager
func NewKeyRotation(currentKey string, oldKeys []string) (*KeyRotation, error) {
	currentService, err := NewEncryptionService(currentKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create current encryption service: %w", err)
	}

	oldServices := make([]*EncryptionService, len(oldKeys))
	for i, key := range oldKeys {
		service, err := NewEncryptionService(key)
		if err != nil {
			return nil, fmt.Errorf("failed to create old encryption service %d: %w", i, err)
		}
		oldServices[i] = service
	}

	return &KeyRotation{
		currentService: currentService,
		oldServices:    oldServices,
	}, nil
}

// Encrypt always uses the current key for encryption
func (kr *KeyRotation) Encrypt(data []byte) (*EncryptedData, error) {
	return kr.currentService.Encrypt(data)
}

// Decrypt tries current key first, then falls back to old keys
func (kr *KeyRotation) Decrypt(encData *EncryptedData) ([]byte, error) {
	// Try current key first
	if data, err := kr.currentService.Decrypt(encData); err == nil {
		return data, nil
	}

	// Try old keys
	for i, service := range kr.oldServices {
		if data, err := service.Decrypt(encData); err == nil {
			// Key rotation detected - could trigger re-encryption with current key
			return data, nil
		}
		_ = i // Suppress unused variable warning
	}

	return nil, fmt.Errorf("failed to decrypt data with any available key")
}

// ReencryptWithCurrentKey re-encrypts data using the current key
func (kr *KeyRotation) ReencryptWithCurrentKey(encData *EncryptedData) (*EncryptedData, error) {
	// Decrypt with any available key
	plaintext, err := kr.Decrypt(encData)
	if err != nil {
		return nil, err
	}

	// Re-encrypt with current key
	return kr.currentService.Encrypt(plaintext)
}
