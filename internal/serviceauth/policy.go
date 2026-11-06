package serviceauth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// ServicePolicy binds a verified HMAC key to one service and allowed actions.
// It grants no user- or tenant-level isolation.
type ServicePolicy struct {
	Service string   `json:"service"`
	Actions []string `json:"actions"`
}

type ServicePolicies map[string]ServicePolicy

var serviceActions = map[string]bool{
	"otp:create": true, "otp:verify": true, "otp:revoke": true,
	"totp:status": true, "totp:verify": true, "totp:enroll": true, "totp:revoke": true,
}

// ParseServicePolicies accepts a complete policy map; empty input preserves
// the existing trusted-service model. Unknown fields or actions are rejected.
func ParseServicePolicies(raw string) (ServicePolicies, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var policies ServicePolicies
	dec := json.NewDecoder(bytes.NewBufferString(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&policies); err != nil {
		return nil, fmt.Errorf("invalid HERALD_SERVICE_POLICIES")
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("invalid HERALD_SERVICE_POLICIES trailing data")
	}
	if len(policies) == 0 {
		return nil, fmt.Errorf("HERALD_SERVICE_POLICIES must not be empty")
	}
	for keyID, policy := range policies {
		if strings.TrimSpace(keyID) != keyID || keyID == "" || strings.Contains(keyID, "|kid:") || strings.TrimSpace(policy.Service) != policy.Service || policy.Service == "" || strings.Contains(policy.Service, "|kid:") || len(policy.Actions) == 0 {
			return nil, fmt.Errorf("HERALD_SERVICE_POLICIES requires key IDs, services, and actions")
		}
		seen := make(map[string]bool)
		for _, action := range policy.Actions {
			if !serviceActions[action] || seen[action] {
				return nil, fmt.Errorf("HERALD_SERVICE_POLICIES contains an unknown or duplicate action")
			}
			seen[action] = true
		}
	}
	return policies, nil
}
