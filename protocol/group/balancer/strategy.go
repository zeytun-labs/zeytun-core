package balancer

import "github.com/sagernet/sing-box/adapter"

type Strategy interface {
	UpdateOutboundsInfo(outbounds map[string]*adapter.URLTestHistory) (changed bool)
	Select(metadata adapter.InboundContext, network string, touch bool) adapter.Outbound
	Now() string
}

// connectionLifecycle is optional; least-connections implements it.
type connectionLifecycle interface {
	Opened(out adapter.Outbound)
	Closed(out adapter.Outbound)
}
