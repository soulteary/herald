package handlers

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/soulteary/herald-totp/v2/pkg/heraldtotp"
)

// Keep the SDK wire types and signing helper while owning the transport
// configuration, redirect policy, and bounded response decoding.
type totpProxyClient struct {
	baseURL          *url.URL
	options          heraldtotp.Options
	client           *http.Client
	maxResponseBytes int64
}

func newTOTPProxyClient(opts *heraldtotp.Options, maxResponseBytes int64, requireHTTPS bool) (*totpProxyClient, error) {
	if opts == nil {
		return nil, errors.New("TOTP options are required")
	}
	base, err := url.Parse(opts.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid TOTP base URL: %w", err)
	}
	if (base.Scheme != "http" && base.Scheme != "https") || base.Hostname() == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return nil, errors.New("TOTP base URL must be an HTTP(S) origin or path without credentials, query, or fragment")
	}
	if requireHTTPS && base.Scheme != "https" {
		return nil, errors.New("TOTP base URL must use HTTPS in production")
	}
	copyOptions := *opts
	if copyOptions.Timeout <= 0 {
		copyOptions.Timeout = 10 * time.Second
	}
	if maxResponseBytes <= 0 {
		maxResponseBytes = 1 << 20
	}
	return &totpProxyClient{
		baseURL: base, options: copyOptions, maxResponseBytes: maxResponseBytes,
		client: &http.Client{
			Timeout:       copyOptions.Timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("TOTP redirects are disabled") },
			Transport: &http.Transport{
				Proxy:           http.ProxyFromEnvironment,
				TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
				IdleConnTimeout: 90 * time.Second,
			},
		},
	}, nil
}

func (c *totpProxyClient) call(ctx context.Context, method, endpoint string, query url.Values, payload, result interface{}) error {
	var body []byte
	var err error
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return err
		}
	}
	u := *c.baseURL
	u.Path = strings.TrimRight(u.Path, "/") + endpoint
	u.RawPath = ""
	u.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.options.HMACSecret != "" {
		if err := heraldtotp.SignRequest(req, body, c.options.HMACSecret, c.options.Service, c.options.HMACKeyID); err != nil {
			return fmt.Errorf("sign TOTP request: %w", err)
		}
	} else if c.options.APIKey != "" {
		req.Header.Set("X-API-Key", c.options.APIKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("TOTP request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	return c.decodeResponse(resp, result)
}

func totpResponse[T any](c *totpProxyClient, ctx context.Context, method, path string, query url.Values, payload interface{}) (*T, error) {
	var result T
	if err := c.call(ctx, method, path, query, payload, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *totpProxyClient) Status(ctx context.Context, subject string) (*heraldtotp.StatusResponse, error) {
	return totpResponse[heraldtotp.StatusResponse](c, ctx, http.MethodGet, "/v1/status", url.Values{"subject": {subject}}, nil)
}

func (c *totpProxyClient) Verify(ctx context.Context, req *heraldtotp.VerifyRequest) (*heraldtotp.VerifyResponse, error) {
	return totpResponse[heraldtotp.VerifyResponse](c, ctx, http.MethodPost, "/v1/verify", nil, req)
}

func (c *totpProxyClient) EnrollStart(ctx context.Context, req *heraldtotp.EnrollStartRequest) (*heraldtotp.EnrollStartResponse, error) {
	return totpResponse[heraldtotp.EnrollStartResponse](c, ctx, http.MethodPost, "/v1/enroll/start", nil, req)
}

func (c *totpProxyClient) EnrollConfirm(ctx context.Context, req *heraldtotp.EnrollConfirmRequest) (*heraldtotp.EnrollConfirmResponse, error) {
	return totpResponse[heraldtotp.EnrollConfirmResponse](c, ctx, http.MethodPost, "/v1/enroll/confirm", nil, req)
}

func (c *totpProxyClient) Revoke(ctx context.Context, subject string) (*heraldtotp.RevokeResponse, error) {
	return totpResponse[heraldtotp.RevokeResponse](c, ctx, http.MethodPost, "/v1/revoke", nil, heraldtotp.RevokeRequest{Subject: subject})
}

func (c *totpProxyClient) decodeResponse(resp *http.Response, result interface{}) error {
	readLimit := c.maxResponseBytes
	if readLimit < 1<<63-1 {
		readLimit++
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, readLimit))
	if err != nil {
		return fmt.Errorf("read TOTP response: %w", err)
	}
	if int64(len(data)) > c.maxResponseBytes {
		return errors.New("TOTP response exceeds size limit")
	}
	// All five SDK endpoints use HTTP 200 for their normal response contract.
	// Never decode a success result from a failed HTTP response.
	if resp.StatusCode != http.StatusOK {
		return &heraldtotp.HTTPError{StatusCode: resp.StatusCode}
	}
	if trimmed := bytes.TrimSpace(data); len(trimmed) == 0 || trimmed[0] != '{' {
		return errors.New("TOTP response must be a JSON object")
	}
	if err := json.Unmarshal(data, result); err != nil {
		return fmt.Errorf("invalid TOTP response: %w", err)
	}
	return nil
}
