package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	challengekit "github.com/soulteary/challenge-kit"
	"github.com/soulteary/herald/internal/config"
	provider "github.com/soulteary/provider-kit"
	"github.com/stretchr/testify/require"
)

func TestHashBudgetAdmissionAndCancellation(t *testing.T) {
	for _, limit := range []int{0, -1} {
		_, err := newHashBudget(limit)
		require.Error(t, err)
	}
	budget, err := newHashBudget(1)
	require.NoError(t, err)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = budget.acquire(canceled)
	require.ErrorIs(t, err, context.Canceled)
	release, err := budget.acquire(context.Background())
	require.NoError(t, err)
	_, err = budget.acquire(context.Background())
	require.ErrorIs(t, err, errHashCapacity)
	release()
	release, err = budget.acquire(context.Background())
	require.NoError(t, err)
	release()
	var missing *hashBudget
	_, err = missing.acquire(context.Background())
	require.ErrorIs(t, err, errHashCapacity)
	old := config.MaxConcurrentHashOperations
	defer func() { config.MaxConcurrentHashOperations = old }()
	config.MaxConcurrentHashOperations = 0
	h, err := NewHandlersWithError(nil, testLogger())
	require.Error(t, err)
	require.Nil(t, h)
}

func budgetVerifyRequest(t *testing.T, app *fiber.App, path, id, code string) (int, []byte, string) {
	t.Helper()
	body := map[string]string{"challenge_id": id, "code": code}
	if path == "/v2/otp/verifications" {
		body["expected_user_id"] = "victim"
		body["expected_channel"] = "email"
		body["expected_purpose"] = "login"
	}
	data, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest("POST", path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	payload, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, payload, resp.Header.Get("Retry-After")
}

func TestHashBudgetSharesRoutesWithoutConsumingRejectedRequests(t *testing.T) {
	rc := testRedisClient(t)
	t.Cleanup(func() { _ = rc.Close() })
	h := newIdemHandlers(t, rc)
	budget, err := newHashBudget(1)
	require.NoError(t, err)
	h.hashBudget = budget
	victim, code := createActivationOTP(t, h.challengeManager, "victim")
	release := make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	blocking := &blockingHashManager{ManagerInterface: h.challengeManager, entered: make(chan struct{}, 1), release: release}
	h.challengeManager = blocking
	app := fiber.New()
	app.Post("/v1/otp/challenges", h.CreateChallenge)
	app.Post("/v1/otp/verifications", h.VerifyChallenge)
	app.Post("/v2/otp/verifications", h.VerifyChallengeV2)
	done := make(chan error, 1)
	go func() {
		_, err := idemRequest(app, "holder", `{"user_id":"holder","channel":"email","destination":"holder@example.com","purpose":"login"}`)
		done <- err
	}()
	select {
	case <-blocking.entered:
	case <-time.After(4 * time.Second):
		t.Fatal("holder did not enter hash stage")
	}
	before := rc.Keys(context.Background(), "*").Val()
	for _, path := range []string{"/v1/otp/verifications", "/v2/otp/verifications"} {
		status, body, retry := budgetVerifyRequest(t, app, path, victim.ID, code)
		require.Equal(t, 503, status)
		require.Contains(t, string(body), "backend_unavailable")
		require.Equal(t, "1", retry)
	}
	body := `{"user_id":"overload","channel":"email","destination":"overload@example.com","purpose":"login"}`
	rejected, err := idemRequest(app, "overload", body)
	require.NoError(t, err)
	require.Equal(t, 503, rejected.status)
	require.ElementsMatch(t, before, rc.Keys(context.Background(), "*").Val(), "overload must not retain an idempotency claim or reserve a cooldown")
	stored, err := blocking.Get(context.Background(), victim.ID)
	require.NoError(t, err)
	require.Zero(t, stored.Attempts)
	once.Do(func() { close(release) })
	require.NoError(t, <-done)
	h.challengeManager = blocking.ManagerInterface
	cp := &countingProvider{channel: provider.ChannelEmail}
	h.providerRegistry = provider.NewRegistry()
	require.NoError(t, h.providerRegistry.Register(cp))
	successful, err := idemRequest(app, "overload", body)
	require.NoError(t, err)
	require.Equal(t, 200, successful.status)
	// A completed replay needs no crypto slot and must remain available under load.
	held, err := budget.acquire(context.Background())
	require.NoError(t, err)
	replay, err := idemRequest(app, "overload", body)
	held()
	require.NoError(t, err)
	require.Equal(t, 200, replay.status)
	require.JSONEq(t, string(successful.body), string(replay.body))
	require.EqualValues(t, 1, cp.calls)
	status, _, _ := budgetVerifyRequest(t, app, "/v2/otp/verifications", victim.ID, code)
	require.Equal(t, 200, status)
}

type heldDelivery struct {
	entered chan struct{}
	release <-chan struct{}
}

func (p *heldDelivery) Send(context.Context, *provider.Message) (*provider.SendResult, error) {
	p.entered <- struct{}{}
	<-p.release
	return provider.NewSuccessResult("held", provider.ChannelEmail, "sent"), nil
}
func (*heldDelivery) Name() string              { return "held" }
func (*heldDelivery) Channel() provider.Channel { return provider.ChannelEmail }
func (*heldDelivery) Validate() error           { return nil }

func TestHashBudgetReleasedBeforeProviderDelivery(t *testing.T) {
	rc := testRedisClient(t)
	t.Cleanup(func() { _ = rc.Close() })
	h := newIdemHandlers(t, rc)
	budget, err := newHashBudget(1)
	require.NoError(t, err)
	h.hashBudget = budget
	victim, code := createActivationOTP(t, h.challengeManager, "victim")
	release := make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	p := &heldDelivery{entered: make(chan struct{}, 1), release: release}
	h.providerRegistry = provider.NewRegistry()
	require.NoError(t, h.providerRegistry.Register(p))
	app := fiber.New()
	app.Post("/v1/otp/challenges", h.CreateChallenge)
	app.Post("/v1/otp/verifications", h.VerifyChallenge)
	done := make(chan error, 1)
	go func() { _, err := idemRequest(app, "held-delivery", faultBody); done <- err }()
	select {
	case <-p.entered:
	case <-time.After(4 * time.Second):
		t.Fatal("provider did not receive delivery")
	}
	status, _, _ := budgetVerifyRequest(t, app, "/v1/otp/verifications", victim.ID, code)
	require.Equal(t, 200, status)
	once.Do(func() { close(release) })
	require.NoError(t, <-done)
}

type failingBudgetVerifier struct {
	challengekit.ManagerInterface
	panicOnVerify bool
}

func (m *failingBudgetVerifier) Verify(context.Context, string, string, string) (*challengekit.VerifyResult, error) {
	if m.panicOnVerify {
		panic("synthetic verifier panic")
	}
	return nil, errors.New("synthetic verifier error")
}
func TestVerificationBudgetReleasedAfterFailureOrPanic(t *testing.T) {
	budget, err := newHashBudget(1)
	require.NoError(t, err)
	verifier := &failingBudgetVerifier{}
	h := &Handlers{hashBudget: budget, challengeManager: verifier}
	_, err = h.verifyWithBudget(context.Background(), "id", "code", "", nil)
	require.Error(t, err)
	verifier.panicOnVerify = true
	require.Panics(t, func() { _, _ = h.verifyWithBudget(context.Background(), "id", "code", "", nil) })
	release, err := budget.acquire(context.Background())
	require.NoError(t, err)
	release()
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := h.verifyWithBudget(canceled, "id", "code", "", nil)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, challengekit.ReasonBackendUnavailable, result.Reason)
}

func TestHashBudgetDoesNotPollContendedChallengeLocks(t *testing.T) {
	rc := testRedisClient(t)
	t.Cleanup(func() { _ = rc.Close() })
	h := newIdemHandlers(t, rc)
	victim, code := createActivationOTP(t, h.challengeManager, "victim")
	require.NoError(t, rc.Set(context.Background(), challengeVerifyLockPrefix+victim.ID, "other-owner", time.Minute).Err())
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := h.verifyWithBudget(ctx, victim.ID, code, "", nil)
	require.ErrorIs(t, err, challengekit.ErrLockUnavailable)
	require.NotErrorIs(t, err, context.DeadlineExceeded, "lock contention must not wait with the shared budget held")
	require.Equal(t, challengekit.ReasonLockContention, result.Reason)
	require.Empty(t, h.hashBudget.slots)
	stored, err := h.challengeManager.Get(context.Background(), victim.ID)
	require.NoError(t, err)
	require.Zero(t, stored.Attempts)
	other, otherCode := createActivationOTP(t, h.challengeManager, "other")
	_, err = h.challengeManager.SwapActive(context.Background(), other)
	require.NoError(t, err)
	result, err = h.verifyWithBudget(context.Background(), other.ID, otherCode, "", nil)
	require.NoError(t, err)
	require.True(t, result.OK)
}

type uncancellableBudgetVerifier struct {
	challengekit.ManagerInterface
	entered chan struct{}
	release <-chan struct{}
}

func (m *uncancellableBudgetVerifier) Verify(context.Context, string, string, string) (*challengekit.VerifyResult, error) {
	close(m.entered)
	<-m.release
	return nil, errors.New("synthetic completed hash")
}

func TestCanceledRunningHashRetainsCapacityUntilCompletion(t *testing.T) {
	budget, err := newHashBudget(1)
	require.NoError(t, err)
	release := make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	verifier := &uncancellableBudgetVerifier{entered: make(chan struct{}), release: release}
	h := &Handlers{hashBudget: budget, challengeManager: verifier}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := h.verifyWithBudget(ctx, "id", "code", "", nil); done <- err }()
	select {
	case <-verifier.entered:
	case <-time.After(time.Second):
		t.Fatal("hash did not start")
	}
	cancel()
	_, err = budget.acquire(context.Background())
	require.ErrorIs(t, err, errHashCapacity)
	once.Do(func() { close(release) })
	require.Error(t, <-done)
	slot, err := budget.acquire(context.Background())
	require.NoError(t, err)
	slot()
}
