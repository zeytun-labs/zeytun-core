package adapter

import (
	"context"
	"github.com/miekg/dns"
	"sync"
	"testing"
)

func TestDNSDiagnosticsSeparateWinningResponseAndBootstrap(t *testing.T) {
	message := new(dns.Msg)
	message.SetQuestion("app.example.", dns.TypeA)
	ctx, diagnostic := WithDNSQueryDiagnostics(context.Background(), message)
	winner, loser := new(dns.Msg), new(dns.Msg)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); RecordDNSDiagnosticResponse(ctx, loser, false, "upstream") }()
	}
	RecordDNSDiagnosticResponse(ctx, winner, true, "cache")
	bootstrap := new(dns.Msg)
	bootstrap.SetQuestion("dns.example.", dns.TypeAAAA)
	bootstrapCtx := DNSDiagnosticContext(ctx, bootstrap)
	if DNSDiagnosticsFromContext(bootstrapCtx) != nil {
		t.Fatal("bootstrap inherited parent diagnostics")
	}
	RecordDNSDiagnosticRule(bootstrapCtx, 99, "bootstrap", "local-dns")
	wg.Wait()
	RecordDNSDiagnosticRule(ctx, 0, "domain=app.example => respond", "")
	CompleteDNSDiagnostic(ctx, winner, nil, false)
	trace := diagnostic.Snapshot()
	if trace.CacheHit == nil || !*trace.CacheHit || trace.Source != "cache" || trace.RuleIndex == nil || *trace.RuleIndex != 0 {
		t.Fatalf("wrong winning trace: %+v", trace)
	}
	CompleteDNSDiagnostic(ctx, winner, nil, true)
	trace = diagnostic.Snapshot()
	if trace.Selection != "explicit" || trace.RuleIndex != nil {
		t.Fatalf("explicit selection claimed a rule: %+v", trace)
	}
}
