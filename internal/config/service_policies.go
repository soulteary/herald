package config

import (
	"fmt"
	"strings"

	"github.com/soulteary/herald/internal/serviceauth"
)

// ServicePolicies validates the optional HMAC service/action boundary.
// It does not partition users or TOTP subjects between services.
func ServicePolicies() (serviceauth.ServicePolicies, error) {
	policies, err := serviceauth.ParseServicePolicies(ServicePoliciesJSON)
	if err != nil {
		return nil, err
	}
	if policies == nil {
		return nil, nil
	}
	mode, err := serviceauth.ParseMode(RequestAuthMode)
	if err != nil {
		return nil, err
	}
	if mode != serviceauth.ModeHMACV2 || HMACV1Enabled {
		return nil, fmt.Errorf("HERALD_SERVICE_POLICIES requires hmac_v2 authentication with HMAC_V1_ENABLED=false")
	}
	for keyID := range policies {
		if strings.TrimSpace(GetHMACSecret(keyID)) == "" {
			return nil, fmt.Errorf("HERALD_SERVICE_POLICIES references an unconfigured HMAC key")
		}
	}
	return policies, nil
}
