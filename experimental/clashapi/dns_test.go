package clashapi

import (
	"context"
	"encoding/json"
	"github.com/miekg/dns"
	"github.com/sagernet/sing-box/adapter"
	"net/http/httptest"
	"testing"
)

type diagnosticDNSRouter struct {
	adapter.DNSRouter
	called bool
	server string
	bypass bool
	qtype  uint16
}

func (f *diagnosticDNSRouter) ExchangeWithDiagnostics(ctx context.Context, message *dns.Msg, server string, bypass bool) (*dns.Msg, adapter.DNSQueryDiagnostic, error) {
	f.called = true
	f.server = server
	f.bypass = bypass
	f.qtype = message.Question[0].Qtype
	response := new(dns.Msg)
	response.SetReply(message)
	hit := true
	return response, adapter.DNSQueryDiagnostic{Server: server, Transport: "https", CacheHit: &hit, Selection: "explicit"}, nil
}
func TestDNSQueryExplicitServerRecordTypeAndTrace(t *testing.T) {
	router := new(diagnosticDNSRouter)
	request := httptest.NewRequest("GET", "/dns/query?name=app.example&type=aaaa&server=google&bypass_cache=true", nil)
	response := httptest.NewRecorder()
	queryDNS(router)(response, request)
	if response.Code != 200 || !router.called || router.server != "google" || !router.bypass || router.qtype != dns.TypeAAAA {
		t.Fatalf("options lost: status=%d router=%+v body=%s", response.Code, router, response.Body)
	}
	var body struct {
		Server string
		Trace  adapter.DNSQueryDiagnostic
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Server != "google" || body.Trace.CacheHit == nil || !*body.Trace.CacheHit {
		t.Fatalf("trace lost: %+v", body)
	}
}
func TestDNSQueryRejectsInvalidOptions(t *testing.T) {
	for _, query := range []string{"type=invalid", "type=A&bypass_cache=maybe"} {
		router := new(diagnosticDNSRouter)
		response := httptest.NewRecorder()
		queryDNS(router)(response, httptest.NewRequest("GET", "/dns/query?name=app.example&"+query, nil))
		if response.Code != 400 || router.called {
			t.Fatalf("invalid options accepted: %s", query)
		}
	}
}
