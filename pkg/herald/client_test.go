package herald

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultOptions(t *testing.T) {
	opts := DefaultOptions()
	assert.Equal(t, 10*time.Second, opts.Timeout)
	assert.Equal(t, "stargate", opts.Service)
	assert.Equal(t, "", opts.BaseURL)
}

func TestOptionsValidate(t *testing.T) {
	err := (&Options{}).Validate()
	assert.NotNil(t, err)

	opts := &Options{BaseURL: "http://example.com"}
	assert.NoError(t, opts.Validate())
}

func TestOptionsFluentSetters(t *testing.T) {
	opts := DefaultOptions().
		WithBaseURL("http://example.com").
		WithAPIKey("api-key").
		WithHMACSecret("hmac-secret").
		WithService("custom-service").
		WithTimeout(3 * time.Second)

	assert.Equal(t, "http://example.com", opts.BaseURL)
	assert.Equal(t, "api-key", opts.APIKey)
	assert.Equal(t, "hmac-secret", opts.HMACSecret)
	assert.Equal(t, "custom-service", opts.Service)
	assert.Equal(t, 3*time.Second, opts.Timeout)
}

func TestNewClient_MissingBaseURL(t *testing.T) {
	client, err := NewClient(&Options{})
	assert.Nil(t, client)
	assert.NotNil(t, err)
}

func TestNewClient_Success(t *testing.T) {
	opts := DefaultOptions().
		WithBaseURL("http://example.com").
		WithAPIKey("api-key").
		WithHMACSecret("hmac-secret").
		WithService("custom-service").
		WithTimeout(5 * time.Second)

	client, err := NewClient(opts)
	assert.NoError(t, err)
	assert.NotNil(t, client)
	assert.Equal(t, "http://example.com", client.baseURL)
	assert.Equal(t, "api-key", client.apiKey)
	assert.Equal(t, "hmac-secret", client.hmacSecret)
	assert.Equal(t, "custom-service", client.service)
	assert.Equal(t, 5*time.Second, client.httpClient.GetHTTPClient().Timeout)
}

func TestAddAuthHeaders_APIKeyOnly(t *testing.T) {
	client := &Client{
		apiKey:  "api-key",
		service: "stargate",
	}

	req, err := http.NewRequest(http.MethodPost, "http://example.com", nil)
	assert.NoError(t, err)

	client.addAuthHeaders(req, []byte(`{"ok":true}`))

	assert.Equal(t, "api-key", req.Header.Get("X-API-Key"))
	assert.Equal(t, "", req.Header.Get("X-Timestamp"))
	assert.Equal(t, "", req.Header.Get("X-Signature"))
	assert.Equal(t, "", req.Header.Get("X-Service"))
}

func TestAddAuthHeaders_HMACOnly(t *testing.T) {
	body := []byte(`{"ok":true}`)
	client := &Client{
		hmacSecret: "hmac-secret",
		service:    "custom-service",
	}

	req, err := http.NewRequest(http.MethodPost, "http://example.com", nil)
	assert.NoError(t, err)

	client.addAuthHeaders(req, body)

	timestamp := req.Header.Get("X-Timestamp")
	service := req.Header.Get("X-Service")
	signature := req.Header.Get("X-Signature")

	assert.NotNil(t, timestamp)
	assert.Equal(t, "custom-service", service)
	expectedSig := client.computeHMAC(timestamp, service, body)
	assert.Equal(t, expectedSig, signature)
}

func TestComputeHMACv2_GoldenCanonical(t *testing.T) {
	// Golden test that locks the v2 canonical string format. It MUST stay in sync
	// with internal/auth.CanonicalRequest.Canonical() on the server:
	//   HERALD-HMAC-V2\nMETHOD\nPATH\nQUERY\nTS\nNONCE\nSERVICE\nKEYID\nsha256hex(body)
	client := &Client{hmacSecret: "test-secret"}
	body := []byte(`{"user_id":"u1"}`)

	bodyHash := sha256.Sum256(body)
	canonical := "HERALD-HMAC-V2\n" +
		"POST\n" +
		"/v1/otp/challenges\n" +
		"a=1&b=2\n" +
		"1700000000\n" +
		"nonce-xyz\n" +
		"stargate\n" +
		"key-1\n" +
		hex.EncodeToString(bodyHash[:])
	mac := hmac.New(sha256.New, []byte("test-secret"))
	mac.Write([]byte(canonical))
	expected := hex.EncodeToString(mac.Sum(nil))

	got := client.computeHMACv2("POST", "/v1/otp/challenges", "a=1&b=2", "1700000000", "nonce-xyz", "stargate", "key-1", body)
	assert.Equal(t, expected, got, "SDK v2 canonical must match the server canonical format")
}

func TestComputeHMAC(t *testing.T) {
	client := &Client{hmacSecret: "hmac-secret"}
	timestamp := "1700000000"
	service := "stargate"
	body := []byte("payload")

	signature := client.computeHMAC(timestamp, service, body)

	mac := hmac.New(sha256.New, []byte("hmac-secret"))
	message := timestamp + ":" + service + ":" + string(body)
	mac.Write([]byte(message))
	expected := hex.EncodeToString(mac.Sum(nil))

	assert.Equal(t, expected, signature)
}

func TestAddAuthHeaders_V2Default(t *testing.T) {
	client := &Client{
		hmacSecret: "hmac-secret",
		service:    "stargate",
		sigVersion: SignatureV2,
		keyID:      "key-1",
		now:        func() time.Time { return time.Unix(1700000000, 0) },
		newNonce:   func() string { return "fixed-nonce" },
	}
	body := []byte(`{"a":1}`)
	req, err := http.NewRequest(http.MethodPost, "http://example.com/v1/otp/challenges?x=1", nil)
	assert.NoError(t, err)

	client.addAuthHeaders(req, body)

	assert.Equal(t, "v2", req.Header.Get("X-Signature-Version"))
	assert.Equal(t, "1700000000", req.Header.Get("X-Timestamp"))
	assert.Equal(t, "fixed-nonce", req.Header.Get("X-Nonce"))
	assert.Equal(t, "stargate", req.Header.Get("X-Service"))
	assert.Equal(t, "key-1", req.Header.Get("X-Key-Id"))
	// v2 must NOT be confused with v1: the signature must match the v2 canonical.
	expected := client.computeHMACv2(http.MethodPost, "/v1/otp/challenges", "x=1", "1700000000", "fixed-nonce", "stargate", "key-1", body)
	assert.Equal(t, expected, req.Header.Get("X-Signature"))
}

func TestAddAuthHeaders_V2NonceChangesPerRequest(t *testing.T) {
	client, err := NewClient(DefaultOptions().WithBaseURL("http://example.com").WithHMACSecret("s"))
	assert.NoError(t, err)
	req1, _ := http.NewRequest(http.MethodPost, "http://example.com/v1/otp/challenges", nil)
	req2, _ := http.NewRequest(http.MethodPost, "http://example.com/v1/otp/challenges", nil)
	client.addAuthHeaders(req1, []byte(`{}`))
	client.addAuthHeaders(req2, []byte(`{}`))
	n1 := req1.Header.Get("X-Nonce")
	n2 := req2.Header.Get("X-Nonce")
	assert.NotEmpty(t, n1)
	assert.NotEmpty(t, n2)
	assert.NotEqual(t, n1, n2, "each request must use a fresh nonce")
}

func TestAddAuthHeaders_V1OptIn(t *testing.T) {
	client, err := NewClient(DefaultOptions().
		WithBaseURL("http://example.com").
		WithHMACSecret("hmac-secret").
		WithService("stargate").
		WithSignatureVersion(SignatureV1))
	assert.NoError(t, err)

	body := []byte(`{"a":1}`)
	req, _ := http.NewRequest(http.MethodPost, "http://example.com/v1/otp/challenges", nil)
	client.addAuthHeaders(req, body)

	// v1 must not emit v2-only headers.
	assert.Equal(t, "", req.Header.Get("X-Signature-Version"))
	assert.Equal(t, "", req.Header.Get("X-Nonce"))
	assert.Equal(t, client.computeHMAC(req.Header.Get("X-Timestamp"), "stargate", body), req.Header.Get("X-Signature"))
}

func TestCreateChallenge_Success(t *testing.T) {
	expectedReq := &CreateChallengeRequest{
		UserID:      "user-1",
		Channel:     "sms",
		Destination: "13800138000",
		Purpose:     "login",
		Locale:      "zh-CN",
		ClientIP:    "127.0.0.1",
		UA:          "stargate-test",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/otp/challenges", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "api-key", r.Header.Get("X-API-Key"))

		bodyBytes, err := io.ReadAll(r.Body)
		assert.NoError(t, err)

		// NewClient defaults to HMAC v2: verify replay-resistant headers and the
		// canonical signature binding.
		assert.Equal(t, "v2", r.Header.Get("X-Signature-Version"))
		timestamp := r.Header.Get("X-Timestamp")
		service := r.Header.Get("X-Service")
		nonce := r.Header.Get("X-Nonce")
		signature := r.Header.Get("X-Signature")
		assert.Equal(t, "stargate", service)
		assert.NotEmpty(t, timestamp)
		assert.NotEmpty(t, nonce)

		expectedSig := (&Client{hmacSecret: "hmac-secret"}).computeHMACv2(
			r.Method, r.URL.Path, r.URL.RawQuery, timestamp, nonce, service, r.Header.Get("X-Key-Id"), bodyBytes)
		assert.Equal(t, expectedSig, signature)

		var got CreateChallengeRequest
		err = json.Unmarshal(bodyBytes, &got)
		assert.NoError(t, err)
		assert.Equal(t, expectedReq, &got)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(CreateChallengeResponse{
			ChallengeID:  "challenge-1",
			ExpiresIn:    120,
			NextResendIn: 30,
		})
	}))
	defer server.Close()

	opts := DefaultOptions().
		WithBaseURL(server.URL).
		WithAPIKey("api-key").
		WithHMACSecret("hmac-secret").
		WithService("stargate")
	client, err := NewClient(opts)
	assert.NoError(t, err)

	resp, err := client.CreateChallenge(context.Background(), expectedReq)
	assert.NoError(t, err)
	assert.Equal(t, "challenge-1", resp.ChallengeID)
	assert.Equal(t, 120, resp.ExpiresIn)
	assert.Equal(t, 30, resp.NextResendIn)
}

func TestCreateChallenge_StatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad request"))
	}))
	defer server.Close()

	client, err := NewClient(DefaultOptions().WithBaseURL(server.URL))
	assert.NoError(t, err)

	_, err = client.CreateChallenge(context.Background(), &CreateChallengeRequest{
		UserID:      "user-1",
		Channel:     "sms",
		Destination: "13800138000",
	})
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "status 400"))
	assert.True(t, strings.Contains(err.Error(), "bad request"))
}

func TestCreateChallenge_DecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not-json"))
	}))
	defer server.Close()

	client, err := NewClient(DefaultOptions().WithBaseURL(server.URL))
	assert.NoError(t, err)

	_, err = client.CreateChallenge(context.Background(), &CreateChallengeRequest{
		UserID:      "user-1",
		Channel:     "sms",
		Destination: "13800138000",
	})
	assert.NotNil(t, err)
}

func TestVerifyChallenge_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/otp/verifications", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(VerifyChallengeResponse{
			OK:       true,
			UserID:   "user-1",
			AMR:      []string{"sms"},
			IssuedAt: 1700000000,
		})
	}))
	defer server.Close()

	client, err := NewClient(DefaultOptions().WithBaseURL(server.URL))
	assert.NoError(t, err)

	resp, err := client.VerifyChallenge(context.Background(), &VerifyChallengeRequest{
		ChallengeID: "challenge-1",
		Code:        "123456",
	})
	assert.NoError(t, err)
	assert.True(t, resp.OK)
	assert.Equal(t, "user-1", resp.UserID)
}

func TestVerifyChallenge_FailureStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(VerifyChallengeResponse{
			OK:     false,
			Reason: "invalid",
		})
	}))
	defer server.Close()

	client, err := NewClient(DefaultOptions().WithBaseURL(server.URL))
	assert.NoError(t, err)

	resp, err := client.VerifyChallenge(context.Background(), &VerifyChallengeRequest{
		ChallengeID: "challenge-1",
		Code:        "000000",
	})
	assert.NotNil(t, err)
	assert.NotNil(t, resp)
	assert.False(t, resp.OK)
	assert.Equal(t, "invalid", resp.Reason)
	assert.True(t, strings.Contains(err.Error(), "invalid"))
}

func TestVerifyChallenge_DecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not-json"))
	}))
	defer server.Close()

	client, err := NewClient(DefaultOptions().WithBaseURL(server.URL))
	assert.NoError(t, err)

	_, err = client.VerifyChallenge(context.Background(), &VerifyChallengeRequest{
		ChallengeID: "challenge-1",
		Code:        "123456",
	})
	assert.NotNil(t, err)
}

func TestOptions_TLSFluentSetters(t *testing.T) {
	opts := DefaultOptions().
		WithTLSCACert("/path/to/ca.pem").
		WithTLSClientCert("/path/to/cert.pem", "/path/to/key.pem").
		WithTLSServerName("herald.example.com").
		WithInsecureSkipVerify(true)

	assert.Equal(t, "/path/to/ca.pem", opts.TLSCACertFile)
	assert.Equal(t, "/path/to/cert.pem", opts.TLSClientCert)
	assert.Equal(t, "/path/to/key.pem", opts.TLSClientKey)
	assert.Equal(t, "herald.example.com", opts.TLSServerName)
	assert.True(t, opts.InsecureSkipVerify)
}

func TestHeraldError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *HeraldError
		contains []string
	}{
		{
			name:     "status and message",
			err:      &HeraldError{StatusCode: 400, Message: "bad request"},
			contains: []string{"400", "bad request"},
		},
		{
			name:     "status and reason",
			err:      &HeraldError{StatusCode: 401, Reason: "unauthorized"},
			contains: []string{"401", "unauthorized"},
		},
		{
			name:     "status only",
			err:      &HeraldError{StatusCode: 500},
			contains: []string{"500"},
		},
		{
			name:     "connection error with message",
			err:      &HeraldError{StatusCode: 0, Message: "connection refused"},
			contains: []string{"connection refused"},
		},
		{
			name:     "connection error with reason",
			err:      &HeraldError{StatusCode: 0, Reason: "timeout"},
			contains: []string{"timeout"},
		},
		{
			name:     "connection error only",
			err:      &HeraldError{StatusCode: 0},
			contains: []string{"Herald API error"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			for _, sub := range tt.contains {
				if !strings.Contains(got, sub) {
					t.Errorf("Error() = %q, want to contain %q", got, sub)
				}
			}
		})
	}
}
