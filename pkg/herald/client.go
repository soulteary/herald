package herald

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
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

	// sigVersion selects the HMAC signing scheme ("v2" default via NewClient,
	// "v1" legacy). A bare &Client{} literal (used in some tests) has an empty
	// value which is treated as v1 for backward compatibility.
	sigVersion string
	keyID      string

	// now and newNonce are injectable for deterministic tests.
	now      func() time.Time
	newNonce func() string

	maxResponseBytes int64
}

// Signature scheme identifiers.
const (
	SignatureV1 = "v1"
	SignatureV2 = "v2"

	hmacV2CanonicalPrefix = "HERALD-HMAC-V2"
)

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

	// SignatureVersion selects the HMAC scheme. Defaults to v2 (replay-resistant).
	// Set to v1 only during a migration window against an old server.
	SignatureVersion string
	// KeyID is sent as X-Key-Id and bound into the v2 signature. Empty means the
	// server's configured default key id is used.
	KeyID string
	// MaxResponseBytes caps every response body read by the SDK. Zero uses the
	// default 1 MiB limit.
	MaxResponseBytes int64
}

const defaultMaxResponseBytes int64 = 1 << 20

// DefaultOptions returns default options
func DefaultOptions() *Options {
	return &Options{
		Timeout:          10 * time.Second,
		Service:          "stargate",
		SignatureVersion: SignatureV2,
		MaxResponseBytes: defaultMaxResponseBytes,
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

// WithSignatureVersion selects the HMAC signature scheme (v2 default, v1 legacy).
func (o *Options) WithSignatureVersion(v string) *Options {
	o.SignatureVersion = v
	return o
}

// WithKeyID sets the HMAC key id sent as X-Key-Id and bound into the signature.
func (o *Options) WithKeyID(id string) *Options {
	o.KeyID = id
	return o
}

// WithMaxResponseBytes sets the maximum response body size accepted by the SDK.
func (o *Options) WithMaxResponseBytes(limit int64) *Options {
	o.MaxResponseBytes = limit
	return o
}

// Validate validates the options
func (o *Options) Validate() error {
	if o.BaseURL == "" {
		return fmt.Errorf("base URL is required")
	}
	parsed, err := url.Parse(o.BaseURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("base URL must be an absolute http or https URL")
	}
	if o.Timeout < 0 {
		return fmt.Errorf("timeout must not be negative")
	}
	if o.MaxResponseBytes < 0 {
		return fmt.Errorf("max response bytes must not be negative")
	}
	if (o.TLSClientCert == "") != (o.TLSClientKey == "") {
		return fmt.Errorf("TLS client certificate and key must be configured together")
	}
	if o.SignatureVersion != "" && o.SignatureVersion != SignatureV1 && o.SignatureVersion != SignatureV2 {
		return fmt.Errorf("signature version must be v1 or v2")
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

	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	maxResponseBytes := opts.MaxResponseBytes
	if maxResponseBytes == 0 {
		maxResponseBytes = defaultMaxResponseBytes
	}
	clientOpts := &httpkit.Options{
		BaseURL:            strings.TrimRight(opts.BaseURL, "/"),
		Timeout:            timeout,
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
		httpClient:       httpClient,
		baseURL:          strings.TrimRight(opts.BaseURL, "/"),
		apiKey:           opts.APIKey,
		hmacSecret:       opts.HMACSecret,
		service:          opts.Service,
		sigVersion:       opts.SignatureVersion,
		keyID:            opts.KeyID,
		now:              time.Now,
		newNonce:         defaultNonce,
		maxResponseBytes: maxResponseBytes,
	}
	if client.sigVersion == "" {
		client.sigVersion = SignatureV2
	}

	return client, nil
}

// defaultNonce returns a random 128-bit hex nonce.
func defaultNonce() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Fall back to a timestamp-derived value; the server's single-use nonce
		// store still prevents replay even if entropy is degraded.
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(b[:])
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
	ChallengeID    string `json:"challenge_id"`
	ExpiresIn      int    `json:"expires_in"`
	NextResendIn   int    `json:"next_resend_in"`
	DeliveryStatus string `json:"delivery_status,omitempty"`
	// DebugCode is set by Herald only when HERALD_TEST_MODE=true (for debugging)
	DebugCode string `json:"debug_code,omitempty"`
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

// VerifyChallengeV2Request binds verification to the expected challenge
// context, preventing a valid code from being used for a different flow.
type VerifyChallengeV2Request struct {
	ChallengeID     string `json:"challenge_id"`
	Code            string `json:"code"`
	ClientIP        string `json:"client_ip,omitempty"`
	ExpectedUserID  string `json:"expected_user_id,omitempty"`
	ExpectedPurpose string `json:"expected_purpose,omitempty"`
	ExpectedChannel string `json:"expected_channel,omitempty"`
}

// VerifyChallengeV2Response is returned by /v2/otp/verifications.
type VerifyChallengeV2Response struct {
	OK                bool     `json:"ok"`
	UserID            string   `json:"user_id,omitempty"`
	Purpose           string   `json:"purpose,omitempty"`
	Channel           string   `json:"channel,omitempty"`
	ChallengeID       string   `json:"challenge_id,omitempty"`
	AMR               []string `json:"amr,omitempty"`
	VerifiedAt        int64    `json:"verified_at,omitempty"`
	IssuedAt          int64    `json:"issued_at,omitempty"`
	Reason            string   `json:"reason,omitempty"`
	RemainingAttempts *int     `json:"remaining_attempts,omitempty"`
}

// RevokeChallengeResponse is returned after revoking a challenge.
type RevokeChallengeResponse struct {
	OK     bool   `json:"ok"`
	Reason string `json:"reason,omitempty"`
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
	bodyBytes, readErr := c.readResponseBody(resp)
	if readErr != nil {
		return nil, responseReadError(resp.StatusCode, readErr)
	}

	if resp.StatusCode != http.StatusOK {
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
	if err := json.Unmarshal(bodyBytes, &challengeResp); err != nil {
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
	respBody, readErr := c.readResponseBody(resp)
	if readErr != nil {
		return nil, responseReadError(resp.StatusCode, readErr)
	}

	var verifyResp VerifyChallengeResponse
	if err := json.Unmarshal(respBody, &verifyResp); err != nil {
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

// VerifyChallengeV2 performs context-bound verification using the v2 endpoint.
func (c *Client) VerifyChallengeV2(ctx context.Context, req *VerifyChallengeV2Request) (*VerifyChallengeV2Response, error) {
	endpoint := fmt.Sprintf("%s/v2/otp/verifications", c.baseURL)
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	c.httpClient.InjectTraceContext(ctx, httpReq)
	c.addAuthHeaders(httpReq, body)
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, &HeraldError{StatusCode: 0, Reason: "connection_failed", Message: err.Error()}
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := c.readResponseBody(resp)
	if err != nil {
		return nil, responseReadError(resp.StatusCode, err)
	}
	var out VerifyChallengeV2Response
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, &HeraldError{StatusCode: resp.StatusCode, Reason: "invalid_response", Message: err.Error()}
	}
	if resp.StatusCode != http.StatusOK {
		return &out, &HeraldError{StatusCode: resp.StatusCode, Reason: out.Reason, Message: string(respBody)}
	}
	return &out, nil
}

// RevokeChallenge revokes a challenge by id.
func (c *Client) RevokeChallenge(ctx context.Context, challengeID string) (*RevokeChallengeResponse, error) {
	if challengeID == "" {
		return nil, fmt.Errorf("challenge ID is required")
	}
	endpoint := fmt.Sprintf("%s/v1/otp/challenges/%s/revoke", c.baseURL, url.PathEscape(challengeID))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	c.httpClient.InjectTraceContext(ctx, httpReq)
	c.addAuthHeaders(httpReq, nil)
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, &HeraldError{StatusCode: 0, Reason: "connection_failed", Message: err.Error()}
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := c.readResponseBody(resp)
	if err != nil {
		return nil, responseReadError(resp.StatusCode, err)
	}
	var out RevokeChallengeResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, &HeraldError{StatusCode: resp.StatusCode, Reason: "invalid_response", Message: err.Error()}
	}
	if resp.StatusCode != http.StatusOK {
		return &out, &HeraldError{StatusCode: resp.StatusCode, Reason: out.Reason, Message: string(respBody)}
	}
	return &out, nil
}

func (c *Client) readResponseBody(resp *http.Response) ([]byte, error) {
	limit := c.maxResponseBytes
	if limit <= 0 {
		limit = defaultMaxResponseBytes
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("response exceeds %d byte limit", limit)
	}
	return body, nil
}

func responseReadError(status int, err error) *HeraldError {
	return &HeraldError{StatusCode: status, Reason: "response_too_large", Message: err.Error()}
}

// --- TOTP (proxied by Herald to herald-totp) ---

// TOTPStatusResponse is the response from GET /v1/totp/status.
type TOTPStatusResponse struct {
	Subject     string `json:"subject"`
	TotpEnabled bool   `json:"totp_enabled"`
}

// TOTPVerifyRequest is the request for POST /v1/totp/verify.
type TOTPVerifyRequest struct {
	Subject     string `json:"subject"`
	Code        string `json:"code"`
	ChallengeID string `json:"challenge_id,omitempty"`
}

// TOTPVerifyResponse is the response from POST /v1/totp/verify.
type TOTPVerifyResponse struct {
	OK     bool   `json:"ok"`
	Reason string `json:"reason,omitempty"`
}

// TOTPEnrollStartRequest is the request for POST /v1/totp/enroll/start.
type TOTPEnrollStartRequest struct {
	Subject string `json:"subject"`
	Label   string `json:"label"`
}

// TOTPEnrollStartResponse is the response from POST /v1/totp/enroll/start.
type TOTPEnrollStartResponse struct {
	EnrollID     string `json:"enroll_id"`
	SecretBase32 string `json:"secret_base32,omitempty"`
	OtpauthURI   string `json:"otpauth_uri"`
}

// TOTPEnrollConfirmRequest is the request for POST /v1/totp/enroll/confirm.
type TOTPEnrollConfirmRequest struct {
	EnrollID string `json:"enroll_id"`
	Code     string `json:"code"`
}

// TOTPEnrollConfirmResponse is the response from POST /v1/totp/enroll/confirm.
type TOTPEnrollConfirmResponse struct {
	Subject     string   `json:"subject"`
	TotpEnabled bool     `json:"totp_enabled"`
	BackupCodes []string `json:"backup_codes,omitempty"`
}

// TOTPRevokeResponse is the response from POST /v1/totp/revoke.
type TOTPRevokeResponse struct {
	OK      bool   `json:"ok"`
	Subject string `json:"subject"`
}

// TOTPStatus returns whether the subject has TOTP enabled (via Herald TOTP proxy).
func (c *Client) TOTPStatus(ctx context.Context, subject string) (*TOTPStatusResponse, error) {
	url := fmt.Sprintf("%s/v1/totp/status?subject=%s", c.baseURL, url.QueryEscape(subject))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	c.httpClient.InjectTraceContext(ctx, httpReq)
	c.addAuthHeaders(httpReq, nil)
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, &HeraldError{StatusCode: 0, Reason: "connection_failed", Message: err.Error()}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := c.readResponseBody(resp)
	if err != nil {
		return nil, responseReadError(resp.StatusCode, err)
	}
	var out TOTPStatusResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, &HeraldError{StatusCode: resp.StatusCode, Reason: "invalid_response", Message: string(body)}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &HeraldError{StatusCode: resp.StatusCode, Reason: "totp_proxy_failed", Message: string(body)}
	}
	return &out, nil
}

// TOTPVerify verifies a TOTP code for the subject (via Herald TOTP proxy).
func (c *Client) TOTPVerify(ctx context.Context, req *TOTPVerifyRequest) (*TOTPVerifyResponse, error) {
	url := fmt.Sprintf("%s/v1/totp/verify", c.baseURL)
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	c.httpClient.InjectTraceContext(ctx, httpReq)
	c.addAuthHeaders(httpReq, body)
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, &HeraldError{StatusCode: 0, Reason: "connection_failed", Message: err.Error()}
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := c.readResponseBody(resp)
	if err != nil {
		return nil, responseReadError(resp.StatusCode, err)
	}
	var out TOTPVerifyResponse
	_ = json.Unmarshal(respBody, &out)
	if resp.StatusCode != http.StatusOK {
		return &out, &HeraldError{StatusCode: resp.StatusCode, Reason: out.Reason, Message: string(respBody)}
	}
	return &out, nil
}

// TOTPEnrollStart starts TOTP enrollment (via Herald TOTP proxy).
func (c *Client) TOTPEnrollStart(ctx context.Context, req *TOTPEnrollStartRequest) (*TOTPEnrollStartResponse, error) {
	url := fmt.Sprintf("%s/v1/totp/enroll/start", c.baseURL)
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	c.httpClient.InjectTraceContext(ctx, httpReq)
	c.addAuthHeaders(httpReq, body)
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, &HeraldError{StatusCode: 0, Reason: "connection_failed", Message: err.Error()}
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := c.readResponseBody(resp)
	if err != nil {
		return nil, responseReadError(resp.StatusCode, err)
	}
	var out TOTPEnrollStartResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, &HeraldError{StatusCode: resp.StatusCode, Reason: "invalid_response", Message: string(respBody)}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &HeraldError{StatusCode: resp.StatusCode, Reason: "totp_proxy_failed", Message: string(respBody)}
	}
	return &out, nil
}

// TOTPEnrollConfirm confirms TOTP enrollment (via Herald TOTP proxy).
func (c *Client) TOTPEnrollConfirm(ctx context.Context, req *TOTPEnrollConfirmRequest) (*TOTPEnrollConfirmResponse, error) {
	url := fmt.Sprintf("%s/v1/totp/enroll/confirm", c.baseURL)
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	c.httpClient.InjectTraceContext(ctx, httpReq)
	c.addAuthHeaders(httpReq, body)
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, &HeraldError{StatusCode: 0, Reason: "connection_failed", Message: err.Error()}
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := c.readResponseBody(resp)
	if err != nil {
		return nil, responseReadError(resp.StatusCode, err)
	}
	var out TOTPEnrollConfirmResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, &HeraldError{StatusCode: resp.StatusCode, Reason: "invalid_response", Message: string(respBody)}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &HeraldError{StatusCode: resp.StatusCode, Reason: "invalid", Message: string(respBody)}
	}
	return &out, nil
}

// TOTPRevoke revokes TOTP for the subject (via Herald TOTP proxy).
func (c *Client) TOTPRevoke(ctx context.Context, subject string) (*TOTPRevokeResponse, error) {
	url := fmt.Sprintf("%s/v1/totp/revoke", c.baseURL)
	body, err := json.Marshal(struct {
		Subject string `json:"subject"`
	}{Subject: subject})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	c.httpClient.InjectTraceContext(ctx, httpReq)
	c.addAuthHeaders(httpReq, body)
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, &HeraldError{StatusCode: 0, Reason: "connection_failed", Message: err.Error()}
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := c.readResponseBody(resp)
	if err != nil {
		return nil, responseReadError(resp.StatusCode, err)
	}
	var out TOTPRevokeResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, &HeraldError{StatusCode: resp.StatusCode, Reason: "invalid_response", Message: string(respBody)}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &HeraldError{StatusCode: resp.StatusCode, Reason: "proxy_failed", Message: string(respBody)}
	}
	return &out, nil
}

// addAuthHeaders adds authentication headers to the request. It signs with the
// configured scheme (v2 by default). It never downgrades: if a scheme is
// selected, only that scheme's headers are produced.
func (c *Client) addAuthHeaders(req *http.Request, body []byte) {
	// Use API key if available
	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}

	if c.hmacSecret == "" {
		return
	}

	now := c.now
	if now == nil {
		now = time.Now
	}
	timestamp := strconv.FormatInt(now().Unix(), 10)

	if c.sigVersion != SignatureV2 {
		signature := c.computeHMAC(timestamp, c.service, body)
		req.Header.Set("X-Timestamp", timestamp)
		req.Header.Set("X-Service", c.service)
		req.Header.Set("X-Signature", signature)
		return
	}

	// v2: replay-resistant canonical binding with a single-use nonce.
	nonceFn := c.newNonce
	if nonceFn == nil {
		nonceFn = defaultNonce
	}
	nonce := nonceFn()

	query := ""
	if req.URL != nil {
		query = req.URL.RawQuery
	}
	path := ""
	if req.URL != nil {
		path = req.URL.Path
	}

	signature := c.computeHMACv2(req.Method, path, query, timestamp, nonce, c.service, c.keyID, body)

	req.Header.Set("X-Signature-Version", SignatureV2)
	req.Header.Set("X-Signature", signature)
	req.Header.Set("X-Timestamp", timestamp)
	req.Header.Set("X-Nonce", nonce)
	req.Header.Set("X-Service", c.service)
	if c.keyID != "" {
		req.Header.Set("X-Key-Id", c.keyID)
	}
}

// computeHMAC computes the legacy v1 HMAC-SHA256 signature.
func (c *Client) computeHMAC(timestamp, service string, body []byte) string {
	message := fmt.Sprintf("%s:%s:%s", timestamp, service, string(body))
	mac := hmac.New(sha256.New, []byte(c.hmacSecret))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

// computeHMACv2 builds the v2 canonical string and signs it. The canonical form
// MUST match internal/auth.CanonicalRequest.Canonical() on the server:
//
//	HERALD-HMAC-V2\nMETHOD\nPATH\nQUERY\nTS\nNONCE\nSERVICE\nKEYID\nsha256hex(body)
func (c *Client) computeHMACv2(method, path, query, timestamp, nonce, service, keyID string, body []byte) string {
	bodyHash := sha256.Sum256(body)
	var b strings.Builder
	b.WriteString(hmacV2CanonicalPrefix)
	b.WriteByte('\n')
	b.WriteString(strings.ToUpper(method))
	b.WriteByte('\n')
	b.WriteString(path)
	b.WriteByte('\n')
	b.WriteString(query)
	b.WriteByte('\n')
	b.WriteString(timestamp)
	b.WriteByte('\n')
	b.WriteString(nonce)
	b.WriteByte('\n')
	b.WriteString(service)
	b.WriteByte('\n')
	b.WriteString(keyID)
	b.WriteByte('\n')
	b.WriteString(hex.EncodeToString(bodyHash[:]))

	mac := hmac.New(sha256.New, []byte(c.hmacSecret))
	mac.Write([]byte(b.String()))
	return hex.EncodeToString(mac.Sum(nil))
}
