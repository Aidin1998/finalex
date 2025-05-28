package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"net"
)

// Encrypt encrypts plaintext using AES and returns base64 ciphertext.
func Encrypt(key, plaintext string) (string, error) {
	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return "", err
	}
	ciphertext := make([]byte, aes.BlockSize+len(plaintext))
	iv := ciphertext[:aes.BlockSize]
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return "", err
	}
	stream := cipher.NewCFBEncrypter(block, iv)
	stream.XORKeyStream(ciphertext[aes.BlockSize:], []byte(plaintext))
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts base64 ciphertext using AES and returns plaintext.
func Decrypt(key, cryptoText string) (string, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(cryptoText)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return "", err
	}
	if len(ciphertext) < aes.BlockSize {
		return "", errors.New("ciphertext too short")
	}
	iv := ciphertext[:aes.BlockSize]
	ciphertext = ciphertext[aes.BlockSize:]
	stream := cipher.NewCFBDecrypter(block, iv)
	stream.XORKeyStream(ciphertext, ciphertext)
	return string(ciphertext), nil
}

// IsIPWhitelisted checks if the given IP is in the whitelist.
func IsIPWhitelisted(ip string, whitelist []string) bool {
	for _, allowed := range whitelist {
		if ip == allowed {
			return true
		}
		if _, ipnet, err := net.ParseCIDR(allowed); err == nil {
			if ipnet.Contains(net.ParseIP(ip)) {
				return true
			}
		}
	}
	return false
}

// HasAPIPermission checks if the API key has the required permission.
type APIPermissions map[string][]string // apiKey -> ["read", "trade", ...]

func HasAPIPermission(apiKey, permission string, perms APIPermissions) bool {
	allowed, ok := perms[apiKey]
	if !ok {
		return false
	}
	for _, p := range allowed {
		if p == permission {
			return true
		}
	}
	return false
}
