package healthhttp

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func serve(t *testing.T, h gin.HandlerFunc, path string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET(path, h)
	req := httptest.NewRequest("GET", path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestLiveAlways200(t *testing.T) {
	start := time.Now()
	if w := serve(t, LiveHandler(start), "/health/live"); w.Code != http.StatusOK {
		t.Fatalf("live with db down: want 200, got %d", w.Code)
	}
}

func TestReadyReflectsDependency(t *testing.T) {
	start := time.Now()
	ok := func() error { return nil }
	down := func() error { return errors.New("connect refused") }

	if w := serve(t, ReadyHandler(ok, start), "/health/ready"); w.Code != http.StatusOK {
		t.Fatalf("ready db up: want 200, got %d %s", w.Code, w.Body.String())
	}
	w := serve(t, ReadyHandler(down, start), "/health/ready")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("ready db down: want 503, got %d %s", w.Code, w.Body.String())
	}
	if body := w.Body.String(); !contains(body, "degraded") || !contains(body, "down") {
		t.Fatalf("ready db down body must report degraded/down: %s", body)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
