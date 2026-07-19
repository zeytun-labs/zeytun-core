package option

import (
	"net/http"
	"strings"

	Xbadoption "github.com/sagernet/sing-box/common/xray/json/badoption"
	C "github.com/sagernet/sing-box/constant"
	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/json"
	"github.com/sagernet/sing/common/json/badjson"
	"github.com/sagernet/sing/common/json/badoption"
)

type _V2RayTransportOptions struct {
	Type               string                  `json:"type"`
	HTTPOptions        V2RayHTTPOptions        `json:"-"`
	WebsocketOptions   V2RayWebsocketOptions   `json:"-"`
	QUICOptions        V2RayQUICOptions        `json:"-"`
	GRPCOptions        V2RayGRPCOptions        `json:"-"`
	HTTPUpgradeOptions V2RayHTTPUpgradeOptions `json:"-"`
	XHTTPOptions       V2RayXHTTPOptions       `json:"-"`
}

type V2RayTransportOptions _V2RayTransportOptions

func (o V2RayTransportOptions) MarshalJSON() ([]byte, error) {
	var v any
	switch o.Type {
	case C.V2RayTransportTypeHTTP:
		v = o.HTTPOptions
	case C.V2RayTransportTypeWebsocket:
		v = o.WebsocketOptions
	case C.V2RayTransportTypeQUIC:
		v = o.QUICOptions
	case C.V2RayTransportTypeGRPC:
		v = o.GRPCOptions
	case C.V2RayTransportTypeHTTPUpgrade:
		v = o.HTTPUpgradeOptions
	case C.V2RayTransportTypeXHTTP:
		v = o.XHTTPOptions
	case "":
		return nil, E.New("missing transport type")
	default:
		return nil, E.New("unknown transport type: " + o.Type)
	}
	return badjson.MarshallObjects((_V2RayTransportOptions)(o), v)
}

func (o *V2RayTransportOptions) UnmarshalJSON(bytes []byte) error {
	err := json.Unmarshal(bytes, (*_V2RayTransportOptions)(o))
	if err != nil {
		return err
	}
	var v any
	switch o.Type {
	case C.V2RayTransportTypeHTTP:
		v = &o.HTTPOptions
	case C.V2RayTransportTypeWebsocket:
		v = &o.WebsocketOptions
	case C.V2RayTransportTypeQUIC:
		v = &o.QUICOptions
	case C.V2RayTransportTypeGRPC:
		v = &o.GRPCOptions
	case C.V2RayTransportTypeHTTPUpgrade:
		v = &o.HTTPUpgradeOptions
	case C.V2RayTransportTypeXHTTP:
		v = &o.XHTTPOptions
	default:
		return E.New("unknown transport type: " + o.Type)
	}
	err = badjson.UnmarshallExcluded(bytes, (*_V2RayTransportOptions)(o), v)
	if err != nil {
		return err
	}
	return nil
}

type V2RayHTTPOptions struct {
	Host        badoption.Listable[string] `json:"host,omitempty"`
	Path        string                     `json:"path,omitempty"`
	Method      string                     `json:"method,omitempty"`
	Headers     badoption.HTTPHeader       `json:"headers,omitempty"`
	IdleTimeout badoption.Duration         `json:"idle_timeout,omitempty"`
	PingTimeout badoption.Duration         `json:"ping_timeout,omitempty"`
}

type V2RayWebsocketOptions struct {
	Path                string               `json:"path,omitempty"`
	Headers             badoption.HTTPHeader `json:"headers,omitempty"`
	MaxEarlyData        uint32               `json:"max_early_data,omitempty"`
	EarlyDataHeaderName string               `json:"early_data_header_name,omitempty"`
}

type V2RayQUICOptions struct{}

type V2RayGRPCOptions struct {
	ServiceName         string             `json:"service_name,omitempty"`
	IdleTimeout         badoption.Duration `json:"idle_timeout,omitempty"`
	PingTimeout         badoption.Duration `json:"ping_timeout,omitempty"`
	PermitWithoutStream bool               `json:"permit_without_stream,omitempty"`
	ForceLite           bool               `json:"-"` // for test
}

type V2RayHTTPUpgradeOptions struct {
	Host    string               `json:"host,omitempty"`
	Path    string               `json:"path,omitempty"`
	Headers badoption.HTTPHeader `json:"headers,omitempty"`
}

type V2RayXHTTPBaseOptions struct {
	Host                 string                 `json:"host,omitempty"`
	Path                 string                 `json:"path,omitempty"`
	Headers              map[string]string      `json:"headers,omitempty"`
	DomainStrategy       DomainStrategy         `json:"domainStrategy,omitempty"`
	XPaddingBytes        *Xbadoption.Range      `json:"xPaddingBytes,omitempty"`
	NoGRPCHeader         bool                   `json:"noGRPCHeader,omitempty"`
	NoSSEHeader          bool                   `json:"noSSEHeader,omitempty"`
	ScMaxEachPostBytes   *Xbadoption.Range      `json:"scMaxEachPostBytes,omitempty"`
	ScMinPostsIntervalMs *Xbadoption.Range      `json:"scMinPostsIntervalMs,omitempty"`
	ScMaxBufferedPosts   int64                  `json:"scMaxBufferedPosts,omitempty"`
	ScStreamUpServerSecs *Xbadoption.Range      `json:"scStreamUpServerSecs,omitempty"`
	Xmux                 *V2RayXHTTPXmuxOptions `json:"xmux,omitempty"`
	XPaddingObfsMode     bool                   `json:"xPaddingObfsMode,omitempty"`
	XPaddingKey          string                 `json:"xPaddingKey,omitempty"`
	XPaddingHeader       string                 `json:"xPaddingHeader,omitempty"`
	XPaddingPlacement    string                 `json:"xPaddingPlacement,omitempty"`
	XPaddingMethod       string                 `json:"xPaddingMethod,omitempty"`
	UplinkHTTPMethod     string                 `json:"uplinkHTTPMethod,omitempty"`
	SessionIDPlacement   string                 `json:"sessionIDPlacement,omitempty"`
	SessionIDKey         string                 `json:"sessionIDKey,omitempty"`
	SeqPlacement         string                 `json:"seqPlacement,omitempty"`
	SeqKey               string                 `json:"seqKey,omitempty"`
	UplinkDataPlacement  string                 `json:"uplinkDataPlacement,omitempty"`
	UplinkDataKey        string                 `json:"uplinkDataKey,omitempty"`
	UplinkChunkSize      *Xbadoption.Range      `json:"uplinkChunkSize,omitempty"`
	ServerMaxHeaderBytes int32                  `json:"serverMaxHeaderBytes,omitempty"`
	SessionIDTable       string                 `json:"sessionIDTable,omitempty"`
	SessionIDLength      *Xbadoption.Range      `json:"sessionIDLength,omitempty"`
}

type V2RayXHTTPOptions struct {
	Mode string `json:"mode,omitempty"`
	V2RayXHTTPBaseOptions
	Download *V2RayXHTTPDownloadOptions `json:"downloadSettings,omitempty"`
}

type V2RayXHTTPDownloadOptions struct {
	V2RayXHTTPBaseOptions
	ServerOptions
	OutboundTLSOptionsContainer
	Detour string `json:"detour,omitempty"`
}

func (c *V2RayXHTTPBaseOptions) GetNormalizedPath() string {
	pathAndQuery := strings.SplitN(c.Path, "?", 2)
	path := pathAndQuery[0]
	if path == "" || path[0] != '/' {
		path = "/" + path
	}
	if c.GetNormalizedSessionPlacement() == "path" || c.GetNormalizedSeqPlacement() == "path" {
		if path[len(path)-1] != '/' {
			path = path + "/"
		}
	}
	return path
}

func (c *V2RayXHTTPBaseOptions) GetNormalizedQuery() string {
	pathAndQuery := strings.SplitN(c.Path, "?", 2)
	query := ""
	if len(pathAndQuery) > 1 {
		query = pathAndQuery[1]
	}
	return query
}

func (c *V2RayXHTTPBaseOptions) GetRequestHeader() http.Header {
	header := http.Header{}
	for k, v := range c.Headers {
		header.Add(k, v)
	}
	if header.Get("User-Agent") == "" {
		header.Set("User-Agent", C.DefaultBrowserAgent)
	}
	return header
}

func (c *V2RayXHTTPBaseOptions) GetNormalizedXPaddingBytes() Xbadoption.Range {
	if c.XPaddingBytes == nil || c.XPaddingBytes.To == 0 {
		return Xbadoption.Range{From: 100, To: 1000}
	}
	return *c.XPaddingBytes
}

func (c *V2RayXHTTPBaseOptions) GetNormalizedScMaxEachPostBytes() Xbadoption.Range {
	if c.ScMaxEachPostBytes == nil || c.ScMaxEachPostBytes.To == 0 {
		return Xbadoption.Range{From: 1000000, To: 1000000}
	}
	return *c.ScMaxEachPostBytes
}

func (c *V2RayXHTTPBaseOptions) GetNormalizedScMinPostsIntervalMs() Xbadoption.Range {
	if c.ScMinPostsIntervalMs == nil || c.ScMinPostsIntervalMs.To == 0 {
		return Xbadoption.Range{From: 30, To: 30}
	}
	return *c.ScMinPostsIntervalMs
}

func (c *V2RayXHTTPBaseOptions) GetNormalizedScMaxBufferedPosts() int {
	if c.ScMaxBufferedPosts == 0 {
		return 30
	}
	return int(c.ScMaxBufferedPosts)
}

func (c *V2RayXHTTPBaseOptions) GetNormalizedScStreamUpServerSecs() Xbadoption.Range {
	if c.ScStreamUpServerSecs == nil || c.ScStreamUpServerSecs.To == 0 {
		return Xbadoption.Range{From: 20, To: 80}
	}
	return *c.ScStreamUpServerSecs
}

func (c *V2RayXHTTPBaseOptions) GetNormalizedUplinkHTTPMethod() string {
	if c.UplinkHTTPMethod == "" {
		return "POST"
	}
	return c.UplinkHTTPMethod
}

func (c *V2RayXHTTPBaseOptions) GetNormalizedSessionPlacement() string {
	if c.SessionIDPlacement == "" {
		return "path"
	}
	return c.SessionIDPlacement
}

func (c *V2RayXHTTPBaseOptions) GetNormalizedSeqPlacement() string {
	if c.SeqPlacement == "" {
		return "path"
	}
	return c.SeqPlacement
}

func (c *V2RayXHTTPBaseOptions) GetNormalizedUplinkDataPlacement() string {
	if c.UplinkDataPlacement == "" {
		return "body"
	}
	return c.UplinkDataPlacement
}

func (c *V2RayXHTTPBaseOptions) GetNormalizedSessionKey() string {
	if c.SessionIDKey != "" {
		return c.SessionIDKey
	}
	switch c.GetNormalizedSessionPlacement() {
	case "header":
		return "X-Session"
	case "cookie", "query":
		return "x_session"
	default:
		return ""
	}
}

func (c *V2RayXHTTPBaseOptions) GetNormalizedSeqKey() string {
	if c.SeqKey != "" {
		return c.SeqKey
	}
	switch c.GetNormalizedSeqPlacement() {
	case "header":
		return "X-Seq"
	case "cookie", "query":
		return "x_seq"
	default:
		return ""
	}
}

func (c *V2RayXHTTPBaseOptions) GetNormalizedUplinkChunkSize() Xbadoption.Range {
	if c.UplinkChunkSize == nil || c.UplinkChunkSize.To == 0 {
		switch c.UplinkDataPlacement {
		case "cookie":
			return Xbadoption.Range{From: 2 * 1024, To: 3 * 1024}
		case "header":
			return Xbadoption.Range{From: 3 * 1000, To: 4 * 1000}
		default:
			return c.GetNormalizedScMaxEachPostBytes()
		}
	} else if c.UplinkChunkSize.From < 64 {
		to := c.UplinkChunkSize.To
		if to < 64 {
			to = 64
		}
		return Xbadoption.Range{From: 64, To: to}
	}
	return *c.UplinkChunkSize
}

func (c *V2RayXHTTPBaseOptions) GetNormalizedServerMaxHeaderBytes() int {
	if c.ServerMaxHeaderBytes <= 0 {
		return 8192
	}
	return int(c.ServerMaxHeaderBytes)
}

type V2RayXHTTPXmuxOptions struct {
	MaxConcurrency   Xbadoption.Range `json:"maxConcurrency"`
	MaxConnections   Xbadoption.Range `json:"maxConnections"`
	CMaxReuseTimes   Xbadoption.Range `json:"cMaxReuseTimes"`
	HMaxRequestTimes Xbadoption.Range `json:"hMaxRequestTimes"`
	HMaxReusableSecs Xbadoption.Range `json:"hMaxReusableSecs"`
	HKeepAlivePeriod int64            `json:"hKeepAlivePeriod"`
}

func (m *V2RayXHTTPXmuxOptions) GetNormalizedMaxConcurrency() Xbadoption.Range {
	return m.MaxConcurrency
}

func (m *V2RayXHTTPXmuxOptions) GetNormalizedMaxConnections() Xbadoption.Range {
	return m.MaxConnections
}

func (m *V2RayXHTTPXmuxOptions) GetNormalizedCMaxReuseTimes() Xbadoption.Range {
	return m.CMaxReuseTimes
}

func (m *V2RayXHTTPXmuxOptions) GetNormalizedHMaxRequestTimes() Xbadoption.Range {
	return m.HMaxRequestTimes
}

func (m *V2RayXHTTPXmuxOptions) GetNormalizedHMaxReusableSecs() Xbadoption.Range {
	return m.HMaxReusableSecs
}
