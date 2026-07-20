package balancer

import (
	"context"
	"net"
	"testing"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/option"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

type testOB struct{ tag string }

func (t *testOB) Type() string                              { return "test" }
func (t *testOB) Tag() string                               { return t.tag }
func (t *testOB) Network() []string                         { return []string{N.NetworkTCP, N.NetworkUDP} }
func (t *testOB) Dependencies() []string                    { return nil }
func (t *testOB) DialContext(context.Context, string, M.Socksaddr) (net.Conn, error) {
	return nil, net.ErrClosed
}
func (t *testOB) ListenPacket(context.Context, M.Socksaddr) (net.PacketConn, error) {
	return nil, net.ErrClosed
}

func outs(tags ...string) []adapter.Outbound {
	o := make([]adapter.Outbound, len(tags))
	for i, t := range tags {
		o[i] = &testOB{tag: t}
	}
	return o
}

func TestFailoverPrimaryBackup(t *testing.T) {
	s := NewFailover(outs("a", "b", "c"), option.BalancerOutboundOptions{})
	md := adapter.InboundContext{}

	if got := s.Select(md, N.NetworkTCP, true); got.Tag() != "a" {
		t.Fatalf("want a, got %s", got.Tag())
	}
	// mark a dead
	s.UpdateOutboundsInfo(map[string]*adapter.URLTestHistory{
		"a": {Delay: TimeoutDelay},
		"b": {Delay: 50},
		"c": {Delay: 60},
	})
	if got := s.Select(md, N.NetworkTCP, true); got.Tag() != "b" {
		t.Fatalf("want b after a dead, got %s", got.Tag())
	}
	// primary recovers
	s.UpdateOutboundsInfo(map[string]*adapter.URLTestHistory{
		"a": {Delay: 40},
		"b": {Delay: 50},
		"c": {Delay: 60},
	})
	if got := s.Select(md, N.NetworkTCP, true); got.Tag() != "a" {
		t.Fatalf("want a after recover, got %s", got.Tag())
	}
}

func TestWeightedRoundRobin(t *testing.T) {
	s := NewWeighted(outs("a", "b"), option.BalancerOutboundOptions{Weights: []int{3, 1}})
	// no history → all alive
	counts := map[string]int{}
	md := adapter.InboundContext{}
	for i := 0; i < 40; i++ {
		got := s.Select(md, N.NetworkTCP, true)
		counts[got.Tag()]++
	}
	// 3:1 → ~30:10
	if counts["a"] < 25 || counts["b"] < 5 {
		t.Fatalf("bad distribution: %#v", counts)
	}
}

func TestLeastConnections(t *testing.T) {
	s := NewLeastConnections(outs("a", "b"), option.BalancerOutboundOptions{})
	md := adapter.InboundContext{}
	a := s.Select(md, N.NetworkTCP, true)
	s.Opened(a)
	b := s.Select(md, N.NetworkTCP, true)
	if b.Tag() == a.Tag() {
		t.Fatalf("expected other outbound after open on %s, got %s", a.Tag(), b.Tag())
	}
	s.Opened(b)
	s.Closed(a)
	// a now 0, b 1 → next should be a
	next := s.Select(md, N.NetworkTCP, true)
	if next.Tag() != a.Tag() {
		t.Fatalf("want %s after close, got %s", a.Tag(), next.Tag())
	}
}

func TestRoundRobin(t *testing.T) {
	s := NewRoundRobin(outs("a", "b", "c"), option.BalancerOutboundOptions{})
	md := adapter.InboundContext{}
	
	counts := map[string]int{}
	for i := 0; i < 6; i++ {
		got := s.Select(md, N.NetworkTCP, true)
		counts[got.Tag()]++
	}
	
	if counts["a"] != 2 || counts["b"] != 2 || counts["c"] != 2 {
		t.Fatalf("bad distribution for round-robin: %#v", counts)
	}
	
	// Update with history: c is dead (TimeoutDelay)
	s.UpdateOutboundsInfo(map[string]*adapter.URLTestHistory{
		"a": {Delay: 50},
		"b": {Delay: 50},
		"c": {Delay: TimeoutDelay},
	})
	
	counts = map[string]int{}
	for i := 0; i < 4; i++ {
		got := s.Select(md, N.NetworkTCP, true)
		counts[got.Tag()]++
	}
	
	if counts["a"] != 2 || counts["b"] != 2 || counts["c"] != 0 {
		t.Fatalf("bad distribution after c died: %#v", counts)
	}
}

func TestConsistentHashing(t *testing.T) {
	s := NewConsistentHashing(outs("a", "b", "c"), option.BalancerOutboundOptions{})
	md1 := adapter.InboundContext{Destination: M.Socksaddr{Fqdn: "google.com"}}
	md2 := adapter.InboundContext{Destination: M.Socksaddr{Fqdn: "yahoo.com"}}
	
	// Hash must be deterministic for the same metadata
	first := s.Select(md1, N.NetworkTCP, true)
	for i := 0; i < 5; i++ {
		if got := s.Select(md1, N.NetworkTCP, true); got.Tag() != first.Tag() {
			t.Fatalf("consistent hashing returned different result for same md: %s vs %s", first.Tag(), got.Tag())
		}
	}
	
	// Different metadata should likely hash to different outbound (probabilistic, but highly likely for small sets)
	// We just ensure it doesn't panic and returns a valid alive tag.
	second := s.Select(md2, N.NetworkTCP, true)
	if second == nil {
		t.Fatalf("select returned nil")
	}
}

func TestStickySession(t *testing.T) {
	s := NewStickySession(outs("a", "b", "c"), option.BalancerOutboundOptions{})
	md1 := adapter.InboundContext{Destination: M.Socksaddr{Fqdn: "google.com"}}
	
	first := s.Select(md1, N.NetworkTCP, true)
	for i := 0; i < 5; i++ {
		if got := s.Select(md1, N.NetworkTCP, true); got.Tag() != first.Tag() {
			t.Fatalf("sticky session returned different result: %s vs %s", first.Tag(), got.Tag())
		}
	}
}
