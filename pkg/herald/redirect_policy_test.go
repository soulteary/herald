package herald

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestClientRejectsRedirectBeforeForwardingCredentials(t *testing.T) {
	for _, status := range []int{301, 302, 303, 307, 308} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var hits atomic.Int32
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { hits.Add(1); w.WriteHeader(200) }))
			defer target.Close()
			origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, strings.Replace(target.URL, "127.0.0.1", "localhost", 1), status)
			}))
			defer origin.Close()
			client, err := NewClient(DefaultOptions().WithBaseURL(origin.URL).WithAPIKey("test-api-key").WithHMACSecret("test-hmac-secret"))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := client.TOTPStatus(context.Background(), "test-user"); err == nil {
				t.Fatal("redirect unexpectedly accepted")
			}
			if _, err := client.RevokeChallenge(context.Background(), "test-challenge"); err == nil {
				t.Fatal("POST redirect unexpectedly accepted")
			}
			if hits.Load() != 0 {
				t.Fatalf("redirect target received %d requests", hits.Load())
			}
		})
	}
}
