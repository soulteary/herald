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
	oldKey, oldVerifyOnly, oldPrevious := config.CodeHashKey, config.CodeHashVerifyOnly, config.CodeHashKeyPrevious
	t.Cleanup(func() {
		config.CodeHashKey, config.CodeHashVerifyOnly, config.CodeHashKeyPrevious = oldKey, oldVerifyOnly, oldPrevious
	})
	config.CodeHashKey, config.CodeHashVerifyOnly, config.CodeHashKeyPrevious = "", false, ""
	return &codeHashFixture{base: newUserLockFixture(t, 3)}
}

// instance builds handlers with the given key on the fixture's Redis, as a
// restarted or second instance would be.
func (f *codeHashFixture) instance(t *testing.T, key string, verifyOnly bool) *codeHashInstance {
	t.Helper()
	return f.instanceWithPrevious(t, key, "", verifyOnly)
}

// instanceWithPrevious is instance with a HERALD_CODE_HASH_KEY_PREVIOUS value.
func (f *codeHashFixture) instanceWithPrevious(t *testing.T, key, previous string, verifyOnly bool) *codeHashInstance {
	t.Helper()
	config.CodeHashKey, config.CodeHashVerifyOnly, config.CodeHashKeyPrevious = key, verifyOnly, previous
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

// The documented key change on instances that restart one at a time. Stage 1
// is "key A, previous B", stage 2 is "key B, previous A", stage 3 is "key B".
// At every step of the rollout, each instance verifies what any instance of
// the same or the neighbouring stage created, and refuses a wrong code.
func TestCodeHashKeyPrevious_RotationStages(t *testing.T) {
	f := newCodeHashFixture(t)
	stages := []struct {
		name string
		i    *codeHashInstance
	}{
		{"key A", f.instance(t, codeHashKeyA, false)},
		{"key A, previous B", f.instanceWithPrevious(t, codeHashKeyA, codeHashKeyB, false)},
		{"key B, previous A", f.instanceWithPrevious(t, codeHashKeyB, " "+codeHashKeyA+" ,", false)},
		{"key B", f.instance(t, codeHashKeyB, false)},
	}
	hasher := map[string]challengekit.CodeHasher{}
	for name, key := range map[string]string{"A": codeHashKeyA, "B": codeHashKeyB} {
		h, err := challengekit.NewHMACCodeHasher([]byte(key))
		require.NoError(t, err)
		hasher[name] = h
	}
	writesWith := []string{"A", "A", "B", "B"}

	user := 0
	for s := 0; s+1 < len(stages); s++ {
		for _, pair := range [][2]int{{s, s + 1}, {s + 1, s}, {s + 1, s + 1}} {
			creator, verifier := stages[pair[0]], stages[pair[1]]
			user++
			ch, code := creator.i.newChallenge(t, "user-rotation-"+string(rune('a'+user)))

			// The stored value is the single-key digest under the creator's
			// primary key: the format carries no trace of the previous keys.
			want, err := hasher[writesWith[pair[0]]].Hash(context.Background(), ch.ID, code)
			require.NoError(t, err)
			require.Equal(t, want, creator.i.stored(t, ch.ID).CodeHash, "created by %q", creator.name)

			requireWrongCode(t, verifier.i, ch.ID, wrongCode(code), 1)
			status, body := verifier.i.verify(t, ch.ID, code)
			require.Equal(t, fiber.StatusOK, status, "created by %q, verified by %q: %v", creator.name, verifier.name, body)
			require.Equal(t, true, body["ok"], body)
		}
	}
}

// Without stage 1 the change is not safe on a mixed fleet: an instance that
// still has only key A refuses the right code of a challenge created by an
// instance that already writes with key B. In the other direction it works.
func TestCodeHashKeyPrevious_SkippingTheFirstStageFailsOneWay(t *testing.T) {
	f := newCodeHashFixture(t)
	onlyA := f.instance(t, codeHashKeyA, false)
	switched := f.instanceWithPrevious(t, codeHashKeyB, codeHashKeyA, false)

	ch, code := switched.newChallenge(t, "user-skip-new")
	requireWrongCode(t, onlyA, ch.ID, code, 1)
	requireVerified(t, switched, ch.ID, code)

	ch, code = onlyA.newChallenge(t, "user-skip-old")
	requireVerified(t, switched, ch.ID, code)
}

// A key that is neither the primary nor a listed one is still refused.
func TestCodeHashKeyPrevious_UnlistedKeyIsRefused(t *testing.T) {
	f := newCodeHashFixture(t)
	const keyC = "code-hash-key-C-0123456789abcdef"
	writer := f.instance(t, keyC, false)
	ch, code := writer.newChallenge(t, "user-unlisted")

	requireWrongCode(t, f.instanceWithPrevious(t, codeHashKeyB, codeHashKeyA, false), ch.ID, code, 1)
	requireVerified(t, f.instanceWithPrevious(t, codeHashKeyB, codeHashKeyA+","+keyC, false), ch.ID, code)
}

// Verify-only together with previous keys: Argon2 is written, and digests
// under the primary key and under a previous key are both verified.
func TestCodeHashKeyPrevious_VerifyOnlyStage(t *testing.T) {
	f := newCodeHashFixture(t)
	plain := f.instance(t, "", false)
	verifyOnly := f.instanceWithPrevious(t, codeHashKeyB, codeHashKeyA, true)

	ch, code := verifyOnly.newChallenge(t, "user-vo-argon2")
	require.True(t, isArgon2CodeHash(verifyOnly.stored(t, ch.ID).CodeHash))
	requireVerified(t, plain, ch.ID, code)

	for name, key := range map[string]string{"primary": codeHashKeyB, "previous": codeHashKeyA} {
		ch, code := f.instance(t, key, false).newChallenge(t, "user-vo-"+name)
		requireWrongCode(t, verifyOnly, ch.ID, wrongCode(code), 1)
		requireVerified(t, verifyOnly, ch.ID, code)
	}
}

// NewHandlersWithError refuses a previous key challenge-kit cannot use, and
// names the variable without repeating a value.
func TestCodeHashKeyPrevious_ShortKeyFailsHandlerConstruction(t *testing.T) {
	f := newCodeHashFixture(t)
	const short = "short-previous-code-hash-key"
	config.CodeHashKey, config.CodeHashKeyPrevious = codeHashKeyA, codeHashKeyB+","+short

	h, err := NewHandlersWithError(f.base.rc, testLogger())
	require.Error(t, err)
	require.Nil(t, h)
	require.Contains(t, err.Error(), "HERALD_CODE_HASH_KEY_PREVIOUS")
	require.NotContains(t, err.Error(), short)
	require.NotContains(t, err.Error(), codeHashKeyA)
	require.NotContains(t, err.Error(), codeHashKeyB)
}
