package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"serverhub/internal/config"
	"serverhub/internal/events"
	"serverhub/internal/handlers"
	"serverhub/internal/monitoring"
)

func init() { gin.SetMode(gin.TestMode) }

func newMonitoringHandler(promURL, amURL, webhookSecret string) *handlers.MonitoringHandler {
	cfg := &config.Config{
		PrometheusURL:             promURL,
		PrometheusTimeoutSec:      5,
		AlertmanagerURL:           amURL,
		AlertmanagerTimeoutSec:    5,
		AlertmanagerWebhookSecret: webhookSecret,
	}
	return &handlers.MonitoringHandler{
		Cfg:        cfg,
		Broker:     events.NewBroker(),
		Prometheus: monitoring.NewPrometheusClient(promURL, 5),
		Alertmgr:   monitoring.NewAlertmanagerClient(amURL, 5),
	}
}

// ── Webhook tests ─────────────────────────────────────────────────────────────

func TestAlertmanagerWebhook_ValidPayload(t *testing.T) {
	h := newMonitoringHandler("", "", "")
	r := gin.New()
	r.POST("/webhook", h.AlertmanagerWebhook)

	payload := map[string]interface{}{
		"version":  "4",
		"status":   "firing",
		"receiver": "serverhub",
		"alerts": []map[string]interface{}{
			{
				"status":      "firing",
				"labels":      map[string]string{"alertname": "TestAlert", "severity": "warning"},
				"annotations": map[string]string{"summary": "Test"},
				"startsAt":    "2024-01-01T00:00:00Z",
				"endsAt":      "0001-01-01T00:00:00Z",
				"fingerprint": "abc123",
			},
		},
		"groupLabels":       map[string]string{},
		"commonLabels":      map[string]string{},
		"commonAnnotations": map[string]string{},
		"externalURL":       "http://alertmanager:9093",
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["ok"] != true {
		t.Errorf("expected ok=true, got %v", resp)
	}
}

func TestAlertmanagerWebhook_MalformedPayload(t *testing.T) {
	h := newMonitoringHandler("", "", "")
	r := gin.New()
	r.POST("/webhook", h.AlertmanagerWebhook)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader([]byte(`not json`)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAlertmanagerWebhook_WithSecret_Valid(t *testing.T) {
	secret := "my-webhook-secret"
	h := newMonitoringHandler("", "", secret)
	r := gin.New()
	r.POST("/webhook", h.AlertmanagerWebhook)

	payload := map[string]interface{}{
		"version": "4", "status": "resolved", "receiver": "test",
		"alerts": []interface{}{}, "groupLabels": map[string]string{},
		"commonLabels": map[string]string{}, "commonAnnotations": map[string]string{},
	}
	body, _ := json.Marshal(payload)

	// Sign with HMAC-SHA256 (same as GitHub pattern)
	sig := handlers.SignBody(secret, body)
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", sig)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAlertmanagerWebhook_WithSecret_Invalid(t *testing.T) {
	h := newMonitoringHandler("", "", "correct-secret")
	r := gin.New()
	r.POST("/webhook", h.AlertmanagerWebhook)

	body := []byte(`{"version":"4","status":"firing","alerts":[],"groupLabels":{},"commonLabels":{},"commonAnnotations":{}}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", "sha256=badhash")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAlertmanagerWebhook_ResolvedStatus(t *testing.T) {
	published := make([]string, 0)
	broker := events.NewBroker()
	// Subscribe to capture events
	_ = broker

	h := &handlers.MonitoringHandler{
		Cfg:        &config.Config{},
		Broker:     broker,
		Prometheus: monitoring.NewPrometheusClient("", 5),
		Alertmgr:   monitoring.NewAlertmanagerClient("", 5),
	}
	r := gin.New()
	r.POST("/webhook", h.AlertmanagerWebhook)

	payload := map[string]interface{}{
		"version": "4", "status": "resolved", "receiver": "test",
		"alerts": []map[string]interface{}{
			{"status": "resolved", "labels": map[string]string{"alertname": "HostDown"},
				"annotations": map[string]string{}, "startsAt": "2024-01-01T00:00:00Z",
				"endsAt": "2024-01-01T01:00:00Z", "fingerprint": "xyz"},
		},
		"groupLabels": map[string]string{}, "commonLabels": map[string]string{},
		"commonAnnotations": map[string]string{},
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	_ = published
}

// ── Prometheus status endpoint tests ─────────────────────────────────────────

func TestPrometheusStatus_NotConfigured(t *testing.T) {
	h := newMonitoringHandler("", "", "")
	r := gin.New()
	r.GET("/status", h.PrometheusStatus)

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["available"] != false {
		t.Errorf("expected available=false, got %v", resp["available"])
	}
}

func TestAlertmanagerStatus_NotConfigured(t *testing.T) {
	h := newMonitoringHandler("", "", "")
	r := gin.New()
	r.GET("/status", h.AlertmanagerStatus)

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["available"] != false {
		t.Errorf("expected available=false, got %v", resp["available"])
	}
}

func TestPrometheusQuery_NotConfigured(t *testing.T) {
	h := newMonitoringHandler("", "", "")
	r := gin.New()
	r.GET("/query", h.PrometheusQuery)

	req := httptest.NewRequest(http.MethodGet, "/query?query=up", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestPrometheusQuery_MissingParam(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	h := newMonitoringHandler(srv.URL, "", "")
	r := gin.New()
	r.GET("/query", h.PrometheusQuery)

	req := httptest.NewRequest(http.MethodGet, "/query", nil) // no query param
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestOverview_NotConfigured(t *testing.T) {
	h := newMonitoringHandler("", "", "")
	r := gin.New()
	r.GET("/overview", h.Overview)

	req := httptest.NewRequest(http.MethodGet, "/overview", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["available"] != false {
		t.Errorf("expected available=false, got %v", resp["available"])
	}
}

func TestCreateSilence_NotConfigured(t *testing.T) {
	h := newMonitoringHandler("", "", "")
	r := gin.New()
	r.POST("/silences", h.CreateSilence)

	body := []byte(`{"matchers":[],"startsAt":"2024-01-01T00:00:00Z","endsAt":"2024-01-01T01:00:00Z","createdBy":"test","comment":"test"}`)
	req := httptest.NewRequest(http.MethodPost, "/silences", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestCreateSilence_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	h := newMonitoringHandler("", srv.URL, "")
	r := gin.New()
	r.POST("/silences", h.CreateSilence)

	req := httptest.NewRequest(http.MethodPost, "/silences", bytes.NewReader([]byte(`not json`)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ── Metrics range/step validation ───────────────────────────────────────────

func promRangeStub(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[]}}`))
	}))
}

func TestMetrics_Valid(t *testing.T) {
	srv := promRangeStub(t)
	defer srv.Close()

	h := newMonitoringHandler(srv.URL, "", "")
	r := gin.New()
	r.GET("/metrics", h.Metrics)

	req := httptest.NewRequest(http.MethodGet, "/metrics?metric=cpu&range=1h&step=60", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMetrics_Validation(t *testing.T) {
	srv := promRangeStub(t)
	defer srv.Close()

	h := newMonitoringHandler(srv.URL, "", "")
	r := gin.New()
	r.GET("/metrics", h.Metrics)

	cases := []struct {
		name  string
		query string
	}{
		{"unknown range", "metric=cpu&range=30d&step=60"},
		{"unknown metric", "metric=bogus&range=1h&step=60"},
		{"zero step", "metric=cpu&range=1h&step=0"},
		{"negative step", "metric=cpu&range=1h&step=-5"},
		{"too-small step", "metric=cpu&range=1h&step=5"},
		{"malformed step", "metric=cpu&range=1h&step=abc"},
		{"step exceeds window", "metric=cpu&range=1h&step=2h"},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, "/metrics?"+tc.query, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: expected 400, got %d (%s)", tc.name, w.Code, w.Body.String())
		}
	}
}

func TestMetrics_NotConfigured(t *testing.T) {
	h := newMonitoringHandler("", "", "")
	r := gin.New()
	r.GET("/metrics", h.Metrics)

	req := httptest.NewRequest(http.MethodGet, "/metrics?metric=cpu&range=1h&step=60", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestMetrics_UpstreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	h := newMonitoringHandler(srv.URL, "", "")
	r := gin.New()
	r.GET("/metrics", h.Metrics)

	req := httptest.NewRequest(http.MethodGet, "/metrics?metric=cpu&range=1h&step=60", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Errorf("expected 502, got %d", w.Code)
	}
}

func TestQueryRange_Validation(t *testing.T) {
	srv := promRangeStub(t)
	defer srv.Close()

	h := newMonitoringHandler(srv.URL, "", "")
	r := gin.New()
	r.GET("/query-range", h.PrometheusQueryRange)

	valid := httptest.NewRequest(http.MethodGet, "/query-range?query=up&start=1000&end=4600&step=60", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, valid)
	if w.Code != http.StatusOK {
		t.Fatalf("valid range query: expected 200, got %d (%s)", w.Code, w.Body.String())
	}

	for _, q := range []string{
		"/query-range?query=up&start=4600&end=1000&step=60",
		"/query-range?query=up&start=0&end=99999999&step=60",
		"/query-range?query=up&start=1000&end=4600&step=5",
		"/query-range?query=up&start=abc&end=4600&step=60",
	} {
		req := httptest.NewRequest(http.MethodGet, q, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: expected 400, got %d", q, w.Code)
		}
	}
}
