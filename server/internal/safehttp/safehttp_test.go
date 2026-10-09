package safehttp

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestValidateURL(t *testing.T) {
	for _, ok := range []string{
		"http://localhost:8080/health",
		"https://internal:8443/x",
		"http://app/health?deep=true",
	} {
		if err := ValidateURL(ok); err != nil {
			t.Fatalf("ValidateURL(%q) = %v", ok, err)
		}
	}
	for _, bad := range []string{
		"", "ftp://x/y", "javascript:alert(1)", "http://user:pass@h/",
		"http:///nohost", "http://h/a b", "//noscheme",
	} {
		if err := ValidateURL(bad); err == nil {
			t.Fatalf("ValidateURL(%q) = nil, want error", bad)
		}
	}
}

func TestDeniedByPolicy(t *testing.T) {
	cases := []struct {
		ip            string
		strict        bool
		wantDenied    bool
	}{
		{"169.254.169.254", false, true}, // metadata: always denied
		{"169.254.169.254", true, true},
		{"fd00:ec2::254", false, true},
		{"8.8.8.8", false, false},
		{"8.8.8.8", true, false},
		{"127.0.0.1", false, false}, // loopback allowed by default (monitoring)
		{"127.0.0.1", true, true},   // denied in strict mode
		{"10.1.2.3", false, false},
		{"10.1.2.3", true, true},
		{"::1", true, true},
		{"::ffff:127.0.0.1", true, true},  // IPv4-mapped IPv6 loopback
		{"::ffff:10.0.0.1", true, true},   // IPv4-mapped private
		{"::ffff:8.8.8.8", true, false},   // IPv4-mapped public is fine
		{"0x7f.0.0.1", true, true},        // hex-form loopback
		{"2130706433", false, false},      // decimal form: unparseable → see below
		{"0.0.0.0", false, true},          // unspecified always denied
		{"224.0.0.1", false, true},        // multicast always denied
	}
	for _, tc := range cases {
		ip := net.ParseIP(tc.ip)
		if ip == nil {
			// Unparseable literals never reach the dialer as IPs: the
			// hostname path fails closed on unresolvable names.
			continue
		}
		if got := deniedByPolicy(ip, tc.strict); got != tc.wantDenied {
			t.Fatalf("deniedByPolicy(%s -> %v, strict=%v) = %v, want %v",
				tc.ip, ip, tc.strict, got, tc.wantDenied)
		}
	}
}

func TestResolveLocalhostStrict(t *testing.T) {
	// localhost always resolves locally: strict mode must refuse it,
	// default mode allows it (monitoring use-case).
	if err := resolveAndCheck(t.Context(), "localhost", true); err == nil {
		t.Fatal("strict: localhost must be denied")
	}
	if err := resolveAndCheck(t.Context(), "localhost", false); err != nil {
		t.Fatalf("default: localhost must be allowed: %v", err)
	}
}

func TestBypassRepresentationsFailClosed(t *testing.T) {
	// Every unusual-but-loopback representation must fail closed in
	// strict mode — whether parsed as an IP literal or rejected at DNS.
	for _, host := range []string{
		"127.0.0.1", "0x7f.0.0.1", "0177.0.0.1", "2130706433",
		"::ffff:127.0.0.1", "[::ffff:127.0.0.1]", "localhost",
		"169.254.169.254", "[fd00::ec2:254]",
	} {
		h := host
		if len(h) > 0 && h[0] == '[' {
			h = h[1 : len(h)-1]
		}
		if ip := net.ParseIP(h); ip != nil {
			if !deniedByPolicy(ip, true) {
				t.Fatalf("strict must deny literal %q (parsed as %v)", host, ip)
			}
			continue
		}
		// Not an IP literal: DNS must fail closed (unresolvable) or the
		// test environment resolves it — either way strict must deny or
		// the name must not resolve to a public address silently.
		if err := resolveAndCheck(t.Context(), h, true); err == nil {
			// Resolved and allowed: only acceptable for names that do
			// not point at local/private space — verify explicitly.
			ips, rerr := net.DefaultResolver.LookupIP(t.Context(), "ip", h)
			if rerr != nil {
				t.Fatalf("%q unexpectedly allowed without resolution", host)
			}
			for _, ip := range ips {
				if deniedByPolicy(ip, true) {
					t.Fatalf("%q resolved to %v but was allowed", host, ip)
				}
			}
		}
	}
}

func TestGetOKLoopbackAllowedByDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	if !GetOK(srv.URL, 2*time.Second, 5*time.Second, false) {
		t.Fatal("default policy must reach loopback test server")
	}
	if GetOK(srv.URL, 2*time.Second, 5*time.Second, true) {
		t.Fatal("strict policy must deny loopback")
	}
}

func TestNoRedirectFollow(t *testing.T) {
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer final.Close()
	redir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL, http.StatusFound)
	}))
	defer redir.Close()
	if GetOK(redir.URL, 2*time.Second, 5*time.Second, false) {
		t.Fatal("redirects must not be followed")
	}
}
