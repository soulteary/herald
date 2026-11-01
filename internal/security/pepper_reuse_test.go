package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"testing"
)

func TestDigestReuseMatchesHMAC(t *testing.T) {
	pepper := []byte("original pepper")
	d := NewDigester(pepper)
	// The caller cannot change the key while pooled states are in use.
	pepper[0] = 'X'
	var wg sync.WaitGroup
	for worker := 0; worker < 16; worker++ {
		wg.Go(func() {
			for i := 0; i < 100; i++ {
				value := fmt.Sprintf("user-%d@example.com", i)
				mac := hmac.New(sha256.New, []byte("original pepper"))
				_, _ = mac.Write([]byte(value))
				if got, want := d.Digest(value), hex.EncodeToString(mac.Sum(nil)); got != want {
					t.Errorf("Digest(%q)=%s want %s", value, got, want)
				}
				mac.Reset()
				for _, part := range []string{value, "login", "email"} {
					_, _ = mac.Write([]byte(part))
					_, _ = mac.Write([]byte{0})
				}
				if got, want := d.DigestParts(value, "login", "email"), hex.EncodeToString(mac.Sum(nil)); got != want {
					t.Errorf("DigestParts=%s want %s", got, want)
				}
			}
		})
	}
	wg.Wait()
}

func TestZeroValueDigester(t *testing.T) {
	var d Digester
	mac := hmac.New(sha256.New, nil)
	_, _ = mac.Write([]byte("value"))
	if got, want := d.Digest("value"), hex.EncodeToString(mac.Sum(nil)); got != want {
		t.Fatalf("zero-value digest=%s want %s", got, want)
	}
}
