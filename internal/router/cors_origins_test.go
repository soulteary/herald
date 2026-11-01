package router

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/soulteary/herald/internal/config"
	"github.com/soulteary/herald/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestProductionRouterRejectsWildcardInCORSOriginList(t *testing.T) {
	defer withV2Config(t)()
	oldEnv, oldOrigins := config.Env, config.CORSAllowOrigins
	t.Cleanup(func() { config.Env, config.CORSAllowOrigins = oldEnv, oldOrigins })
	config.Env = config.EnvProduction
	rc, _ := testutil.NewTestRedisClient()
	t.Cleanup(func() { _ = rc.Close() })
	for _, origins := range []string{"https://trusted.example,*", "*,https://trusted.example", "https://one.example, * ,https://two.example"} {
		config.CORSAllowOrigins = origins
		rw, err := NewRouterWithClientAndHandlersE(rc, testLogger())
		if err == nil {
			req := httptest.NewRequest("OPTIONS", "/v1/otp/challenges", nil)
			req.Header.Set("Origin", "https://untrusted.example")
			req.Header.Set("Access-Control-Request-Method", "POST")
			req.Header.Set("Access-Control-Request-Headers", "Content-Type,X-Signature")
			resp, requestErr := rw.App.Test(req)
			require.NoError(t, requestErr)
			t.Logf("origins=%q untrusted preflight status=%d allow-origin=%q", origins, resp.StatusCode, resp.Header.Get("Access-Control-Allow-Origin"))
			require.NoError(t, resp.Body.Close())
			unsigned := httptest.NewRequest("POST", "/v1/otp/challenges", strings.NewReader(`{}`))
			unsigned.Header.Set("Content-Type", "application/json")
			resp, requestErr = rw.App.Test(unsigned)
			require.NoError(t, requestErr)
			t.Logf("unsigned business request status=%d", resp.StatusCode)
			require.Equal(t, 401, resp.StatusCode, "CORS does not replace request authentication")
			require.NoError(t, resp.Body.Close())
		}
		require.ErrorContains(t, err, "CORS wildcard", "production must refuse a CORS list containing a wildcard")
		require.Nil(t, rw)
	}
}

func TestRouterCORSOriginListCompatibility(t *testing.T) {
	defer withV2Config(t)()
	oldEnv, oldOrigins := config.Env, config.CORSAllowOrigins
	t.Cleanup(func() { config.Env, config.CORSAllowOrigins = oldEnv, oldOrigins })
	rc, _ := testutil.NewTestRedisClient()
	t.Cleanup(func() { _ = rc.Close() })
	for _, tc := range []struct {
		name     string
		env      config.Environment
		origins  string
		wildcard bool
	}{
		{"production explicit", config.EnvProduction, " https://trusted.example , https://other.example ", false},
		{"production disabled", config.EnvProduction, " \t ", false},
		{"development wildcard", config.EnvDevelopment, "https://trusted.example, *", true},
		{"test wildcard", config.EnvTest, "*", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config.Env, config.CORSAllowOrigins = tc.env, tc.origins
			rw, err := NewRouterWithClientAndHandlersE(rc, testLogger())
			require.NoError(t, err)
			for _, method := range []string{"GET", "OPTIONS"} {
				for _, origin := range []string{"https://trusted.example", "https://untrusted.example"} {
					req := httptest.NewRequest(method, "/livez", nil)
					req.Header.Set("Origin", origin)
					if method == "OPTIONS" {
						req.Header.Set("Access-Control-Request-Method", "GET")
					}
					resp, err := rw.App.Test(req)
					require.NoError(t, err)
					want := ""
					if tc.wildcard {
						want = "*"
					} else if strings.TrimSpace(tc.origins) != "" && origin == "https://trusted.example" {
						want = origin
					}
					require.Equal(t, want, resp.Header.Get("Access-Control-Allow-Origin"))
					require.NoError(t, resp.Body.Close())
				}
			}
			req := httptest.NewRequest("POST", "/v1/otp/challenges", strings.NewReader(`{}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Origin", "https://untrusted.example")
			resp, err := rw.App.Test(req)
			require.NoError(t, err)
			require.Equal(t, 401, resp.StatusCode)
			require.NoError(t, resp.Body.Close())
		})
	}
}
