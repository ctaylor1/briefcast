package service

import (
	"strings"
	"testing"
)

func TestOutboundUserAgentPrecedence(t *testing.T) {
	t.Setenv(userAgentEnv, "")
	if value, source := OutboundUserAgent(""); value != DefaultUserAgent || source != UserAgentSourceDefault {
		t.Fatalf("default = %q (%s)", value, source)
	}
	if value, source := OutboundUserAgent("  LegacyApp/2.0  "); value != "LegacyApp/2.0" || source != UserAgentSourceSetting {
		t.Fatalf("legacy = %q (%s)", value, source)
	}

	t.Setenv(userAgentEnv, " MyBriefcast/1.0 (+https://example.org/contact) ")
	if value, source := OutboundUserAgent("LegacyApp/2.0"); value != "MyBriefcast/1.0 (+https://example.org/contact)" || source != UserAgentSourceEnv {
		t.Fatalf("env = %q (%s)", value, source)
	}
}

func TestOutboundUserAgentRejectsUnsafeValues(t *testing.T) {
	for name, raw := range map[string]string{
		"header injection": "Briefcast/1.0\r\nX-Injected: 1",
		"tab":              "Briefcast/1.0\tx",
		"non-ascii":        "Briefcast/1.0 (café)",
		"too long":         "Briefcast/" + strings.Repeat("x", maxUserAgentLength),
		"whitespace only":  "   ",
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(userAgentEnv, raw)
			if value, source := OutboundUserAgent(""); value != DefaultUserAgent || source != UserAgentSourceDefault {
				t.Fatalf("invalid env fell through to %q (%s)", value, source)
			}
			if value, source := OutboundUserAgent("Legacy/1.0"); value != "Legacy/1.0" || source != UserAgentSourceSetting {
				t.Fatalf("invalid env should fall back to legacy, got %q (%s)", value, source)
			}
		})
	}
}

func TestGetRequestSendsConfiguredUserAgent(t *testing.T) {
	t.Setenv(userAgentEnv, "")
	req, err := getRequest("https://feeds.example.com/podcast.xml")
	if err != nil {
		t.Fatalf("getRequest: %v", err)
	}
	if got := req.Header.Get("User-Agent"); got != DefaultUserAgent {
		t.Fatalf("User-Agent = %q, want %q", got, DefaultUserAgent)
	}
	if values := req.Header.Values("User-Agent"); len(values) != 1 {
		t.Fatalf("User-Agent sent %d times", len(values))
	}

	t.Setenv(userAgentEnv, "Custom/3.1")
	req, err = getRequestWithMethod("HEAD", "https://media.example.com/episode.mp3")
	if err != nil {
		t.Fatalf("getRequestWithMethod: %v", err)
	}
	if got := req.Header.Get("User-Agent"); got != "Custom/3.1" {
		t.Fatalf("User-Agent = %q, want Custom/3.1", got)
	}
}
