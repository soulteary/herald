package handlers

import (
	"context"
	"github.com/redis/go-redis/v9"
	challengekit "github.com/soulteary/challenge-kit"
)

// Verification fixtures represent codes whose provider delivery has completed.
// Lifecycle tests use the real pending manager directly.
type activeTestChallengeManager struct{ challengekit.ManagerInterface }

func newActiveTestChallengeManager(rc *redis.Client, cfg challengekit.Config) challengekit.ManagerInterface {
	cfg.RequireActiveVerification = true
	cfg.ChallengeKeyPrefix = challengeRecordPrefix
	cfg.VerifyLockPrefix = challengeVerifyLockPrefix
	cfg.ActiveIndexPrefix = challengeActiveIndexPrefix
	return &activeTestChallengeManager{challengekit.NewManager(rc, cfg)}
}
func (m *activeTestChallengeManager) Create(ctx context.Context, req challengekit.CreateRequest) (*challengekit.Challenge, string, error) {
	ch, code, err := m.ManagerInterface.Create(ctx, req)
	if err != nil {
		return ch, code, err
	}
	_, err = m.SwapActive(ctx, ch)
	return ch, code, err
}
