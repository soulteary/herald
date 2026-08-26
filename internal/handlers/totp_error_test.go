package handlers

import (
	"errors"
	"testing"

	"github.com/gofiber/fiber/v3"
)

func TestClassifyEnrollConfirmError(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		reason string
	}{
		{name: "invalid enrollment", err: errors.New("enroll/confirm returned 400: invalid"), status: fiber.StatusBadRequest, reason: "invalid"},
		{name: "upstream failure", err: errors.New("enroll/confirm returned 500: failed"), status: fiber.StatusBadGateway, reason: "proxy_failed"},
		{name: "transport failure", err: errors.New("connection refused"), status: fiber.StatusBadGateway, reason: "proxy_failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, reason := classifyEnrollConfirmError(tt.err)
			if status != tt.status || reason != tt.reason {
				t.Fatalf("classifyEnrollConfirmError() = (%d, %q), want (%d, %q)", status, reason, tt.status, tt.reason)
			}
		})
	}
}
