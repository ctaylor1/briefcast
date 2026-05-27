package service

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	outboundAllowPrivateEnv           = "BRIEFCAST_ALLOW_PRIVATE_OUTBOUND_URLS"
	outboundAllowPrivateBriefpointEnv = "BRIEFCAST_ALLOW_PRIVATE_BRIEFPOINT_URLS"
	outboundAllowPrivateLLMEnv        = "BRIEFCAST_ALLOW_PRIVATE_LLM_URLS"
	outboundMaxResponseBytesEnv       = "BRIEFCAST_MAX_OUTBOUND_RESPONSE_BYTES"
	defaultOutboundMaxResponseBytes   = int64(20 * 1024 * 1024)
)

type outboundURLPurpose string

const (
	outboundPurposeHTTP       outboundURLPurpose = "http request"
	outboundPurposeBriefpoint outboundURLPurpose = "briefpoint"
	outboundPurposeLLM        outboundURLPurpose = "llm"
)

type outboundURLPolicy struct {
	purpose           outboundURLPurpose
	allowPrivateHosts bool
}

var lookupOutboundIPAddrs = net.DefaultResolver.LookupIPAddr

func validateOutboundURL(rawURL string, purpose outboundURLPurpose) (*url.URL, error) {
	return validateOutboundURLWithPolicy(rawURL, outboundPolicyForPurpose(purpose))
}

func outboundPolicyForPurpose(purpose outboundURLPurpose) outboundURLPolicy {
	allowPrivate := getEnvBool(outboundAllowPrivateEnv, false)
	if purpose == outboundPurposeBriefpoint {
		allowPrivate = getEnvBool(outboundAllowPrivateBriefpointEnv, true)
	}
	if purpose == outboundPurposeLLM {
		allowPrivate = getEnvBool(outboundAllowPrivateLLMEnv, true)
	}
	return outboundURLPolicy{
		purpose:           purpose,
		allowPrivateHosts: allowPrivate,
	}
}

func validateOutboundURLWithPolicy(rawURL string, policy outboundURLPolicy) (*url.URL, error) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return nil, outboundURLPolicyError(policy.purpose, "URL is empty")
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil, outboundURLPolicyError(policy.purpose, "URL is invalid")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, outboundURLPolicyError(policy.purpose, "scheme must be http or https")
	}
	if parsed.Host == "" || parsed.Hostname() == "" {
		return nil, outboundURLPolicyError(policy.purpose, "host is required")
	}
	if parsed.User != nil {
		return nil, outboundURLPolicyError(policy.purpose, "embedded credentials are not allowed")
	}
	if !policy.allowPrivateHosts && isPrivateOutboundHost(parsed.Hostname()) {
		return nil, outboundURLPolicyError(policy.purpose, "private or loopback hosts are not allowed")
	}
	return parsed, nil
}

func outboundHTTPTransport(purpose outboundURLPurpose) *http.Transport {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		base = &http.Transport{}
	}
	transport := base.Clone()
	baseDialContext := transport.DialContext
	if baseDialContext == nil {
		dialer := &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}
		baseDialContext = dialer.DialContext
	}

	policy := outboundPolicyForPurpose(purpose)
	transport.DialContext = func(ctx context.Context, network string, address string) (net.Conn, error) {
		dialAddress, err := outboundDialAddress(ctx, network, address, policy)
		if err != nil {
			return nil, err
		}
		return baseDialContext(ctx, network, dialAddress)
	}
	return transport
}

func outboundDialAddress(ctx context.Context, network string, address string, policy outboundURLPolicy) (string, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return "", err
	}
	host = strings.Trim(host, "[]")
	if policy.allowPrivateHosts {
		return address, nil
	}

	if ip := net.ParseIP(host); ip != nil {
		if isPrivateOutboundIP(ip) {
			return "", outboundURLPolicyError(policy.purpose, "private or loopback hosts are not allowed")
		}
		return net.JoinHostPort(ip.String(), port), nil
	}

	resolved, err := lookupOutboundIPAddrs(ctx, host)
	if err != nil {
		return "", err
	}
	if len(resolved) == 0 {
		return "", outboundURLPolicyError(policy.purpose, "host did not resolve")
	}

	var selected net.IP
	for _, addr := range resolved {
		ip := addr.IP
		if isPrivateOutboundIP(ip) {
			return "", outboundURLPolicyError(policy.purpose, "private or loopback hosts are not allowed")
		}
		if selected == nil && ipMatchesDialNetwork(ip, network) {
			selected = ip
		}
	}
	if selected == nil {
		return "", outboundURLPolicyError(policy.purpose, "host did not resolve to an address usable by the dial network")
	}

	return net.JoinHostPort(selected.String(), port), nil
}

func outboundURLPolicyError(purpose outboundURLPurpose, reason string) error {
	return fmt.Errorf("outbound URL rejected for %s: %s", purpose, reason)
}

func isPrivateOutboundHost(host string) bool {
	normalized := strings.Trim(strings.ToLower(host), "[]")
	if normalized == "" {
		return true
	}
	if normalized == "localhost" || strings.HasSuffix(normalized, ".localhost") {
		return true
	}
	if ip := net.ParseIP(normalized); ip != nil {
		return isPrivateOutboundIP(ip)
	}
	return false
}

func isPrivateOutboundIP(ip net.IP) bool {
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified()
}

func ipMatchesDialNetwork(ip net.IP, network string) bool {
	switch {
	case strings.HasSuffix(network, "4"):
		return ip.To4() != nil
	case strings.HasSuffix(network, "6"):
		return ip.To4() == nil
	default:
		return true
	}
}

func maxOutboundResponseBytes() int64 {
	raw := strings.TrimSpace(os.Getenv(outboundMaxResponseBytesEnv))
	if raw == "" {
		return defaultOutboundMaxResponseBytes
	}
	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || parsed <= 0 {
		return defaultOutboundMaxResponseBytes
	}
	return parsed
}

func readBoundedOutboundBody(reader io.Reader, contentLength int64) ([]byte, error) {
	maxBytes := maxOutboundResponseBytes()
	if contentLength > maxBytes {
		return nil, fmt.Errorf("outbound response exceeds %d bytes", maxBytes)
	}

	body, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("outbound response exceeds %d bytes", maxBytes)
	}
	return body, nil
}
