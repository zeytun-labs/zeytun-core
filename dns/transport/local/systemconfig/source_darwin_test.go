//go:build darwin && cgo

package systemconfig

import (
	"net/netip"
	"slices"
	"testing"

	M "github.com/sagernet/sing/common/metadata"
)

func TestPhysicalDNSFollowsDefaultInterface(t *testing.T) {
	wifi := M.SocksaddrFrom(netip.MustParseAddr("192.0.2.1"), 53)
	ethernet := M.SocksaddrFrom(netip.MustParseAddr("192.0.2.2"), 5353)
	config := &dnsInfoConfig{
		resolvers: []dnsInfoResolver{{servers: []M.Socksaddr{wifi}}},
		scopedResolvers: []dnsInfoResolver{
			{interfaceIndex: 4, servers: []M.Socksaddr{wifi}, search: []string{"wifi.example."}},
			{interfaceIndex: 7, servers: []M.Socksaddr{ethernet}, search: []string{"lan.example."}},
		},
	}
	first := config.build(4)
	second := config.build(7)
	if !slices.Equal(first.Servers, []M.Socksaddr{wifi}) || !slices.Equal(second.Servers, []M.Socksaddr{ethernet}) {
		t.Fatalf("physical DNS did not follow uplink: %v -> %v", first.Servers, second.Servers)
	}
	if !slices.Equal(second.Search, []string{"lan.example."}) {
		t.Fatalf("new uplink search domains were lost: %v", second.Search)
	}
	// Selection must not mutate the source snapshot or an already selected config.
	if !slices.Equal(first.Servers, []M.Socksaddr{wifi}) || config.scopedResolvers[1].servers[0] != ethernet {
		t.Fatal("building a new configuration changed the source or old selection")
	}
}

func TestPhysicalDNSFallsBackToUnscopedDefault(t *testing.T) {
	physical := M.SocksaddrFrom(netip.MustParseAddr("192.0.2.53"), 53)
	split := M.SocksaddrFrom(netip.MustParseAddr("192.0.2.54"), 53)
	config := &dnsInfoConfig{resolvers: []dnsInfoResolver{
		{domain: "private.example", servers: []M.Socksaddr{split}},
		{servers: []M.Socksaddr{physical}},
	}}
	if result := config.build(99); !slices.Equal(result.Servers, []M.Socksaddr{physical}) {
		t.Fatalf("fallback selected domain-specific DNS: %v", result.Servers)
	}
}
