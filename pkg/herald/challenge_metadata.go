package herald

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// CreateChallengeMetadata contains optional timing for a rejected creation.
// A nil RetryAfterSeconds means that the server supplied no usable deadline.
// The time is an earliest retry opportunity, not a guarantee of admission.
type CreateChallengeMetadata struct {
	RetryAfterSeconds *int
}

const maximumChallengeRetrySeconds = 24 * 60 * 60

func validChallengeRetry(seconds int) bool {
	return seconds > 0 && seconds <= maximumChallengeRetrySeconds
}

func challengeRetryMetadata(status int, headers http.Header, body []byte) CreateChallengeMetadata {
	unknown := CreateChallengeMetadata{}
	if status != http.StatusTooManyRequests {
		return unknown
	}
	var envelope struct {
		NextResendIn json.RawMessage `json:"next_resend_in"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return unknown
	}
	var seconds *int
	if len(envelope.NextResendIn) > 0 {
		var value int
		if err := json.Unmarshal(envelope.NextResendIn, &value); err != nil || !validChallengeRetry(value) {
			return unknown
		}
		seconds = &value
	}
	values := headers.Values("Retry-After")
	if len(values) > 1 {
		return unknown
	}
	if len(values) == 1 {
		value := values[0]
		if value == "" {
			return unknown
		}
		for _, character := range value {
			if character < '0' || character > '9' {
				return unknown
			}
		}
		parsed, err := strconv.Atoi(value)
		if err != nil || !validChallengeRetry(parsed) || (seconds != nil && *seconds != parsed) {
			return unknown
		}
		seconds = &parsed
	}
	return CreateChallengeMetadata{RetryAfterSeconds: seconds}
}
