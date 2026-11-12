package config

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
)

// RedisTLSConfig returns the TLS configuration for the Redis connection, or
// nil when REDIS_TLS_ENABLED is false. Server certificates are always
// verified: against REDIS_TLS_CA_FILE when set, otherwise against the system
// roots. REDIS_TLS_SERVER_NAME overrides the name checked in the certificate;
// when empty the host part of REDIS_ADDR is used.
func RedisTLSConfig() (*tls.Config, error) {
	if !RedisTLSEnabled {
		return nil, nil
	}
	cfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
		ServerName: RedisTLSServerName,
	}
	if RedisTLSCAFile != "" {
		pem, err := os.ReadFile(RedisTLSCAFile)
		if err != nil {
			return nil, fmt.Errorf("read REDIS_TLS_CA_FILE: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("REDIS_TLS_CA_FILE contains no PEM certificates")
		}
		cfg.RootCAs = pool
	}
	return cfg, nil
}

// redisTransportProblems reports Redis transport settings that would be
// silently ignored, leaving the operator with weaker protection than the
// configuration suggests.
func redisTransportProblems() []string {
	var problems []string
	if !RedisTLSEnabled && (RedisTLSCAFile != "" || RedisTLSServerName != "") {
		problems = append(problems, "REDIS_TLS_CA_FILE and REDIS_TLS_SERVER_NAME have no effect unless REDIS_TLS_ENABLED=true")
	}
	if RedisUsername != "" && RedisPassword == "" {
		problems = append(problems, "REDIS_USERNAME requires REDIS_PASSWORD; a username without a password is not sent to Redis")
	}
	return problems
}
