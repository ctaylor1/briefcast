package service

import (
	"os"
	"strings"
	"sync"

	"github.com/ctaylor1/briefcast/db"
)

const (
	userAgentEnv = "BRIEFCAST_USER_AGENT"
	// DefaultUserAgent identifies Briefcast honestly to feed, media and chart
	// hosts, following the podcast-app convention "Name/version (+info URL)".
	DefaultUserAgent   = "Briefcast/1.0 (+https://github.com/ctaylor1/briefcast)"
	maxUserAgentLength = 256

	// UserAgentSourceEnv and the other sources say where the effective value came from.
	UserAgentSourceEnv     = "env"
	UserAgentSourceSetting = "legacy_setting"
	UserAgentSourceDefault = "default"
)

var userAgentWarnings sync.Map

// OutboundUserAgent returns the User-Agent sent on feed, media, image and
// chart requests and where it came from. BRIEFCAST_USER_AGENT wins; a value
// saved through the legacy settings form is kept for installs that used it;
// otherwise DefaultUserAgent. Invalid values are ignored with a warning so a
// typo cannot break every outbound request.
func OutboundUserAgent(legacySetting string) (string, string) {
	if value, ok := validUserAgent(os.Getenv(userAgentEnv), userAgentEnv); ok {
		return value, UserAgentSourceEnv
	}
	if value, ok := validUserAgent(legacySetting, "legacy user agent setting"); ok {
		return value, UserAgentSourceSetting
	}
	return DefaultUserAgent, UserAgentSourceDefault
}

// CurrentOutboundUserAgent resolves the User-Agent using the stored setting.
func CurrentOutboundUserAgent() (string, string) {
	legacy := ""
	if db.DB != nil {
		legacy = db.GetOrCreateSetting().UserAgent
	}
	return OutboundUserAgent(legacy)
}

func validUserAgent(raw string, origin string) (string, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", false
	}
	reason := ""
	if len(value) > maxUserAgentLength {
		reason = "longer than 256 characters"
	} else {
		for _, r := range value {
			// Printable ASCII only: header values must not carry control
			// characters (CR/LF would allow header injection).
			if r < 0x20 || r > 0x7e {
				reason = "contains non-printable or non-ASCII characters"
				break
			}
		}
	}
	if reason != "" {
		if _, seen := userAgentWarnings.LoadOrStore(origin+"\x00"+value, true); !seen && Logger != nil {
			Logger.Warnw("ignoring invalid user agent", "source", origin, "reason", reason)
		}
		return "", false
	}
	return value, true
}
