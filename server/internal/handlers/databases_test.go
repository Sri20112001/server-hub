package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"serverhub/internal/config"
	"serverhub/internal/dockerx"
	"serverhub/internal/testdb"
)

func TestDbEngineFromImage(t *testing.T) {
	cases := map[string]string{
		"postgres:16-alpine":   "postgres",
		"postgis/postgis:16":   "postgres",
		"mysql:8":              "mysql",
		"mariadb:11":           "mysql",
		"redis:7-alpine":       "redis",
		"valkey/valkey:8":      "redis",
		"mongo:7":              "mongo",
		"node:20":              "",
		"nginx:alpine":         "",
		"postgres@sha256:abcd": "postgres",
	}
	for img, want := range cases {
		eng, _, ok := dbEngineFromImage(img)
		if want == "" && ok {
			t.Errorf("engineFromImage(%q) = %q, want none", img, eng)
		}
		if want != "" && (!ok || eng != want) {
			t.Errorf("engineFromImage(%q) = %q,%v, want %q", img, eng, ok, want)
		}
	}
	if _, ver, ok := dbEngineFromImage("postgres:16-alpine"); !ok || ver != "16-alpine" {
		t.Errorf("version not parsed: %q %v", ver, ok)
	}
}

func TestDbServerKey(t *testing.T) {
	a := dbServerKey("docker", "postgres", "", 5432, "pg")
	b := dbServerKey("docker", "postgres", "", 5432, "pg")
	c := dbServerKey("host", "postgres", "127.0.0.1", 5432, "")
	if a == "" || a != b || a == c {
		t.Fatalf("server keys not stable/unique: %q %q %q", a, b, c)
	}
	if dbDefaultPort("postgres") != 5432 || dbDefaultPort("mysql") != 3306 ||
		dbDefaultPort("redis") != 6379 || dbDefaultPort("mongo") != 27017 {
		t.Fatal("wrong default ports")
	}
}

func TestParseRedisKeyspace(t *testing.T) {
	ks := parseRedisKeyspace("# Keyspace\r\ndb0:keys=12,expires=0,avg_ttl=0\r\ndb3:keys=1,expires=0\r\n")
	if ks["db0"] != 12 || ks["db3"] != 1 || len(ks) != 2 {
		t.Fatalf("bad parse: %v", ks)
	}
}

func dbTestSetup(t *testing.T) (*gin.Engine, *DatabasesHandler) {
	t.Helper()
	t.Setenv("DOCKER_HOST", "tcp://127.0.0.1:12345") // force docker down
	gin.SetMode(gin.TestMode)
	db := testdb.Open(t)
	h := &DatabasesHandler{DB: db, Docker: dockerx.New(), Cfg: &config.Config{
		JWTSecret:     "test-jwt-secret-min-32-chars-long!!",
		EncryptionKey: testEncKey,
	}}
	r := gin.New()
	r.GET("/databases/servers", h.Servers)
	r.POST("/databases/browse", h.Browse)
	r.POST("/databases/connect", h.Connect)
	r.POST("/databases/register", h.Register)
	return r, h
}

func TestServersEndpoint(t *testing.T) {
	r, _ := dbTestSetup(t)
	req := httptest.NewRequest("GET", "/databases/servers", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("servers: %d %s", w.Code, w.Body.String())
	}
	var out struct {
		Servers []dbServerInfo `json:"servers"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	// No docker, no host DBs in CI, empty fleet → empty list, not an error.
	if out.Servers == nil {
		t.Fatal("servers must be an array, not null")
	}
}

func TestBrowseValidation(t *testing.T) {
	r, _ := dbTestSetup(t)
	for _, body := range []string{`{}`, `{"engine":"oracle"}`, `not-json`} {
		req := httptest.NewRequest("POST", "/databases/browse", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code == http.StatusOK {
			t.Fatalf("browse(%q) should fail, got %s", body, w.Body.String())
		}
	}
}

func TestConnectValidation(t *testing.T) {
	r, _ := dbTestSetup(t)
	for _, body := range []string{
		`{"engine":"oracle","host":"x"}`,
		`{"engine":"postgres","source":"docker"}`,
		`{"engine":"postgres","source":"host"}`,
	} {
		req := httptest.NewRequest("POST", "/databases/connect", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code == http.StatusOK {
			t.Fatalf("connect(%q) should fail, got %s", body, w.Body.String())
		}
	}
}

func TestRegisterValidation(t *testing.T) {
	r, _ := dbTestSetup(t)
	// Unknown project → 404.
	body := `{"projectId":9999,"server":{"engine":"postgres","source":"host","host":"127.0.0.1","port":5432},"databases":["app"]}`
	req := httptest.NewRequest("POST", "/databases/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d %s", w.Code, w.Body.String())
	}
}
