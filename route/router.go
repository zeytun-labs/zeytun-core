package route

import (
	"context"
	sjson "github.com/sagernet/sing/common/json"
	"os"
	"runtime"
	"time"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/common/process"
	"github.com/sagernet/sing-box/common/taskmonitor"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	R "github.com/sagernet/sing-box/route/rule"
	"github.com/sagernet/sing/common"
	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/task"
	"github.com/sagernet/sing/contrab/freelru"
	"github.com/sagernet/sing/contrab/maphash"
	"github.com/sagernet/sing/service"
	"github.com/sagernet/sing/service/pause"
)

var _ adapter.Router = (*Router)(nil)

type Router struct {
	ctx               context.Context
	logger            log.ContextLogger
	inbound           adapter.InboundManager
	outbound          adapter.OutboundManager
	dns               adapter.DNSRouter
	dnsTransport      adapter.DNSTransportManager
	connection        adapter.ConnectionManager
	network           adapter.NetworkManager
	httpClientManager adapter.HTTPClientManager
	rules             []adapter.Rule
	needFindProcess   bool
	needFindNeighbor  bool
	leaseFiles        []string
	ruleSets          []adapter.RuleSet
	ruleSetMap        map[string]adapter.RuleSet
	ruleSetUpdater    *R.RuleSetUpdater
	processSearcher   process.Searcher
	processCache      *freelru.Cache[processCacheKey, processCacheEntry]
	neighborResolver  adapter.NeighborResolver
	pauseManager      pause.Manager
	trackers          []adapter.ConnectionTracker
	platformInterface adapter.PlatformInterface
	started           bool
	connectionAsk     *ConnectionAsk
	liveRules         *LiveRuleStore
	liveSeed          *option.LiveRulesOptions
}

func NewRouter(ctx context.Context, logFactory log.Factory, options option.RouteOptions, dnsOptions option.DNSOptions) *Router {
	logger := logFactory.NewLogger("router")
	// connection_ask needs process path for grouping
	needFindProcess := hasRule(options.Rules, isProcessRule) || hasDNSRule(dnsOptions.Rules, isProcessDNSRule) || options.FindProcess
	if options.ConnectionAsk != nil && options.ConnectionAsk.Enabled {
		needFindProcess = true
	}
	if options.LiveRules != nil {
		needFindProcess = needFindProcess || hasOptionLiveProcess(options.LiveRules.Temp) || hasOptionLiveProcess(options.LiveRules.Permanent)
	}
	return &Router{
		ctx:               ctx,
		logger:            logger,
		inbound:           service.FromContext[adapter.InboundManager](ctx),
		outbound:          service.FromContext[adapter.OutboundManager](ctx),
		dns:               service.FromContext[adapter.DNSRouter](ctx),
		dnsTransport:      service.FromContext[adapter.DNSTransportManager](ctx),
		connection:        service.FromContext[adapter.ConnectionManager](ctx),
		network:           service.FromContext[adapter.NetworkManager](ctx),
		httpClientManager: service.FromContext[adapter.HTTPClientManager](ctx),
		rules:             make([]adapter.Rule, 0, len(options.Rules)),
		ruleSetMap:        make(map[string]adapter.RuleSet),
		needFindProcess:   needFindProcess,
		needFindNeighbor:  hasRule(options.Rules, isNeighborRule) || hasDNSRule(dnsOptions.Rules, isNeighborDNSRule) || hasLocalNeighborDNSServer(dnsOptions.Servers) || options.FindNeighbor,
		leaseFiles:        options.DHCPLeaseFiles,
		pauseManager:      service.FromContext[pause.Manager](ctx),
		platformInterface: service.FromContext[adapter.PlatformInterface](ctx),
		connectionAsk:     newConnectionAsk(ctx, logger, options.ConnectionAsk),
		liveRules:         newLiveRuleStore(ctx, logger),
		liveSeed:          options.LiveRules,
	}
}

func hasOptionLiveProcess(items []option.LiveRule) bool {
	for _, item := range items {
		if item.Rule.Type == "" || item.Rule.Type == C.RuleTypeDefault {
			if isProcessRule(item.Rule.DefaultOptions) {
				return true
			}
		}
	}
	return false
}

// DecideConnectionAsk resolves a held unmatched connection (Clash API / gRPC).
func (r *Router) DecideConnectionAsk(id, outbound string, reject bool) error {
	if r.connectionAsk == nil || !r.connectionAsk.Enabled() {
		return E.New("connection ask disabled")
	}
	return r.connectionAsk.Decide(id, outbound, reject)
}

// ForgetAskSessionKeys removes cached ask decisions for the given group keys.
func (r *Router) ForgetAskSessionKeys(keys []string) {
	if r.connectionAsk != nil && r.connectionAsk.Enabled() {
		r.connectionAsk.ForgetKeys(keys)
	}
}

func (r *Router) markProcessFromSpecs(items []LiveRuleSpec) {
	for _, item := range items {
		if item.Rule.Type == "" || item.Rule.Type == C.RuleTypeDefault {
			if isProcessRule(item.Rule.DefaultOptions) {
				r.needFindProcess = true
				return
			}
		}
	}
}

func optionLiveToSpecs(items []option.LiveRule) []LiveRuleSpec {
	out := make([]LiveRuleSpec, 0, len(items))
	for _, it := range items {
		out = append(out, LiveRuleSpec{ID: it.ID, ExpiresAt: it.ExpiresAt, Rule: it.Rule})
	}
	return out
}

// ReplaceLiveRules swaps temp+permanent user overlays (no config reload).
func (r *Router) ReplaceLiveRules(payload LiveRulesPayload) error {
	if r.liveRules == nil {
		return E.New("live rules unavailable")
	}
	r.markProcessFromSpecs(payload.Temp)
	r.markProcessFromSpecs(payload.Permanent)
	return r.liveRules.Replace(payload)
}

// ReplaceTempRules swaps only the temp segment.
func (r *Router) ReplaceTempRules(items []TempRuleSpec) error {
	if r.liveRules == nil {
		return E.New("live rules unavailable")
	}
	r.markProcessFromSpecs(items)
	return r.liveRules.ReplaceTemp(items)
}

// ReplacePermanentRules swaps only the permanent user segment.
func (r *Router) ReplacePermanentRules(items []LiveRuleSpec) error {
	if r.liveRules == nil {
		return E.New("live rules unavailable")
	}
	r.markProcessFromSpecs(items)
	return r.liveRules.ReplacePermanent(items)
}

// ReplaceTempRulesJSON implements adapter.Router (JSON array of temp specs).
// Must use sing json + context so option.Rule UnmarshalJSONContext runs
// (stdlib encoding/json silently leaves rules empty → "missing conditions").
func (r *Router) ReplaceTempRulesJSON(payload []byte) error {
	var items []TempRuleSpec
	if len(payload) == 0 || string(payload) == "null" {
		return r.ReplaceTempRules(nil)
	}
	if err := sjson.UnmarshalContext(r.ctx, payload, &items); err != nil {
		return err
	}
	return r.ReplaceTempRules(items)
}

// ReplacePermanentRulesJSON implements adapter.Router (JSON array).
func (r *Router) ReplacePermanentRulesJSON(payload []byte) error {
	var items []LiveRuleSpec
	if len(payload) == 0 || string(payload) == "null" {
		return r.ReplacePermanentRules(nil)
	}
	if err := sjson.UnmarshalContext(r.ctx, payload, &items); err != nil {
		return err
	}
	return r.ReplacePermanentRules(items)
}

// ReplaceLiveRulesJSON full {temp,permanent}.
func (r *Router) ReplaceLiveRulesJSON(payload []byte) error {
	if len(payload) == 0 || string(payload) == "null" {
		return r.ReplaceLiveRules(LiveRulesPayload{})
	}
	var body LiveRulesPayload
	if err := sjson.UnmarshalContext(r.ctx, payload, &body); err != nil {
		return err
	}
	return r.ReplaceLiveRules(body)
}

// seedLiveRulesFromConfig loads route.live_rules into the store (cold start / SIGHUP).
func (r *Router) seedLiveRulesFromConfig() error {
	if r.liveRules == nil || r.liveSeed == nil {
		return nil
	}
	return r.ReplaceLiveRules(LiveRulesPayload{
		Temp:      optionLiveToSpecs(r.liveSeed.Temp),
		Permanent: optionLiveToSpecs(r.liveSeed.Permanent),
	})
}

func (r *Router) Initialize(rules []option.Rule, ruleSets []option.RuleSet) error {
	for i, options := range rules {
		err := R.ValidateNoNestedRuleActions(options)
		if err != nil {
			return E.Cause(err, "parse rule[", i, "]")
		}
		rule, err := R.NewRule(r.ctx, r.logger, options, false)
		if err != nil {
			return E.Cause(err, "parse rule[", i, "]")
		}
		r.rules = append(r.rules, rule)
	}
	for i, options := range ruleSets {
		for _, tag := range options.Tag {
			if _, exists := r.ruleSetMap[tag]; exists {
				return E.New("duplicate rule-set tag: ", tag)
			}
			ruleSet, err := R.NewRuleSet(r.ctx, r.logger, tag, options)
			if err != nil {
				return E.Cause(err, "parse rule-set[", i, "]")
			}
			r.ruleSets = append(r.ruleSets, ruleSet)
			r.ruleSetMap[tag] = ruleSet
		}
	}
	if err := r.seedLiveRulesFromConfig(); err != nil {
		return err
	}
	return nil
}

func (r *Router) Start(stage adapter.StartStage) error {
	monitor := taskmonitor.New(r.logger, C.StartTimeout)
	switch stage {
	case adapter.StartStateInitialize:
		if r.needFindNeighbor {
			if r.platformInterface != nil && r.platformInterface.UsePlatformNeighborResolver() {
				monitor.Start("initialize neighbor resolver")
				resolver := newPlatformNeighborResolver(r.logger, r.platformInterface)
				err := resolver.Start()
				monitor.Finish()
				if err != nil {
					r.logger.Error(E.Cause(err, "start neighbor resolver"))
				} else {
					r.neighborResolver = resolver
				}
			} else {
				monitor.Start("initialize neighbor resolver")
				resolver, err := newNeighborResolver(r.logger, r.leaseFiles)
				monitor.Finish()
				if err != nil {
					if err != os.ErrInvalid {
						r.logger.Error(E.Cause(err, "create neighbor resolver"))
					}
				} else {
					err = resolver.Start()
					if err != nil {
						r.logger.Error(E.Cause(err, "start neighbor resolver"))
					} else {
						r.neighborResolver = resolver
					}
				}
			}
		}
	case adapter.StartStateStart:
		var startContext *adapter.HTTPStartContext
		if len(r.ruleSets) > 0 {
			monitor.Start("initialize rule-set")
			startContext = adapter.NewHTTPStartContext()
			var ruleSetStartGroup task.Group
			for i, ruleSet := range r.ruleSets {
				ruleSetInPlace := ruleSet
				ruleSetStartGroup.Append0(func(ctx context.Context) error {
					err := ruleSetInPlace.StartContext(ctx, startContext)
					if err != nil {
						return E.Cause(err, "initialize rule-set[", i, "]")
					}
					return nil
				})
			}
			ruleSetStartGroup.Concurrency(5)
			ruleSetStartGroup.FastFail()
			err := ruleSetStartGroup.Run(r.ctx)
			monitor.Finish()
			if err != nil {
				return err
			}
		}
		if startContext != nil {
			startContext.Close()
		}
		r.ruleSetUpdater = R.NewRuleSetUpdater(r.ctx, r.ruleSets)
		r.network.Initialize(r.ruleSets)
		needFindProcess := r.needFindProcess
		for _, ruleSet := range r.ruleSets {
			metadata := ruleSet.Metadata()
			if metadata.ContainsProcessRule {
				needFindProcess = true
			}
		}
		if C.IsAndroid && r.platformInterface != nil {
			needFindProcess = true
		}
		r.needFindProcess = needFindProcess
		if needFindProcess {
			if r.platformInterface != nil && r.platformInterface.UsePlatformConnectionOwnerFinder() {
				r.processSearcher = newPlatformSearcher(r.platformInterface)
			} else {
				monitor.Start("initialize process searcher")
				searcher, err := process.NewSearcher(process.Config{
					Logger:         r.logger,
					PackageManager: r.network.PackageManager(),
				})
				monitor.Finish()
				if err != nil {
					if err != os.ErrInvalid {
						r.logger.Warn(E.Cause(err, "create process searcher"))
					}
				} else {
					r.processSearcher = searcher
				}
			}
		}
		if r.processSearcher != nil {
			processCache := common.Must1(freelru.New[processCacheKey, processCacheEntry](256, maphash.NewHasher[processCacheKey]().Hash32, true))
			processCache.SetLifetime(200 * time.Millisecond)
			r.processCache = processCache
		}
	case adapter.StartStatePostStart:
		for i, rule := range r.rules {
			monitor.Start("initialize rule[", i, "]")
			err := rule.Start()
			monitor.Finish()
			if err != nil {
				return E.Cause(err, "initialize rule[", i, "]")
			}
		}
		if r.ruleSetUpdater != nil {
			r.ruleSetUpdater.Start()
		}
		r.started = true
		return nil
	case adapter.StartStateStarted:
		for _, ruleSet := range r.ruleSets {
			ruleSet.Cleanup()
		}
		runtime.GC()
	}
	return nil
}

func (r *Router) Close() error {
	monitor := taskmonitor.New(r.logger, C.StopTimeout)
	var err error
	if r.neighborResolver != nil {
		monitor.Start("close neighbor resolver")
		err = E.Append(err, r.neighborResolver.Close(), func(closeErr error) error {
			return E.Cause(closeErr, "close neighbor resolver")
		})
		monitor.Finish()
	}
	for i, rule := range r.rules {
		monitor.Start("close rule[", i, "]")
		err = E.Append(err, rule.Close(), func(err error) error {
			return E.Cause(err, "close rule[", i, "]")
		})
		monitor.Finish()
	}
	if r.ruleSetUpdater != nil {
		monitor.Start("close rule-set updater")
		err = E.Append(err, r.ruleSetUpdater.Close(), func(err error) error {
			return E.Cause(err, "close rule-set updater")
		})
		monitor.Finish()
	}
	for i, ruleSet := range r.ruleSets {
		monitor.Start("close rule-set[", i, "]")
		err = E.Append(err, ruleSet.Close(), func(err error) error {
			return E.Cause(err, "close rule-set[", i, "]")
		})
		monitor.Finish()
	}
	if r.processSearcher != nil {
		monitor.Start("close process searcher")
		err = E.Append(err, r.processSearcher.Close(), func(err error) error {
			return E.Cause(err, "close process searcher")
		})
		monitor.Finish()
	}
	return err
}

func (r *Router) RuleSet(tag string) (adapter.RuleSet, bool) {
	ruleSet, loaded := r.ruleSetMap[tag]
	return ruleSet, loaded
}

func (r *Router) Rules() []adapter.Rule {
	return r.rules
}

func (r *Router) AppendTracker(tracker adapter.ConnectionTracker) {
	r.trackers = append(r.trackers, tracker)
}

func (r *Router) NeedFindProcess() bool {
	return r.needFindProcess
}

func (r *Router) NeedFindNeighbor() bool {
	return r.needFindNeighbor
}

func (r *Router) NeighborResolver() adapter.NeighborResolver {
	return r.neighborResolver
}

func (r *Router) ResetNetwork() {
	r.httpClientManager.ResetNetwork()
	r.dns.ResetNetwork()
}
