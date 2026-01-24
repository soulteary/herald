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
	assert.Equal(t, 5*time.Second, client.httpClient.Timeout)
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

		timestamp := r.Header.Get("X-Timestamp")
		service := r.Header.Get("X-Service")
		signature := r.Header.Get("X-Signature")
		assert.Equal(t, "stargate", service)

		expectedSig := (&Client{hmacSecret: "hmac-secret"}).computeHMAC(timestamp, service, bodyBytes)
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
