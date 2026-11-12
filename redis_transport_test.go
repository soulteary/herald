package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	rediskit "github.com/soulteary/redis-kit/client"

	"github.com/soulteary/herald/internal/config"
)

// redisTestCertificate returns a self-signed server certificate for 127.0.0.1
// and redis.test, and the path of a PEM file that trusts it.
func redisTestCertificate(t *testing.T) (tls.Certificate, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "redis.test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              []string{"redis.test"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	caFile := filepath.Join(t.TempDir(), "redis-ca.pem")
	if err := os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, caFile
}

func TestRedisClientConfigDefaultsToPlaintextDefaultUser(t *testing.T) {
	oldUser, oldEnabled, oldCA, oldName := config.RedisUsername, config.RedisTLSEnabled, config.RedisTLSCAFile, config.RedisTLSServerName
	defer func() {
		config.RedisUsername, config.RedisTLSEnabled, config.RedisTLSCAFile, config.RedisTLSServerName = oldUser, oldEnabled, oldCA, oldName
	}()
	config.RedisUsername, config.RedisTLSEnabled, config.RedisTLSCAFile, config.RedisTLSServerName = "", false, "", ""

	cfg, err := redisClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TLSConfig != nil || cfg.Username != "" {
		t.Fatalf("defaults changed the Redis transport: tls=%v username=%q", cfg.TLSConfig != nil, cfg.Username)
	}
	if cfg.Addr != config.RedisAddr || cfg.Password != config.RedisPassword || cfg.DB != config.RedisDB {
		t.Fatal("address, password or database not passed through")
	}
}

// TestRedisClientConfigConnectsWithTLSAndACLUser proves REDIS_TLS_ENABLED,
// REDIS_TLS_CA_FILE, REDIS_TLS_SERVER_NAME and REDIS_USERNAME reach the
// connection: a TLS-only server requiring a named ACL user accepts the client
// only when all of them are configured correctly.
func TestRedisClientConfigConnectsWithTLSAndACLUser(t *testing.T) {
	cert, caFile := redisTestCertificate(t)
	mr, err := miniredis.RunTLS(&tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})
	if err != nil {
		t.Fatal(err)
	}
	defer mr.Close()
	mr.RequireUserAuth("herald", "acl-user-password")

	oldAddr, oldPass, oldDB := config.RedisAddr, config.RedisPassword, config.RedisDB
	oldUser, oldEnabled, oldCA, oldName := config.RedisUsername, config.RedisTLSEnabled, config.RedisTLSCAFile, config.RedisTLSServerName
	defer func() {
		config.RedisAddr, config.RedisPassword, config.RedisDB = oldAddr, oldPass, oldDB
		config.RedisUsername, config.RedisTLSEnabled, config.RedisTLSCAFile, config.RedisTLSServerName = oldUser, oldEnabled, oldCA, oldName
	}()

	connect := func() error {
		cfg, err := redisClientConfig()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		client, err := rediskit.NewClientContext(ctx, cfg.WithMaxRetries(-1).WithDialTimeout(2*time.Second))
		if err != nil {
			return err
		}
		return client.Close()
	}
	set := func(username string, tlsEnabled bool, ca, serverName string) {
		config.RedisAddr, config.RedisPassword, config.RedisDB = mr.Addr(), "acl-user-password", 0
		config.RedisUsername, config.RedisTLSEnabled, config.RedisTLSCAFile, config.RedisTLSServerName = username, tlsEnabled, ca, serverName
	}

	set("herald", true, caFile, "")
	if err := connect(); err != nil {
		t.Fatalf("TLS with ACL user (name from REDIS_ADDR): %v", err)
	}
	set("herald", true, caFile, "redis.test")
	if err := connect(); err != nil {
		t.Fatalf("TLS with ACL user and server name: %v", err)
	}

	set("herald", false, "", "")
	if err := connect(); err == nil {
		t.Fatal("plaintext client connected to a TLS-only Redis")
	}
	set("herald", true, "", "")
	if err := connect(); err == nil {
		t.Fatal("server certificate accepted without a trusted CA")
	}
	set("herald", true, caFile, "other.test")
	if err := connect(); err == nil {
		t.Fatal("server certificate accepted for the wrong server name")
	}
	set("", true, caFile, "")
	if err := connect(); err == nil {
		t.Fatal("default user accepted by a Redis that requires the ACL user")
	}
	set("herald", true, filepath.Join(t.TempDir(), "missing.pem"), "")
	if err := connect(); err == nil {
		t.Fatal("unreadable REDIS_TLS_CA_FILE accepted")
	}
}
