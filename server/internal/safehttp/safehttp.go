// Package safehttp is an SSRF-hardened HTTP client for server-side fetches
// of operator/user-controlled URLs (deploy health_url polling, periodic
// health-check probes).
//
// Always-on guardrails (both policies):
//   - http/https scheme only, no userinfo, no control characters.
//   - Cloud metadata IP (169.254.169.254, fd00:ec2::254) always denied —
//     these exfiltrate cloud credentials and are never legitimate targets.
//   - Redirects never followed (redirects bypass destination checks).
//   - Response body capped at 1 MiB, per-request timeout enforced.
//
// Strict policy (EgressStrict, HEALTHCHECK_EGRESS_STRICT=true) additionally
// denies loopback, private, link-local, unspecified and multicast
// destinations after DNS resolution. It is opt-in because monitoring
// internal services is this product's job — blanket private-range blocking
// would disable the health-check feature. Locked-down deployments should
// enable it plus a network-level egress policy.
package safehttp

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// MaxBodyBytes caps any server-side fetched response body.
const MaxBodyBytes = 1 << 20

var metadataIPs = map[string]bool{
	"169.254.169.254": true,
	"fd00:ec2::254":   true,
}

// ValidateURL checks syntax only: http/https scheme, a host, no userinfo.
// Safe to call at project/check save time (no DNS involved).
func ValidateURL(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return fmt.Errorf("url is required")
	}
	if strings.ContainsAny(raw, " \t\r\n") {
		return fmt.Errorf("url must not contain whitespace")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid url: %v", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("url scheme must be http or https")
	}
	if u.Host == "" {
		return fmt.Errorf("url must have a host")
	}
	if u.User != nil {
		return fmt.Errorf("url must not contain credentials")
	}
	return nil
}

// deniedByPolicy reports whether ip is an unacceptable dial target.
func deniedByPolicy(ip net.IP, strict bool) bool {
	if ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}
	if metadataIPs[ip.String()] {
		return true
	}
	if !strict {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
}

// resolveAndCheck resolves host and rejects denied destinations. Every
// resolved IP must pass: a hostname that flaps between good and bad
// addresses (DNS rebinding) fails closed.
func resolveAndCheck(ctx context.Context, host string, strict bool) error {
	// Literal IP fast path (also strips any zone).
	if ip := net.ParseIP(strings.Trim(strings.Split(host, "%")[0], "[]")); ip != nil {
		if deniedByPolicy(ip, strict) {
			return fmt.Errorf("destination %q is not allowed", host)
		}
		return nil
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return fmt.Errorf("cannot resolve %q: %v", host, err)
	}
	if len(ips) == 0 {
		return fmt.Errorf("cannot resolve %q", host)
	}
	for _, ip := range ips {
		if deniedByPolicy(ip, strict) {
			return fmt.Errorf("destination %q resolves to a disallowed address", host)
		}
	}
	return nil
}

// dialer returns a DialContext that re-resolves and re-checks at connect
// time, so TOCTOU between validation and connection fails closed.
func dialer(timeout time.Duration, strict bool) func(context.Context, string, string) (net.Conn, error) {
	base := &net.Dialer{Timeout: timeout}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, fmt.Errorf("invalid address %q", addr)
		}
		if err := resolveAndCheck(ctx, host, strict); err != nil {
			return nil, err
		}
		return base.DialContext(ctx, network, addr)
	}
}

// Client returns an HTTP client with SSRF guardrails and no redirects.
func Client(timeout time.Duration, strict bool) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext:           dialer(timeout, strict),
			TLSHandshakeTimeout:   timeout,
			ResponseHeaderTimeout: timeout,
		},
		// Never follow redirects: a 302 to the metadata service would
		// bypass the destination check performed on the original URL.
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return fmt.Errorf("redirects are not followed")
		},
	}
}

// GetOK fetches rawURL and reports whether it returns 2xx. The body is
// drained (capped) so connections reuse; false covers all failure modes
// (DNS, policy denial, timeout, non-2xx) — callers log the reason via Get.
func GetOK(rawURL string, timeout, budget time.Duration, strict bool) bool {
	ok, _ := Get(rawURL, timeout, budget, strict)
	return ok
}

// Get fetches rawURL until budget expires, polling every 2s. Returns
// (healthy, reason): reason is non-empty only on final failure.
func Get(rawURL string, timeout, budget time.Duration, strict bool) (bool, string) {
	if err := ValidateURL(rawURL); err != nil {
		return false, err.Error()
	}
	client := Client(timeout, strict)
	deadline := time.Now().Add(budget)
	for time.Now().Before(deadline) {
		resp, err := client.Get(rawURL)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, MaxBodyBytes))
		code := resp.StatusCode
		resp.Body.Close()
		if code >= 200 && code < 300 {
			return true, ""
		}
		time.Sleep(2 * time.Second)
	}
	return false, "endpoint did not return 2xx in time"
}
