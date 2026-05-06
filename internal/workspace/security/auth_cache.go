package security

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

const AuthDecisionTTL = 5 * time.Second

type AuthDecisionKey struct {
	TokenHash    string
	WorkspaceID  string
	RunID        string
	LeaseVersion int64
}

func NewAuthDecisionKey(token, workspaceID, runID string, leaseVersion int64) AuthDecisionKey {
	sum := sha256.Sum256([]byte(token))
	return AuthDecisionKey{
		TokenHash:    hex.EncodeToString(sum[:]),
		WorkspaceID:  workspaceID,
		RunID:        runID,
		LeaseVersion: leaseVersion,
	}
}

type AuthDecisionCache struct {
	mu      sync.Mutex
	now     func() time.Time
	ttl     time.Duration
	entries map[AuthDecisionKey]time.Time
}

func NewAuthDecisionCache(now func() time.Time) *AuthDecisionCache {
	if now == nil {
		now = time.Now
	}
	return &AuthDecisionCache{
		now:     now,
		ttl:     AuthDecisionTTL,
		entries: make(map[AuthDecisionKey]time.Time),
	}
}

func (c *AuthDecisionCache) Allow(key AuthDecisionKey) {
	c.mu.Lock()
	c.entries[key] = c.now().Add(c.ttl)
	c.mu.Unlock()
}

func (c *AuthDecisionCache) Allowed(key AuthDecisionKey) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	expiresAt, ok := c.entries[key]
	if !ok {
		return false
	}
	if !c.now().Before(expiresAt) {
		delete(c.entries, key)
		return false
	}
	return true
}
