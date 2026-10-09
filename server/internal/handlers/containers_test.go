package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeDaemon mimics just enough of the Docker Engine API for lifecycle
// tests: version negotiation via HEAD /_ping, and 404 "No such container"
// for inspect/stop. No real daemon is needed, so the test is deterministic
// on any machine (including the Jenkins agent that exposed the 502).
func fakeDaemon(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "HEAD" && r.URL.Path == "/_ping":
			w.Header().Set("Api-Version", "1.47")
			w.WriteHeader(http.StatusOK)
		case r.Method == "GET" && r.URL.Path == "/version":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ApiVersion":"1.47","Version":"28.0.0"}`))
		case strings.HasSuffix(r.URL.Path, "/containers/abc/json"),
			strings.HasSuffix(r.URL.Path, "/containers/abc/stop"):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"No such container: abc"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

// TestContainerStopNotFound pins the error-mapping contract for lifecycle
// operations against a reachable daemon:
//
//   - daemon unreachable → 503 (covered deterministically by
//     TestContainerStopRequiresConfirm via a dead DOCKER_HOST)
//   - daemon reachable, container missing → 404 (this test), consistent
//     with Inspect/Logs/Stats/exec — never 502
//   - anything else from the daemon → 502 (unchanged behavior)
//
// Authorization and confirmation still run first: anon 401, viewer and
// operator 403, admin without confirm 400 — even with a live daemon.
func TestContainerStopNotFound(t *testing.T) {
	srv := fakeDaemon(t)
	defer srv.Close()
	t.Setenv("DOCKER_HOST", "tcp://"+strings.TrimPrefix(srv.URL, "http://"))

	_, _, r := testSetupSecure(t)
	adminC := loginAs(t, r, "admin", "testpass123")
	opC := loginAs(t, r, "op", "testpass123")
	viewC := loginAs(t, r, "view", "testpass123")

	if w := doReq(t, r, "POST", "/api/containers/abc/stop?confirm=true", nil, nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("anon: want 401, got %d", w.Code)
	}
	if w := doReq(t, r, "POST", "/api/containers/abc/stop?confirm=true", nil, viewC); w.Code != http.StatusForbidden {
		t.Fatalf("viewer: want 403, got %d", w.Code)
	}
	if w := doReq(t, r, "POST", "/api/containers/abc/stop?confirm=true", nil, opC); w.Code != http.StatusForbidden {
		t.Fatalf("operator: want 403, got %d", w.Code)
	}
	if w := doReq(t, r, "POST", "/api/containers/abc/stop", nil, adminC); w.Code != http.StatusBadRequest {
		t.Fatalf("admin without confirm: want 400, got %d", w.Code)
	}
	w := doReq(t, r, "POST", "/api/containers/abc/stop?confirm=true", nil, adminC)
	if w.Code != http.StatusNotFound {
		t.Fatalf("admin stop missing container: want 404, got %d %s", w.Code, w.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("response not JSON: %v", err)
	}
	if _, ok := body["operationId"].(string); !ok {
		t.Fatalf("404 must still carry operationId for tracing: %v", body)
	}
}
