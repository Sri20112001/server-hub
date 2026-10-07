package monitoring_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"serverhub/internal/monitoring"
)

// ── Prometheus client tests ───────────────────────────────────────────────────

func TestPrometheusClient_Available(t *testing.T) {
	c := monitoring.NewPrometheusClient("", 10)
	if c.Available() {
		t.Fatal("empty URL should not be available")
	}
	c2 := monitoring.NewPrometheusClient("http://localhost:9090", 10)
	if !c2.Available() {
		t.Fatal("non-empty URL should be available")
	}
}

func TestPrometheusClient_Query_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("query") == "" {
			t.Error("query param missing")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "success",
			"data": map[string]interface{}{
				"resultType": "vector",
				"result": []map[string]interface{}{
					{"metric": map[string]string{}, "value": []interface{}{1234567890, "42.5"}},
				},
			},
		})
	}))
	defer srv.Close()

	c := monitoring.NewPrometheusClient(srv.URL, 5)
	data, err := c.Query(context.Background(), `up`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	v, ok := monitoring.ScalarFloat(data)
	if !ok {
		t.Fatal("ScalarFloat returned false")
	}
	if v != 42.5 {
		t.Errorf("expected 42.5, got %f", v)
	}
}

func TestPrometheusClient_Query_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":    "error",
			"errorType": "bad_data",
			"error":     "invalid query",
		})
	}))
	defer srv.Close()

	c := monitoring.NewPrometheusClient(srv.URL, 5)
	_, err := c.Query(context.Background(), `bad{`)
	if err == nil {
		t.Fatal("expected error for prometheus error status")
	}
}

func TestPrometheusClient_Unavailable(t *testing.T) {
	c := monitoring.NewPrometheusClient("", 5)
	_, err := c.Query(context.Background(), `up`)
	if err == nil {
		t.Fatal("expected error when not configured")
	}
}

func TestPrometheusClient_Status_Healthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/-/healthy" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Prometheus is Healthy.\n"))
		}
	}))
	defer srv.Close()

	c := monitoring.NewPrometheusClient(srv.URL, 5)
	ok, msg := c.Status(context.Background())
	if !ok {
		t.Errorf("expected healthy, got: %s", msg)
	}
}

func TestPrometheusClient_Status_Unhealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := monitoring.NewPrometheusClient(srv.URL, 5)
	ok, _ := c.Status(context.Background())
	if ok {
		t.Error("expected unhealthy")
	}
}

func TestPrometheusClient_Targets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/targets" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "success",
			"data": map[string]interface{}{
				"activeTargets":  []interface{}{},
				"droppedTargets": []interface{}{},
			},
		})
	}))
	defer srv.Close()

	c := monitoring.NewPrometheusClient(srv.URL, 5)
	data, err := c.Targets(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data == nil {
		t.Fatal("expected non-nil data")
	}
}

func TestPrometheusClient_QueryRange(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query_range" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("query") == "" || q.Get("start") == "" || q.Get("end") == "" {
			t.Error("missing required params")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "success",
			"data": map[string]interface{}{
				"resultType": "matrix",
				"result":     []interface{}{},
			},
		})
	}))
	defer srv.Close()

	c := monitoring.NewPrometheusClient(srv.URL, 5)
	_, err := c.QueryRange(context.Background(), `up`, "0", "3600", "60")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ── Alertmanager client tests ─────────────────────────────────────────────────

func TestAlertmanagerClient_Available(t *testing.T) {
	c := monitoring.NewAlertmanagerClient("", 10)
	if c.Available() {
		t.Fatal("empty URL should not be available")
	}
}

func TestAlertmanagerClient_Alerts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/alerts" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := monitoring.NewAlertmanagerClient(srv.URL, 5)
	data, err := c.Alerts(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != `[]` {
		t.Errorf("unexpected data: %s", data)
	}
}

func TestAlertmanagerClient_Silences(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := monitoring.NewAlertmanagerClient(srv.URL, 5)
	data, err := c.Silences(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data == nil {
		t.Fatal("expected non-nil data")
	}
}

func TestAlertmanagerClient_CreateSilence(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"silenceID":"abc-123"}`))
	}))
	defer srv.Close()

	c := monitoring.NewAlertmanagerClient(srv.URL, 5)
	body := []byte(`{"matchers":[],"startsAt":"2024-01-01T00:00:00Z","endsAt":"2024-01-01T01:00:00Z","createdBy":"test","comment":"test"}`)
	data, err := c.CreateSilence(context.Background(), body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data == nil {
		t.Fatal("expected non-nil response")
	}
}

func TestAlertmanagerClient_DeleteSilence(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := monitoring.NewAlertmanagerClient(srv.URL, 5)
	if err := c.DeleteSilence(context.Background(), "abc-123"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAlertmanagerClient_Status_Healthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer srv.Close()

	c := monitoring.NewAlertmanagerClient(srv.URL, 5)
	ok, msg := c.Status(context.Background())
	if !ok {
		t.Errorf("expected healthy, got: %s", msg)
	}
}

func TestAlertmanagerClient_Unavailable(t *testing.T) {
	c := monitoring.NewAlertmanagerClient("", 5)
	_, err := c.Alerts(context.Background())
	if err == nil {
		t.Fatal("expected error when not configured")
	}
}

// ── ScalarFloat tests ─────────────────────────────────────────────────────────

func TestScalarFloat_Valid(t *testing.T) {
	data := json.RawMessage(`{"resultType":"vector","result":[{"metric":{},"value":[1234567890,"31.5"]}]}`)
	v, ok := monitoring.ScalarFloat(data)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if v != 31.5 {
		t.Errorf("expected 31.5, got %f", v)
	}
}

func TestScalarFloat_Empty(t *testing.T) {
	data := json.RawMessage(`{"resultType":"vector","result":[]}`)
	_, ok := monitoring.ScalarFloat(data)
	if ok {
		t.Fatal("expected ok=false for empty result")
	}
}

func TestScalarFloat_Invalid(t *testing.T) {
	_, ok := monitoring.ScalarFloat(json.RawMessage(`not json`))
	if ok {
		t.Fatal("expected ok=false for invalid JSON")
	}
}
