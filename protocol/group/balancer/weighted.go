package balancer

import (
	"sync"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/option"
	N "github.com/sagernet/sing/common/network"
)

var _ Strategy = (*Weighted)(nil)

// Weighted does weighted round-robin over alive outbounds.
// weights parallel outbounds; missing/zero weight treated as 1.
type Weighted struct {
	outbounds map[string][]adapter.Outbound
	weights   []int
	// expanded indices into outbounds[net] by weight (rebuilt per network from same weight list)
	slots   map[string][]int
	idx     map[string]int
	delays  map[string]uint16
	mu      sync.Mutex
}

func NewWeighted(outbounds []adapter.Outbound, options option.BalancerOutboundOptions) *Weighted {
	c := convertOutbounds(outbounds)
	weights := normalizeWeights(len(outbounds), options.Weights)
	s := &Weighted{
		outbounds: c,
		weights:   weights,
		slots:     map[string][]int{},
		idx:       map[string]int{},
	}
	for net, outs := range c {
		s.slots[net] = buildSlots(len(outs), weights)
		s.idx[net] = 0
	}
	return s
}

func normalizeWeights(n int, raw []int) []int {
	w := make([]int, n)
	for i := 0; i < n; i++ {
		if i < len(raw) && raw[i] > 0 {
			w[i] = raw[i]
		} else {
			w[i] = 1
		}
	}
	return w
}

func buildSlots(n int, weights []int) []int {
	var slots []int
	for i := 0; i < n && i < len(weights); i++ {
		for c := 0; c < weights[i]; c++ {
			slots = append(slots, i)
		}
	}
	if len(slots) == 0 {
		for i := 0; i < n; i++ {
			slots = append(slots, i)
		}
	}
	return slots
}

func (s *Weighted) Now() string { return "" }

func (s *Weighted) UpdateOutboundsInfo(history map[string]*adapter.URLTestHistory) bool {
	s.mu.Lock()
	s.delays = getDelayMap(history)
	s.mu.Unlock()
	return false
}

func (s *Weighted) Select(_ adapter.InboundContext, net string, touch bool) adapter.Outbound {
	s.mu.Lock()
	defer s.mu.Unlock()
	if net != N.NetworkTCP && net != N.NetworkUDP {
		net = N.NetworkTCP
	}
	outs := s.outbounds[net]
	slots := s.slots[net]
	if len(outs) == 0 || len(slots) == 0 {
		return nil
	}
	start := s.idx[net]
	for i := 0; i < len(slots); i++ {
		pos := (start + i) % len(slots)
		oi := slots[pos]
		if oi < 0 || oi >= len(outs) {
			continue
		}
		out := outs[oi]
		if isAliveTag(out.Tag(), s.delays) {
			if touch {
				s.idx[net] = (pos + 1) % len(slots)
			}
			return out
		}
	}
	// all dead: still rotate for fairness
	pos := start % len(slots)
	if touch {
		s.idx[net] = (pos + 1) % len(slots)
	}
	return outs[slots[pos]%len(outs)]
}
