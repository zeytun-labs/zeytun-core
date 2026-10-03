package option

import "github.com/sagernet/sing/common/json/badoption"

type BalancerOutboundOptions struct {
	Outbounds                 []string           `json:"outbounds"`
	Tolerance                 uint16             `json:"tolerance,omitempty"` // ms; delay pool / alive band (default 50, like urltest)
	InterruptExistConnections bool               `json:"interrupt_exist_connections,omitempty"`
	Strategy                  string             `json:"strategy,omitempty"` // round-robin | consistent-hashing | sticky-sessions | failover | weighted | least-connections
	DelayAcceptableRatio      float64            `json:"delay_acceptable_ratio,omitempty"`
	TTL                       badoption.Duration `json:"ttl,omitempty"`       // sticky-sessions cache lifetime
	MaxRetry                  int                `json:"max_retry,omitempty"` // consistent-hashing / sticky-sessions
	Weights                   []int              `json:"weights,omitempty"`   // weighted: parallel to outbounds
}
