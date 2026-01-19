package otp

import (
	"testing"
)

func TestGenerateCode(t *testing.T) {
	tests := []struct {
		name    string
		length  int
		wantErr bool
	}{
		{
			name:    "valid length 6",
			length:  6,
			wantErr: false,
		},
		{
			name:    "valid length 4",
			length:  4,
			wantErr: false,
		},
		{
			name:    "valid length 10",
			length:  10,
			wantErr: false,
		},
		{
			name:    "invalid length 0",
			length:  0,
			wantErr: true,
		},
		{
			name:    "invalid length negative",
			length:  -1,
			wantErr: true,
		},
		{
			name:    "invalid length 11",
			length:  11,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, err := GenerateCode(tt.length)
			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateCode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if len(code) != tt.length {
					t.Errorf("GenerateCode() code length = %d, want %d", len(code), tt.length)
				}
				// Verify all characters are digits
				for _, c := range code {
					if c < '0' || c > '9' {
						t.Errorf("GenerateCode() code contains non-digit: %c", c)
					}
				}
			}
		})
	}
}

func TestGenerateCode_Uniqueness(t *testing.T) {
	// Test that generated codes are reasonably unique
	codes := make(map[string]bool)
	for i := 0; i < 100; i++ {
		code, err := GenerateCode(6)
		if err != nil {
			t.Fatalf("GenerateCode() error = %v", err)
		}
		if codes[code] {
			t.Errorf("GenerateCode() generated duplicate code: %s", code)
		}
		codes[code] = true
	}
}

func TestValidateCodeFormat(t *testing.T) {
	tests := []struct {
		name   string
		code   string
		length int
		want   bool
	}{
		{
			name:   "valid 6-digit code",
			code:   "123456",
			length: 6,
			want:   true,
		},
		{
			name:   "valid 4-digit code",
			code:   "0000",
			length: 4,
			want:   true,
		},
		{
			name:   "wrong length",
			code:   "12345",
			length: 6,
			want:   false,
		},
		{
			name:   "contains letter",
			code:   "12345a",
			length: 6,
			want:   false,
		},
		{
			name:   "contains special character",
			code:   "12345-",
			length: 6,
			want:   false,
		},
		{
			name:   "empty string",
			code:   "",
			length: 6,
			want:   false,
		},
		{
			name:   "all zeros",
			code:   "000000",
			length: 6,
			want:   true,
		},
		{
			name:   "all nines",
			code:   "999999",
			length: 6,
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidateCodeFormat(tt.code, tt.length); got != tt.want {
				t.Errorf("ValidateCodeFormat() = %v, want %v", got, tt.want)
			}
		})
	}
}
