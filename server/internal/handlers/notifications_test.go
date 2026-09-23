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

func notifySetup(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testdb.Open(t)
	r := gin.New()
	ntH := &NotificationsHandler{DB: db}
	r.GET("/settings/notifications", ntH.Get)
	r.PUT("/settings/notifications", ntH.Update)
	r.POST("/settings/notifications/test", ntH.Test)
	return r
}

func TestNotifySettingsDefaults(t *testing.T) {
	r := notifySetup(t)
	req := httptest.NewRequest("GET", "/settings/notifications", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get: %d %s", w.Code, w.Body.String())
	}
	var out struct {
		Enabled bool `json:"enabled"`
		Email   struct {
			Port        string `json:"port"`
			TLS         bool   `json:"tls"`
			HasPassword bool   `json:"hasPassword"`
		} `json:"email"`
		Telegram struct {
			HasToken bool `json:"hasToken"`
		} `json:"telegram"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Enabled || out.Email.HasPassword || out.Telegram.HasToken {
		t.Fatalf("fresh install must be disabled with no secrets: %+v", out)
	}
	if out.Email.Port != "587" || !out.Email.TLS {
		t.Fatalf("bad email defaults: %+v", out.Email)
	}
}

func TestNotifySettingsRoundtrip(t *testing.T) {
	r := notifySetup(t)
	body := `{"enabled":true,"events":{"deployFailed":true,"threshold":false,"backupFailed":true,"projectFailed":false},` +
		`"telegram":{"enabled":true,"chatId":"12345"},` +
		`"email":{"enabled":true,"host":"smtp.example.com","port":"587","from":"a@x.y","to":"b@x.y","tls":true}}`
	req := httptest.NewRequest("PUT", "/settings/notifications", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("put: %d %s", w.Code, w.Body.String())
	}
	req2 := httptest.NewRequest("GET", "/settings/notifications", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	var out map[string]interface{}
	if err := json.Unmarshal(w2.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["enabled"] != true {
		t.Fatalf("enabled not persisted: %v", out)
	}
	events := out["events"].(map[string]interface{})
	if events["threshold"] != false || events["deployFailed"] != true {
		t.Fatalf("events not persisted: %v", events)
	}
	email := out["email"].(map[string]interface{})
	if email["host"] != "smtp.example.com" || email["from"] != "a@x.y" {
		t.Fatalf("email not persisted: %v", email)
	}
}

func TestNotifyTestDisabled(t *testing.T) {
	r := notifySetup(t)
	req := httptest.NewRequest("POST", "/settings/notifications/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("test: %d %s", w.Code, w.Body.String())
	}
	var out map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["telegram"] != "disabled" || out["email"] != "disabled" {
		t.Fatalf("fresh install must report disabled: %v", out)
	}
}
