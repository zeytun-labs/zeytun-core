package balancer

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/adapter/outbound"
	"github.com/sagernet/sing-box/common/interrupt"
	"github.com/sagernet/sing-box/common/urltest"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/batch"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/common/x/list"
	"github.com/sagernet/sing/service"
	"github.com/sagernet/sing/service/pause"
)

func RegisterLoadBalance(registry *outbound.Registry) {
	outbound.Register[option.BalancerOutboundOptions](registry, C.TypeBalancer, NewLoadBalance)
}

var (
	_ adapter.OutboundGroup           = (*Balancer)(nil)
	_ adapter.ConnectionHandler       = (*Balancer)(nil)
	_ adapter.PacketConnectionHandler = (*Balancer)(nil)
)

const (
	StrategyRoundRobin        = "round-robin"
	StrategyConsistentHashing = "consistent-hashing"
	StrategyStickySessions    = "sticky-sessions"
	StrategyFailover          = "failover"
	StrategyWeighted          = "weighted"
	StrategyLeastConnections  = "least-connections"

	defaultInterval    = 3 * time.Minute
	defaultIdleTimeout = 30 * time.Minute
)

type Balancer struct {
	outbound.Adapter
	ctx                          context.Context
	outbound                     adapter.OutboundManager
	connection                   adapter.ConnectionManager
	logger                       log.ContextLogger
	tags                         []string
	link                         string
	interval                     time.Duration
	idleTimeout                  time.Duration
	strategyFn                   Strategy
	options                      option.BalancerOutboundOptions
	interruptExternalConnections bool

	history        *urltest.HistoryStorage
	outbounds      []adapter.Outbound
	close          chan struct{}
	interruptGroup *interrupt.Group
	pause          pause.Manager
	pauseCallback  *list.Element[pause.Callback]
	ticker         *time.Ticker
	checking       atomic.Bool
	started        bool
	lastActive     common.TypedValue[time.Time]
	access         sync.Mutex
}

func NewLoadBalance(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.BalancerOutboundOptions) (adapter.Outbound, error) {
	b := &Balancer{
		Adapter:                      outbound.NewAdapter(C.TypeBalancer, tag, []string{N.NetworkTCP, N.NetworkUDP}, options.Outbounds),
		ctx:                          ctx,
		outbound:                     service.FromContext[adapter.OutboundManager](ctx),
		connection:                   service.FromContext[adapter.ConnectionManager](ctx),
		logger:                       logger,
		tags:                         options.Outbounds,
		link:                         "",
		interval:                     defaultInterval,
		idleTimeout:                  defaultIdleTimeout,
		interruptExternalConnections: options.InterruptExistConnections,
		options:                      options,
		interruptGroup:               interrupt.NewGroup(),
		close:                        make(chan struct{}),
		history:                      service.PtrFromContext[urltest.HistoryStorage](ctx),
		pause:                        service.FromContext[pause.Manager](ctx),
	}
	if len(b.tags) == 0 {
		return nil, E.New("missing tags")
	}
	if b.history == nil {
		return nil, E.New("missing URL test history storage")
	}
	if options.Strategy == "" {
		b.options.Strategy = StrategyRoundRobin
	}
	return b, nil
}

func (s *Balancer) Strategy() string {
	return s.options.Strategy
}

func (s *Balancer) Start() error {
	outbounds := make([]adapter.Outbound, 0, len(s.tags))
	for i, tag := range s.tags {
		detour, loaded := s.outbound.Outbound(tag)
		if !loaded {
			return E.New("outbound ", i, " not found: ", tag)
		}
		outbounds = append(outbounds, detour)
	}
	s.outbounds = outbounds
	switch s.options.Strategy {
	case StrategyRoundRobin:
		s.strategyFn = NewRoundRobin(outbounds, s.options)
	case StrategyConsistentHashing:
		s.strategyFn = NewConsistentHashing(outbounds, s.options)
	case StrategyStickySessions:
		s.strategyFn = NewStickySession(outbounds, s.options)
	case StrategyFailover:
		s.strategyFn = NewFailover(outbounds, s.options)
	case StrategyWeighted:
		s.strategyFn = NewWeighted(outbounds, s.options)
	case StrategyLeastConnections:
		s.strategyFn = NewLeastConnections(outbounds, s.options)
	default:
		return E.New("unknown load balance strategy: ", s.options.Strategy)
	}
	return nil
}

func (s *Balancer) PostStart() error {
	s.access.Lock()
	s.started = true
	s.lastActive.Store(time.Now())
	s.access.Unlock()
	go s.checkOutbounds(false)
	return nil
}

func (s *Balancer) Touch() {
	if !s.started {
		return
	}
	s.access.Lock()
	defer s.access.Unlock()
	if s.ticker != nil {
		s.lastActive.Store(time.Now())
		return
	}
	ticker := time.NewTicker(s.interval)
	s.ticker = ticker
	s.pauseCallback = pause.RegisterTicker(s.pause, ticker, s.interval, nil)
	go s.loopCheck(ticker)
}

func (s *Balancer) loopCheck(ticker *time.Ticker) {
	if time.Since(s.lastActive.Load()) > s.interval {
		s.lastActive.Store(time.Now())
		s.checkOutbounds(false)
	}
	for {
		select {
		case <-s.close:
			return
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			if time.Since(s.lastActive.Load()) > s.idleTimeout {
				s.access.Lock()
				s.ticker.Stop()
				s.ticker = nil
				s.pause.UnregisterCallback(s.pauseCallback)
				s.pauseCallback = nil
				s.access.Unlock()
				return
			}
			go s.checkOutbounds(false)
		}
	}
}

func (s *Balancer) checkOutbounds(force bool) {
	if s.checking.Swap(true) {
		return
	}
	defer s.checking.Store(false)

	b, _ := batch.New(s.ctx, batch.WithConcurrencyNum[any](10))
	checked := make(map[string]bool)
	for _, detour := range s.outbounds {
		realTag := RealTag(detour)
		if checked[realTag] {
			continue
		}
		checked[realTag] = true
		p, loaded := s.outbound.Outbound(realTag)
		if !loaded {
			continue
		}
		b.Go(realTag, func() (any, error) {
			ctx, cancel := context.WithTimeout(s.ctx, C.TCPTimeout)
			defer cancel()
			t, err := urltest.URLTest(ctx, s.link, p)
			if err != nil {
				s.logger.Debug("outbound ", realTag, " unavailable: ", err)
				s.history.DeleteURLTestHistory(realTag)
			} else {
				s.logger.Debug("outbound ", realTag, " available: ", t, "ms")
				s.history.StoreURLTestHistory(realTag, &adapter.URLTestHistory{
					Time:  time.Now(),
					Delay: t,
				})
			}
			return nil, nil
		})
	}
	b.Wait()

	histories := make(map[string]*adapter.URLTestHistory, len(s.outbounds))
	for _, detour := range s.outbounds {
		tag := detour.Tag()
		histories[tag] = s.history.LoadURLTestHistory(RealTag(detour))
	}
	if s.strategyFn.UpdateOutboundsInfo(histories) {
		s.interruptGroup.Interrupt(s.interruptExternalConnections)
	}
	_ = force
}

func (s *Balancer) Close() error {
	s.access.Lock()
	defer s.access.Unlock()
	select {
	case <-s.close:
	default:
		close(s.close)
	}
	if s.ticker != nil {
		s.ticker.Stop()
		s.ticker = nil
	}
	if s.pauseCallback != nil {
		s.pause.UnregisterCallback(s.pauseCallback)
		s.pauseCallback = nil
	}
	return nil
}

func (s *Balancer) Now() string {
	if s.strategyFn == nil {
		return ""
	}
	if now := s.strategyFn.Now(); now != "" {
		return now
	}
	if len(s.tags) > 0 {
		return s.tags[0]
	}
	return ""
}

func (s *Balancer) All() []string {
	return s.tags
}

func (s *Balancer) DialContext(ctx context.Context, network string, destination M.Socksaddr) (net.Conn, error) {
	s.Touch()
	metadata := adapter.ContextFrom(ctx)
	if metadata == nil {
		metadata = &adapter.InboundContext{}
	}
	selected := s.strategyFn.Select(*metadata, network, true)
	if selected == nil {
		return nil, E.New("missing supported outbound")
	}
	conn, err := selected.DialContext(ctx, network, destination)
	if err != nil {
		s.logger.ErrorContext(ctx, err)
		s.history.DeleteURLTestHistory(RealTag(selected))
		return nil, err
	}
	life, _ := s.strategyFn.(connectionLifecycle)
	conn = wrapConnLifecycle(conn, life, selected)
	return s.interruptGroup.NewConn(conn, interrupt.IsExternalConnectionFromContext(ctx)), nil
}

func (s *Balancer) ListenPacket(ctx context.Context, destination M.Socksaddr) (net.PacketConn, error) {
	s.Touch()
	metadata := adapter.ContextFrom(ctx)
	if metadata == nil {
		metadata = &adapter.InboundContext{}
	}
	selected := s.strategyFn.Select(*metadata, N.NetworkUDP, true)
	if selected == nil {
		return nil, E.New("missing supported outbound")
	}
	conn, err := selected.ListenPacket(ctx, destination)
	if err != nil {
		s.logger.ErrorContext(ctx, err)
		s.history.DeleteURLTestHistory(RealTag(selected))
		return nil, err
	}
	life, _ := s.strategyFn.(connectionLifecycle)
	conn = wrapPacketLifecycle(conn, life, selected)
	return s.interruptGroup.NewPacketConn(conn, interrupt.IsExternalConnectionFromContext(ctx)), nil
}

func (s *Balancer) NewConnection(ctx context.Context, conn net.Conn, metadata adapter.InboundContext, onClose N.CloseHandlerFunc) {
	s.Touch()
	ctx = interrupt.ContextWithIsExternalConnection(ctx)
	selected := s.strategyFn.Select(metadata, metadata.Network, true)
	if selected == nil {
		if onClose != nil {
			onClose(E.New("missing supported outbound"))
		}
		return
	}
	life, _ := s.strategyFn.(connectionLifecycle)
	onClose = wrapOnCloseLifecycle(onClose, life, selected)
	if outboundHandler, isHandler := selected.(adapter.ConnectionHandler); isHandler {
		outboundHandler.NewConnection(ctx, conn, metadata, onClose)
	} else {
		s.connection.NewConnection(ctx, selected, conn, metadata, onClose)
	}
}

func (s *Balancer) NewPacketConnection(ctx context.Context, conn N.PacketConn, metadata adapter.InboundContext, onClose N.CloseHandlerFunc) {
	s.Touch()
	ctx = interrupt.ContextWithIsExternalConnection(ctx)
	selected := s.strategyFn.Select(metadata, metadata.Network, true)
	if selected == nil {
		if onClose != nil {
			onClose(E.New("missing supported outbound"))
		}
		return
	}
	life, _ := s.strategyFn.(connectionLifecycle)
	onClose = wrapOnCloseLifecycle(onClose, life, selected)
	if outboundHandler, isHandler := selected.(adapter.PacketConnectionHandler); isHandler {
		outboundHandler.NewPacketConnection(ctx, conn, metadata, onClose)
	} else {
		s.connection.NewPacketConnection(ctx, selected, conn, metadata, onClose)
	}
}

func RealTag(detour adapter.Outbound) string {
	if group, isGroup := detour.(adapter.OutboundGroup); isGroup {
		return group.Now()
	}
	return detour.Tag()
}
