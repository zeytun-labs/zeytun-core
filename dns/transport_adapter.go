package dns

import (
	"github.com/sagernet/sing-box/option"
)

type TransportAdapter struct {
	transportType string
	transportTag  string
	dependencies  []string
	references    []string
	network       bool
}

func NewTransportAdapter(transportType string, transportTag string, dependencies []string) TransportAdapter {
	return TransportAdapter{
		transportType: transportType,
		transportTag:  transportTag,
		dependencies:  dependencies,
	}
}

func NewTransportAdapterWithLocalOptions(transportType string, transportTag string, localOptions option.LocalDNSServerOptions) TransportAdapter {
	var dependencies []string
	if localOptions.DomainResolver != nil && localOptions.DomainResolver.Server != "" {
		dependencies = append(dependencies, localOptions.DomainResolver.Server)
	}
	var references []string
	if localOptions.Detour != "" {
		references = []string{localOptions.Detour}
	}
	return TransportAdapter{
		transportType: transportType,
		transportTag:  transportTag,
		dependencies:  dependencies,
		references:    references,
		network:       true,
	}
}

func NewTransportAdapterWithRemoteOptions(transportType string, transportTag string, remoteOptions option.RemoteDNSServerOptions) TransportAdapter {
	var dependencies []string
	if remoteOptions.DomainResolver != nil && remoteOptions.DomainResolver.Server != "" {
		dependencies = append(dependencies, remoteOptions.DomainResolver.Server)
	}
	var references []string
	if remoteOptions.Detour != "" {
		references = []string{remoteOptions.Detour}
	}
	return TransportAdapter{
		transportType: transportType,
		transportTag:  transportTag,
		dependencies:  dependencies,
		references:    references,
		network:       true,
	}
}

func (a *TransportAdapter) Type() string {
	return a.transportType
}

func (a *TransportAdapter) Tag() string {
	return a.transportTag
}

func (a *TransportAdapter) Dependencies() []string {
	return a.dependencies
}

func (a *TransportAdapter) References() []string {
	return a.references
}

// DiagnosticOutbound is the configured runtime detour, not an inferred route
// from the app's saved profile. A policy tag may select a changing child proxy.
func (a *TransportAdapter) DiagnosticOutbound() string {
	if len(a.references) > 0 {
		return a.references[0]
	}
	if a.network {
		return "direct"
	}
	return ""
}
