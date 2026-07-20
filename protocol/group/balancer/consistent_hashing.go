package balancer

import (
	"sync"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/option"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/contrab/maphash"
)

var _ Strategy = (*ConsistentHashing)(nil)

type ConsistentHashing struct {
	outbounds            map[string][]adapter.Outbound
	delays               map[string]uint16
	hash                 maphash.Hasher[string]
	maxRetry             int
	maxAcceptableDelay   map[string]uint16
	mu                   sync.Mutex
	delayAcceptableRatio float64
	tolerance            uint16
}

func NewConsistentHashing(outbounds []adapter.Outbound, options option.BalancerOutboundOptions) *ConsistentHashing {
	return &ConsistentHashing{
		outbounds:            convertOutbounds(outbounds),
		hash:                 maphash.NewHasher[string](),
		maxRetry:             defaultMaxRetry(options.MaxRetry),
		delayAcceptableRatio: defaultDelayRatio(options.DelayAcceptableRatio),
		tolerance:            defaultTolerance(options.Tolerance),
	}
}

func (s *ConsistentHashing) Now() string {
	return ""
}

func (s *ConsistentHashing) UpdateOutboundsInfo(history map[string]*adapter.URLTestHistory) bool {
	_, minDelay := getMinDelay(s.outbounds, history)
	delayMap := getDelayMap(history)
	res := map[string]uint16{}
	for net, d := range minDelay {
		res[net] = maxAcceptableDelay(d, s.delayAcceptableRatio, s.tolerance)
	}
	s.mu.Lock()
	s.delays = delayMap
	s.maxAcceptableDelay = res
	s.mu.Unlock()
	return true
}

func (g *ConsistentHashing) Select(metadata adapter.InboundContext, net string, touch bool) adapter.Outbound {
	g.mu.Lock()
	defer g.mu.Unlock()
	if net != N.NetworkTCP && net != N.NetworkUDP {
		net = N.NetworkTCP
	}
	outs := g.outbounds[net]
	if len(outs) == 0 {
		return nil
	}
	key := g.hash.Hash(getKey(&metadata))
	buckets := int32(len(outs))
	for i := 0; i < g.maxRetry; i, key = i+1, key+1 {
		idx := jumpHash(key, buckets)
		proxy := outs[idx]
		if g.Alive(proxy, net) {
			return proxy
		}
	}
	for _, proxy := range outs {
		if g.Alive(proxy, net) {
			return proxy
		}
	}
	return outs[jumpHash(key, buckets)]
}

func (s *ConsistentHashing) Alive(proxy adapter.Outbound, net string) bool {
	if s.delays == nil {
		return true
	}
	if delay, ok := s.delays[proxy.Tag()]; ok {
		if max, ok2 := s.maxAcceptableDelay[net]; ok2 {
			return delay <= max
		}
		return delay < TimeoutDelay
	}
	return false
}
