package dns

import (
	"context"
	mDNS "github.com/miekg/dns"
	"github.com/sagernet/sing-box/adapter"
	E "github.com/sagernet/sing/common/exceptions"
)

func (r *Router) ExchangeWithDiagnostics(ctx context.Context, message *mDNS.Msg, server string, bypassCache bool) (*mDNS.Msg, adapter.DNSQueryDiagnostic, error) {
	ctx, diagnostic := adapter.WithDNSQueryDiagnostics(ctx, message)
	options := adapter.DNSQueryOptions{DisableCache: bypassCache, DisableOptimisticCache: bypassCache}
	if server != "" {
		transport, loaded := r.transport.Transport(server)
		if !loaded {
			return nil, adapter.DNSQueryDiagnostic{}, E.New("DNS server is not active: ", server)
		}
		options.Transport = transport
	}
	response, err := r.Exchange(ctx, message, options)
	return response, diagnostic.Snapshot(), err
}
