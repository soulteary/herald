package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultProviderTimeout = 5 * time.Second

type externalAPIResponse struct {
	OK      *bool  `json:"ok"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

func newHTTPClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = defaultProviderTimeout
	}
	return &http.Client{Timeout: timeout}
}

func validateEndpoint(endpoint string) error {
	if endpoint == "" {
		return fmt.Errorf("provider endpoint is not configured")
	}

	parsed, err := url.ParseRequestURI(endpoint)
	if err != nil {
		return fmt.Errorf("provider endpoint is invalid: %w", err)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("provider endpoint scheme must be http or https")
	}

	return nil
}

func postJSON(ctx context.Context, client *http.Client, endpoint string, apiKey string, payload any) (retErr error) {
	if ctx == nil {
		ctx = context.Background()
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("external api request failed: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil && retErr == nil {
			retErr = fmt.Errorf("failed to close response body: %w", cerr)
		}
	}()

	respBody, _ := io.ReadAll(resp.Body)
	respText := strings.TrimSpace(string(respBody))

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		if respText != "" {
			return fmt.Errorf("external api responded with status %d: %s", resp.StatusCode, respText)
		}
		return fmt.Errorf("external api responded with status %d", resp.StatusCode)
	}

	if respText == "" {
		return nil
	}

	var apiResp externalAPIResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil
	}

	if apiResp.OK != nil && !*apiResp.OK {
		if apiResp.Error != "" {
			return fmt.Errorf("external api error: %s", apiResp.Error)
		}
		if apiResp.Message != "" {
			return fmt.Errorf("external api error: %s", apiResp.Message)
		}
		return fmt.Errorf("external api error: ok=false")
	}

	return nil
}
