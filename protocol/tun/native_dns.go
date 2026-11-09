package tun

import (
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/common/systemdns"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-tun"
	E "github.com/sagernet/sing/common/exceptions"
)

func (t *Inbound) startNativeDNS(scope *adapter.Scope) error {
	if !C.IsDarwin || t.platformInterface != nil || !t.tunOptions.AutoRoute ||
		t.tunOptions.DNSModeOrDefault() == tun.DNSModeDisabled {
		return nil
	}
	// All DNS transports and the TUN stack have completed PostStart before this
	// stage. Scope closes this session before tearing down the TUN interface.
	addresses := t.tunOptions.DNSAddress
	if len(addresses) == 0 {
		addresses = t.dnsHijackAddress
	}
	resolver, err := systemdns.Open(t.tag, addresses)
	if err != nil {
		return E.Cause(err, "configure macOS system DNS")
	}
	if resolver != nil {
		scope.Add(resolver.Close)
		t.logger.Info("system DNS routed through ", t.tunOptions.Name)
	}
	return nil
}
