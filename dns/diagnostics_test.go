package dns

import (
	"context"
	mDNS "github.com/miekg/dns"
	"github.com/sagernet/sing-box/option"
	"github.com/stretchr/testify/require"
	"net/netip"
	"testing"
	"time"
)

func TestDNSDiagnosticTraceUsesWinningRaceRule(t *testing.T) {
	slow := &fakeDNSTransport{tag: "slow", delay: 100 * time.Millisecond, address: netip.MustParseAddr("192.0.2.1")}
	fast := &fakeDNSTransport{tag: "fast", delay: 5 * time.Millisecond, address: netip.MustParseAddr("192.0.2.2")}
	router := raceTestRouter(t, slow, fast)
	router.rules = raceTestRules(t, []option.DNSRule{
		evaluateRule("slow", "s", false), evaluateRule("fast", "f", false),
		respondRule("s", true, true), respondRule("f", true, true),
	})
	msg := new(mDNS.Msg)
	msg.SetQuestion("race.example.org.", mDNS.TypeA)
	response, trace, err := router.ExchangeWithDiagnostics(context.Background(), msg, "", false)
	require.NoError(t, err)
	require.Equal(t, netip.MustParseAddr("192.0.2.2"), responseAddress(t, response))
	require.Equal(t, "fast", trace.Server)
	require.NotNil(t, trace.RuleIndex)
	require.Equal(t, 3, *trace.RuleIndex)
	require.NotNil(t, trace.CacheHit)
	require.False(t, *trace.CacheHit)
	require.Equal(t, "rule", trace.Selection)
}

func TestDNSDiagnosticTraceSupportsLegacyMatching(t *testing.T) {
	upstream := &fakeDNSTransport{tag: "upstream", address: netip.MustParseAddr("192.0.2.1")}
	router := raceTestRouter(t, upstream)
	router.legacyDNSMode = true
	router.rules = raceTestRules(t, []option.DNSRule{routeRule("upstream", false)})
	msg := new(mDNS.Msg)
	msg.SetQuestion("app.example.", mDNS.TypeA)
	_, trace, err := router.ExchangeWithDiagnostics(context.Background(), msg, "", false)
	require.NoError(t, err)
	require.Equal(t, "upstream", trace.Server)
	require.NotNil(t, trace.RuleIndex)
	require.Equal(t, 0, *trace.RuleIndex)
}
