// Package proxy resolves indexit's INDEXIT_PROXY_* configuration into a
// gotd/td dcs.Resolver, ready to be plugged into telegram.Options.
//
// See the PoC plan §5.2 for the full configuration contract.
package proxy

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/gotd/td/mtproxy"
	"github.com/gotd/td/telegram/dcs"
	xproxy "golang.org/x/net/proxy"
)

// Type enumerates supported proxy kinds.
type Type string

const (
	TypeSOCKS5  Type = "socks5"
	TypeMTProto Type = "mtproto"
	TypeHTTP    Type = "http"
)

// Descriptor is a credential-bearing, validated proxy descriptor.
type Descriptor struct {
	Type   Type
	Host   string
	Port   int
	User   string
	Pass   string
	Secret string // hex or base64 encoded; required for MTProto
}

// FromEnv builds a Descriptor from INDEXIT_PROXY_* variables. Returns
// (nil, nil) when no proxy is configured.
func FromEnv() (*Descriptor, error) {
	if raw := os.Getenv("INDEXIT_PROXY_URL"); raw != "" {
		return fromURL(raw)
	}

	typeStr := os.Getenv("INDEXIT_PROXY_TYPE")
	hostStr := os.Getenv("INDEXIT_PROXY_HOST")
	portStr := os.Getenv("INDEXIT_PROXY_PORT")

	if typeStr == "" && hostStr == "" && portStr == "" &&
		os.Getenv("INDEXIT_PROXY_USER") == "" &&
		os.Getenv("INDEXIT_PROXY_PASS") == "" &&
		os.Getenv("INDEXIT_PROXY_SECRET") == "" {
		return nil, nil
	}

	if typeStr == "" {
		return nil, fmt.Errorf("INDEXIT_PROXY_TYPE is required when other INDEXIT_PROXY_* are set")
	}
	if hostStr == "" {
		return nil, fmt.Errorf("INDEXIT_PROXY_HOST is required")
	}
	if portStr == "" {
		return nil, fmt.Errorf("INDEXIT_PROXY_PORT is required")
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("parse INDEXIT_PROXY_PORT: %w", err)
	}

	d := &Descriptor{
		Type:   Type(strings.ToLower(typeStr)),
		Host:   hostStr,
		Port:   port,
		User:   os.Getenv("INDEXIT_PROXY_USER"),
		Pass:   os.Getenv("INDEXIT_PROXY_PASS"),
		Secret: os.Getenv("INDEXIT_PROXY_SECRET"),
	}
	if err := d.validate(); err != nil {
		return nil, err
	}
	return d, nil
}

func fromURL(raw string) (*Descriptor, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse INDEXIT_PROXY_URL: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("INDEXIT_PROXY_URL must be a full URL with scheme and host")
	}
	if kind, ok := linkKind(u); ok {
		return fromLink(u, kind)
	}
	portStr := u.Port()
	if portStr == "" {
		return nil, fmt.Errorf("INDEXIT_PROXY_URL must include a port")
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("parse port in INDEXIT_PROXY_URL: %w", err)
	}
	d := &Descriptor{
		Type: Type(strings.ToLower(u.Scheme)),
		Host: u.Hostname(),
		Port: port,
	}
	if u.User != nil {
		d.User = u.User.Username()
		d.Pass, _ = u.User.Password()
	}
	if s := u.Query().Get("secret"); s != "" {
		d.Secret = s
	}
	if err := d.validate(); err != nil {
		return nil, err
	}
	return d, nil
}

// linkKind reports whether the URL is a Telegram proxy-sharing link
// (tg://proxy, tg://socks, https://t.me/proxy, https://t.me/socks) and which
// kind it denotes. Those links are what Telegram itself hands out, so accepting
// them verbatim spares the user a manual conversion.
func linkKind(u *url.URL) (string, bool) {
	kind := ""
	switch strings.ToLower(u.Scheme) {
	case "tg":
		kind = strings.ToLower(u.Host)
	case "http", "https":
		switch strings.ToLower(u.Hostname()) {
		case "t.me", "telegram.me", "telegram.dog":
			kind = strings.ToLower(strings.Trim(u.Path, "/"))
		default:
			return "", false
		}
	default:
		return "", false
	}
	switch kind {
	case "proxy", "socks":
		return kind, true
	}
	return "", false
}

// fromLink builds a Descriptor from a Telegram proxy-sharing link.
func fromLink(u *url.URL, kind string) (*Descriptor, error) {
	q := u.Query()
	host := q.Get("server")
	if host == "" {
		return nil, fmt.Errorf("INDEXIT_PROXY_URL proxy link must include server=")
	}
	portStr := q.Get("port")
	if portStr == "" {
		return nil, fmt.Errorf("INDEXIT_PROXY_URL proxy link must include port=")
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("parse port in INDEXIT_PROXY_URL: %w", err)
	}
	d := &Descriptor{
		Host:   host,
		Port:   port,
		User:   q.Get("user"),
		Pass:   q.Get("pass"),
		Secret: q.Get("secret"),
	}
	if kind == "socks" {
		d.Type = TypeSOCKS5
	} else {
		d.Type = TypeMTProto
	}
	if err := d.validate(); err != nil {
		return nil, err
	}
	return d, nil
}

func (d *Descriptor) validate() error {
	if d.Host == "" {
		return fmt.Errorf("proxy host is required")
	}
	if d.Port <= 0 || d.Port > 65535 {
		return fmt.Errorf("proxy port %d is out of range", d.Port)
	}
	switch d.Type {
	case TypeSOCKS5, TypeHTTP:
		return nil
	case TypeMTProto:
		if d.Secret == "" {
			return fmt.Errorf("mtproto proxy requires INDEXIT_PROXY_SECRET")
		}
		if _, err := decodeSecret(d.Secret); err != nil {
			return err
		}
		return nil
	default:
		return fmt.Errorf("unsupported INDEXIT_PROXY_TYPE %q (want socks5|mtproto|http)", d.Type)
	}
}

// decodeSecret decodes an MTProto secret given in hex or base64 form and checks
// that gotd can make sense of it. Telegram shares secrets as base64url inside
// tg://proxy links and as hex elsewhere, and both forms reach us verbatim from
// whatever the user copied, so both are accepted; a plain 16-byte secret, a
// dd-prefixed (secured) one, and an ee-prefixed (fakeTLS) one all pass.
//
// Hex is tried first: a 32-character hex string is also valid base64, and
// reading it as base64 would yield a different, wrong secret.
func decodeSecret(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("mtproto proxy requires INDEXIT_PROXY_SECRET")
	}

	candidates := make([][]byte, 0, 2)
	if b, err := hex.DecodeString(raw); err == nil {
		candidates = append(candidates, b)
	}
	if b, ok := decodeBase64(raw); ok {
		candidates = append(candidates, b)
	}
	for _, candidate := range candidates {
		if _, err := mtproxy.ParseSecret(candidate); err == nil {
			return candidate, nil
		}
	}
	return nil, fmt.Errorf(
		"INDEXIT_PROXY_SECRET is not a valid MTProto secret: want hex or base64 " +
			"holding 16 bytes, or a dd/ee-prefixed 17+ byte secret")
}

// decodeBase64 tries every base64 flavour Telegram links are seen with. Query
// decoding turns a "+" of standard base64 into a space, so that is undone first.
func decodeBase64(raw string) ([]byte, bool) {
	plus := strings.ReplaceAll(raw, " ", "+")
	for _, enc := range []*base64.Encoding{
		base64.RawURLEncoding,
		base64.URLEncoding,
		base64.RawStdEncoding,
		base64.StdEncoding,
	} {
		if b, err := enc.DecodeString(plus); err == nil {
			return b, true
		}
	}
	return nil, false
}

// Display returns a human-readable, credential-free descriptor. Safe to log.
func (d *Descriptor) Display() string {
	if d == nil {
		return "none"
	}
	return fmt.Sprintf("%s %s:%d", d.Type, d.Host, d.Port)
}

func (d *Descriptor) addr() string {
	return net.JoinHostPort(d.Host, strconv.Itoa(d.Port))
}

// Resolver builds a gotd dcs.Resolver from the descriptor.
func (d *Descriptor) Resolver() (dcs.Resolver, error) {
	switch d.Type {
	case TypeMTProto:
		secret, err := decodeSecret(d.Secret)
		if err != nil {
			return nil, err
		}
		return dcs.MTProxy(d.addr(), secret, dcs.MTProxyOptions{})
	case TypeSOCKS5:
		var auth *xproxy.Auth
		if d.User != "" || d.Pass != "" {
			auth = &xproxy.Auth{User: d.User, Password: d.Pass}
		}
		dialer, err := xproxy.SOCKS5("tcp", d.addr(), auth, xproxy.Direct)
		if err != nil {
			return nil, fmt.Errorf("build socks5 dialer: %w", err)
		}
		ctxDialer, ok := dialer.(xproxy.ContextDialer)
		if !ok {
			return nil, fmt.Errorf("socks5 dialer does not implement ContextDialer")
		}
		return dcs.Plain(dcs.PlainOptions{Dial: ctxDialer.DialContext}), nil
	case TypeHTTP:
		dialer := &httpConnectDialer{
			proxyAddr: d.addr(),
			user:      d.User,
			pass:      d.Pass,
		}
		return dcs.Plain(dcs.PlainOptions{Dial: dialer.DialContext}), nil
	}
	return nil, fmt.Errorf("unsupported proxy type %q", d.Type)
}

// httpConnectDialer tunnels an arbitrary TCP destination through an HTTP CONNECT
// proxy. The implementation is intentionally minimal — MTProto over HTTP proxies
// is fragile (plan §5.2 flags it as best-effort).
type httpConnectDialer struct {
	proxyAddr string
	user      string
	pass      string
}

func (h *httpConnectDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return nil, fmt.Errorf("http proxy supports tcp only, got %q", network)
	}
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", h.proxyAddr)
	if err != nil {
		return nil, fmt.Errorf("dial http proxy: %w", err)
	}
	req := &http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Opaque: addr},
		Host:   addr,
		Header: http.Header{},
	}
	if h.user != "" || h.pass != "" {
		token := base64.StdEncoding.EncodeToString([]byte(h.user + ":" + h.pass))
		req.Header.Set("Proxy-Authorization", "Basic "+token)
	}
	if err := req.Write(conn); err != nil {
		conn.Close()
		return nil, fmt.Errorf("write CONNECT: %w", err)
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, req)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("read CONNECT response: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		conn.Close()
		return nil, fmt.Errorf("http proxy CONNECT failed: %s", resp.Status)
	}
	if br.Buffered() > 0 {
		conn.Close()
		return nil, fmt.Errorf("http proxy returned unexpected payload after CONNECT")
	}
	return conn, nil
}
