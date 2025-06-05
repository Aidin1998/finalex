//go:build userauth

package test

import (
	"errors"
)

// Error constants for password validation
var (
	ErrPasswordTooShort             = errors.New("password must be at least 8 characters")
	ErrPasswordRequirements         = errors.New("password must contain uppercase, lowercase, number, and special character")
	ErrPasswordContainsPersonalInfo = errors.New("password cannot contain personal information")
)

// Mock for strict password policy
type mockStrictPasswordPolicy struct{}

func (m *mockStrictPasswordPolicy) ValidateNewPassword(password, email string) error {
	// Simple validation for test purposes:
	// At least 8 characters, 1 uppercase, 1 lowercase, 1 number, 1 special char
	hasUpper := false
	hasLower := false
	hasNumber := false
	hasSpecial := false
	if len(password) < 8 {
		return ErrPasswordTooShort
	}

	for _, char := range password {
		switch {
		case char >= 'A' && char <= 'Z':
			hasUpper = true
		case char >= 'a' && char <= 'z':
			hasLower = true
		case char >= '0' && char <= '9':
			hasNumber = true
		case char == '!' || char == '@' || char == '#' || char == '$' || char == '%' || char == '^' || char == '&' || char == '*':
			hasSpecial = true
		}
	}
	if !hasUpper || !hasLower || !hasNumber || !hasSpecial {
		return ErrPasswordRequirements
	}
	// Check if password contains the email
	if len(email) > 0 && password == email {
		return ErrPasswordContainsPersonalInfo
	}

	return nil
}
