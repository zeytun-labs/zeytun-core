package xhttp

import (
	"encoding/base64"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strings"

	"github.com/sagernet/sing-box/common/xray/buf"
	"github.com/sagernet/sing-box/common/xray/uuid"
	"github.com/sagernet/sing-box/option"
)

var PredefinedTable = map[string]string{
	"ALPHABET": "ABCDEFGHIJKLMNOPQRSTUVWXYZ",
	"Alphabet": "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz",
	"BASE36":   "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ",
	"Base62":   "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz",
	"HEX":      "0123456789ABCDEF",
	"alphabet": "abcdefghijklmnopqrstuvwxyz",
	"base36":   "0123456789abcdefghijklmnopqrstuvwxyz",
	"hex":      "0123456789abcdef",
	"number":   "0123456789",
}

func appendToPath(path, value string) string {
	if strings.HasSuffix(path, "/") {
		return path + value
	}
	return path + "/" + value
}

func GenerateSessionID(c *option.V2RayXHTTPBaseOptions) string {
	var length int32
	if c.SessionIDLength != nil {
		length = c.SessionIDLength.Rand()
	}
	table := c.SessionIDTable
	if predefined, ok := PredefinedTable[table]; ok {
		table = predefined
	}
	if table != "" && length > 0 {
		id := make([]byte, length)
		for i := range id {
			id[i] = table[rand.N(len(table))]
		}
		return string(id)
	}
	u := uuid.New()
	return u.String()
}

func WriteResponseHeader(c *option.V2RayXHTTPBaseOptions, writer http.ResponseWriter, requestMethod string, requestHeader http.Header) {
	if origin := requestHeader.Get("Origin"); origin == "" {
		writer.Header().Set("Access-Control-Allow-Origin", "*")
	} else {
		writer.Header().Set("Access-Control-Allow-Origin", origin)
	}
	if c.GetNormalizedSessionPlacement() == PlacementCookie ||
		c.GetNormalizedSeqPlacement() == PlacementCookie ||
		c.XPaddingPlacement == PlacementCookie ||
		c.GetNormalizedUplinkDataPlacement() == PlacementCookie {
		writer.Header().Set("Access-Control-Allow-Credentials", "true")
	}
	if requestMethod == "OPTIONS" {
		if requestedMethod := requestHeader.Get("Access-Control-Request-Method"); requestedMethod != "" {
			writer.Header().Set("Access-Control-Allow-Methods", requestedMethod)
		} else {
			writer.Header().Set("Access-Control-Allow-Methods", "*")
		}
		if requestedHeaders := requestHeader.Get("Access-Control-Request-Headers"); requestedHeaders == "" {
			writer.Header().Set("Access-Control-Allow-Headers", "*")
		} else {
			writer.Header().Set("Access-Control-Allow-Headers", requestedHeaders)
		}
	}
}

func GetRequestHeaderWithPayload(c *option.V2RayXHTTPBaseOptions, payload []byte) http.Header {
	header := c.GetRequestHeader()
	key := c.UplinkDataKey
	if key == "" {
		key = "x_uplink"
	}
	encodedData := base64.RawURLEncoding.EncodeToString(payload)
	for i := 0; len(encodedData) > 0; i++ {
		chunkSize := min(int(c.GetNormalizedUplinkChunkSize().Rand()), len(encodedData))
		chunk := encodedData[:chunkSize]
		encodedData = encodedData[chunkSize:]
		header.Set(fmt.Sprintf("%s-%d", key, i), chunk)
	}
	return header
}

func GetRequestCookiesWithPayload(c *option.V2RayXHTTPBaseOptions, payload []byte) []*http.Cookie {
	cookies := []*http.Cookie{}
	key := c.UplinkDataKey
	if key == "" {
		key = "x_uplink"
	}
	encodedData := base64.RawURLEncoding.EncodeToString(payload)
	for i := 0; len(encodedData) > 0; i++ {
		chunkSize := min(int(c.GetNormalizedUplinkChunkSize().Rand()), len(encodedData))
		chunk := encodedData[:chunkSize]
		encodedData = encodedData[chunkSize:]
		cookies = append(cookies, &http.Cookie{Name: fmt.Sprintf("%s_%d", key, i), Value: chunk})
	}
	return cookies
}

func ApplyMetaToRequest(c *option.V2RayXHTTPBaseOptions, req *http.Request, sessionId, seqStr string) {
	sessionPlacement := c.GetNormalizedSessionPlacement()
	seqPlacement := c.GetNormalizedSeqPlacement()
	sessionKey := c.GetNormalizedSessionKey()
	seqKey := c.GetNormalizedSeqKey()
	if sessionId != "" {
		switch sessionPlacement {
		case PlacementPath:
			req.URL.Path = appendToPath(req.URL.Path, sessionId)
		case PlacementQuery:
			q := req.URL.Query()
			q.Set(sessionKey, sessionId)
			req.URL.RawQuery = q.Encode()
		case PlacementHeader:
			req.Header.Set(sessionKey, sessionId)
		case PlacementCookie:
			req.AddCookie(&http.Cookie{Name: sessionKey, Value: sessionId})
		}
	}
	if seqStr != "" {
		switch seqPlacement {
		case PlacementPath:
			req.URL.Path = appendToPath(req.URL.Path, seqStr)
		case PlacementQuery:
			q := req.URL.Query()
			q.Set(seqKey, seqStr)
			req.URL.RawQuery = q.Encode()
		case PlacementHeader:
			req.Header.Set(seqKey, seqStr)
		case PlacementCookie:
			req.AddCookie(&http.Cookie{Name: seqKey, Value: seqStr})
		}
	}
}

func FillStreamRequest(c *option.V2RayXHTTPBaseOptions, request *http.Request, sessionId, seqStr string) {
	request.Header = c.GetRequestHeader()
	length := int(c.GetNormalizedXPaddingBytes().Rand())
	cfg := XPaddingConfig{Length: length}
	if c.XPaddingObfsMode {
		cfg.Placement = XPaddingPlacement{
			Placement: c.XPaddingPlacement,
			Key:       c.XPaddingKey,
			Header:    c.XPaddingHeader,
			RawURL:    request.URL.String(),
		}
		cfg.Method = PaddingMethod(c.XPaddingMethod)
	} else {
		cfg.Placement = XPaddingPlacement{
			Placement: PlacementQueryInHeader,
			Key:       "x_padding",
			Header:    "Referer",
			RawURL:    request.URL.String(),
		}
	}
	ApplyXPaddingToRequest(request, cfg)
	ApplyMetaToRequest(c, request, sessionId, "")
	if request.Body != nil && !c.NoGRPCHeader {
		request.Header.Set("Content-Type", "application/grpc")
	}
}

func FillPacketRequest(c *option.V2RayXHTTPBaseOptions, request *http.Request, sessionId, seqStr string, payload buf.MultiBuffer) error {
	dataPlacement := c.GetNormalizedUplinkDataPlacement()
	if dataPlacement == PlacementBody || dataPlacement == PlacementAuto {
		request.Header = c.GetRequestHeader()
		request.Body = io.NopCloser(&buf.MultiBufferContainer{MultiBuffer: payload})
		request.ContentLength = int64(payload.Len())
	} else {
		data := make([]byte, payload.Len())
		payload.Copy(data)
		buf.ReleaseMulti(payload)
		switch dataPlacement {
		case PlacementHeader:
			request.Header = GetRequestHeaderWithPayload(c, data)
		case PlacementCookie:
			request.Header = c.GetRequestHeader()
			for _, cookie := range GetRequestCookiesWithPayload(c, data) {
				request.AddCookie(cookie)
			}
		}
	}
	length := int(c.GetNormalizedXPaddingBytes().Rand())
	cfg := XPaddingConfig{Length: length}
	if c.XPaddingObfsMode {
		cfg.Placement = XPaddingPlacement{
			Placement: c.XPaddingPlacement,
			Key:       c.XPaddingKey,
			Header:    c.XPaddingHeader,
			RawURL:    request.URL.String(),
		}
		cfg.Method = PaddingMethod(c.XPaddingMethod)
	} else {
		cfg.Placement = XPaddingPlacement{
			Placement: PlacementQueryInHeader,
			Key:       "x_padding",
			Header:    "Referer",
			RawURL:    request.URL.String(),
		}
	}
	ApplyXPaddingToRequest(request, cfg)
	ApplyMetaToRequest(c, request, sessionId, seqStr)
	return nil
}

func ExtractMetaFromRequest(c *option.V2RayXHTTPBaseOptions, req *http.Request, path string) (sessionId string, seqStr string) {
	sessionPlacement := c.GetNormalizedSessionPlacement()
	seqPlacement := c.GetNormalizedSeqPlacement()
	sessionKey := c.GetNormalizedSessionKey()
	seqKey := c.GetNormalizedSeqKey()
	var subpath []string
	pathPart := 0
	if sessionPlacement == PlacementPath || seqPlacement == PlacementPath {
		if len(req.URL.Path) >= len(path) {
			subpath = strings.Split(req.URL.Path[len(path):], "/")
		}
	}
	switch sessionPlacement {
	case PlacementPath:
		if len(subpath) > pathPart {
			sessionId = subpath[pathPart]
			pathPart++
		}
	case PlacementQuery:
		sessionId = req.URL.Query().Get(sessionKey)
	case PlacementHeader:
		sessionId = req.Header.Get(sessionKey)
	case PlacementCookie:
		if cookie, e := req.Cookie(sessionKey); e == nil {
			sessionId = cookie.Value
		}
	}
	switch seqPlacement {
	case PlacementPath:
		if len(subpath) > pathPart {
			seqStr = subpath[pathPart]
		}
	case PlacementQuery:
		seqStr = req.URL.Query().Get(seqKey)
	case PlacementHeader:
		seqStr = req.Header.Get(seqKey)
	case PlacementCookie:
		if cookie, e := req.Cookie(seqKey); e == nil {
			seqStr = cookie.Value
		}
	}
	return sessionId, seqStr
}
