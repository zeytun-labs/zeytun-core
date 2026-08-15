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
