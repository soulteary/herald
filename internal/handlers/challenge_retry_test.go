package handlers

import (
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
)

func TestChallengeRateLimitResponse(t *testing.T) {
	for _, test := range []struct {
		name  string
		reset time.Time
		known bool
	}{
		{"current cooldown", time.Now().Add(120 * time.Second), true},
		{"unknown reset", time.Time{}, false},
		{"expired reset", time.Now().Add(-time.Second), false},
		{"excessive reset", time.Now().Add(48 * time.Hour), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			app := fiber.New()
			app.Get("/", func(c fiber.Ctx) error {
				_, err := challengeRateLimitResponse(c, "resend_cooldown", test.reset)
				return err
			})
			response, err := app.Test(httptest.NewRequest("GET", "/", nil))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = response.Body.Close() }()
			var body struct {
				OK     bool   `json:"ok"`
				Reason string `json:"reason"`
				Next   *int   `json:"next_resend_in"`
			}
			if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != 429 || body.OK || body.Reason != "resend_cooldown" {
				t.Fatal("existing rate-limit response changed")
			}
			if test.known {
				seconds, err := strconv.Atoi(response.Header.Get("Retry-After"))
				if err != nil || body.Next == nil || *body.Next != seconds || seconds < 119 || seconds > 120 {
					t.Fatalf("invalid retry response: header=%q next=%v", response.Header.Get("Retry-After"), body.Next)
				}
			} else if body.Next != nil || response.Header.Get("Retry-After") != "" {
				t.Fatal("unknown timing must remain absent")
			}
		})
	}
}
