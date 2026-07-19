package xhttp

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sagernet/quic-go"
	"github.com/sagernet/quic-go/http3"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/common/tls"
	"github.com/sagernet/sing-box/common/xray/buf"
	"github.com/sagernet/sing-box/common/xray/signal/done"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	qtls "github.com/sagernet/sing-quic"
	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/logger"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	aTLS "github.com/sagernet/sing/common/tls"
	sHttp "github.com/sagernet/sing/protocol/http"
)

var _ adapter.V2RayServerTransport = (*Server)(nil)

type Server struct {
	ctx         context.Context
	logger      logger.ContextLogger
	tlsConfig   tls.ServerConfig
	quicConfig  *quic.Config
	handler     adapter.V2RayServerTransportHandler
	httpServer  *http.Server
	http3Server *http3.Server
	localAddr   net.Addr
	options     *option.V2RayXHTTPOptions
	host        string
	path        string
	sessionMu   sync.Mutex
	sessions    sync.Map
}

type httpSession struct {
	uploadQueue      *uploadQueue
	isFullyConnected *done.Instance
}

func NewServer(ctx context.Context, logger logger.ContextLogger, options option.V2RayXHTTPOptions, tlsConfig tls.ServerConfig, handler adapter.V2RayServerTransportHandler) (*Server, error) {
	server := &Server{
		ctx:       ctx,
		logger:    logger,
		tlsConfig: tlsConfig,
		handler:   handler,
		options:   &options,
		host:      options.Host,
		path:      options.GetNormalizedPath(),
	}
	if server.network() == N.NetworkTCP {
		protocols := new(http.Protocols)
		protocols.SetHTTP1(true)
		protocols.SetUnencryptedHTTP2(true)
		server.httpServer = &http.Server{
			Handler:           server,
			ReadHeaderTimeout: time.Second * 4,
			MaxHeaderBytes:    options.GetNormalizedServerMaxHeaderBytes(),
			Protocols:         protocols,
			BaseContext: func(net.Listener) context.Context {
				return ctx
			},
			ConnContext: func(ctx context.Context, c net.Conn) context.Context {
				return log.ContextWithNewID(ctx)
			},
		}
	} else {
		server.quicConfig = &quic.Config{
			DisablePathMTUDiscovery: !C.IsLinux && !C.IsWindows,
		}
		server.http3Server = &http3.Server{Handler: server}
	}
	return server, nil
}

func (s *Server) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if len(s.host) > 0 && !isValidHTTPHost(request.Host, s.host) {
		s.logger.ErrorContext(request.Context(), "failed to validate host, request:", request.Host, ", config:", s.host)
		writer.WriteHeader(http.StatusNotFound)
		return
	}
	if !strings.HasPrefix(request.URL.Path, s.path) {
		s.logger.ErrorContext(request.Context(), "failed to validate path, request:", request.URL.Path, ", config:", s.path)
		writer.WriteHeader(http.StatusNotFound)
		return
	}
	WriteResponseHeader(&s.options.V2RayXHTTPBaseOptions, writer, request.Method, request.Header)
	length := int(s.options.GetNormalizedXPaddingBytes().Rand())
	padCfg := XPaddingConfig{Length: length}
	if s.options.XPaddingObfsMode {
		padCfg.Placement = XPaddingPlacement{
			Placement: s.options.XPaddingPlacement,
			Key:       s.options.XPaddingKey,
			Header:    s.options.XPaddingHeader,
		}
		padCfg.Method = PaddingMethod(s.options.XPaddingMethod)
	} else {
		padCfg.Placement = XPaddingPlacement{Placement: PlacementHeader, Header: "X-Padding"}
	}
	ApplyXPaddingToResponse(writer, padCfg)
	if request.Method == "OPTIONS" {
		writer.WriteHeader(http.StatusOK)
		return
	}
	validRange := s.options.GetNormalizedXPaddingBytes()
	paddingValue, _ := ExtractXPaddingFromRequest(&s.options.V2RayXHTTPBaseOptions, request, s.options.XPaddingObfsMode)
	if !IsPaddingValid(&s.options.V2RayXHTTPBaseOptions, paddingValue, validRange.From, validRange.To, PaddingMethod(s.options.XPaddingMethod)) {
		s.logger.ErrorContext(request.Context(), "invalid padding length:", int32(len(paddingValue)))
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	obfsPaddingAccepted := s.options.XPaddingObfsMode && paddingValue != ""
	sessionId, seqStr := ExtractMetaFromRequest(&s.options.V2RayXHTTPBaseOptions, request, s.path)
	if sessionId == "" && s.options.Mode != "" && s.options.Mode != "auto" && s.options.Mode != "stream-one" && s.options.Mode != "stream-up" {
		s.logger.ErrorContext(request.Context(), "stream-one mode is not allowed")
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	forwardedAddrs := parseXForwardedFor(request.Header)
	var remoteAddr net.Addr
	var err error
	remoteAddr, err = net.ResolveTCPAddr("tcp", request.RemoteAddr)
	if err != nil {
		remoteAddr = &net.TCPAddr{IP: []byte{0, 0, 0, 0}, Port: 0}
	}
	if request.ProtoMajor == 3 {
		remoteAddr = &net.UDPAddr{IP: remoteAddr.(*net.TCPAddr).IP, Port: remoteAddr.(*net.TCPAddr).Port}
	}
	if len(forwardedAddrs) > 0 && forwardedAddrs[0].Family().IsIP() {
		remoteAddr = &net.TCPAddr{IP: forwardedAddrs[0].IP(), Port: 0}
	}
	var currentSession *httpSession
	if sessionId != "" {
		currentSession = s.upsertSession(sessionId)
	}
	scMaxEachPostBytes := int(s.options.GetNormalizedScMaxEachPostBytes().To)
	isUplinkRequest := false
	switch request.Method {
	case "GET":
		isUplinkRequest = seqStr != ""
	default:
		isUplinkRequest = true
	}
	uplinkDataKey := s.options.UplinkDataKey
	if uplinkDataKey == "" {
		uplinkDataKey = "x_uplink"
	}
	if isUplinkRequest && sessionId != "" {
		if seqStr == "" {
			if s.options.Mode != "" && s.options.Mode != "auto" && s.options.Mode != "stream-up" {
				s.logger.ErrorContext(request.Context(), "stream-up mode is not allowed")
				writer.WriteHeader(http.StatusBadRequest)
				return
			}
			httpSC := &httpServerConn{Instance: done.New(), Reader: request.Body, ResponseWriter: writer}
			err = currentSession.uploadQueue.Push(Packet{Reader: httpSC})
			if err != nil {
				s.logger.InfoContext(request.Context(), err, "failed to upload (PushReader)")
				writer.WriteHeader(http.StatusConflict)
			} else {
				writer.Header().Set("X-Accel-Buffering", "no")
				writer.Header().Set("Cache-Control", "no-store")
				writer.WriteHeader(http.StatusOK)
				scStreamUpServerSecs := s.options.GetNormalizedScStreamUpServerSecs()
				hasLegacyRefererCompatMarker := request.Header.Get("Referer") != ""
				if (hasLegacyRefererCompatMarker || obfsPaddingAccepted) && scStreamUpServerSecs.To > 0 {
					go func() {
						for {
							_, err := httpSC.Write(bytes.Repeat([]byte{'X'}, int(s.options.GetNormalizedXPaddingBytes().Rand())))
							if err != nil {
								break
							}
							time.Sleep(time.Duration(scStreamUpServerSecs.Rand()) * time.Second)
						}
					}()
				}
				select {
				case <-request.Context().Done():
				case <-httpSC.Wait():
				}
			}
			httpSC.Close()
			return
		}
		if s.options.Mode != "" && s.options.Mode != "auto" && s.options.Mode != "packet-up" {
			s.logger.ErrorContext(request.Context(), "packet-up mode is not allowed")
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		dataPlacement := s.options.GetNormalizedUplinkDataPlacement()
		var headerPayload []byte
		if dataPlacement == PlacementAuto || dataPlacement == PlacementHeader {
			var headerPayloadChunks []string
			for i := 0; true; i++ {
				chunk := request.Header.Get(fmt.Sprintf("%s-%d", uplinkDataKey, i))
				if chunk == "" {
					break
				}
				headerPayloadChunks = append(headerPayloadChunks, chunk)
			}
			headerPayloadEncoded := strings.Join(headerPayloadChunks, "")
			headerPayload, err = base64.RawURLEncoding.DecodeString(headerPayloadEncoded)
			if err != nil {
				s.logger.InfoContext(request.Context(), "Invalid base64 in header's payload: ", err.Error())
				writer.WriteHeader(http.StatusBadRequest)
				return
			}
		}
		var cookiePayload []byte
		if dataPlacement == PlacementAuto || dataPlacement == PlacementCookie {
			var cookiePayloadChunks []string
			for i := 0; true; i++ {
				cookieName := fmt.Sprintf("%s_%d", uplinkDataKey, i)
				if c, _ := request.Cookie(cookieName); c != nil {
					cookiePayloadChunks = append(cookiePayloadChunks, c.Value)
				} else {
					break
				}
			}
			cookiePayloadEncoded := strings.Join(cookiePayloadChunks, "")
			cookiePayload, err = base64.RawURLEncoding.DecodeString(cookiePayloadEncoded)
			if err != nil {
				s.logger.InfoContext(request.Context(), "Invalid base64 in cookies' payload: ", err.Error())
				writer.WriteHeader(http.StatusBadRequest)
				return
			}
		}
		var bodyPayload []byte
		if dataPlacement == PlacementAuto || dataPlacement == PlacementBody {
			if request.ContentLength > int64(scMaxEachPostBytes) {
				s.logger.ErrorContext(request.Context(), "Too large upload. scMaxEachPostBytes is set to ", scMaxEachPostBytes)
				writer.WriteHeader(http.StatusRequestEntityTooLarge)
				return
			}
			var readErr error
			if request.ContentLength > 0 {
				bodyPayload = make([]byte, request.ContentLength)
				_, readErr = io.ReadFull(request.Body, bodyPayload)
			} else {
				bodyPayload, readErr = buf.ReadAllToBytes(io.LimitReader(request.Body, int64(scMaxEachPostBytes)+1))
			}
			if readErr != nil {
				s.logger.InfoContext(request.Context(), readErr, "failed to read body payload")
				writer.WriteHeader(http.StatusBadRequest)
				return
			}
		}
		var payload []byte
		switch dataPlacement {
		case PlacementHeader:
			payload = headerPayload
		case PlacementCookie:
			payload = cookiePayload
		case PlacementBody:
			payload = bodyPayload
		case PlacementAuto:
			payload = slices.Concat(headerPayload, cookiePayload, bodyPayload)
		}
		if len(payload) > scMaxEachPostBytes {
			s.logger.ErrorContext(request.Context(), "Too large upload. scMaxEachPostBytes is set to ", scMaxEachPostBytes)
			writer.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		seq, err := strconv.ParseUint(seqStr, 10, 64)
		if err != nil {
			s.logger.InfoContext(request.Context(), err, "failed to upload (ParseUint)")
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
		err = currentSession.uploadQueue.Push(Packet{Payload: payload, Seq: seq})
		if err != nil {
			s.logger.InfoContext(request.Context(), err, "failed to upload (PushPayload)")
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
		if len(bodyPayload) == 0 {
			writer.Header().Set("Cache-Control", "no-store")
		}
		writer.WriteHeader(http.StatusOK)
	} else if request.Method == "GET" || sessionId == "" {
		if sessionId != "" {
			currentSession.isFullyConnected.Close()
			defer s.sessions.Delete(sessionId)
		}
		writer.Header().Set("X-Accel-Buffering", "no")
		writer.Header().Set("Cache-Control", "no-store")
		if !s.options.NoSSEHeader {
			writer.Header().Set("Content-Type", "text/event-stream")
		}
		writer.WriteHeader(http.StatusOK)
		writer.(http.Flusher).Flush()
		httpSC := &httpServerConn{Instance: done.New(), Reader: request.Body, ResponseWriter: writer}
		conn := splitConn{writer: httpSC, reader: httpSC, remoteAddr: remoteAddr, localAddr: s.localAddr}
		if sessionId != "" {
			conn.reader = currentSession.uploadQueue
		}
		s.handler.NewConnectionEx(request.Context(), &conn, sHttp.SourceAddress(request), M.Socksaddr{}, func(it error) {})
		select {
		case <-request.Context().Done():
		case <-httpSC.Wait():
		}
		conn.Close()
	} else {
		s.logger.ErrorContext(request.Context(), "unsupported method: ", request.Method)
		writer.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) Network() []string { return []string{s.network()} }

func (s *Server) Serve(listener net.Listener) error {
	if s.network() == N.NetworkTCP {
		if s.tlsConfig != nil {
			listener = aTLS.NewListener(listener, s.tlsConfig)
		}
		s.localAddr = listener.Addr()
		return s.httpServer.Serve(listener)
	}
	return os.ErrInvalid
}

func (s *Server) ServePacket(listener net.PacketConn) error {
	if s.network() == N.NetworkUDP {
		quicListener, err := qtls.ListenEarly(listener, s.tlsConfig, s.quicConfig)
		if err != nil {
			return err
		}
		s.localAddr = quicListener.Addr()
		return s.http3Server.ServeListener(quicListener)
	}
	return os.ErrInvalid
}

func (s *Server) Close() error {
	if s.network() == N.NetworkTCP {
		return common.Close(s.httpServer)
	}
	return common.Close(s.http3Server)
}

func (s *Server) network() string {
	if s.tlsConfig != nil && len(s.tlsConfig.NextProtos()) == 1 && s.tlsConfig.NextProtos()[0] == "h3" {
		return N.NetworkUDP
	}
	return N.NetworkTCP
}

func (s *Server) upsertSession(sessionId string) *httpSession {
	if currentSessionAny, ok := s.sessions.Load(sessionId); ok {
		return currentSessionAny.(*httpSession)
	}
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	if currentSessionAny, ok := s.sessions.Load(sessionId); ok {
		return currentSessionAny.(*httpSession)
	}
	session := &httpSession{
		uploadQueue:      NewUploadQueue(s.options.GetNormalizedScMaxBufferedPosts()),
		isFullyConnected: done.New(),
	}
	s.sessions.Store(sessionId, session)
	shouldReap := done.New()
	go func() {
		time.Sleep(30 * time.Second)
		shouldReap.Close()
	}()
	go func() {
		select {
		case <-shouldReap.Wait():
			s.sessions.Delete(sessionId)
			session.uploadQueue.Close()
		case <-session.isFullyConnected.Wait():
		case <-s.ctx.Done():
		}
	}()
	return session
}

func parseXForwardedFor(header http.Header) []netxAddress {
	xff := header.Get("X-Forwarded-For")
	if xff == "" {
		return nil
	}
	list := strings.Split(xff, ",")
	addrs := make([]netxAddress, 0, len(list))
	for _, proxy := range list {
		addrs = append(addrs, parseAddress(proxy))
	}
	return addrs
}

type netxAddress interface {
	IP() []byte
	Family() addressFamily
}

type addressFamily byte

func (af addressFamily) IsIP() bool { return af == 0 || af == 1 }

func parseAddress(addr string) netxAddress {
	addr = strings.TrimSpace(addr)
	ip := net.ParseIP(addr)
	if ip == nil {
		return domainAddr(addr)
	}
	if v4 := ip.To4(); v4 != nil {
		return ipAddr{v4, 0}
	}
	return ipAddr{ip, 1}
}

type ipAddr struct {
	ip []byte
	f  addressFamily
}

func (a ipAddr) IP() []byte           { return a.ip }
func (a ipAddr) Family() addressFamily { return a.f }

type domainAddr string

func (domainAddr) IP() []byte           { return nil }
func (domainAddr) Family() addressFamily { return 2 }

func isValidHTTPHost(request string, config string) bool {
	r := strings.ToLower(request)
	c := strings.ToLower(config)
	if strings.Contains(r, ":") {
		h, _, _ := net.SplitHostPort(r)
		return h == c
	}
	return r == c
}
