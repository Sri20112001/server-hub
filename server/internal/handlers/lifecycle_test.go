package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"serverhub/internal/testdb"
)

func bulkSetup(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testdb.Open(t)
	r := gin.New()
	lifeH := &ProjectLifecycle{DB: db}
	r.POST("/projects/bulk-lifecycle", lifeH.Bulk)
	return r
}

func postBulk(t *testing.T, r *gin.Engine, body string) (int, map[string]interface{}) {
	t.Helper()
	req := httptest.NewRequest("POST", "/projects/bulk-lifecycle", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var out map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func TestBulkValidation(t *testing.T) {
	r := bulkSetup(t)
	for _, body := range []string{
		`{}`,
		`{"ids":[],"action":"start"}`,
		`{"ids":[1],"action":"explode"}`,
		`{"ids":[1],"action":"stop"}`,
		`{"ids":[1],"action":"restart"}`,
	} {
		if code, _ := postBulk(t, r, body); code != http.StatusBadRequest {
			t.Fatalf("bulk(%q) should be 400, got %d", body, code)
		}
	}
}

func TestBulkMissingProjects(t *testing.T) {
	r := bulkSetup(t)
	// Unknown ids (and no compose dirs) report per-item errors, not a 500.
	// "start" needs no confirm gate.
	code, out := postBulk(t, r, `{"ids":[9991,9992],"action":"start"}`)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d %v", code, out)
	}
	if out["succeeded"] != float64(0) || out["failed"] != float64(2) {
		t.Fatalf("unexpected summary: %v", out)
	}
	arr, ok := out["results"].([]interface{})
	if !ok || len(arr) != 2 {
		t.Fatalf("expected 2 results, got %v", out)
	}
}
