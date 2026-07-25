package route

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/experimental/coreevent"
	"github.com/sagernet/sing-box/option"
	E "github.com/sagernet/sing/common/exceptions"
	F "github.com/sagernet/sing/common/format"
	"github.com/sagernet/sing/common/logger"
)

// ConnectionAsk holds unmatched TCP connections until UI Decide or timeout→final.
type ConnectionAsk struct {
	enabled      bool
	timeout      time.Duration
	groupBy      string // process | process_dest
	logger       logger.ContextLogger
	ctx          context.Context
	access       sync.Mutex
	session      map[string]askDecision
	pendingByKey map[string]*askPending
	pendingByID  map[string]*askPending
	maxPending   int
}

type askDecision struct {
	Outbound string
	Reject   bool
}

type askPending struct {
	id     string
	key    string
	meta   adapter.InboundContext
	done   chan struct{}
	once   sync.Once
	result askDecision
	// timeoutCached: false so next conn can re-ask after timeout
	fromTimeout atomic.Bool
}

func newConnectionAsk(ctx context.Context, logger logger.ContextLogger, opts *option.ConnectionAskOptions) *ConnectionAsk {
	if opts == nil || !opts.Enabled {
		return &ConnectionAsk{enabled: false}
	}
	timeout := time.Duration(opts.Timeout) * time.Millisecond
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	groupBy := opts.GroupBy
	if groupBy != "process_dest" {
		groupBy = "process"
	}
	return &ConnectionAsk{
		enabled:      true,
		timeout:      timeout,
		groupBy:      groupBy,
		logger:       logger,
		ctx:          ctx,
		session:      make(map[string]askDecision),
		pendingByKey: make(map[string]*askPending),
		pendingByID:  make(map[string]*askPending),
		maxPending:   64,
	}
}

func (a *ConnectionAsk) Enabled() bool {
	return a != nil && a.enabled
}

func (a *ConnectionAsk) groupKey(meta *adapter.InboundContext) string {
	proc := ""
	if meta.ProcessInfo != nil {
		proc = meta.ProcessInfo.ProcessPath
	}
	if proc == "" {
		proc = "unknown"
	}
	if a.groupBy == "process_dest" {
		dest := meta.Domain
		if dest == "" {
			dest = meta.Destination.Fqdn
		}
		if dest == "" {
			dest = meta.Destination.AddrString()
		}
		return proc + "\x00" + dest + "\x00" + F.ToString(meta.Destination.Port)
	}
	return proc
}

// Resolve blocks until Decide/timeout. ok=false → skip ask (use final without wait).
func (a *ConnectionAsk) Resolve(ctx context.Context, meta *adapter.InboundContext) (askDecision, bool) {
	if !a.Enabled() {
		return askDecision{}, false
	}
	// ponytail: dest-only ask when process missing
	if meta.ProcessInfo == nil || meta.ProcessInfo.ProcessPath == "" {
		return askDecision{}, false
	}

	key := a.groupKey(meta)

	a.access.Lock()
	if d, ok := a.session[key]; ok {
		a.access.Unlock()
		return d, true
	}
	if p, ok := a.pendingByKey[key]; ok {
		a.access.Unlock()
		return a.wait(ctx, p)
	}
	if len(a.pendingByID) >= a.maxPending {
		a.access.Unlock()
		a.logger.WarnContext(ctx, "connection ask queue full, using final")
		return askDecision{}, false
	}

	id := uuid.Must(uuid.NewV4()).String()
	p := &askPending{
		id:   id,
		key:  key,
		meta: *meta,
		done: make(chan struct{}),
	}
	a.pendingByKey[key] = p
	a.pendingByID[id] = p
	a.access.Unlock()

	coreevent.EmitConnectionAsk(ctx, id, metaAttrs(meta, key, a.groupBy, a.timeout))

	go func() {
		timer := time.NewTimer(a.timeout)
		defer timer.Stop()
		select {
		case <-timer.C:
			a.complete(p, askDecision{}, true)
		case <-a.ctx.Done():
			a.complete(p, askDecision{}, true)
		case <-p.done:
		}
	}()

	return a.wait(ctx, p)
}

func (a *ConnectionAsk) wait(ctx context.Context, p *askPending) (askDecision, bool) {
	select {
	case <-p.done:
		return p.result, true
	case <-ctx.Done():
		return askDecision{}, false
	case <-a.ctx.Done():
		return askDecision{}, false
	}
}

func (a *ConnectionAsk) complete(p *askPending, d askDecision, fromTimeout bool) {
	p.once.Do(func() {
		p.result = d
		p.fromTimeout.Store(fromTimeout)
		a.access.Lock()
		if !fromTimeout {
			a.session[p.key] = d
		}
		delete(a.pendingByKey, p.key)
		delete(a.pendingByID, p.id)
		a.access.Unlock()
		// Don't re-push resolved ask on gRPC resubscribe (stale id → UI Allow dead).
		if h := coreevent.FromContext(a.ctx); h != nil {
			h.Forget(coreevent.CodeConnectionAsk, p.id)
		}
		close(p.done)
	})
}

// Decide applies UI choice. outbound empty + reject=false → final. reject → drop.
func (a *ConnectionAsk) Decide(id, outbound string, reject bool) error {
	if !a.Enabled() {
		return E.New("connection ask disabled")
	}
	a.access.Lock()
	p, ok := a.pendingByID[id]
	a.access.Unlock()
	if !ok {
		return E.New("pending connection not found: ", id)
	}
	a.complete(p, askDecision{Outbound: outbound, Reject: reject}, false)
	return nil
}

func metaAttrs(meta *adapter.InboundContext, key, groupBy string, timeout time.Duration) map[string]string {
	attrs := map[string]string{
		"group_by":   groupBy,
		"group_key":  key,
		"timeout_ms": F.ToString(int(timeout / time.Millisecond)),
		"network":    meta.Network,
		"dest":       meta.Destination.String(),
		"dest_host":  meta.Destination.AddrString(),
		"dest_port":  F.ToString(meta.Destination.Port),
		"domain":     meta.Domain,
		"protocol":   meta.Protocol,
		"inbound":    meta.Inbound,
	}
	if meta.Destination.IsFqdn() {
		attrs["dest_host"] = meta.Destination.Fqdn
	}
	if meta.Domain != "" {
		attrs["dest_host"] = meta.Domain
	}
	if meta.ProcessInfo != nil {
		attrs["process_path"] = meta.ProcessInfo.ProcessPath
		attrs["process_name"] = filepath.Base(meta.ProcessInfo.ProcessPath)
		if i := strings.Index(meta.ProcessInfo.ProcessPath, ".app/"); i >= 0 {
			bundle := meta.ProcessInfo.ProcessPath[:i+4]
			attrs["process_name"] = filepath.Base(bundle)
			attrs["process_bundle"] = bundle
		}
		attrs["process_id"] = F.ToString(meta.ProcessInfo.ProcessID)
	}
	return attrs
}
