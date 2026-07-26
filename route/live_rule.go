package route

import (
	"context"
	"sync"
	"time"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	R "github.com/sagernet/sing-box/route/rule"
	E "github.com/sagernet/sing/common/exceptions"
)

// LiveRuleSpec is one user rule in config / Clash API (expires_at=0 → permanent).
type LiveRuleSpec struct {
	ID        uint64      `json:"id"`
	ExpiresAt int64       `json:"expires_at,omitempty"` // unix ms; 0 = permanent
	Rule      option.Rule `json:"rule"`
}

// LiveRulesPayload is the full live overlay (temp evaluated before permanent).
type LiveRulesPayload struct {
	Temp      []LiveRuleSpec `json:"temp"`
	Permanent []LiveRuleSpec `json:"permanent"`
}

type liveEntry struct {
	id        uint64
	expiresAt int64
	rule      adapter.Rule
}

// LiveRuleStore holds user temp + permanent rules. Match skips expired.
// System rules (clash mode, rulesets) stay in Router.rules.
type LiveRuleStore struct {
	ctx    context.Context
	logger log.ContextLogger
	access sync.RWMutex
	temp   []liveEntry
	perm   []liveEntry
}

// TempRuleSpec kept for API compat (same shape as LiveRuleSpec).
type TempRuleSpec = LiveRuleSpec

func newLiveRuleStore(ctx context.Context, logger log.ContextLogger) *LiveRuleStore {
	return &LiveRuleStore{ctx: ctx, logger: logger}
}

func (s *LiveRuleStore) build(items []LiveRuleSpec, label string) ([]liveEntry, error) {
	now := time.Now().UnixMilli()
	out := make([]liveEntry, 0, len(items))
	for i, item := range items {
		if item.ExpiresAt > 0 && item.ExpiresAt <= now {
			continue
		}
		rule, err := R.NewRule(s.ctx, s.logger, item.Rule, true)
		if err != nil {
			return nil, E.Cause(err, label, "[", i, "]")
		}
		id := item.ID
		if id == 0 {
			id = uint64(i + 1)
		}
		out = append(out, liveEntry{id: id, expiresAt: item.ExpiresAt, rule: rule})
	}
	return out, nil
}

// Replace sets temp + permanent overlays (no box reload).
func (s *LiveRuleStore) Replace(payload LiveRulesPayload) error {
	if s == nil {
		return E.New("live rule store unavailable")
	}
	temp, err := s.build(payload.Temp, "temp rule")
	if err != nil {
		return err
	}
	perm, err := s.build(payload.Permanent, "user rule")
	if err != nil {
		return err
	}
	s.access.Lock()
	s.temp = temp
	s.perm = perm
	s.access.Unlock()
	return nil
}

// ReplaceTemp only swaps temp segment (permanent unchanged).
func (s *LiveRuleStore) ReplaceTemp(items []LiveRuleSpec) error {
	if s == nil {
		return E.New("live rule store unavailable")
	}
	temp, err := s.build(items, "temp rule")
	if err != nil {
		return err
	}
	s.access.Lock()
	s.temp = temp
	s.access.Unlock()
	return nil
}

// ReplacePermanent only swaps permanent segment.
func (s *LiveRuleStore) ReplacePermanent(items []LiveRuleSpec) error {
	if s == nil {
		return E.New("live rule store unavailable")
	}
	perm, err := s.build(items, "user rule")
	if err != nil {
		return err
	}
	s.access.Lock()
	s.perm = perm
	s.access.Unlock()
	return nil
}

func matchEntries(entries []liveEntry, metadata *adapter.InboundContext, now int64) adapter.Rule {
	for _, e := range entries {
		if e.expiresAt > 0 && e.expiresAt <= now {
			continue
		}
		if e.rule != nil && e.rule.Match(metadata) {
			return e.rule
		}
	}
	return nil
}

// MatchTemp: temporary user rules (before rulesets).
func (s *LiveRuleStore) MatchTemp(metadata *adapter.InboundContext) adapter.Rule {
	if s == nil {
		return nil
	}
	now := time.Now().UnixMilli()
	s.access.RLock()
	defer s.access.RUnlock()
	return matchEntries(s.temp, metadata, now)
}

// MatchPermanent: permanent user rules (after rulesets).
func (s *LiveRuleStore) MatchPermanent(metadata *adapter.InboundContext) adapter.Rule {
	if s == nil {
		return nil
	}
	now := time.Now().UnixMilli()
	s.access.RLock()
	defer s.access.RUnlock()
	return matchEntries(s.perm, metadata, now)
}
