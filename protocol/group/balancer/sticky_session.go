package balancer

import (
	"sync"
	"time"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/contrab/freelru"
	"github.com/sagernet/sing/contrab/maphash"
)

var _ Strategy = (*StickySession)(nil)

type StickySession struct {
	outbounds            map[string][]adapter.Outbound
	hash                 maphash.Hasher[string]
	maxRetry             int
	delays               map[string]uint16
	maxAcceptableDelay   map[string]uint16
	mu                   sync.Mutex
	delayAcceptableRatio float64
	tolerance            uint16
	lruCache             *freelru.Cache[uint64, int]
}

func NewStickySession(outbounds []adapter.Outbound, options option.BalancerOutboundOptions) *StickySession {
	lruCache := common.Must1(freelru.New[uint64, int](1000, maphash.NewHasher[uint64]().Hash32, true))
	ttl := options.TTL.Build()
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	lruCache.SetLifetime(ttl)
	return &StickySession{
		outbounds:            convertOutbounds(outbounds),
		lruCache:             lruCache,
		hash:                 maphash.NewHasher[string](),
		maxRetry:             defaultMaxRetry(options.MaxRetry),
		delayAcceptableRatio: defaultDelayRatio(options.DelayAcceptableRatio),
		tolerance:            defaultTolerance(options.Tolerance),
	}
}

func (s *StickySession) Select(metadata adapter.InboundContext, net string, touch bool) adapter.Outbound {
	s.mu.Lock()
	defer s.mu.Unlock()

	if net != N.NetworkTCP && net != N.NetworkUDP {
		net = N.NetworkTCP
	}
	outs := s.outbounds[net]
	length := len(outs)
	if length == 0 {
		return nil
	}
	key := s.hash.Hash(getKeyWithSrcAndDst(&metadata))
	idx, has := s.lruCache.Get(key)
	if !has || idx >= length {
		idx = int(jumpHash(key+uint64(time.Now().UnixNano()), int32(length)))
	}

	nowIdx := idx
	for i := 1; i < s.maxRetry; i++ {
		proxy := outs[nowIdx]
		if s.Alive(proxy, net) {
			if !has || nowIdx != idx {
				s.lruCache.Add(key, nowIdx)
			}
			return proxy
		}
		nowIdx = int(jumpHash(key+uint64(time.Now().UnixNano()), int32(length)))
	}
	s.lruCache.Add(key, nowIdx)
	return outs[nowIdx]
}

func (s *StickySession) Now() string {
	return ""
}

func (s *StickySession) UpdateOutboundsInfo(history map[string]*adapter.URLTestHistory) bool {
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

func (s *StickySession) Alive(proxy adapter.Outbound, net string) bool {
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
