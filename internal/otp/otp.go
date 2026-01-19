package otp

import (
	"crypto/rand"
	"fmt"
)

// GenerateCode generates a random numeric code of specified length
func GenerateCode(length int) (string, error) {
	if length <= 0 || length > 10 {
		return "", fmt.Errorf("code length must be between 1 and 10")
	}

	b := make([]byte, length)
	for i := range b {
		// Generate random digit 0-9
		digit := make([]byte, 1)
		if _, err := rand.Read(digit); err != nil {
			return "", fmt.Errorf("failed to generate random digit: %w", err)
		}
		b[i] = byte('0' + (digit[0] % 10))
	}

	return string(b), nil
}

// ValidateCodeFormat validates that a code matches the expected format
func ValidateCodeFormat(code string, length int) bool {
	if len(code) != length {
		return false
	}
	for _, c := range code {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
