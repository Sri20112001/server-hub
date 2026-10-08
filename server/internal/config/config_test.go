package config

import (
	"slices"
	"testing"
)

func TestSplitOrigins(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		valid    []string
		rejected []string
	}{
		{
			name:     "bare production origin",
			in:       "https://imagesoft.in",
			valid:    []string{"https://imagesoft.in"},
			rejected: nil,
		},
		{
			name:     "origin with app path is rejected",
			in:       "https://imagesoft.in/server-hub",
			valid:    nil,
			rejected: []string{"https://imagesoft.in/server-hub"},
		},
		{
			name:     "localhost dev origin",
			in:       "http://localhost:5173",
			valid:    []string{"http://localhost:5173"},
			rejected: nil,
		},
		{
			name:     "comma-separated mixed list",
			in:       "https://a.example, http://localhost:3000 ,https://b.example/app,notaurl,",
			valid:    []string{"https://a.example", "http://localhost:3000"},
			rejected: []string{"https://b.example/app", "notaurl"},
		},
		{
			name:     "empty input",
			in:       "",
			valid:    nil,
			rejected: nil,
		},
		{
			name:     "trailing slash is stripped, not rejected",
			in:       "https://imagesoft.in/",
			valid:    []string{"https://imagesoft.in"},
			rejected: nil,
		},
		{
			name:     "query and fragment rejected",
			in:       "https://x.example/?a=b,https://y.example/#f",
			valid:    nil,
			rejected: []string{"https://x.example/?a=b", "https://y.example/#f"},
		},
		{
			name:     "non-http schemes rejected",
			in:       "ftp://x.example,ws://y.example",
			valid:    nil,
			rejected: []string{"ftp://x.example", "ws://y.example"},
		},
		{
			name:     "duplicates collapsed",
			in:       "https://a.example, https://a.example",
			valid:    []string{"https://a.example"},
			rejected: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			valid, rejected := SplitOrigins(tc.in)
			if !slices.Equal(valid, tc.valid) {
				t.Fatalf("valid = %v, want %v", valid, tc.valid)
			}
			if !slices.Equal(rejected, tc.rejected) {
				t.Fatalf("rejected = %v, want %v", rejected, tc.rejected)
			}
		})
	}
}

func TestResolveFrontendOriginsPrefersOriginVar(t *testing.T) {
	t.Setenv("FRONTEND_ORIGIN", "https://imagesoft.in")
	t.Setenv("FRONTEND_URL", "https://legacy.example/server-hub")
	got := resolveFrontendOrigins()
	if !slices.Equal(got, []string{"https://imagesoft.in"}) {
		t.Fatalf("FRONTEND_ORIGIN must win: %v", got)
	}
}

func TestResolveFrontendOriginsLegacyFallback(t *testing.T) {
	t.Setenv("FRONTEND_ORIGIN", "")
	t.Setenv("FRONTEND_URL", "https://imagesoft.in/server-hub")
	got := resolveFrontendOrigins()
	if !slices.Equal(got, []string{"https://imagesoft.in"}) {
		t.Fatalf("legacy URL path must normalize to its origin: %v", got)
	}
}

func TestMonitoringEnabledFlags(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-that-is-long-enough-32")
	t.Setenv("ADMIN_PASSWORD", "test-password")
	t.Setenv("PROMETHEUS_URL", "http://prometheus:9090")
	t.Setenv("ALERTMANAGER_URL", "http://alertmanager:9093")
	for _, tc := range []struct {
		prom, am   string
		wantP, wantA bool
	}{
		{"", "", true, true},         // unset = enabled (backward compatible)
		{"true", "true", true, true}, // explicit
		{"false", "0", false, false}, // explicit disable
	} {
		t.Setenv("PROMETHEUS_ENABLED", tc.prom)
		t.Setenv("ALERTMANAGER_ENABLED", tc.am)
		cfg := Load()
		if cfg.PrometheusEnabled != tc.wantP || cfg.AlertmanagerEnabled != tc.wantA {
			t.Fatalf("flags %q/%q: got %v/%v", tc.prom, tc.am, cfg.PrometheusEnabled, cfg.AlertmanagerEnabled)
		}
	}
}

func TestMetricsRetentionBounds(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-that-is-long-enough-32")
	t.Setenv("ADMIN_PASSWORD", "test-password")
	for _, tc := range []struct {
		in   string
		want int
	}{
		{"", 30},       // unset = default
		{"30", 30},     // explicit
		{"1", 1},       // minimum
		{"365", 365},   // maximum
		{"0", 30},      // zero can never wipe everything
		{"-5", 30},     // negative falls back
		{"abc", 30},    // garbage falls back
		{"366", 30},    // over maximum falls back
	} {
		t.Setenv("SERVER_METRICS_RETENTION_DAYS", tc.in)
		if got := Load().MetricsRetentionDays; got != tc.want {
			t.Fatalf("retention %q = %d, want %d", tc.in, got, tc.want)
		}
	}
}
