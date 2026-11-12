package herald

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientIPHeader(t *testing.T) {
	const header = "X-Forwarded-For"

	tests := []struct {
		name     string
		header   string
		clientIP string
		want     string
	}{
		{name: "disabled by default", header: "", clientIP: "203.0.113.7", want: ""},
		{name: "ipv4", header: header, clientIP: "203.0.113.7", want: "203.0.113.7"},
		{name: "ipv6 is normalised", header: header, clientIP: "2001:DB8::0001", want: "2001:db8::1"},
		{name: "surrounding space", header: header, clientIP: " 203.0.113.7 ", want: "203.0.113.7"},
		{name: "empty", header: header, clientIP: "", want: ""},
		{name: "not an address", header: header, clientIP: "203.0.113.7, 10.0.0.1", want: ""},
		{name: "header injection attempt", header: header, clientIP: "203.0.113.7\r\nX-Service: admin", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = append(got, r.Header.Get(header))
				switch r.URL.Path {
				case "/v1/otp/challenges":
					_ = json.NewEncoder(w).Encode(CreateChallengeResponse{ChallengeID: "c1"})
				case "/v1/otp/verifications":
					_ = json.NewEncoder(w).Encode(VerifyChallengeResponse{OK: true})
				default:
					_ = json.NewEncoder(w).Encode(VerifyChallengeV2Response{OK: true})
				}
			}))
			defer server.Close()

			client, err := NewClient(DefaultOptions().
				WithBaseURL(server.URL).
				WithHMACSecret("secret").
				WithClientIPHeader(tt.header))
			require.NoError(t, err)

			ctx := context.Background()
			_, err = client.CreateChallenge(ctx, &CreateChallengeRequest{
				UserID: "u", Channel: "email", Destination: "u@example.com", ClientIP: tt.clientIP,
			})
			require.NoError(t, err)
			_, err = client.VerifyChallenge(ctx, &VerifyChallengeRequest{ChallengeID: "c1", Code: "123456", ClientIP: tt.clientIP})
			require.NoError(t, err)
			_, err = client.VerifyChallengeV2(ctx, &VerifyChallengeV2Request{
				ChallengeID: "c1", Code: "123456", ClientIP: tt.clientIP,
				ExpectedUserID: "u", ExpectedPurpose: "login", ExpectedChannel: "email",
			})
			require.NoError(t, err)

			assert.Equal(t, []string{tt.want, tt.want, tt.want}, got)
		})
	}
}

func TestClientIPHeaderValidation(t *testing.T) {
	for _, name := range []string{"", "X-Forwarded-For", "X-Real-IP", "x-client-ip"} {
		opts := DefaultOptions().WithBaseURL("http://herald.test").WithClientIPHeader(name)
		assert.NoError(t, opts.Validate(), "header %q", name)
	}
	for _, name := range []string{"X Forwarded", "X-Fwd:", "X-Fwd\r\n", "x-signature", "X-API-Key", "Idempotency-Key"} {
		opts := DefaultOptions().WithBaseURL("http://herald.test").WithClientIPHeader(name)
		assert.Error(t, opts.Validate(), "header %q", name)
	}
}
