package balancer

import (
	"sync"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/option"
	N "github.com/sagernet/sing/common/network"
)

var _ Strategy = (*Failover)(nil)

// Failover prefers outbounds in config order. First alive is primary; falls back
// down the list; switches back when a higher-priority outbound recovers.
type Failover struct {
	outbounds map[string][]adapter.Outbound
	delays    map[string]uint16
	selected  map[string]adapter.Outbound
	mu        sync.Mutex
}

func NewFailover(outbounds []adapter.Outbound, _ option.BalancerOutboundOptions) *Failover {
	c := convertOutbounds(outbounds)
	selected := map[string]adapter.Outbound{}
	for net, outs := range c {
		if len(outs) > 0 {
			selected[net] = outs[0]
		}
	}
	return &Failover{outbounds: c, selected: selected}
}

func (s *Failover) Now() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if out := s.selected[N.NetworkTCP]; out != nil {
		return out.Tag()
	}
	if out := s.selected[N.NetworkUDP]; out != nil {
		return out.Tag()
	}
	return ""
}

func (s *Failover) UpdateOutboundsInfo(history map[string]*adapter.URLTestHistory) bool {
	delayMap := getDelayMap(history)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.delays = delayMap
	changed := false
	for net := range s.outbounds {
		next := s.pickLocked(net)
		prev := s.selected[net]
		if next != nil && (prev == nil || next.Tag() != prev.Tag()) {
			changed = changed || prev != nil
			s.selected[net] = next
		}
	}
	return changed
}

func (s *Failover) Select(_ adapter.InboundContext, net string, _ bool) adapter.Outbound {
	s.mu.Lock()
	defer s.mu.Unlock()
	if net != N.NetworkTCP && net != N.NetworkUDP {
		net = N.NetworkTCP
	}
	return s.pickLocked(net)
}

func (s *Failover) pickLocked(net string) adapter.Outbound {
	outs := s.outbounds[net]
	for _, out := range outs {
		if isAliveTag(out.Tag(), s.delays) {
			s.selected[net] = out
			return out
		}
	}
	if len(outs) > 0 {
		s.selected[net] = outs[0]
		return outs[0]
	}
	return nil
}
