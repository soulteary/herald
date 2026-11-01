package config

import (
	"errors"
	"strings"
)

// CORSOrigins returns the normalized origin list used by both startup validation
// and routing. A wildcard entry anywhere in the list is forbidden in production.
// Empty configuration disables CORS; development and test retain wildcard support.
func CORSOrigins() ([]string, error) {
	value := strings.TrimSpace(CORSAllowOrigins)
	if value == "" {
		return nil, nil
	}
	origins := strings.Split(value, ",")
	for i := range origins {
		origins[i] = strings.TrimSpace(origins[i])
		if IsProduction() && origins[i] == "*" {
			return nil, errors.New("CORS wildcard (HERALD_CORS_ALLOW_ORIGINS entry '*') is forbidden in production")
		}
	}
	return origins, nil
}
