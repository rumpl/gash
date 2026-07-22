package network

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"path"
	"strings"
	"time"
)

const (
	DefaultMaxResponseBytes int64 = 10 << 20
	DefaultMaxRequestBytes  int64 = 10 << 20
	DefaultTimeout                = 30 * time.Second
	DefaultMaxRedirects           = 10
)

// Policy is the explicit network capability used by opt-in network commands.
// A zero Policy denies every request. Use NewPolicy to fill practical defaults.
type Policy struct {
	Rules            []Rule
	Timeout          time.Duration
	MaxResponseBytes int64
	MaxRequestBytes  int64
	MaxRedirects     int
	AllowPrivateIPs  bool
	Resolver         Resolver
	Client           *http.Client
}

type Rule struct {
	Scheme  string
	Host    string
	Port    string
	Path    string
	Methods []string
	Headers []string
}

type Resolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}

type netResolver struct{}

func (netResolver) LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error) {
	return net.DefaultResolver.LookupNetIP(ctx, network, host)
}

func NewPolicy(rules ...Rule) Policy {
	return Policy{
		Rules:            rules,
		Timeout:          DefaultTimeout,
		MaxResponseBytes: DefaultMaxResponseBytes,
		MaxRequestBytes:  DefaultMaxRequestBytes,
		MaxRedirects:     DefaultMaxRedirects,
	}
}

func AllowOrigin(raw string) Rule {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return Rule{}
	}
	return Rule{Scheme: strings.ToLower(u.Scheme), Host: strings.ToLower(u.Hostname()), Port: canonicalPort(u), Path: path.Clean("/" + strings.TrimPrefix(u.EscapedPath(), "/"))}
}

func (p Policy) Normalized() Policy {
	if p.Timeout == 0 {
		p.Timeout = DefaultTimeout
	}
	if p.MaxResponseBytes == 0 {
		p.MaxResponseBytes = DefaultMaxResponseBytes
	}
	if p.MaxRequestBytes == 0 {
		p.MaxRequestBytes = DefaultMaxRequestBytes
	}
	if p.MaxRedirects == 0 {
		p.MaxRedirects = DefaultMaxRedirects
	}
	if p.Resolver == nil {
		p.Resolver = netResolver{}
	}
	return p
}

func (p Policy) Check(req *http.Request) error {
	if req == nil || req.URL == nil {
		return errors.New("network denied: invalid request")
	}
	p = p.Normalized()
	if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
		return fmt.Errorf("network denied: scheme %q is not allowed", req.URL.Scheme)
	}
	if req.URL.User != nil {
		return errors.New("network denied: credentials in URLs are not allowed")
	}
	if req.URL.Hostname() == "" {
		return errors.New("network denied: missing host")
	}
	if !p.ruleAllows(req) {
		return fmt.Errorf("network denied: %s %s is not allowed by policy", req.Method, safeURL(req.URL))
	}
	return nil
}

func (p Policy) ruleAllows(req *http.Request) bool {
	for _, rule := range p.Rules {
		if rule.Scheme != "" && !strings.EqualFold(rule.Scheme, req.URL.Scheme) {
			continue
		}
		if rule.Host != "" && !hostMatches(strings.ToLower(req.URL.Hostname()), strings.ToLower(rule.Host)) {
			continue
		}
		if rule.Port != "" && rule.Port != canonicalPort(req.URL) {
			continue
		}
		if rule.Path != "" && !pathAllowed(req.URL.EscapedPath(), rule.Path) {
			continue
		}
		if len(rule.Methods) > 0 && !containsFold(rule.Methods, req.Method) {
			continue
		}
		if len(rule.Headers) > 0 && !headersAllowed(req.Header, rule.Headers) {
			continue
		}
		return true
	}
	return false
}

func hostMatches(host, rule string) bool {
	if rule == "*" || host == rule {
		return true
	}
	if strings.HasPrefix(rule, "*.") {
		suffix := strings.TrimPrefix(rule, "*")
		return strings.HasSuffix(host, suffix) && host != strings.TrimPrefix(suffix, ".")
	}
	return false
}

func pathAllowed(escaped, allowed string) bool {
	if escaped == "" {
		escaped = "/"
	}
	if !strings.HasPrefix(allowed, "/") {
		allowed = "/" + allowed
	}
	cleanReq := path.Clean("/" + strings.TrimPrefix(escaped, "/"))
	cleanAllowed := path.Clean("/" + strings.TrimPrefix(allowed, "/"))
	if cleanAllowed == "/" {
		return true
	}
	return cleanReq == cleanAllowed || strings.HasPrefix(cleanReq, cleanAllowed+"/")
}

func canonicalPort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	if u.Scheme == "https" {
		return "443"
	}
	if u.Scheme == "http" {
		return "80"
	}
	return ""
}

func containsFold(list []string, v string) bool {
	for _, item := range list {
		if strings.EqualFold(item, v) {
			return true
		}
	}
	return false
}

func headersAllowed(headers http.Header, allowed []string) bool {
	allowedSet := map[string]bool{}
	for _, h := range allowed {
		allowedSet[http.CanonicalHeaderKey(h)] = true
	}
	for h := range headers {
		if !allowedSet[http.CanonicalHeaderKey(h)] {
			return false
		}
	}
	return true
}

func (p Policy) HTTPClient() *http.Client {
	p = p.Normalized()
	if p.Client != nil {
		client := *p.Client
		client.CheckRedirect = p.redirectChecker()
		return &client
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = p.dialContext
	return &http.Client{Timeout: p.Timeout, Transport: transport, CheckRedirect: p.redirectChecker()}
}

func (p Policy) redirectChecker() func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		p = p.Normalized()
		if len(via) >= p.MaxRedirects {
			return errors.New("network denied: too many redirects")
		}
		return p.Check(req)
	}
}

func (p Policy) dialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	ips, err := p.resolveHost(ctx, host)
	if err != nil {
		return nil, err
	}
	var last error
	for _, ip := range ips {
		if !p.AllowPrivateIPs && !isPublicIP(ip) {
			last = fmt.Errorf("network denied: resolved private address %s", ip)
			continue
		}
		d := net.Dialer{Timeout: p.Timeout}
		conn, err := d.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
		last = err
	}
	if last != nil {
		return nil, last
	}
	return nil, errors.New("network denied: no resolved addresses")
}

func (p Policy) resolveHost(ctx context.Context, host string) ([]netip.Addr, error) {
	if ip, err := netip.ParseAddr(host); err == nil {
		return []netip.Addr{ip}, nil
	}
	return p.Resolver.LookupNetIP(ctx, "ip", host)
}

func isPublicIP(ip netip.Addr) bool {
	if !ip.IsValid() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return false
	}
	if ip.Is4() {
		a := ip.As4()
		if a[0] == 0 || a[0] == 10 || a[0] == 127 || (a[0] == 169 && a[1] == 254) || (a[0] == 172 && a[1] >= 16 && a[1] <= 31) || (a[0] == 192 && a[1] == 168) {
			return false
		}
	}
	return true
}

func safeURL(u *url.URL) string {
	clone := *u
	clone.User = nil
	return clone.String()
}
