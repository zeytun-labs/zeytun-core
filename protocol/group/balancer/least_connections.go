package balancer

import (
	"sync"
	"sync/atomic"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/option"
	N "github.com/sagernet/sing/common/network"
)

var (
	_ Strategy             = (*LeastConnections)(nil)
	_ connectionLifecycle  = (*LeastConnections)(nil)
)

// LeastConnections routes to the alive outbound with the fewest open connections.
type LeastConnections struct {
	outbounds map[string][]adapter.Outbound
	delays    map[string]uint16
	counts    map[string]*atomic.Int64
	mu        sync.Mutex
}

func NewLeastConnections(outbounds []adapter.Outbound, _ option.BalancerOutboundOptions) *LeastConnections {
	c := convertOutbounds(outbounds)
	counts := map[string]*atomic.Int64{}
	for _, out := range outbounds {
		counts[out.Tag()] = &atomic.Int64{}
	}
	return &LeastConnections{outbounds: c, counts: counts}
}

func (s *LeastConnections) Now() string {
	s.mu.Lock()
	outs := s.outbounds[N.NetworkTCP]
	s.mu.Unlock()
	if out := s.pick(outs); out != nil {
		return out.Tag()
	}
	return ""
}

func (s *LeastConnections) UpdateOutboundsInfo(history map[string]*adapter.URLTestHistory) bool {
	s.mu.Lock()
	s.delays = getDelayMap(history)
	s.mu.Unlock()
	return false
}

func (s *LeastConnections) Select(_ adapter.InboundContext, net string, _ bool) adapter.Outbound {
	s.mu.Lock()
	if net != N.NetworkTCP && net != N.NetworkUDP {
		net = N.NetworkTCP
	}
	outs := s.outbounds[net]
	s.mu.Unlock()
	return s.pick(outs)
}

func (s *LeastConnections) pick(outs []adapter.Outbound) adapter.Outbound {
	if len(outs) == 0 {
		return nil
	}
	s.mu.Lock()
	delays := s.delays
	s.mu.Unlock()

	var best adapter.Outbound
	var bestCount int64 = -1
	var fallback adapter.Outbound
	var fallbackCount int64 = -1

	for _, out := range outs {
		n := s.countOf(out.Tag())
		if fallback == nil || n < fallbackCount {
			fallback = out
			fallbackCount = n
		}
		if !isAliveTag(out.Tag(), delays) {
			continue
		}
		if best == nil || n < bestCount {
			best = out
			bestCount = n
		}
	}
	if best != nil {
		return best
	}
	return fallback
}

func (s *LeastConnections) countOf(tag string) int64 {
	if c, ok := s.counts[tag]; ok {
		return c.Load()
	}
	return 0
}

func (s *LeastConnections) Opened(out adapter.Outbound) {
	if out == nil {
		return
	}
	if c, ok := s.counts[out.Tag()]; ok {
		c.Add(1)
	}
}

func (s *LeastConnections) Closed(out adapter.Outbound) {
	if out == nil {
		return
	}
	if c, ok := s.counts[out.Tag()]; ok {
		for {
			cur := c.Load()
			if cur <= 0 {
				return
			}
			if c.CompareAndSwap(cur, cur-1) {
				return
			}
		}
	}
}
