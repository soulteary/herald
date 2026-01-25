package otp

import (
	"fmt"

	secure "github.com/soulteary/secure-kit"
)

// GenerateCode generates a random numeric code of specified length
func GenerateCode(length int) (string, error) {
	if length <= 0 || length > 10 {
		return "", fmt.Errorf("code length must be between 1 and 10")
	}

	return secure.RandomDigits(length)
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
