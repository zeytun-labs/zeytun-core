package coreevent

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/sagernet/sing/common/observable"
	"github.com/sagernet/sing/service"
)

// Semantic notification bus for desktop clients (not traffic/log streams).

type Scope int32

const (
	ScopeUnspecified   Scope = 0
	ScopeService       Scope = 1
	ScopeClashMode     Scope = 2
	ScopeRuleSet       Scope = 3
	ScopeSystem        Scope = 4
	ScopeConnectionAsk Scope = 5
)

type Severity int32

const (
	SeverityInfo     Severity = 0
	SeverityWarning  Severity = 1
	SeverityError    Severity = 2
	SeverityCritical Severity = 3
)

// Stable codes (UI i18n / action map keys).
const (
	CodeRuleSetInitialFetchFailed = "RULESET_INITIAL_FETCH_FAILED"
	CodeRuleSetReady              = "RULESET_READY"
	CodeRuleSetUpdateFailed       = "RULESET_UPDATE_FAILED"
	CodeRuleSetUpdated            = "RULESET_UPDATED"

	CodeServiceIdle     = "SERVICE_IDLE"
	CodeServiceStarting = "SERVICE_STARTING"
	CodeServiceStarted  = "SERVICE_STARTED"
	CodeServiceStopping = "SERVICE_STOPPING"
	CodeServiceFatal    = "SERVICE_FATAL"

	CodeClashModeChanged = "CLASH_MODE_CHANGED"

	CodeConnectionAsk = "CONNECTION_ASK"
)

type Event struct {
	ID       string
	TsMs     int64
	Scope    Scope
	Code     string
	Severity Severity
	Title    string
	Message  string
	Attrs    map[string]string
}

type Hub struct {
	subscriber *observable.Subscriber[*Event]
	observer   *observable.Observer[*Event]
	access     sync.Mutex
	// last by "scope|code|tag" for snapshot on subscribe
	last map[string]*Event
	ring []*Event
}

const ringCap = 64

func NewHub() *Hub {
	sub := observable.NewSubscriber[*Event](64)
	return &Hub{
		subscriber: sub,
		observer:   observable.NewObserver(sub, 32),
		last:       make(map[string]*Event),
	}
}

func (h *Hub) Subscribe() (observable.Subscription[*Event], <-chan struct{}, error) {
	return h.observer.Subscribe()
}

func (h *Hub) UnSubscribe(sub observable.Subscription[*Event]) {
	h.observer.UnSubscribe(sub)
}

func (h *Hub) Snapshot() []*Event {
	h.access.Lock()
	defer h.access.Unlock()
	out := make([]*Event, 0, len(h.last))
	for _, e := range h.last {
		out = append(out, e)
	}
	return out
}

func eventDedupeKey(e *Event) string {
	key := e.Code
	if e.Scope == ScopeService {
		return "SERVICE"
	}
	if e.Scope == ScopeClashMode {
		return "CLASH_MODE"
	}
	if tag, ok := e.Attrs["tag"]; ok && tag != "" {
		return e.Code + "|" + tag
	}
	return key
}

func (h *Hub) Emit(e *Event) {
	if e == nil {
		return
	}
	if e.ID == "" {
		e.ID = uuid.Must(uuid.NewV4()).String()
	}
	if e.TsMs == 0 {
		e.TsMs = time.Now().UnixMilli()
	}
	key := eventDedupeKey(e)
	h.access.Lock()
	h.last[key] = e
	h.ring = append(h.ring, e)
	if len(h.ring) > ringCap {
		h.ring = h.ring[len(h.ring)-ringCap:]
	}
	h.access.Unlock()
	h.subscriber.Emit(e)
}

// Forget drops last-snapshot entry so SubscribeEvent won't re-deliver resolved asks.
func (h *Hub) Forget(code, tag string) {
	if h == nil {
		return
	}
	key := code
	if tag != "" {
		key = code + "|" + tag
	}
	h.access.Lock()
	delete(h.last, key)
	h.access.Unlock()
}

// ForgetCodePrefix clears last-snapshot keys for a code (and code|tag variants).
func (h *Hub) ForgetCodePrefix(code string) {
	if h == nil || code == "" {
		return
	}
	prefix := code + "|"
	h.access.Lock()
	for k := range h.last {
		if k == code || strings.HasPrefix(k, prefix) {
			delete(h.last, k)
		}
	}
	h.access.Unlock()
}

func FromContext(ctx context.Context) *Hub {
	return service.FromContext[*Hub](ctx)
}

func Emit(ctx context.Context, e *Event) {
	if h := FromContext(ctx); h != nil {
		h.Emit(e)
	}
}

func EmitRuleSetInitialFetchFailed(ctx context.Context, tag string, err error) {
	EmitRuleSetFetchFailed(ctx, tag, CodeRuleSetInitialFetchFailed, SeverityError, "Ruleset not downloaded", err, nil)
}

func EmitRuleSetReady(ctx context.Context, tag string) {
	// Drop fail snapshots so SubscribeEvent re-connect doesn't re-poison UI after success.
	if h := FromContext(ctx); h != nil {
		h.Forget(CodeRuleSetInitialFetchFailed, tag)
		h.Forget(CodeRuleSetUpdateFailed, tag)
	}
	Emit(ctx, &Event{
		Scope:    ScopeRuleSet,
		Code:     CodeRuleSetReady,
		Severity: SeverityInfo,
		Title:    "Ruleset ready",
		Message:  "",
		Attrs: map[string]string{
			"tag":    tag,
			"status": "ready",
		},
	})
}

func EmitRuleSetUpdateFailed(ctx context.Context, tag string, err error) {
	EmitRuleSetFetchFailed(ctx, tag, CodeRuleSetUpdateFailed, SeverityWarning, "Ruleset update failed", err, nil)
}

// EmitRuleSetFetchFailed is the shared fail path; extra attrs (url, detour) optional.
func EmitRuleSetFetchFailed(ctx context.Context, tag, code string, sev Severity, title string, err error, extra map[string]string) {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	attrs := map[string]string{
		"tag":    tag,
		"status": "failed",
		"error":  msg,
	}
	for k, v := range extra {
		if v != "" {
			attrs[k] = v
		}
	}
	Emit(ctx, &Event{
		Scope:    ScopeRuleSet,
		Code:     code,
		Severity: sev,
		Title:    title,
		Message:  msg,
		Attrs:    attrs,
	})
}

func EmitRuleSetUpdated(ctx context.Context, tag string) {
	if h := FromContext(ctx); h != nil {
		h.Forget(CodeRuleSetInitialFetchFailed, tag)
		h.Forget(CodeRuleSetUpdateFailed, tag)
	}
	Emit(ctx, &Event{
		Scope:    ScopeRuleSet,
		Code:     CodeRuleSetUpdated,
		Severity: SeverityInfo,
		Title:    "Ruleset updated",
		Message:  "",
		Attrs: map[string]string{
			"tag":    tag,
			"status": "ready",
		},
	})
}

// EmitServiceStatus dual-publishes lifecycle (alongside SubscribeServiceStatus).
// statusName: idle|starting|started|stopping|fatal
func (h *Hub) EmitServiceStatus(statusName, errMsg string) {
	if h == nil {
		return
	}
	// Pending ask ids die with the router; drop snapshots so UI/gRPC don't revive them.
	switch statusName {
	case "starting", "started", "stopping", "idle", "fatal":
		h.ForgetCodePrefix(CodeConnectionAsk)
	}
	code := CodeServiceIdle
	sev := SeverityInfo
	title := "Core idle"
	switch statusName {
	case "starting":
		code, title = CodeServiceStarting, "Core starting"
	case "started":
		code, title = CodeServiceStarted, "Core started"
	case "stopping":
		code, title = CodeServiceStopping, "Core stopping"
	case "fatal":
		code, sev, title = CodeServiceFatal, SeverityCritical, "Core fatal"
	case "idle":
		// defaults
	default:
		title = "Core " + statusName
	}
	attrs := map[string]string{"status": statusName}
	if errMsg != "" {
		attrs["error"] = errMsg
	}
	h.Emit(&Event{
		Scope:    ScopeService,
		Code:     code,
		Severity: sev,
		Title:    title,
		Message:  errMsg,
		Attrs:    attrs,
	})
}

// EmitClashMode dual-publishes mode changes (alongside SubscribeClashMode).
func (h *Hub) EmitClashMode(mode string) {
	if h == nil {
		return
	}
	h.Emit(&Event{
		Scope:    ScopeClashMode,
		Code:     CodeClashModeChanged,
		Severity: SeverityInfo,
		Title:    "Outbound mode changed",
		Message:  mode,
		Attrs:    map[string]string{"mode": mode},
	})
}

// EmitConnectionAsk notifies desktop to choose outbound for a held connection.
func EmitConnectionAsk(ctx context.Context, id string, attrs map[string]string) {
	if attrs == nil {
		attrs = map[string]string{}
	}
	attrs["id"] = id
	// snapshot key by pending id so multiple asks coexist
	if attrs["tag"] == "" {
		attrs["tag"] = id
	}
	proc := attrs["process_name"]
	if proc == "" {
		proc = attrs["process_path"]
	}
	dest := attrs["dest_host"]
	if dest == "" {
		dest = attrs["dest"]
	}
	Emit(ctx, &Event{
		Scope:    ScopeConnectionAsk,
		Code:     CodeConnectionAsk,
		Severity: SeverityWarning,
		Title:    "New connection",
		Message:  proc + " → " + dest,
		Attrs:    attrs,
	})
}
