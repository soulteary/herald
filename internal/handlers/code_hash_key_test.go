package handlers

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
	challengekit "github.com/soulteary/challenge-kit"
	"github.com/stretchr/testify/require"

	"github.com/soulteary/herald/internal/config"
)

const (
	codeHashKeyA = "code-hash-key-A-0123456789abcdef"
	codeHashKeyB = "code-hash-key-B-0123456789abcdef"
)

// codeHashInstance is one Herald instance: handlers built with one
// HERALD_CODE_HASH_KEY setting. Instances of a fixture share one Redis.
type codeHashInstance struct {
	*userLockFixture
}

type codeHashFixture struct {
	base *userLockFixture
}

func newCodeHashFixture(t *testing.T) *codeHashFixture {
	t.Helper()
	oldKey, oldVerifyOnly := config.CodeHashKey, config.CodeHashVerifyOnly
	t.Cleanup(func() { config.CodeHashKey, config.CodeHashVerifyOnly = oldKey, oldVerifyOnly })
	config.CodeHashKey, config.CodeHashVerifyOnly = "", false
	return &codeHashFixture{base: newUserLockFixture(t, 3)}
}

// instance builds handlers with the given key on the fixture's Redis, as a
// restarted or second instance would be.
func (f *codeHashFixture) instance(t *testing.T, key string, verifyOnly bool) *codeHashInstance {
	t.Helper()
	config.CodeHashKey, config.CodeHashVerifyOnly = key, verifyOnly
	rc := redis.NewClient(&redis.Options{Addr: f.base.mr.Addr()})
	t.Cleanup(func() { _ = rc.Close() })
	h := newIdemHandlers(t, rc)
	app := fiber.New()
	app.Post("/v1/otp/verifications", h.VerifyChallenge)
	app.Post("/v2/otp/verifications", h.VerifyChallengeV2)
	return &codeHashInstance{&userLockFixture{
		creationLimitFixture: &creationLimitFixture{app: app, h: h, mr: f.base.mr},
		rc:                   rc,
		// Destinations are numbered per instance; keep them apart.
		sequence: f.base.nextSequenceBlock(),
	}}
}

func (f *userLockFixture) nextSequenceBlock() int {
	f.sequence += 1000
	return f.sequence
}

// stored returns the challenge record as it is in Redis.
func (i *codeHashInstance) stored(t *testing.T, challengeID string) challengekit.Challenge {
	t.Helper()
	raw, err := i.rc.Get(context.Background(), challengeRecordPrefix+challengeID).Result()
	require.NoError(t, err)
	var ch challengekit.Challenge
	require.NoError(t, json.Unmarshal([]byte(raw), &ch), raw)
	return ch
}

func isArgon2CodeHash(stored string) bool {
	return strings.HasPrefix(stored, "$argon2id$") || strings.Contains(stored, ":")
}

func requireVerified(t *testing.T, i *codeHashInstance, challengeID, code string) {
	t.Helper()
	status, body := i.verify(t, challengeID, code)
	require.Equal(t, fiber.StatusOK, status, body)
	require.Equal(t, true, body["ok"], body)
}

// requireWrongCode checks that the code is refused as a wrong code and that
// the refusal consumed an attempt of the challenge.
func requireWrongCode(t *testing.T, i *codeHashInstance, challengeID, code string, wantAttempts int) {
	t.Helper()
	status, body := i.verify(t, challengeID, code)
	require.Equal(t, fiber.StatusUnauthorized, status, body)
	require.Equal(t, "invalid", body["reason"], body)
	require.EqualValues(t, config.MaxAttempts-wantAttempts, body["remaining_attempts"], body)
	require.Equal(t, wantAttempts, i.stored(t, challengeID).Attempts)
}

func TestCodeHashKey_UnsetKeepsArgon2(t *testing.T) {
	f := newCodeHashFixture(t)
	plain := f.instance(t, "", false)

	ch, code := plain.newChallenge(t, "user-plain")
	require.True(t, isArgon2CodeHash(plain.stored(t, ch.ID).CodeHash), "stored %q", plain.stored(t, ch.ID).CodeHash)
	requireVerified(t, plain, ch.ID, code)
}

func TestCodeHashKey_StoresKeyedDigest(t *testing.T) {
	f := newCodeHashFixture(t)
	plain := f.instance(t, "", false)
	keyed := f.instance(t, codeHashKeyA, false)

	plainCh, _ := plain.newChallenge(t, "user-plain")
	ch, code := keyed.newChallenge(t, "user-keyed")
	stored := keyed.stored(t, ch.ID).CodeHash

	// The stored value is the challenge-kit HMAC form, not an Argon2 hash,
	// and is exactly what the configured key yields for this challenge.
	require.True(t, strings.HasPrefix(stored, "hmac-sha256$v1$"), "stored %q", stored)
	require.False(t, isArgon2CodeHash(stored), "stored %q", stored)
	require.True(t, isArgon2CodeHash(plain.stored(t, plainCh.ID).CodeHash))
	require.NotContains(t, stored, code)

	hasherA, err := challengekit.NewHMACCodeHasher([]byte(codeHashKeyA))
	require.NoError(t, err)
	want, err := hasherA.Hash(context.Background(), ch.ID, code)
	require.NoError(t, err)
	require.Equal(t, want, stored)

	// Another key yields another value for the same challenge and code.
	hasherB, err := challengekit.NewHMACCodeHasher([]byte(codeHashKeyB))
	require.NoError(t, err)
	other, err := hasherB.Hash(context.Background(), ch.ID, code)
	require.NoError(t, err)
	require.NotEqual(t, other, stored)

	// A wrong code is refused and counted; the right code then verifies, and
	// a second instance with the same key verifies as well.
	requireWrongCode(t, keyed, ch.ID, wrongCode(code), 1)
	requireWrongCode(t, keyed, ch.ID, wrongCode(code), 2)
	requireVerified(t, f.instance(t, codeHashKeyA, false), ch.ID, code)

	// The challenge is consumed by the successful verification.
	status, body := keyed.verify(t, ch.ID, code)
	require.Equal(t, fiber.StatusUnauthorized, status, body)
}

func TestCodeHashKey_ExhaustsAfterMaxAttempts(t *testing.T) {
	f := newCodeHashFixture(t)
	keyed := f.instance(t, codeHashKeyA, false)

	ch, code := keyed.newChallenge(t, "user-exhaust")
	for attempt := 1; attempt < config.MaxAttempts; attempt++ {
		requireWrongCode(t, keyed, ch.ID, wrongCode(code), attempt)
	}
	status, body := keyed.verify(t, ch.ID, wrongCode(code))
	require.Equal(t, fiber.StatusUnauthorized, status, body)
	require.Equal(t, "locked", body["reason"], body)

	status, body = keyed.verify(t, ch.ID, code)
	require.Equal(t, fiber.StatusUnauthorized, status, body)
	require.NotEqual(t, true, body["ok"], body)
}

// Setting the key does not invalidate challenges created before it.
func TestCodeHashKey_ChallengeCreatedBeforeTheKeyStillVerifies(t *testing.T) {
	f := newCodeHashFixture(t)
	plain := f.instance(t, "", false)
	ch, code := plain.newChallenge(t, "user-before")
	wrongCh, wrongChCode := plain.newChallenge(t, "user-before-wrong")

	keyed := f.instance(t, codeHashKeyA, false)
	require.True(t, isArgon2CodeHash(keyed.stored(t, ch.ID).CodeHash))
	requireVerified(t, keyed, ch.ID, code)
	requireWrongCode(t, keyed, wrongCh.ID, wrongCode(wrongChCode), 1)
	requireVerified(t, keyed, wrongCh.ID, wrongChCode)
}

// An instance without the key, or with another key, cannot verify a keyed
// challenge: the right code is answered as a wrong code and consumes an
// attempt. The challenge is not destroyed, so the right instance still
// verifies it while attempts remain.
func TestCodeHashKey_RemovedOrChangedKeyRejectsOutstandingChallenges(t *testing.T) {
	for name, other := range map[string]string{"key removed": "", "key changed": codeHashKeyB} {
		t.Run(name, func(t *testing.T) {
			f := newCodeHashFixture(t)
			keyed := f.instance(t, codeHashKeyA, false)
			ch, code := keyed.newChallenge(t, "user-transition")

			requireWrongCode(t, f.instance(t, other, false), ch.ID, code, 1)
			requireWrongCode(t, f.instance(t, other, true), ch.ID, code, 2)
			requireVerified(t, keyed, ch.ID, code)
		})
	}
}

// HERALD_CODE_HASH_VERIFY_ONLY is the first stage of a rolling change: such
// an instance keeps writing Argon2 hashes, which every instance can verify,
// and verifies keyed digests as well.
func TestCodeHashKey_VerifyOnlyStage(t *testing.T) {
	f := newCodeHashFixture(t)
	plain := f.instance(t, "", false)
	verifyOnly := f.instance(t, codeHashKeyA, true)
	keyed := f.instance(t, codeHashKeyA, false)

	// Written by a verify-only instance: Argon2, verifiable without the key.
	ch, code := verifyOnly.newChallenge(t, "user-stage-one")
	require.True(t, isArgon2CodeHash(verifyOnly.stored(t, ch.ID).CodeHash))
	requireVerified(t, plain, ch.ID, code)

	// Written by a keyed instance: verifiable by a verify-only instance, so
	// the fleet can be switched over, and back, one instance at a time.
	ch, code = keyed.newChallenge(t, "user-stage-two")
	require.True(t, strings.HasPrefix(keyed.stored(t, ch.ID).CodeHash, "hmac-sha256$v1$"))
	requireWrongCode(t, verifyOnly, ch.ID, wrongCode(code), 1)
	requireVerified(t, verifyOnly, ch.ID, code)
}

// A stored value that is neither an Argon2 hash nor a digest this hasher
// wrote is not a wrong code: nothing is consumed and the answer is 503.
func TestCodeHashKey_UnreadableStoredValueConsumesNoAttempt(t *testing.T) {
	f := newCodeHashFixture(t)
	keyed := f.instance(t, codeHashKeyA, false)
	ch, code := keyed.newChallenge(t, "user-damaged")

	record := keyed.stored(t, ch.ID)
	record.CodeHash = "hmac-sha256$v9$" + strings.TrimPrefix(record.CodeHash, "hmac-sha256$v1$")
	raw, err := json.Marshal(record)
	require.NoError(t, err)
	require.NoError(t, keyed.rc.Set(context.Background(), challengeRecordPrefix+ch.ID, raw, redis.KeepTTL).Err())

	status, body := keyed.verify(t, ch.ID, code)
	require.Equal(t, fiber.StatusServiceUnavailable, status, body)
	require.Equal(t, "backend_unavailable", body["reason"], body)
	require.Equal(t, 0, keyed.stored(t, ch.ID).Attempts)
}

// NewHandlersWithError refuses a key challenge-kit cannot use, and names the
// variable without repeating its value.
func TestCodeHashKey_ShortKeyFailsHandlerConstruction(t *testing.T) {
	f := newCodeHashFixture(t)
	const short = "short-code-hash-key"
	config.CodeHashKey = short

	h, err := NewHandlersWithError(f.base.rc, testLogger())
	require.Error(t, err)
	require.Nil(t, h)
	require.Contains(t, err.Error(), "HERALD_CODE_HASH_KEY")
	require.NotContains(t, err.Error(), short)
}
