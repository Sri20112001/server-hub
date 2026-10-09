package middleware

import "testing"

// Regression test: single-use exec capability URLs must never produce
// per-hit app_logs rows — the token in the path would otherwise leak to
// every viewer able to read GET /api/logs.
func TestSkipRequestLog(t *testing.T) {
	skipped := []string{
		"/health",
		"/health/live",
		"/health/ready",
		"/server-hub/api/events",
		"/server-hub/api/exec/0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	}
	for _, p := range skipped {
		if !skipRequestLog(p) {
			t.Fatalf("path %q must be skipped from request logging", p)
		}
	}
	logged := []string{
		"/server-hub/api/projects",
		"/server-hub/api/containers/abc/start",
		"/server-hub/api/exec", // prefix alone is not a capability URL
	}
	for _, p := range logged {
		if skipRequestLog(p) {
			t.Fatalf("path %q must still be request-logged", p)
		}
	}
}
