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
	// Distinct credentials are required for distinct permission sets. Otherwise
	// any holder of a shared secret could sign as the more privileged key ID.
	seenSecrets := make(map[string]bool, len(hmacKeysMap))
	for _, secret := range hmacKeysMap {
		if seenSecrets[secret] {
			return nil, fmt.Errorf("HERALD_SERVICE_POLICIES requires distinct secrets for every configured HMAC key")
		}
		seenSecrets[secret] = true
	}
	for keyID := range policies {
		if strings.TrimSpace(GetHMACSecret(keyID)) == "" {
			return nil, fmt.Errorf("HERALD_SERVICE_POLICIES references an unconfigured HMAC key")
		}
	}
	return policies, nil
}
