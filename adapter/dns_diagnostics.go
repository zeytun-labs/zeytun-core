package adapter

import (
	"context"
	"strings"
	"sync"

	"github.com/miekg/dns"
)

// DNSDiagnosticRouter is optional so other DNSRouter implementations remain valid.
type DNSDiagnosticRouter interface {
	ExchangeWithDiagnostics(context.Context, *dns.Msg, string, bool) (*dns.Msg, DNSQueryDiagnostic, error)
}

type DNSQueryDiagnostic struct {
	Server    string `json:"server,omitempty"`
	Transport string `json:"transport,omitempty"`
	Rule      string `json:"rule,omitempty"`
	RuleIndex *int   `json:"rule_index,omitempty"`
	Outbound  string `json:"outbound,omitempty"`
	CacheHit  *bool  `json:"cache_hit,omitempty"`
	Source    string `json:"source,omitempty"`
	Selection string `json:"selection,omitempty"`
}

type dnsDiagnosticKey struct{}
type dnsResponseDiagnostic struct {
	cacheHit bool
	source   string
}

// DNSQueryDiagnostics is request scoped. Responses are correlated by identity so
// speculative DNS branches cannot overwrite the winning response's cache status.
type DNSQueryDiagnostics struct {
	sync.Mutex
	name       string
	qtype      uint16
	ruleServer string
	rule       DNSQueryDiagnostic
	responses  map[*dns.Msg]dnsResponseDiagnostic
	result     DNSQueryDiagnostic
}

func WithDNSQueryDiagnostics(ctx context.Context, message *dns.Msg) (context.Context, *DNSQueryDiagnostics) {
	d := &DNSQueryDiagnostics{responses: make(map[*dns.Msg]dnsResponseDiagnostic)}
	if len(message.Question) == 1 {
		d.name = message.Question[0].Name
		d.qtype = message.Question[0].Qtype
	}
	return context.WithValue(ctx, dnsDiagnosticKey{}, d), d
}

func DNSDiagnosticsFromContext(ctx context.Context) *DNSQueryDiagnostics {
	d, _ := ctx.Value(dnsDiagnosticKey{}).(*DNSQueryDiagnostics)
	return d
}

// Bootstrap lookups must never appear as the parent query's rule/cache result.
func DNSDiagnosticContext(ctx context.Context, message *dns.Msg) context.Context {
	d := DNSDiagnosticsFromContext(ctx)
	if d != nil && (len(message.Question) != 1 || !strings.EqualFold(d.name, message.Question[0].Name) || d.qtype != message.Question[0].Qtype) {
		return context.WithValue(ctx, dnsDiagnosticKey{}, (*DNSQueryDiagnostics)(nil))
	}
	return ctx
}

func RecordDNSDiagnosticRule(ctx context.Context, index int, description, server string) {
	if d := DNSDiagnosticsFromContext(ctx); d != nil {
		d.Lock()
		defer d.Unlock()
		d.ruleServer = server
		d.rule = DNSQueryDiagnostic{Rule: description, RuleIndex: &index, Selection: "rule"}
	}
}

func RecordDNSDiagnosticResponse(ctx context.Context, response *dns.Msg, cacheHit bool, source string) {
	if d := DNSDiagnosticsFromContext(ctx); d != nil && response != nil {
		d.Lock()
		defer d.Unlock()
		d.responses[response] = dnsResponseDiagnostic{cacheHit, source}
	}
}

func CompleteDNSDiagnostic(ctx context.Context, response *dns.Msg, transport DNSTransport, explicit bool) {
	if d := DNSDiagnosticsFromContext(ctx); d != nil {
		d.Lock()
		defer d.Unlock()
		result := DNSQueryDiagnostic{Selection: "final"}
		if explicit {
			result.Selection = "explicit"
		} else if d.rule.RuleIndex != nil && (d.ruleServer == "" || transport != nil && transport.Tag() == d.ruleServer) {
			result = d.rule
		}
		if transport != nil {
			result.Server = transport.Tag()
			result.Transport = transport.Type()
			if info, ok := transport.(interface{ DiagnosticOutbound() string }); ok {
				result.Outbound = info.DiagnosticOutbound()
			}
		}
		if info, ok := d.responses[response]; ok {
			hit := info.cacheHit
			result.CacheHit = &hit
			result.Source = info.source
			if !hit && transport != nil {
				switch transport.Type() {
				case "fakeip", "hosts", "local":
					result.Source = transport.Type()
				}
			}
		} else if transport == nil && response != nil {
			result.Source = "predefined"
		}
		d.result = result
	}
}

func (d *DNSQueryDiagnostics) Snapshot() DNSQueryDiagnostic {
	d.Lock()
	defer d.Unlock()
	return d.result
}

// Legacy response-check rules may reject an answer and continue to final.
func ResetDNSDiagnosticRule(ctx context.Context) {
	if d := DNSDiagnosticsFromContext(ctx); d != nil {
		d.Lock()
		defer d.Unlock()
		d.ruleServer = ""
		d.rule = DNSQueryDiagnostic{}
	}
}
