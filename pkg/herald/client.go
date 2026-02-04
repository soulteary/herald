package herald

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	httpkit "github.com/soulteary/http-kit"
)

// Client is the Herald API client
type Client struct {
	httpClient *httpkit.Client
	baseURL    string
	apiKey     string
	hmacSecret string
	service    string
}

// Options for creating a Herald client
type Options struct {
	BaseURL            string
	APIKey             string
	HMACSecret         string
	Service            string
	Timeout            time.Duration
	TLSCACertFile      string // For verifying server certificate
	TLSClientCert      string // Client certificate file for mTLS
	TLSClientKey       string // Client private key file for mTLS
	TLSServerName      string // Server name for TLS verification
	InsecureSkipVerify bool   // Skip TLS certificate verification (not recommended)
}

// DefaultOptions returns default options
func DefaultOptions() *Options {
	return &Options{
		Timeout: 10 * time.Second,
		Service: "stargate",
	}
}

// WithBaseURL sets the base URL
func (o *Options) WithBaseURL(url string) *Options {
	o.BaseURL = url
	return o
}

// WithAPIKey sets the API key
func (o *Options) WithAPIKey(key string) *Options {
	o.APIKey = key
	return o
}

// WithHMACSecret sets the HMAC secret
func (o *Options) WithHMACSecret(secret string) *Options {
	o.HMACSecret = secret
	return o
}

// WithService sets the service name
func (o *Options) WithService(service string) *Options {
	o.Service = service
	return o
}

// WithTimeout sets the timeout
func (o *Options) WithTimeout(timeout time.Duration) *Options {
	o.Timeout = timeout
	return o
}

// WithTLSCACert sets the CA certificate file for TLS verification
func (o *Options) WithTLSCACert(caCertFile string) *Options {
	o.TLSCACertFile = caCertFile
	return o
}

// WithTLSClientCert sets the client certificate and key files for mTLS
func (o *Options) WithTLSClientCert(certFile, keyFile string) *Options {
	o.TLSClientCert = certFile
	o.TLSClientKey = keyFile
	return o
}

// WithTLSServerName sets the server name for TLS verification
func (o *Options) WithTLSServerName(serverName string) *Options {
	o.TLSServerName = serverName
	return o
}

// WithInsecureSkipVerify sets whether to skip TLS certificate verification
func (o *Options) WithInsecureSkipVerify(skip bool) *Options {
	o.InsecureSkipVerify = skip
	return o
}

// Validate validates the options
func (o *Options) Validate() error {
	if o.BaseURL == "" {
		return fmt.Errorf("base URL is required")
	}
	return nil
}

// HeraldError represents an error from Herald API
type HeraldError struct {
	StatusCode int
	Reason     string
	Message    string
}

func (e *HeraldError) Error() string {
	// Always include status code in error message for better debugging
	if e.StatusCode > 0 {
		if e.Message != "" {
			// Always include status code in the message format for consistency
			// Format: "API returned status 400: bad request" (matches test expectations)
			return fmt.Sprintf("API returned status %d: %s", e.StatusCode, e.Message)
		}
		if e.Reason != "" {
			return fmt.Sprintf("Herald API error: %s (status: %d)", e.Reason, e.StatusCode)
		}
		return fmt.Sprintf("Herald API error: status %d", e.StatusCode)
	}
	// Connection errors (status code 0)
	if e.Message != "" {
		return e.Message
	}
	if e.Reason != "" {
		return fmt.Sprintf("Herald API error: %s", e.Reason)
	}
	return "Herald API error"
}

// NewClient creates a new Herald API client
func NewClient(opts *Options) (*Client, error) {
	if opts == nil {
		opts = DefaultOptions()
	}

	if err := opts.Validate(); err != nil {
		return nil, err
	}

	clientOpts := &httpkit.Options{
		BaseURL:            opts.BaseURL,
		Timeout:            opts.Timeout,
		TLSCACertFile:      opts.TLSCACertFile,
		TLSClientCert:      opts.TLSClientCert,
		TLSClientKey:       opts.TLSClientKey,
		TLSServerName:      opts.TLSServerName,
		InsecureSkipVerify: opts.InsecureSkipVerify,
	}

	httpClient, err := httpkit.NewClient(clientOpts)
	if err != nil {
		return nil, err
	}

	client := &Client{
		httpClient: httpClient,
		baseURL:    opts.BaseURL,
		apiKey:     opts.APIKey,
		hmacSecret: opts.HMACSecret,
		service:    opts.Service,
	}

	return client, nil
}

// CreateChallengeRequest represents the request to create a challenge
type CreateChallengeRequest struct {
	UserID      string `json:"user_id"`
	Channel     string `json:"channel"`
	Destination string `json:"destination"`
	Purpose     string `json:"purpose"`
	Locale      string `json:"locale"`
	ClientIP    string `json:"client_ip"`
	UA          string `json:"ua"`
}

// CreateChallengeResponse represents the response from creating a challenge
type CreateChallengeResponse struct {
	ChallengeID  string `json:"challenge_id"`
	ExpiresIn    int    `json:"expires_in"`
	NextResendIn int    `json:"next_resend_in"`
}

// VerifyChallengeRequest represents the request to verify a challenge
type VerifyChallengeRequest struct {
	ChallengeID string `json:"challenge_id"`
	Code        string `json:"code"`
	ClientIP    string `json:"client_ip"`
}

// VerifyChallengeResponse represents the response from verifying a challenge
type VerifyChallengeResponse struct {
	OK                bool     `json:"ok"`
	UserID            string   `json:"user_id,omitempty"`
	AMR               []string `json:"amr,omitempty"`
	IssuedAt          int64    `json:"issued_at,omitempty"`
	Reason            string   `json:"reason,omitempty"`
	RemainingAttempts *int     `json:"remaining_attempts,omitempty"` // Number of remaining attempts
	NextResendIn      *int     `json:"next_resend_in,omitempty"`     // Seconds until next resend is allowed
}

// IdempotencyKeyContextKey is the context key for passing Idempotency-Key to CreateChallenge.
// Use context.WithValue(ctx, herald.IdempotencyKeyContextKey, "your-key") so the client sends the header.
var IdempotencyKeyContextKey = struct{ name string }{name: "idempotency_key"}

// CreateChallenge creates a new challenge and sends verification code
func (c *Client) CreateChallenge(ctx context.Context, req *CreateChallengeRequest) (*CreateChallengeResponse, error) {
	url := fmt.Sprintf("%s/v1/otp/challenges", c.baseURL)

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	if v := ctx.Value(IdempotencyKeyContextKey); v != nil {
		if s, ok := v.(string); ok && s != "" {
			httpReq.Header.Set("Idempotency-Key", s)
		}
	}

	// Inject trace context into headers
	c.httpClient.InjectTraceContext(ctx, httpReq)

	c.addAuthHeaders(httpReq, body)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, &HeraldError{
			StatusCode: 0,
			Reason:     "connection_failed",
			Message:    fmt.Sprintf("failed to send request: %v", err),
		}
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		var errorResp struct {
			OK     bool   `json:"ok"`
			Reason string `json:"reason"`
		}
		_ = json.Unmarshal(bodyBytes, &errorResp)
		return nil, &HeraldError{
			StatusCode: resp.StatusCode,
			Reason:     errorResp.Reason,
			Message:    string(bodyBytes),
		}
	}

	var challengeResp CreateChallengeResponse
	if err := json.NewDecoder(resp.Body).Decode(&challengeResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &challengeResp, nil
}

// VerifyChallenge verifies a challenge code
func (c *Client) VerifyChallenge(ctx context.Context, req *VerifyChallengeRequest) (*VerifyChallengeResponse, error) {
	url := fmt.Sprintf("%s/v1/otp/verifications", c.baseURL)

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	// Inject trace context into headers
	c.httpClient.InjectTraceContext(ctx, httpReq)

	c.addAuthHeaders(httpReq, body)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, &HeraldError{
			StatusCode: 0,
			Reason:     "connection_failed",
			Message:    fmt.Sprintf("failed to send request: %v", err),
		}
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	var verifyResp VerifyChallengeResponse
	if err := json.NewDecoder(resp.Body).Decode(&verifyResp); err != nil {
		return nil, &HeraldError{
			StatusCode: resp.StatusCode,
			Reason:     "invalid_response",
			Message:    fmt.Sprintf("failed to decode response: %v", err),
		}
	}

	if resp.StatusCode != http.StatusOK {
		return &verifyResp, &HeraldError{
			StatusCode: resp.StatusCode,
			Reason:     verifyResp.Reason,
			Message:    fmt.Sprintf("verification failed: %s", verifyResp.Reason),
		}
	}

	return &verifyResp, nil
}

// addAuthHeaders adds authentication headers to the request
func (c *Client) addAuthHeaders(req *http.Request, body []byte) {
	// Use API key if available
	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}

	// Use HMAC signature if secret is available
	if c.hmacSecret != "" {
		timestamp := strconv.FormatInt(time.Now().Unix(), 10)
		signature := c.computeHMAC(timestamp, c.service, body)

		req.Header.Set("X-Timestamp", timestamp)
		req.Header.Set("X-Service", c.service)
		req.Header.Set("X-Signature", signature)
	}
}

// computeHMAC computes HMAC-SHA256 signature
func (c *Client) computeHMAC(timestamp, service string, body []byte) string {
	message := fmt.Sprintf("%s:%s:%s", timestamp, service, string(body))
	mac := hmac.New(sha256.New, []byte(c.hmacSecret))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}
