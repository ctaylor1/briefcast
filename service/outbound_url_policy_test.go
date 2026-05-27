package service

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidateOutboundURLRejectsUnsafeURLs(t *testing.T) {
	t.Setenv(outboundAllowPrivateEnv, "false")

	cases := []string{
		"",
		"://bad-url",
		"ftp://example.com/feed.xml",
		"file:///etc/passwd",
		"https://user:pass@example.com/feed.xml",
		"http://localhost/feed.xml",
		"http://127.0.0.1/feed.xml",
		"http://10.0.0.5/feed.xml",
		"http://[::1]/feed.xml",
	}

	for _, rawURL := range cases {
		if _, err := validateOutboundURL(rawURL, outboundPurposeHTTP); err == nil {
			t.Fatalf("expected %q to be rejected", rawURL)
		}
	}
}

func TestValidateOutboundURLAllowsPublicHTTPURLs(t *testing.T) {
	t.Setenv(outboundAllowPrivateEnv, "false")

	for _, rawURL := range []string{
		"https://example.com/feed.xml",
		"http://example.com/media.mp3",
	} {
		if _, err := validateOutboundURL(rawURL, outboundPurposeHTTP); err != nil {
			t.Fatalf("expected %q to be allowed, got %v", rawURL, err)
		}
	}
}

func TestValidateOutboundURLAllowsPrivateHostsWhenConfigured(t *testing.T) {
	t.Setenv(outboundAllowPrivateEnv, "true")

	if _, err := validateOutboundURL("http://127.0.0.1/feed.xml", outboundPurposeHTTP); err != nil {
		t.Fatalf("expected configured private outbound URL to be allowed, got %v", err)
	}
}

func TestValidateBriefpointURLAllowsLocalhostByDefault(t *testing.T) {
	t.Setenv(outboundAllowPrivateEnv, "false")
	t.Setenv(outboundAllowPrivateBriefpointEnv, "")

	if _, err := validateOutboundURL("http://localhost:12314", outboundPurposeBriefpoint); err != nil {
		t.Fatalf("expected default Briefpoint localhost URL to be allowed, got %v", err)
	}
}

func TestValidateBriefpointURLCanBlockLocalhost(t *testing.T) {
	t.Setenv(outboundAllowPrivateBriefpointEnv, "false")

	if _, err := validateOutboundURL("http://localhost:12314", outboundPurposeBriefpoint); err == nil {
		t.Fatalf("expected Briefpoint localhost URL to be rejected when private Briefpoint URLs are disabled")
	}
}

func TestMakeQueryRejectsOversizedResponse(t *testing.T) {
	setupRetentionTestDB(t)
	t.Setenv(outboundMaxResponseBytesEnv, "4")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("12345"))
	}))
	defer server.Close()

	_, err := makeQuery(server.URL)
	if err == nil {
		t.Fatalf("expected oversized response to fail")
	}
	if !strings.Contains(err.Error(), "exceeds 4 bytes") {
		t.Fatalf("expected outbound size limit error, got %v", err)
	}
}

func TestGetRequestAppliesOutboundURLPolicy(t *testing.T) {
	t.Setenv(outboundAllowPrivateEnv, "false")

	_, err := getRequest("http://127.0.0.1/feed.xml")
	if err == nil {
		t.Fatalf("expected private request URL to be rejected")
	}
	if !strings.Contains(err.Error(), "outbound URL rejected") {
		t.Fatalf("expected an outbound URL policy error, got %v", err)
	}
}

func TestOutboundDialAddressRejectsResolvedPrivateHosts(t *testing.T) {
	originalLookup := lookupOutboundIPAddrs
	lookupOutboundIPAddrs = func(_ context.Context, host string) ([]net.IPAddr, error) {
		if host != "private.example" {
			t.Fatalf("unexpected lookup host %q", host)
		}
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	}
	t.Cleanup(func() {
		lookupOutboundIPAddrs = originalLookup
	})

	policy := outboundURLPolicy{purpose: outboundPurposeHTTP, allowPrivateHosts: false}
	_, err := outboundDialAddress(context.Background(), "tcp", "private.example:80", policy)
	if err == nil {
		t.Fatalf("expected hostname resolving to private address to be rejected")
	}
	if !strings.Contains(err.Error(), "outbound URL rejected") {
		t.Fatalf("expected outbound URL policy error, got %v", err)
	}
}

func TestOutboundDialAddressAllowsResolvedPublicHosts(t *testing.T) {
	originalLookup := lookupOutboundIPAddrs
	lookupOutboundIPAddrs = func(_ context.Context, host string) ([]net.IPAddr, error) {
		if host != "public.example" {
			t.Fatalf("unexpected lookup host %q", host)
		}
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
	}
	t.Cleanup(func() {
		lookupOutboundIPAddrs = originalLookup
	})

	policy := outboundURLPolicy{purpose: outboundPurposeHTTP, allowPrivateHosts: false}
	address, err := outboundDialAddress(context.Background(), "tcp", "public.example:443", policy)
	if err != nil {
		t.Fatalf("expected public resolved host to be allowed, got %v", err)
	}
	if address != "93.184.216.34:443" {
		t.Fatalf("expected resolved dial address, got %q", address)
	}
}
