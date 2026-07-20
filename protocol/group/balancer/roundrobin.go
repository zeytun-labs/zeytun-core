package balancer

import (
	"sync"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/option"
	N "github.com/sagernet/sing/common/network"
)

var _ Strategy = (*RoundRobin)(nil)

type RoundRobin struct {
	outbounds            map[string][]adapter.Outbound
	sortedOutbounds      map[string][]adapter.Outbound
	maxAcceptableIndex   map[string]int
	idx                  map[string]int
	mu                   sync.Mutex
	delayAcceptableRatio float64
	tolerance            uint16
}

func NewRoundRobin(outbounds []adapter.Outbound, options option.BalancerOutboundOptions) *RoundRobin {
	cOutbounds := convertOutbounds(outbounds)
	acceptable := map[string]int{}
	idx := map[string]int{}
	for net, outs := range cOutbounds {
		acceptable[net] = len(outs) - 1
		idx[net] = 0
	}
	return &RoundRobin{
		outbounds:            cOutbounds,
		sortedOutbounds:      cOutbounds,
		maxAcceptableIndex:   acceptable,
		delayAcceptableRatio: defaultDelayRatio(options.DelayAcceptableRatio),
		tolerance:            defaultTolerance(options.Tolerance),
		idx:                  idx,
	}
}

func (s *RoundRobin) Now() string {
	return ""
}

func (s *RoundRobin) UpdateOutboundsInfo(history map[string]*adapter.URLTestHistory) bool {
	sortedOutbounds := sortOutboundsByDelay(s.outbounds, history)
	acceptableIndex := getAcceptableIndex(sortedOutbounds, history, s.delayAcceptableRatio, s.tolerance)

	s.mu.Lock()
	changed := false
	for net, ix := range acceptableIndex {
		changed = changed || ix != s.maxAcceptableIndex[net]
	}
	s.sortedOutbounds = sortedOutbounds
	s.maxAcceptableIndex = acceptableIndex
	s.mu.Unlock()
	return changed
}

func (s *RoundRobin) Select(metadata adapter.InboundContext, net string, touch bool) adapter.Outbound {
	s.mu.Lock()
	defer s.mu.Unlock()
	if net != N.NetworkTCP && net != N.NetworkUDP {
		net = N.NetworkTCP
	}
	length := s.maxAcceptableIndex[net] + 1
	if length <= 0 {
		outs := s.sortedOutbounds[net]
		if len(outs) == 0 {
			return nil
		}
		return outs[0]
	}
	id := (s.idx[net] + 1) % length
	proxy := s.sortedOutbounds[net][id]
	if touch {
		s.idx[net] = id
	}
	return proxy
}
