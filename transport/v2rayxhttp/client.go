package xhttp

import (
	"context"
	gotls "crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sagernet/quic-go"
	"github.com/sagernet/quic-go/http3"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/common/tls"
	"github.com/sagernet/sing-box/common/xray/buf"
	xnet "github.com/sagernet/sing-box/common/xray/net"
	"github.com/sagernet/sing-box/common/xray/pipe"
	"github.com/sagernet/sing-box/common/xray/signal/done"
	"github.com/sagernet/sing-box/option"
	qtls "github.com/sagernet/sing-quic"
	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/bufio"
	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/logger"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	sHTTP "github.com/sagernet/sing/protocol/http"
	"github.com/sagernet/sing/service"
	"golang.org/x/net/http2"
)

type Client struct {
	ctx            context.Context
	options        *option.V2RayXHTTPOptions
	getRequestURL  func() url.URL
	getRequestURL2 func() url.URL
	getHTTPClient  func() (DialerClient, *XmuxClient)
	getHTTPClient2 func() (DialerClient, *XmuxClient)
}

func NewClient(ctx context.Context, dialer N.Dialer, serverAddr M.Socksaddr, options option.V2RayXHTTPOptions, tlsConfig tls.Config) (adapter.V2RayClientTransport, error) {
	mode := options.Mode
	if mode == "" || mode == "auto" {
		mode = "packet-up"
		options.Mode = mode
	}
	dest := serverAddr
	baseRequestURL, err := getBaseRequestURL(&options.V2RayXHTTPBaseOptions, dest, tlsConfig)
	if err != nil {
		return nil, err
	}
	getRequestURL := func() url.URL { return baseRequestURL }
	var xmuxOptions option.V2RayXHTTPXmuxOptions
	if options.Xmux != nil {
		xmuxOptions = *options.Xmux
	}
	xmuxManager := NewXmuxManager(xmuxOptions, func() XmuxConn {
		return createHTTPClient(dest, dialer, &options.V2RayXHTTPBaseOptions, tlsConfig)
	})
	getHTTPClient := func() (DialerClient, *XmuxClient) {
		xmuxClient := xmuxManager.GetXmuxClient(ctx)
		return xmuxClient.XmuxConn.(DialerClient), xmuxClient
	}
	getRequestURL2 := getRequestURL
	getHTTPClient2 := getHTTPClient
	if options.Download != nil {
		options2 := options.Download
		dialer2 := dialer
		if options2.Detour != "" {
			var ok bool
			dialer2, ok = service.FromContext[adapter.OutboundManager](ctx).Outbound(options2.Detour)
			if !ok {
				return nil, E.New("outbound detour not found: ", options2.Detour)
			}
		}
		dest2 := options2.ServerOptions.Build()
		var tlsConfig2 tls.Config
		if options2.TLS != nil {
			tlsConfig2, err = tls.NewClient(ctx, logger.NOP(), options2.Server, common.PtrValueOrDefault(options2.TLS))
			if err != nil {
				return nil, err
			}
		}
		baseRequestURL2, err := getBaseRequestURL(&options2.V2RayXHTTPBaseOptions, dest2, tlsConfig2)
		if err != nil {
			return nil, err
		}
		getRequestURL2 = func() url.URL { return baseRequestURL2 }
		var xmuxOptions2 option.V2RayXHTTPXmuxOptions
		if options2.Xmux != nil {
			xmuxOptions2 = *options2.Xmux
		}
		xmuxManager2 := NewXmuxManager(xmuxOptions2, func() XmuxConn {
			return createHTTPClient(dest2, dialer2, &options2.V2RayXHTTPBaseOptions, tlsConfig2)
		})
		getHTTPClient2 = func() (DialerClient, *XmuxClient) {
			xmuxClient2 := xmuxManager2.GetXmuxClient(ctx)
			return xmuxClient2.XmuxConn.(DialerClient), xmuxClient2
		}
	}
	return &Client{
		ctx:            ctx,
		options:        &options,
		getHTTPClient:  getHTTPClient,
		getHTTPClient2: getHTTPClient2,
		getRequestURL:  getRequestURL,
		getRequestURL2: getRequestURL2,
	}, nil
}

func (c *Client) DialContext(ctx context.Context) (net.Conn, error) {
	options := c.options
	mode := options.Mode
	if mode == "" || mode == "auto" {
		mode = "packet-up"
	}
	sessionId := ""
	if mode != "stream-one" {
		sessionId = GenerateSessionID(&options.V2RayXHTTPBaseOptions)
	}
	requestURL := c.getRequestURL()
	requestURL2 := c.getRequestURL2()
	httpClient, xmuxClient := c.getHTTPClient()
	httpClient2, xmuxClient2 := c.getHTTPClient2()
	if xmuxClient != nil {
		xmuxClient.AddRunning()
	}
	if xmuxClient2 != nil && xmuxClient2 != xmuxClient {
		xmuxClient2.AddRunning()
	}
	var closed atomic.Int32
	reader, writer := io.Pipe()
	conn := splitConn{
		writer: writer,
		onClose: func() {
			if closed.Add(1) > 1 {
				return
			}
			if xmuxClient != nil {
				xmuxClient.DoneRunning()
			}
			if xmuxClient2 != nil && xmuxClient2 != xmuxClient {
				xmuxClient2.DoneRunning()
			}
		},
	}
	var err error
	if mode == "stream-one" {
		if xmuxClient != nil {
			xmuxClient.LeftRequests.Add(-1)
		}
		conn.reader, conn.remoteAddr, conn.localAddr, err = httpClient.OpenStream(ctx, requestURL.String(), sessionId, reader, false)
		if err != nil {
			return nil, err
		}
		return &conn, nil
	}
	if xmuxClient2 != nil {
		xmuxClient2.LeftRequests.Add(-1)
	}
	conn.reader, conn.remoteAddr, conn.localAddr, err = httpClient2.OpenStream(ctx, requestURL2.String(), sessionId, nil, false)
	if err != nil {
		return nil, err
	}
	if mode == "stream-up" {
		if xmuxClient != nil {
			xmuxClient.LeftRequests.Add(-1)
		}
		_, _, _, err = httpClient.OpenStream(ctx, requestURL.String(), sessionId, reader, true)
		if err != nil {
			return nil, err
		}
		return &conn, nil
	}
	scMaxEachPostBytes := options.GetNormalizedScMaxEachPostBytes()
	scMinPostsIntervalMs := options.GetNormalizedScMinPostsIntervalMs()
	if scMaxEachPostBytes.From <= 0 {
		panic("`scMaxEachPostBytes` should be bigger than 0")
	}
	maxUploadSize := scMaxEachPostBytes.Rand()
	uploadPipeReader, uploadPipeWriter := pipe.New(pipe.WithSizeLimit(max(0, maxUploadSize-buf.Size)))
	conn.writer = uploadWriter{uploadPipeWriter, maxUploadSize}
	go func() {
		var seq int64
		var lastWrite time.Time
		dynamicHTTPClient := httpClient
		dynamicXmuxClient := xmuxClient
		for {
			remainder, err := uploadPipeReader.ReadMultiBuffer()
			if err != nil {
				break
			}
			doSplit := atomic.Bool{}
			for doSplit.Store(true); doSplit.Load(); {
				var chunk buf.MultiBuffer
				remainder, chunk = buf.SplitSize(remainder, maxUploadSize)
				if chunk.IsEmpty() {
					break
				}
				wroteRequest := done.New()
				ctx := httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
					WroteRequest: func(httptrace.WroteRequestInfo) { wroteRequest.Close() },
				})
				seqStr := strconv.FormatInt(seq, 10)
				seq += 1
				if scMinPostsIntervalMs.From > 0 {
					time.Sleep(time.Duration(scMinPostsIntervalMs.Rand())*time.Millisecond - time.Since(lastWrite))
				}
				lastWrite = time.Now()
				if dynamicXmuxClient != nil && (dynamicXmuxClient.LeftRequests.Add(-1) <= 0 ||
					(dynamicXmuxClient.UnreusableAt != time.Time{} && lastWrite.After(dynamicXmuxClient.UnreusableAt))) {
					dynamicHTTPClient, dynamicXmuxClient = c.getHTTPClient()
				}
				go func(hClient DialerClient, ch buf.MultiBuffer) {
					err := hClient.PostPacket(ctx, requestURL.String(), sessionId, seqStr, ch)
					wroteRequest.Close()
					if err != nil {
						uploadPipeReader.Interrupt()
						doSplit.Store(false)
					}
				}(dynamicHTTPClient, chunk)
				if _, ok := dynamicHTTPClient.(*DefaultDialerClient); ok {
					<-wroteRequest.Wait()
				}
			}
		}
	}()
	return &conn, nil
}

func (c *Client) Close() error { return nil }

func decideHTTPVersion(tlsConfig tls.Config) string {
	if tlsConfig == nil || len(tlsConfig.NextProtos()) == 0 || tlsConfig.NextProtos()[0] == "http/1.1" {
		return "1.1"
	}
	if tlsConfig.NextProtos()[0] == "h3" {
		return "3"
	}
	return "2"
}

func getBaseRequestURL(options *option.V2RayXHTTPBaseOptions, dest M.Socksaddr, tlsConfig tls.Config) (url.URL, error) {
	var requestURL url.URL
	if tlsConfig == nil {
		requestURL.Scheme = "http"
	} else {
		requestURL.Scheme = "https"
	}
	requestURL.Host = options.Host
	if requestURL.Host == "" && tlsConfig != nil {
		requestURL.Host = tlsConfig.ServerName()
	}
	if requestURL.Host == "" {
		requestURL.Host = dest.AddrString()
	}
	if err := sHTTP.URLSetPath(&requestURL, options.Path); err != nil {
		return requestURL, E.New(err, "parse path")
	}
	requestURL.Path = options.GetNormalizedPath()
	requestURL.RawQuery = options.GetNormalizedQuery()
	return requestURL, nil
}

func createHTTPClient(dest M.Socksaddr, dialer N.Dialer, options *option.V2RayXHTTPBaseOptions, tlsConfig tls.Config) DialerClient {
	httpVersion := decideHTTPVersion(tlsConfig)
	dialContext := func(ctxInner context.Context) (net.Conn, error) {
		conn, err := dialer.DialContext(ctxInner, "tcp", dest)
		if err != nil {
			return nil, err
		}
		if httpVersion != "3" && tlsConfig != nil {
			return tls.ClientHandshake(ctxInner, conn, tlsConfig)
		}
		return conn, nil
	}
	var keepAlivePeriod time.Duration
	if options.Xmux != nil {
		keepAlivePeriod = time.Duration(options.Xmux.HKeepAlivePeriod) * time.Second
	}
	var transport http.RoundTripper
	switch httpVersion {
	case "3":
		if keepAlivePeriod == 0 {
			keepAlivePeriod = xnet.QuicgoH3KeepAlivePeriod
		}
		if keepAlivePeriod < 0 {
			keepAlivePeriod = 0
		}
		quicConfig := &quic.Config{
			MaxIdleTimeout:     xnet.ConnIdleTimeout,
			MaxIncomingStreams: -1,
			KeepAlivePeriod:    keepAlivePeriod,
		}
		transport = &http3.Transport{
			QUICConfig: quicConfig,
			Dial: func(ctx context.Context, addr string, tlsCfg *gotls.Config, cfg *quic.Config) (*quic.Conn, error) {
				udpConn, dErr := dialer.DialContext(ctx, N.NetworkUDP, dest)
				if dErr != nil {
					return nil, dErr
				}
				return qtls.DialEarly(ctx, bufio.NewUnbindPacketConn(udpConn), udpConn.RemoteAddr(), tlsConfig, cfg)
			},
		}
	case "2":
		if keepAlivePeriod == 0 {
			keepAlivePeriod = xnet.ChromeH2KeepAlivePeriod
		}
		if keepAlivePeriod < 0 {
			keepAlivePeriod = 0
		}
		transport = &http2.Transport{
			DialTLSContext: func(ctxInner context.Context, network string, addr string, cfg *gotls.Config) (net.Conn, error) {
				return dialContext(ctxInner)
			},
			IdleConnTimeout: xnet.ConnIdleTimeout,
			ReadIdleTimeout: keepAlivePeriod,
		}
	default:
		httpDialContext := func(ctxInner context.Context, network string, addr string) (net.Conn, error) {
			return dialContext(ctxInner)
		}
		transport = &http.Transport{
			DialTLSContext:    httpDialContext,
			DialContext:       httpDialContext,
			IdleConnTimeout:   xnet.ConnIdleTimeout,
			DisableKeepAlives: true,
		}
	}
	return &DefaultDialerClient{
		options: options,
		client: &http.Client{
			Transport: transport,
		},
		httpVersion:    httpVersion,
		uploadRawPool:  &sync.Pool{},
		dialUploadConn: dialContext,
	}
}
