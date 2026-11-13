package handlers

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
	challengekit "github.com/soulteary/challenge-kit"
	provider "github.com/soulteary/provider-kit"

	"github.com/soulteary/herald/internal/config"
	"github.com/soulteary/herald/internal/idempotency"
)

const faultBody = `{"user_id":"u1","channel":"email","destination":"a@b.com","purpose":"login"}`

// The delivery counter lives outside the handler process, just like a provider.
type faultProvider struct {
	rc        *redis.Client
	afterSend func()
	sendErr   error
}

func (p *faultProvider) Send(ctx context.Context, _ *provider.Message) (*provider.SendResult, error) {
	if err := p.rc.Incr(ctx, "test:delivered").Err(); err != nil {
		return nil, err
	}
	if p.afterSend != nil {
		p.afterSend()
	}
	if p.sendErr != nil {
		return nil, p.sendErr
	}
	return provider.NewSuccessResult("fault", provider.ChannelEmail, "msg-1"), nil
}
func (*faultProvider) Channel() provider.Channel { return provider.ChannelEmail }
func (*faultProvider) Name() string              { return "fault" }
func (*faultProvider) Validate() error           { return nil }

func faultApp(t *testing.T, rc *redis.Client, afterSend func()) *fiber.App {
	app, _ := faultAppWithProvider(t, rc, &faultProvider{rc: rc, afterSend: afterSend})
	return app
}

func faultAppWithProvider(t *testing.T, rc *redis.Client, p *faultProvider) (*fiber.App, *Handlers) {
	t.Helper()
	h := newIdemHandlers(t, rc)
	h.idempotencyStore = idempotency.NewStore(rc, "otp:idem2:", []byte(config.IdempotencySecret), time.Minute)
	h.providerRegistry = provider.NewRegistry()
	if err := h.providerRegistry.Register(p); err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	app.Post("/v1/otp/challenges", h.CreateChallenge)
	return app, h
}

// Inject a failed state write, either before commit or after Redis has applied
// it (a lost acknowledgement). Other writes use real Redis commands.
type rejectStateHook struct {
	rejected    atomic.Int64
	state       string
	afterCommit bool
}

func (h *rejectStateHook) rejectedState() string {
	if h.state == "" {
		return "succeeded"
	}
	return h.state
}
func (*rejectStateHook) DialHook(next redis.DialHook) redis.DialHook          { return next }
func (*rejectStateHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook { return next }
func (h *rejectStateHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		for _, cmd := range cmds {
			if cmd.Name() == "set" && strings.Contains(cmd.String(), `"state":"`+h.rejectedState()+`"`) {
				h.rejected.Add(1)
				if h.afterCommit {
					if err := next(ctx, cmds); err != nil {
						return err
					}
				}
				return errors.New("injected idempotency state write failure")
			}
		}
		return next(ctx, cmds)
	}
}

func assertNoResendAfterRestart(t *testing.T, mr *miniredis.Miniredis) {
	t.Helper()
	// A new client and handler have no in-process knowledge of the first send.
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rc.Close() })
	app := faultApp(t, rc, nil)
	// The advances stay inside the default 24h unresolved TTL.
	for _, advance := range []time.Duration{2 * time.Minute, 0, 23 * time.Hour} {
		mr.FastForward(advance)
		resp, err := idemRequest(app, "fault-key", faultBody)
		if err != nil {
			t.Fatal(err)
		}
		calls, err := rc.Get(context.Background(), "test:delivered").Int()
		if err != nil {
			t.Fatal(err)
		}
		if calls != 1 {
			t.Fatalf("provider deliveries=%d after restart and advance %s, want 1 (response=%s)", calls, advance, resp.body)
		}
		if resp.status != fiber.StatusConflict || !strings.Contains(string(resp.body), `"reason":"delivery_unknown"`) {
			t.Fatalf("retry status=%d body=%s, want 409/delivery_unknown", resp.status, resp.body)
		}
	}
}

func TestIdempotency_SuccessWriteFailureDoesNotResend(t *testing.T) {
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rc.Close() })
	hook := &rejectStateHook{}
	rc.AddHook(hook)
	app := faultApp(t, rc, nil)
	resp, err := idemRequest(app, "fault-key", faultBody)
	if err != nil {
		t.Fatal(err)
	}
	if hook.rejected.Load() == 0 {
		t.Fatal("success write fault was not exercised")
	}
	// Check the restart path first so the unfixed code demonstrates a second
	// delivery once its pending TTL has elapsed.
	assertNoResendAfterRestart(t, mr)
	if resp.status != fiber.StatusServiceUnavailable {
		t.Fatalf("first status=%d body=%s, want 503", resp.status, resp.body)
	}
}

type activationFault struct {
	challengekit.ManagerInterface
	fail bool
	exit bool
}

func (m *activationFault) SwapActive(ctx context.Context, ch *challengekit.Challenge) (string, error) {
	if m.fail {
		return "", errors.New("injected activation failure")
	}
	prev, err := m.ManagerInterface.SwapActive(ctx, ch)
	if err == nil && m.exit {
		os.Exit(23)
	}
	return prev, err
}

func TestIdempotency_ProcessExitAfterSendDoesNotResend(t *testing.T) {
	if addr := os.Getenv("HERALD_IDEM_CRASH_REDIS"); addr != "" {
		rc := redis.NewClient(&redis.Options{Addr: addr})
		p := &faultProvider{rc: rc}
		if os.Getenv("HERALD_IDEM_CRASH_STAGE") == "send" {
			p.afterSend = func() { os.Exit(23) }
		}
		app, h := faultAppWithProvider(t, rc, p)
		h.challengeManager = &activationFault{ManagerInterface: h.challengeManager, exit: true}
		_, _ = idemRequest(app, "fault-key", faultBody)
		t.Fatal("child did not exit at the delivery boundary")
	}
	for _, stage := range []string{"send", "activation"} {
		t.Run(stage, func(t *testing.T) {
			mr := miniredis.RunT(t)
			cmd := exec.Command(os.Args[0], "-test.run=^TestIdempotency_ProcessExitAfterSendDoesNotResend$")
			cmd.Env = append(os.Environ(), "HERALD_IDEM_CRASH_REDIS="+mr.Addr(), "HERALD_IDEM_CRASH_STAGE="+stage)
			out, err := cmd.CombinedOutput()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 23 {
				t.Fatalf("child exit=%v output=%s", err, out)
			}
			assertNoResendAfterRestart(t, mr)
		})
	}
}

func TestIdempotency_PostSendFailuresDoNotResend(t *testing.T) {
	for _, stage := range []string{"redis_outage", "activation", "provider_timeout"} {
		t.Run(stage, func(t *testing.T) {
			mr := miniredis.RunT(t)
			rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
			t.Cleanup(func() { _ = rc.Close() })
			p := &faultProvider{rc: rc}
			if stage == "redis_outage" {
				p.afterSend = func() { _ = rc.Close() }
			}
			if stage == "provider_timeout" {
				p.sendErr = context.DeadlineExceeded
			}
			app, h := faultAppWithProvider(t, rc, p)
			if stage == "activation" {
				h.challengeManager = &activationFault{ManagerInterface: h.challengeManager, fail: true}
			}
			resp, err := idemRequest(app, "fault-key", faultBody)
			if err != nil {
				t.Fatal(err)
			}
			if resp.status < 500 {
				t.Fatalf("first status=%d body=%s, want failure", resp.status, resp.body)
			}
			assertNoResendAfterRestart(t, mr)
		})
	}
}

func TestIdempotency_SendBoundaryWriteFailureDoesNotSend(t *testing.T) {
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rc.Close() })
	hook := &rejectStateHook{state: "sending"}
	rc.AddHook(hook)
	app := faultApp(t, rc, nil)
	resp, err := idemRequest(app, "fault-key", faultBody)
	if err != nil {
		t.Fatal(err)
	}
	if hook.rejected.Load() == 0 || resp.status != fiber.StatusServiceUnavailable {
		t.Fatalf("pre-send fault count=%d status=%d", hook.rejected.Load(), resp.status)
	}
	if mr.Exists("test:delivered") {
		t.Fatal("provider called without a persisted send boundary")
	}
	mr.FastForward(2 * time.Minute)
	restarted := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = restarted.Close() })
	resp, err = idemRequest(faultApp(t, restarted, nil), "fault-key", faultBody)
	if err != nil {
		t.Fatal(err)
	}
	if resp.status != fiber.StatusOK {
		t.Fatalf("safe pre-send retry: status=%d body=%s", resp.status, resp.body)
	}
}

func TestIdempotency_LostWriteAcknowledgement(t *testing.T) {
	for _, state := range []string{"sending", "succeeded"} {
		t.Run(state, func(t *testing.T) {
			mr := miniredis.RunT(t)
			rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
			t.Cleanup(func() { _ = rc.Close() })
			hook := &rejectStateHook{state: state, afterCommit: true}
			rc.AddHook(hook)
			resp, err := idemRequest(faultApp(t, rc, nil), "fault-key", faultBody)
			if err != nil {
				t.Fatal(err)
			}
			if hook.rejected.Load() == 0 || resp.status != fiber.StatusServiceUnavailable {
				t.Fatalf("lost acknowledgement: rejected=%d status=%d body=%s", hook.rejected.Load(), resp.status, resp.body)
			}
			nextClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
			t.Cleanup(func() { _ = nextClient.Close() })
			if state == "sending" {
				mr.FastForward(23 * time.Hour)
			}
			resp, err = idemRequest(faultApp(t, nextClient, nil), "fault-key", faultBody)
			if err != nil {
				t.Fatal(err)
			}
			wantStatus, wantCalls := fiber.StatusOK, 1
			if state == "sending" {
				wantStatus, wantCalls = fiber.StatusConflict, 0
			}
			if resp.status != wantStatus {
				t.Fatalf("retry status=%d body=%s", resp.status, resp.body)
			}
			calls := 0
			if mr.Exists("test:delivered") {
				calls, err = nextClient.Get(context.Background(), "test:delivered").Int()
				if err != nil {
					t.Fatal(err)
				}
			}
			if calls != wantCalls {
				t.Fatalf("deliveries=%d, want %d", calls, wantCalls)
			}
		})
	}
}
