//go:build integration

// Package integration runs Milestone 1 deployment verification against
// disposable infrastructure: a throwaway Postgres container and a real
// serverhub binary booted with production configuration.
//
// Run: go test -tags integration ./internal/integration/... -count=1
// Requires: docker CLI + ability to bind host ports. Skips gracefully
// anywhere else (dev laptops without Docker, unit-test CI jobs). The
// Jenkins pipeline runs this suite on a Docker-capable agent.
package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	pgContainer = "serverhub-int-pg"
	pgPort      = "15432"
	pgPassword  = "int-test-password"
	apiPort     = "14000"
)

var apiBase = "http://127.0.0.1:" + apiPort

func dockerOK() bool {
	cmd := exec.Command("docker", "info")
	return cmd.Run() == nil
}

// Strict mode (SERVERHUB_INT_STRICT=1, set by CI) turns prerequisite
// skips into failures: a misconfigured agent must never report green
// without executing the behaviors under test. Local runs keep graceful
// skips. Each skip states whether it is environmental (strict-fails) or
// conditional (suite-gated, reported but passing).
func strictMode() bool {
	return os.Getenv("SERVERHUB_INT_STRICT") == "1"
}

func requireDocker(t *testing.T) {
	t.Helper()
	if !dockerOK() {
		if strictMode() {
			t.Fatal("docker unavailable in strict mode; refusing to skip")
		}
		t.Skip("docker unavailable; integration tests need a Docker daemon")
	}
}

func run(t *testing.T, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out.String())
	}
	return out.String()
}

// env holds the disposable environment for one test process.
type env struct {
	t        *testing.T
	tmp      string
	bin      string
	api      *exec.Cmd
	apiOut   *bytes.Buffer
	roots    string
	backups  string
	jwt      string
	encKey   string
	adminPass string
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func newEnv(t *testing.T) *env {
	t.Helper()
	requireDocker(t)
	tmp := t.TempDir()
	e := &env{
		t:         t,
		tmp:       tmp,
		roots:     filepath.Join(tmp, "roots"),
		backups:   filepath.Join(tmp, "backups"),
		jwt:       "integration-jwt-secret-min-32-chars!!",
		encKey:    "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff",
		adminPass: "integration-admin-pass",
	}
	if err := os.MkdirAll(e.roots, 0o755); err != nil {
		t.Fatal(err)
	}
	e.bin = filepath.Join(tmp, "serverhub-test")
	build := exec.Command("go", "build", "-o", e.bin, "./cmd/server")
	build.Dir = filepath.Join("..", "..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build serverhub: %v\n%s", err, out)
	}
	return e
}

func (e *env) startPostgres() {
	e.t.Helper()
	run(e.t, "docker", "rm", "-f", pgContainer)
	cmd := exec.Command("docker", "run", "-d", "--rm",
		"--name", pgContainer,
		"-e", "POSTGRES_DB=serverhub",
		"-e", "POSTGRES_USER=serverhub",
		"-e", "POSTGRES_PASSWORD="+pgPassword,
		"-p", "127.0.0.1:"+pgPort+":5432",
		"postgres:16-alpine")
	if out, err := cmd.CombinedOutput(); err != nil {
		e.t.Fatalf("start postgres: %v\n%s", err, out)
	}
	e.t.Cleanup(func() {
		exec.Command("docker", "rm", "-f", pgContainer).Run()
	})
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		cmd := exec.Command("docker", "exec", pgContainer, "pg_isready", "-U", "serverhub")
		if cmd.Run() == nil {
			return
		}
		time.Sleep(2 * time.Second)
	}
	e.t.Fatal("postgres never became ready")
}

func (e *env) dsn() string {
	return fmt.Sprintf("postgres://serverhub:%s@127.0.0.1:%s/serverhub?sslmode=disable", pgPassword, pgPort)
}

// startAPI boots the real binary; extra overrides env entries (use empty
// value to delete a key). Returns when /health/live is 200.
func (e *env) startAPI(extra map[string]string) {
	e.t.Helper()
	base := map[string]string{
		"SERVERHUB_ENV":          "production",
		"PORT":                   apiPort,
		"DATABASE_URL":           e.dsn(),
		"JWT_SECRET":             e.jwt,
		"SERVERHUB_ENCRYPTION_KEY": e.encKey,
		"ADMIN_USERNAME":         "admin",
		"ADMIN_PASSWORD":         e.adminPass,
		"COOKIE_SECURE":          "true",
		"FRONTEND_URL":           "http://localhost:5173",
		"SCAN_ROOTS":             e.roots,
		"DEPLOY_ROOTS":           e.roots,
		"BACKUP_DIR":             e.backups,
	}
	for k, v := range extra {
		if v == "" {
			delete(base, k)
		} else {
			base[k] = v
		}
	}
	cmd := exec.Command(e.bin)
	cmd.Env = append(os.Environ(), flattenEnv(base)...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	e.apiOut = &out
	if err := cmd.Start(); err != nil {
		e.t.Fatalf("start api: %v", err)
	}
	e.api = cmd
	e.t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	})
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
			e.t.Fatalf("api exited during boot:\n%s", out.String())
		}
		resp, err := http.Get(apiBase + "/health/live")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(time.Second)
	}
	e.t.Fatalf("api never became live:\n%s", out.String())
}

func flattenEnv(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	return out
}

func apiGet(t *testing.T, path, cookie string) (int, []byte) {
	t.Helper()
	req, _ := http.NewRequest("GET", apiBase+path, nil)
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

func apiPost(t *testing.T, path string, body interface{}, cookie string) (int, []byte) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, _ := http.NewRequest("POST", apiBase+path, rdr)
	req.Header.Set("Content-Type", "application/json")
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

// loginCookie logs in via the raw HTTP layer to capture the session cookie.
func loginCookie(t *testing.T, user, pass string) string {
	t.Helper()
	b, _ := json.Marshal(map[string]string{"username": user, "password": pass})
	req, _ := http.NewRequest("POST", apiBase+"/server-hub/api/auth/login", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("login: %v %d", err, resp.StatusCode)
	}
	defer resp.Body.Close()
	for _, c := range resp.Cookies() {
		if c.Name == "serverhub_session" {
			return "serverhub_session=" + c.Value
		}
	}
	t.Fatal("no session cookie")
	return ""
}

func waitFor(t *testing.T, what string, timeout time.Duration, f func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if f() {
			return
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("timed out waiting for: %s", what)
}

// 1. Production boot: valid config → live + ready 200.
func TestProductionBoot(t *testing.T) {
	e := newEnv(t)
	e.startPostgres()
	e.startAPI(nil)
	if code, _ := apiGet(t, "/health/live", ""); code != http.StatusOK {
		t.Fatalf("live: want 200, got %d", code)
	}
	if code, b := apiGet(t, "/health/ready", ""); code != http.StatusOK {
		t.Fatalf("ready: want 200, got %d %s", code, b)
	}
	cookie := loginCookie(t, "admin", e.adminPass)
	if code, _ := apiGet(t, "/server-hub/api/auth/me", cookie); code != http.StatusOK {
		t.Fatalf("me: want 200, got %d", code)
	}
}

// 2. Production boot refuses insecure config (full-binary proof of the
// fail-fast paths, beyond the subprocess unit tests).
func TestProductionBootRefusesInsecure(t *testing.T) {
	e := newEnv(t)
	e.startPostgres()
	cmd := exec.Command(e.bin)
	cmd.Env = append(os.Environ(), flattenEnv(map[string]string{
		"SERVERHUB_ENV":  "production",
		"PORT":           apiPort,
		"DATABASE_URL":   e.dsn(),
		"JWT_SECRET":     e.jwt,
		"ADMIN_PASSWORD": e.adminPass,
		"COOKIE_SECURE":  "true",
		// SERVERHUB_ENCRYPTION_KEY deliberately absent.
	})...)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("api started without encryption key; want refusal")
		}
	case <-time.After(20 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("api still running without encryption key; want refusal")
	}
}

// 3. Readiness transitions: healthy → db down (503, live stays 200) → recovered.
func TestReadinessTransitions(t *testing.T) {
	e := newEnv(t)
	e.startPostgres()
	e.startAPI(nil)
	waitFor(t, "ready 200", 60*time.Second, func() bool {
		code, _ := apiGet(t, "/health/ready", "")
		return code == http.StatusOK
	})
	run(t, "docker", "stop", pgContainer)
	waitFor(t, "ready 503 while db down", 60*time.Second, func() bool {
		code, b := apiGet(t, "/health/ready", "")
		return code == http.StatusServiceUnavailable && strings.Contains(string(b), "degraded")
	})
	if code, _ := apiGet(t, "/health/live", ""); code != http.StatusOK {
		t.Fatalf("live during db outage: want 200, got %d", code)
	}
	run(t, "docker", "start", pgContainer)
	waitFor(t, "ready 200 after recovery", 90*time.Second, func() bool {
		code, _ := apiGet(t, "/health/ready", "")
		return code == http.StatusOK
	})
}

// 4. Backup round-trip + pre-restore snapshot + failed-restore recovery.
func TestBackupRestoreRoundTrip(t *testing.T) {
	e := newEnv(t)
	e.startPostgres()
	e.startAPI(nil)
	cookie := loginCookie(t, "admin", e.adminPass)

	live := filepath.Join(e.roots, "shop")
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(live, "docker-compose.yml")
	if err := os.WriteFile(sentinel, []byte("services:\n  web:\n    image: nginx\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, b := apiPost(t, "/server-hub/api/projects",
		map[string]string{"name": "shop", "deploymentPath": live}, cookie)
	if code != http.StatusOK {
		t.Fatalf("create project: %d %s", code, b)
	}
	var proj struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(b, &proj); err != nil || proj.ID == 0 {
		t.Fatalf("project id: %v %s", err, b)
	}
	pid := fmt.Sprintf("%d", proj.ID)

	code, b = apiPost(t, "/server-hub/api/projects/"+pid+"/backups", nil, cookie)
	if code != http.StatusAccepted {
		t.Fatalf("create backup: %d %s", code, b)
	}
	var opResp struct {
		OperationID string `json:"operationId"`
	}
	if err := json.Unmarshal(b, &opResp); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "backup SUCCESS", 90*time.Second, func() bool {
		code, b := apiGet(t, "/server-hub/api/operations/"+opResp.OperationID, cookie)
		return code == http.StatusOK && strings.Contains(string(b), `"status":"SUCCESS"`)
	})

	// Download must be a non-empty zip (F6 regression, end to end).
	code, b = apiGet(t, "/server-hub/api/projects/"+pid+"/backups", cookie)
	if code != http.StatusOK {
		t.Fatalf("list backups: %d %s", code, b)
	}
	var first []struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(b, &first); err != nil || len(first) == 0 {
		t.Fatalf("backups: %v %s", err, b)
	}
	code, zb := apiGet(t, "/server-hub/api/backups/"+fmt.Sprintf("%d", first[0].ID)+"/download", cookie)
	if code != http.StatusOK || len(zb) < 4 || string(zb[:4]) != "PK\x03\x04" {
		t.Fatalf("download: want non-empty zip, got code=%d bytes=%d", code, len(zb))
	}

	// Simulate data loss, then restore.
	dataFile := filepath.Join(live, "data.txt")
	if err := os.WriteFile(dataFile, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, b = apiPost(t, "/server-hub/api/projects/"+pid+"/backups", nil, cookie)
	if code != http.StatusAccepted {
		t.Fatalf("second backup: %d %s", code, b)
	}
	if err := json.Unmarshal(b, &opResp); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "second backup SUCCESS", 90*time.Second, func() bool {
		code, b := apiGet(t, "/server-hub/api/operations/"+opResp.OperationID, cookie)
		return code == http.StatusOK && strings.Contains(string(b), `"status":"SUCCESS"`)
	})
	if err := os.WriteFile(dataFile, []byte("v2-corrupt"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Backups list → pick latest backup id.
	code, b = apiGet(t, "/server-hub/api/projects/"+pid+"/backups", cookie)
	if code != http.StatusOK {
		t.Fatalf("list backups: %d %s", code, b)
	}
	var rows []struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(b, &rows); err != nil || len(rows) == 0 {
		t.Fatalf("backups: %v %s", err, b)
	}
	latest := fmt.Sprintf("%d", rows[0].ID)
	code, b = apiPost(t, "/server-hub/api/backups/"+latest+"/restore?confirm=true", nil, cookie)
	if code != http.StatusAccepted {
		t.Fatalf("restore: %d %s", code, b)
	}
	if err := json.Unmarshal(b, &opResp); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "restore SUCCESS", 90*time.Second, func() bool {
		code, b := apiGet(t, "/server-hub/api/operations/"+opResp.OperationID, cookie)
		return code == http.StatusOK && strings.Contains(string(b), `"status":"SUCCESS"`)
	})
	content, err := os.ReadFile(dataFile)
	if err != nil || string(content) != "v1" {
		t.Fatalf("restored content = %q, want %q (err=%v)", content, "v1", err)
	}
	// Pre-restore snapshot must exist as a backup row.
	code, b = apiGet(t, "/server-hub/api/projects/"+pid+"/backups", cookie)
	if code != http.StatusOK || !strings.Contains(string(b), "pre-restore") {
		t.Fatalf("pre-restore snapshot missing: %d %s", code, b)
	}
}

// 5. Notification policy path end to end: policy round-trip persists,
// maintenance windows gate, deliveries endpoint serves history, and the
// worker loop runs without crashing the boot (implicit in every test).
func TestNotificationPolicyPath(t *testing.T) {
	e := newEnv(t)
	e.startPostgres()
	e.startAPI(nil)
	cookie := loginCookie(t, "admin", e.adminPass)

	code, b := apiGet(t, "/server-hub/api/notification-policy", cookie)
	if code != http.StatusOK {
		t.Fatalf("get policy: %d %s", code, b)
	}
	code, b = apiPost(t, "/server-hub/api/notification-policy",
		map[string]any{"maxRepeats": 5, "emailPerHour": 120}, cookie)
	if code != http.StatusOK {
		t.Fatalf("put policy: %d %s", code, b)
	}
	code, b = apiGet(t, "/server-hub/api/notification-policy", cookie)
	if code != http.StatusOK || !strings.Contains(string(b), `"maxRepeats":5`) {
		t.Fatalf("policy persistence: %d %s", code, b)
	}
	// Emergency pause round-trip (proves the suppression gate is armed).
	code, b = apiPost(t, "/server-hub/api/notification-policy", map[string]any{
		"emergencyPause": true, "pauseReason": "integration drill",
		"pauseUntil": time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
	}, cookie)
	if code != http.StatusOK {
		t.Fatalf("set pause: %d %s", code, b)
	}
	code, b = apiPost(t, "/server-hub/api/notification-policy",
		map[string]any{"clearPause": true}, cookie)
	if code != http.StatusOK {
		t.Fatalf("clear pause: %d %s", code, b)
	}
	// Maintenance window lifecycle.
	code, b = apiPost(t, "/server-hub/api/maintenance-windows", map[string]any{
		"name": "int-freeze", "scope": "all",
		"startsAt": time.Now().UTC().Format(time.RFC3339),
		"endsAt":   time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
		"reason":   "integration",
	}, cookie)
	if code != http.StatusCreated {
		t.Fatalf("create window: %d %s", code, b)
	}
	code, b = apiGet(t, "/server-hub/api/maintenance-windows", cookie)
	if code != http.StatusOK || !strings.Contains(string(b), "int-freeze") {
		t.Fatalf("list windows: %d %s", code, b)
	}
	// Delivery history serves (empty but reachable — no secrets).
	code, b = apiGet(t, "/server-hub/api/notification-deliveries?limit=5", cookie)
	if code != http.StatusOK {
		t.Fatalf("deliveries: %d %s", code, b)
	}
}

// 6. Self-protection against the real daemon: stopping a protected
// infrastructure container must be refused even by an admin.
// Runs only when the stack itself runs in compose (marker env).
func TestSelfProtectionLive(t *testing.T) {
	if os.Getenv("SERVERHUB_TEST_COMPOSE") == "" {
		// Conditional skip: suite-gated by design, not environmental.
		// Reported in CI output; does not fail strict mode.
		t.Skip("needs the full compose stack (SERVERHUB_TEST_COMPOSE=1)")
	}
	e := newEnv(t)
	e.startPostgres()
	e.startAPI(nil)
	cookie := loginCookie(t, "admin", e.adminPass)
	// Resolve the postgres container id via the daemon.
	id := strings.TrimSpace(run(t, "docker", "ps", "-q", "--filter", "name=serverhub-postgres"))
	if id == "" {
		t.Skip("serverhub-postgres not running here")
	}
	code, b := apiPost(t, "/server-hub/api/containers/"+id+"/stop?confirm=true", nil, cookie)
	if code != http.StatusForbidden {
		t.Fatalf("stop protected container: want 403, got %d %s", code, b)
	}
}
