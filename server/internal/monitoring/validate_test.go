package monitoring_test

import (
	"testing"

	"serverhub/internal/monitoring"
)

func TestRangeSeconds(t *testing.T) {
	for r, want := range map[string]int64{"1h": 3600, "6h": 21600, "24h": 86400, "7d": 604800} {
		got, ok := monitoring.RangeSeconds(r)
		if !ok || got != want {
			t.Fatalf("RangeSeconds(%q) = %d,%v want %d,true", r, got, ok, want)
		}
	}
	for _, bad := range []string{"", "1H", "30d", "1y", "5m", "1h "} {
		if _, ok := monitoring.RangeSeconds(bad); ok {
			t.Fatalf("RangeSeconds(%q) must fail", bad)
		}
	}
}

func TestParseStepSeconds(t *testing.T) {
	valid := map[string]int64{
		"60": 60, "15": 15, "3600": 3600,
		"30s": 30, "1m": 60, "5m": 300, "1h": 3600,
		"1h30m": 5400, "7d": 604800, "1w": 604800,
	}
	for in, want := range valid {
		got, err := monitoring.ParseStepSeconds(in)
		if err != nil || got != want {
			t.Fatalf("ParseStepSeconds(%q) = %d,%v want %d,nil", in, got, err, want)
		}
	}
	invalid := []string{"", "0", "-5", "-1m", "abc", "1x", "1m30", "m", "--60", "1.5m", " "}
	for _, in := range invalid {
		if _, err := monitoring.ParseStepSeconds(in); err == nil {
			t.Fatalf("ParseStepSeconds(%q) must fail", in)
		}
	}
}

func TestValidateStep(t *testing.T) {
	const window = 3600 // 1h
	for _, ok := range []string{"15", "60", "1m", "5m", "1h"} {
		if err := monitoring.ValidateStep(ok, window); err != nil {
			t.Fatalf("ValidateStep(%q) must pass: %v", ok, err)
		}
	}
	for _, bad := range []string{"0", "-5", "5", "14", "5s", "abc", "", "2h", "7200"} {
		if err := monitoring.ValidateStep(bad, window); err == nil {
			t.Fatalf("ValidateStep(%q) must fail", bad)
		}
	}
	// Bounds scale with the window: 7d accepts an hour step.
	if err := monitoring.ValidateStep("1h", 604800); err != nil {
		t.Fatalf("hour step in 7d window must pass: %v", err)
	}
}

func TestValidateWindow(t *testing.T) {
	if _, err := monitoring.ValidateWindow("1000", "4600"); err != nil {
		t.Fatalf("valid window must pass: %v", err)
	}
	for _, tc := range [][2]string{
		{"", "4600"}, {"1000", ""}, {"abc", "4600"}, {"1000", "abc"},
		{"4600", "1000"}, {"1000", "1000"}, {"0", "99999999"},
	} {
		if _, err := monitoring.ValidateWindow(tc[0], tc[1]); err == nil {
			t.Fatalf("window %v must fail", tc)
		}
	}
}
