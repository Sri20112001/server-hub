package handlers_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"serverhub/internal/config"
	"serverhub/internal/database"
	"serverhub/internal/events"
	"serverhub/internal/handlers"
	"serverhub/internal/middleware"
	"serverhub/internal/monitoring"
	"serverhub/internal/testdb"
)

// ── Server-scoped Prometheus API (Phase 3C) ─────────────────────────────────

// fakeProm records incoming PromQL and serves scripted matrix/vector bodies.
type fakeProm struct {
	t      *testing.T
	mu     sync.Mutex
	hits   int
	queries []string
	mode   string // "matrix", "vector", "malformed", "error500", "hang"
}

func (f *fakeProm) handler(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.hits++
	f.queries = append(f.queries, r.URL.Query().Get("query"))
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch f.mode {
	case "error500":
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"status":"error","errorType":"internal","error":"boom"}`))
	case "malformed":
		w.Write([]byte(`{not json`))
	case "vector":
		w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1700000000,"42.5"]}]}}`))
	default: // matrix
		w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[{"metric":{},"values":[[1700000000,"1.5"],[1700000060,"NaN"]]}]}}`))
	}
}

func (f *fakeProm) hitCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hits
}

func (f *fakeProm) lastQuery() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.queries) == 0 {
		return ""
	}
	return f.queries[len(f.queries)-1]
}

func serverPromSetup(t *testing.T, mode string) (*gin.Engine, *database.DB, *fakeProm) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testdb.Open(t)
	seed := func(user, pass, role string) {
		hash, _ := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.MinCost)
		if _, err := db.Exec(`INSERT INTO users (username, password_hash, role) VALUES (?,?,?)`,
			user, string(hash), role); err != nil {
			t.Fatal(err)
		}
	}
	seed("admin", "testpass123", "admin")
	seed("op", "testpass123", "operator")
	seed("view", "testpass123", "viewer")

	fp := &fakeProm{t: t, mode: mode}
	srv := httptest.NewServer(http.HandlerFunc(fp.handler))
	t.Cleanup(srv.Close)

	cfg := &config.Config{
		JWTSecret:            "test-jwt-secret-min-32-chars-long!!",
		PrometheusURL:        srv.URL,
		PrometheusTimeoutSec: 2,
	}
	monH := &handlers.MonitoringHandler{
		DB:         db,
		Cfg:        cfg,
		Broker:     events.NewBroker(),
		Prometheus: monitoring.NewPrometheusClient(srv.URL, 2),
		Alertmgr:   monitoring.NewAlertmanagerClient("", 2),
	}
	r := gin.New()
	authH := &handlers.AuthHandler{DB: db, Cfg: cfg}
	limiter := middleware.NewLoginLimiter()
	r.POST("/api/auth/login", middleware.LoginRateLimit(nil, limiter), authH.Login)
	authd := middleware.AuthRequired(cfg.JWTSecret)
	viewer := r.Group("/api", authd, middleware.RequireRole(middleware.RoleViewer))
	viewer.GET("/servers/:id/prometheus/metrics", monH.ServerPrometheusMetrics)
	viewer.GET("/servers/:id/prometheus/metrics/latest", monH.ServerPrometheusLatest)
	return r, db, fp
}

func seedManagedServer(t *testing.T, db *database.DB, name string) uint {
	t.Helper()
	id, err := db.InsertID(`INSERT INTO managed_servers
		(name,hostname,status,agent_status,created_at,updated_at)
		VALUES ($1,'h','UNKNOWN','UNKNOWN',NOW(),NOW())`, name)
	if err != nil {
		t.Fatal(err)
	}
	return uint(id)
}

func loginToken(t *testing.T, r *gin.Engine, user, pass string) string {
	t.Helper()
	body := fmt.Sprintf(`{"username":%q,"password":%q}`, user, pass)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("login %s: %d %s", user, w.Code, w.Body.String())
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || out.Token == "" {
		t.Fatalf("no token: %v %s", err, w.Body.String())
	}
	return out.Token
}

func authed(t *testing.T, r *gin.Engine, method, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// ── RBAC: viewer/operator/admin read; anonymous rejected ─────────────────────

func TestServerProm_RBAC(t *testing.T) {
	r, db, _ := serverPromSetup(t, "matrix")
	sid := seedManagedServer(t, db, "s1")

	for _, user := range []string{"view", "op", "admin"} {
		tok := loginToken(t, r, user, "testpass123")
		w := authed(t, r, "GET",
			fmt.Sprintf("/api/servers/%d/prometheus/metrics?metric=cpu_usage&range=1h&step=60", sid), tok)
		if w.Code != http.StatusOK {
			t.Errorf("%s: expected 200, got %d (%s)", user, w.Code, w.Body.String())
		}
		w = authed(t, r, "GET",
			fmt.Sprintf("/api/servers/%d/prometheus/metrics/latest?metric=cpu_usage", sid), tok)
		if w.Code != http.StatusOK {
			t.Errorf("%s latest: expected 200, got %d (%s)", user, w.Code, w.Body.String())
		}
	}

	// No token → 401.
	w := authed(t, r, "GET",
		fmt.Sprintf("/api/servers/%d/prometheus/metrics?metric=cpu_usage&range=1h&step=60", sid), "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("anonymous: expected 401, got %d", w.Code)
	}
	// Garbage token → 401.
	w = authed(t, r, "GET",
		fmt.Sprintf("/api/servers/%d/prometheus/metrics/latest?metric=cpu_usage", sid), "bogus")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("bogus token: expected 401, got %d", w.Code)
	}
}

// ── Server existence is checked before Prometheus is touched ─────────────────

func TestServerProm_UnknownServer(t *testing.T) {
	r, _, fp := serverPromSetup(t, "matrix")
	tok := loginToken(t, r, "view", "testpass123")

	for _, path := range []string{
		"/api/servers/999999/prometheus/metrics?metric=cpu_usage&range=1h&step=60",
		"/api/servers/999999/prometheus/metrics/latest?metric=cpu_usage",
	} {
		w := authed(t, r, "GET", path, tok)
		if w.Code != http.StatusNotFound {
			t.Errorf("%s: expected 404, got %d", path, w.Code)
		}
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body["error"] != "server not found" {
			t.Errorf("%s: expected not-found behavior, got %s", path, w.Body.String())
		}
	}
	if got := fp.hitCount(); got != 0 {
		t.Fatalf("Prometheus must not be queried for unknown servers (hits=%d)", got)
	}

	// Malformed id → 400, also without touching Prometheus.
	w := authed(t, r, "GET", "/api/servers/abc/prometheus/metrics?metric=cpu_usage&range=1h&step=60", tok)
	if w.Code != http.StatusBadRequest {
		t.Errorf("bad id: expected 400, got %d", w.Code)
	}
	if got := fp.hitCount(); got != 0 {
		t.Fatalf("Prometheus must not be queried for bad ids (hits=%d)", got)
	}
}

// ── Metric allowlist + injection resistance ──────────────────────────────────

func TestServerProm_MetricValidation(t *testing.T) {
	r, db, fp := serverPromSetup(t, "matrix")
	tok := loginToken(t, r, "view", "testpass123")
	sid := seedManagedServer(t, db, "s1")
	base := fmt.Sprintf("/api/servers/%d/prometheus/metrics", sid)

	for _, bad := range []string{
		`cpu_usage"} or vector(1) or {`,
		`cpu_usage&match[]=up`,
		`up`,
		`node_cpu_seconds_total`,
		``,
		`CPU_USAGE`,
	} {
		w := authed(t, r, "GET", base+"?metric="+url.QueryEscape(bad)+"&range=1h&step=60", tok)
		if w.Code != http.StatusBadRequest {
			t.Errorf("metric %q: expected 400, got %d", bad, w.Code)
		}
	}
	if got := fp.hitCount(); got != 0 {
		t.Fatalf("rejected metrics must not reach Prometheus (hits=%d)", got)
	}
}

// ── Generated PromQL structure per metric ────────────────────────────────────

func TestServerProm_QueryGeneration(t *testing.T) {
	r, db, fp := serverPromSetup(t, "matrix")
	tok := loginToken(t, r, "view", "testpass123")
	sid := seedManagedServer(t, db, "s1")
	sel := fmt.Sprintf(`server_id="%d"`, sid)

	frags := map[string][]string{
		"cpu_usage":        {`node_cpu_seconds_total{mode="idle",` + sel + `}`, `rate(`, `* 100`},
		"memory_usage":     {`node_memory_MemAvailable_bytes{` + sel + `}`, `node_memory_MemTotal_bytes{` + sel + `}`},
		"disk_usage":       {`node_filesystem_avail_bytes{` + sel + `,mountpoint="/"}`, `node_filesystem_size_bytes{` + sel + `,mountpoint="/"}`},
		"load_1m":          {`node_load1{` + sel + `}`},
		"network_receive":  {`node_network_receive_bytes_total{` + sel + `,device!="lo"}`},
		"network_transmit": {`node_network_transmit_bytes_total{` + sel + `,device!="lo"}`},
	}
	for metric, want := range frags {
		before := fp.hitCount()
		w := authed(t, r, "GET",
			fmt.Sprintf("/api/servers/%d/prometheus/metrics?metric=%s&range=1h&step=60", sid, metric), tok)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: expected 200, got %d (%s)", metric, w.Code, w.Body.String())
		}
		if fp.hitCount() != before+1 {
			t.Fatalf("%s: expected exactly one upstream query", metric)
		}
		q := fp.lastQuery()
		for _, frag := range want {
			if !strings.Contains(q, frag) {
				t.Errorf("%s: query missing %q: %s", metric, frag, q)
			}
		}
	}
}

func TestServerProm_QueryScopedToServer(t *testing.T) {
	r, db, fp := serverPromSetup(t, "matrix")
	tok := loginToken(t, r, "view", "testpass123")
	sid := seedManagedServer(t, db, "s1")
	sel := fmt.Sprintf(`server_id="%d"`, sid)

	w := authed(t, r, "GET",
		fmt.Sprintf("/api/servers/%d/prometheus/metrics?metric=memory_usage&range=6h&step=5m", sid), tok)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
	}
	q := fp.lastQuery()
	if !strings.Contains(q, sel) {
		t.Fatalf("query missing server selector %s: %s", sel, q)
	}
	if strings.Contains(q, "match[]") || strings.Contains(q, "vector(1)") {
		t.Fatalf("client input leaked into PromQL: %s", q)
	}
	var body struct {
		ServerID string `json:"serverId"`
		Metric   string `json:"metric"`
		Range    string `json:"range"`
		Step     string `json:"step"`
		Series   []struct {
			Name   string `json:"name"`
			Values []struct {
				Timestamp int64    `json:"timestamp"`
				Value     *float64 `json:"value"`
			} `json:"values"`
		} `json:"series"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("bad response shape: %v", err)
	}
	if body.ServerID != fmt.Sprintf("%d", sid) || body.Metric != "memory_usage" ||
		body.Range != "6h" || body.Step != "5m" {
		t.Fatalf("bad envelope: %+v", body)
	}
	if len(body.Series) != 1 || body.Series[0].Name != "memory_usage" {
		t.Fatalf("expected one named series, got %+v", body.Series)
	}
	if len(body.Series[0].Values) != 2 {
		t.Fatalf("expected 2 points, got %+v", body.Series[0].Values)
	}
	if body.Series[0].Values[0].Value == nil || *body.Series[0].Values[0].Value != 1.5 {
		t.Fatalf("bad first value: %+v", body.Series[0].Values[0])
	}
	if body.Series[0].Values[1].Value != nil {
		t.Fatalf("NaN must normalize to null: %+v", body.Series[0].Values[1])
	}
}

// ── Range/step matrix + latest + upstream failures ───────────────────────────

func TestServerProm_RangeStep(t *testing.T) {
	r, db, _ := serverPromSetup(t, "matrix")
	tok := loginToken(t, r, "view", "testpass123")
	sid := seedManagedServer(t, db, "s1")

	for _, rng := range []string{"1h", "6h", "24h", "7d"} {
		w := authed(t, r, "GET",
			fmt.Sprintf("/api/servers/%d/prometheus/metrics?metric=disk_usage&range=%s&step=60", sid, rng), tok)
		if w.Code != http.StatusOK {
			t.Errorf("range %s: expected 200, got %d", rng, w.Code)
		}
	}
	for _, tc := range []struct {
		name, query string
	}{
		{"bad range", "metric=cpu_usage&range=30d&step=60"},
		{"zero step", "metric=cpu_usage&range=1h&step=0"},
		{"small step", "metric=cpu_usage&range=1h&step=5"},
		{"bad step", "metric=cpu_usage&range=1h&step=soon"},
		{"huge step", "metric=cpu_usage&range=1h&step=2h"},
		{"valid duration step", "metric=cpu_usage&range=1h&step=1m"},
	} {
		w := authed(t, r, "GET",
			fmt.Sprintf("/api/servers/%d/prometheus/metrics?%s", sid, tc.query), tok)
		want := http.StatusBadRequest
		if tc.name == "valid duration step" {
			want = http.StatusOK
		}
		if w.Code != want {
			t.Errorf("%s: expected %d, got %d (%s)", tc.name, want, w.Code, w.Body.String())
		}
	}
}

func TestServerProm_Latest(t *testing.T) {
	r, db, _ := serverPromSetup(t, "vector")
	tok := loginToken(t, r, "view", "testpass123")
	sid := seedManagedServer(t, db, "s1")

	w := authed(t, r, "GET",
		fmt.Sprintf("/api/servers/%d/prometheus/metrics/latest?metric=load_1m", sid), tok)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
	}
	var body struct {
		ServerID  string   `json:"serverId"`
		Metric    string   `json:"metric"`
		Value     *float64 `json:"value"`
		Timestamp *int64   `json:"timestamp"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("bad shape: %v", err)
	}
	if body.Value == nil || *body.Value != 42.5 || body.Timestamp == nil || *body.Timestamp != 1700000000 {
		t.Fatalf("bad latest payload: %+v", body)
	}
}

func TestServerProm_UpstreamFailures(t *testing.T) {
	for _, mode := range []string{"error500", "malformed"} {
		r, db, _ := serverPromSetup(t, mode)
		tok := loginToken(t, r, "view", "testpass123")
		sid := seedManagedServer(t, db, "s1")

		w := authed(t, r, "GET",
			fmt.Sprintf("/api/servers/%d/prometheus/metrics?metric=cpu_usage&range=1h&step=60", sid), tok)
		if w.Code != http.StatusBadGateway {
			t.Errorf("%s range: expected 502, got %d", mode, w.Code)
		}
		w = authed(t, r, "GET",
			fmt.Sprintf("/api/servers/%d/prometheus/metrics/latest?metric=cpu_usage", sid), tok)
		if w.Code != http.StatusBadGateway {
			t.Errorf("%s latest: expected 502, got %d", mode, w.Code)
		}
	}
}

func TestServerProm_NotConfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testdb.Open(t)
	cfg := &config.Config{PrometheusTimeoutSec: 2}
	monH := &handlers.MonitoringHandler{
		DB:         db,
		Cfg:        cfg,
		Broker:     events.NewBroker(),
		Prometheus: monitoring.NewPrometheusClient("", 2),
	}
	r := gin.New()
	// No auth middleware: availability is handler-level, tested directly.
	r.GET("/api/servers/:id/prometheus/metrics", monH.ServerPrometheusMetrics)
	r.GET("/api/servers/:id/prometheus/metrics/latest", monH.ServerPrometheusLatest)

	id, err := db.InsertID(`INSERT INTO managed_servers
		(name,hostname,status,agent_status,created_at,updated_at)
		VALUES ('s','h','UNKNOWN','UNKNOWN',NOW(),NOW())`)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		fmt.Sprintf("/api/servers/%d/prometheus/metrics?metric=cpu_usage&range=1h&step=60", id),
		fmt.Sprintf("/api/servers/%d/prometheus/metrics/latest?metric=cpu_usage", id),
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("%s: expected 503, got %d (%s)", path, w.Code, w.Body.String())
		}
	}
}

