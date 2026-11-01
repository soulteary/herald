package handlers

import (
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
)

func TestIdempotencyFingerprintCannotAliasDifferentRequests(t *testing.T) {
	app, p := newIdemApp(t)
	const separatorFields = "\x00email\x00a@example.com\x00login\x00"
	requests := []CreateChallengeRequest{
		{UserID: "user" + separatorFields + "first", Channel: "email", Destination: "a@example.com", Purpose: "login", Locale: "second"},
		{UserID: "user", Channel: "email", Destination: "a@example.com", Purpose: "login", Locale: "first" + separatorFields + "second"},
	}
	responses := make([]*fiberTestResp, 0, 2)
	for _, request := range requests {
		payload, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		response, err := idemRequest(app, "same-key", string(payload))
		if err != nil {
			t.Fatal(err)
		}
		responses = append(responses, response)
	}
	if responses[0].status != 200 {
		t.Fatalf("first create: status=%d body=%s", responses[0].status, responses[0].body)
	}
	if responses[1].status != 409 || !strings.Contains(string(responses[1].body), "idempotency_conflict") {
		t.Fatalf("different user/locale replayed: first=%d %s second=%d %s provider calls=%d", responses[0].status, responses[0].body, responses[1].status, responses[1].body, atomic.LoadInt64(&p.calls))
	}
	if calls := atomic.LoadInt64(&p.calls); calls != 1 {
		t.Fatalf("conflicting request sent another code: %d calls", calls)
	}
	// The original request remains replayable, including its embedded NULs.
	payload, err := json.Marshal(requests[0])
	if err != nil {
		t.Fatal(err)
	}
	replay, err := idemRequest(app, "same-key", string(payload))
	if err != nil {
		t.Fatal(err)
	}
	if replay.status != 200 || string(replay.body) != string(responses[0].body) || atomic.LoadInt64(&p.calls) != 1 {
		t.Fatalf("original replay changed: %d %s calls=%d", replay.status, replay.body, atomic.LoadInt64(&p.calls))
	}
}
