package coreevent

import (
	"context"
	"sync"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/sagernet/sing/common/observable"
	"github.com/sagernet/sing/service"
)

// Semantic notification bus for desktop clients (not traffic/log streams).

type Scope int32

const (
	ScopeUnspecified Scope = 0
	ScopeService     Scope = 1
	ScopeClashMode   Scope = 2
	ScopeRuleSet     Scope = 3
	ScopeSystem      Scope = 4
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
	key := e.Code
	if e.Scope == ScopeService {
		key = "SERVICE"
	} else if e.Scope == ScopeClashMode {
		key = "CLASH_MODE"
	} else if tag, ok := e.Attrs["tag"]; ok && tag != "" {
		key = e.Code + "|" + tag
	}
	h.access.Lock()
	h.last[key] = e
	h.ring = append(h.ring, e)
	if len(h.ring) > ringCap {
		h.ring = h.ring[len(h.ring)-ringCap:]
	}
	h.access.Unlock()
	h.subscriber.Emit(e)
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
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	Emit(ctx, &Event{
		Scope:    ScopeRuleSet,
		Code:     CodeRuleSetInitialFetchFailed,
		Severity: SeverityError,
		Title:    "Ruleset not downloaded",
		Message:  msg,
		Attrs: map[string]string{
			"tag": tag,
		},
	})
}

func EmitRuleSetReady(ctx context.Context, tag string) {
	Emit(ctx, &Event{
		Scope:    ScopeRuleSet,
		Code:     CodeRuleSetReady,
		Severity: SeverityInfo,
		Title:    "Ruleset ready",
		Message:  "",
		Attrs: map[string]string{
			"tag": tag,
		},
	})
}

func EmitRuleSetUpdateFailed(ctx context.Context, tag string, err error) {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	Emit(ctx, &Event{
		Scope:    ScopeRuleSet,
		Code:     CodeRuleSetUpdateFailed,
		Severity: SeverityWarning,
		Title:    "Ruleset update failed",
		Message:  msg,
		Attrs: map[string]string{
			"tag": tag,
		},
	})
}

func EmitRuleSetUpdated(ctx context.Context, tag string) {
	Emit(ctx, &Event{
		Scope:    ScopeRuleSet,
		Code:     CodeRuleSetUpdated,
		Severity: SeverityInfo,
		Title:    "Ruleset updated",
		Message:  "",
		Attrs: map[string]string{
			"tag": tag,
		},
	})
}

// EmitServiceStatus dual-publishes lifecycle (alongside SubscribeServiceStatus).
// statusName: idle|starting|started|stopping|fatal
func (h *Hub) EmitServiceStatus(statusName, errMsg string) {
	if h == nil {
		return
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
