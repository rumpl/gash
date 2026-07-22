package network

import (
	"context"
	"net/http"
	"net/netip"
	"testing"
)

type staticResolver []netip.Addr

func (r staticResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return []netip.Addr(r), nil
}

func TestPolicyAllowListChecksOriginPathMethodAndHeaders(t *testing.T) {
	policy := NewPolicy(Rule{Scheme: "https", Host: "api.example.com", Port: "443", Path: "/v1", Methods: []string{"POST"}, Headers: []string{"Content-Type"}})
	req, _ := http.NewRequest("POST", "https://api.example.com/v1/users", nil)
	req.Header.Set("Content-Type", "application/json")
	if err := policy.Check(req); err != nil {
		t.Fatalf("allowed request denied: %v", err)
	}
	req.Method = "GET"
	if err := policy.Check(req); err == nil {
		t.Fatalf("method outside allow-list was allowed")
	}
	req.Method = "POST"
	req.URL.Path = "/v2"
	if err := policy.Check(req); err == nil {
		t.Fatalf("path outside allow-list was allowed")
	}
}

func TestPolicyRejectsCredentialsAndPrivateDNS(t *testing.T) {
	policy := NewPolicy(AllowOrigin("https://example.com"))
	req, _ := http.NewRequest("GET", "https://user:pass@example.com/", nil)
	if err := policy.Check(req); err == nil {
		t.Fatalf("URL credentials were allowed")
	}
	policy.Resolver = staticResolver{netip.MustParseAddr("127.0.0.1")}
	ips, err := policy.Normalized().Resolver.LookupNetIP(context.Background(), "ip", "example.com")
	if err != nil || len(ips) != 1 {
		t.Fatal(err)
	}
	if isPublicIP(ips[0]) {
		t.Fatalf("loopback address considered public")
	}
}

func TestAllowOriginCanonicalizesDefaultPort(t *testing.T) {
	rule := AllowOrigin("https://example.com/api")
	if rule.Scheme != "https" || rule.Host != "example.com" || rule.Port != "443" || rule.Path != "/api" {
		t.Fatalf("unexpected rule: %#v", rule)
	}
}
