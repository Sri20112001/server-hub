package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	"serverhub/internal/testdb"
)

func rulesSetup(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testdb.Open(t)
	r := gin.New()
	ngH := &NotificationGroupsHandler{DB: db}
	ntH := &NotificationsHandler{DB: db}
	nrH := &NotificationRulesHandler{DB: db}
	r.GET("/notification-groups", ngH.List)
	r.GET("/notification-groups/:id", ngH.Get)
	r.POST("/notification-groups", ngH.Create)
	r.PATCH("/notification-groups/:id", ngH.Update)
	r.POST("/notification-groups/:id/members", ngH.AddMember)
	r.DELETE("/notification-groups/:id", ngH.Delete)
	r.DELETE("/notification-groups/:id/members/:memberId", ngH.RemoveMember)
	r.GET("/settings/notifications", ntH.Get)
	r.PUT("/settings/notifications", ntH.Update)
	r.GET("/notification-rules", nrH.List)
	r.GET("/notification-rules/:id", nrH.Get)
	r.POST("/notification-rules", nrH.Create)
	r.PATCH("/notification-rules/:id", nrH.Update)
	r.DELETE("/notification-rules/:id", nrH.Delete)
	return r
}

func mkGroup(t *testing.T, r *gin.Engine, name string) uint {
	t.Helper()
	w := ngDoReq(t, r, "POST", "/notification-groups", fmt.Sprintf(`{"name":%q}`, name))
	if w.Code != http.StatusCreated {
		t.Fatalf("seed group: %d %s", w.Code, w.Body.String())
	}
	var out struct {
		ID uint `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out.ID
}

func TestRuleCRUD(t *testing.T) {
	r := rulesSetup(t)
	gid := mkGroup(t, r, "G")
	body := fmt.Sprintf(`{"name":"R1","eventType":"AGENT_OFFLINE","notificationGroupId":%d,
		"channels":["EMAIL"],"cooldownSeconds":60,"notifyOnRecovery":true}`, gid)
	w := ngDoReq(t, r, "POST", "/notification-rules", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var created struct {
		ID uint `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil || created.ID == 0 {
		t.Fatalf("bad create: %v %s", err, w.Body.String())
	}
	// Duplicate name → 409.
	w = ngDoReq(t, r, "POST", "/notification-rules", body)
	if w.Code != http.StatusConflict {
		t.Fatalf("duplicate name must 409, got %d", w.Code)
	}
	// List shows group name join.
	w = ngDoReq(t, r, "GET", "/notification-rules", "")
	var list []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil || len(list) != 1 {
		t.Fatalf("list: %v %s", err, w.Body.String())
	}
	if list[0]["groupName"] != "G" {
		t.Fatalf("group join missing: %v", list[0])
	}
	// Get one.
	w = ngDoReq(t, r, "GET", fmt.Sprintf("/notification-rules/%d", created.ID), "")
	if w.Code != http.StatusOK {
		t.Fatalf("get: %d", w.Code)
	}
	// Patch disable + cooldown.
	w = ngDoReq(t, r, "PATCH", fmt.Sprintf("/notification-rules/%d", created.ID),
		`{"enabled":false,"cooldownSeconds":0}`)
	if w.Code != http.StatusOK {
		t.Fatalf("patch: %d %s", w.Code, w.Body.String())
	}
	w = ngDoReq(t, r, "GET", fmt.Sprintf("/notification-rules/%d", created.ID), "")
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["enabled"] != false || got["cooldownSeconds"] != float64(0) {
		t.Fatalf("patch not applied: %v", got)
	}
	if got["eventType"] != "AGENT_OFFLINE" || got["channels"] != "EMAIL" {
		t.Fatalf("patch must preserve untouched fields: %v", got)
	}
	// Unknown ids → 404.
	for _, tc := range [][2]string{
		{"GET", "/notification-rules/999999"},
		{"PATCH", "/notification-rules/999999"},
		{"DELETE", "/notification-rules/999999"},
	} {
		w = ngDoReq(t, r, tc[0], tc[1], `{}`)
		if w.Code != http.StatusNotFound {
			t.Fatalf("%s must 404, got %d", tc[0], w.Code)
		}
	}
	// Delete.
	w = ngDoReq(t, r, "DELETE", fmt.Sprintf("/notification-rules/%d", created.ID), "")
	if w.Code != http.StatusOK {
		t.Fatalf("delete: %d", w.Code)
	}
}

func TestRuleValidation(t *testing.T) {
	r := rulesSetup(t)
	gid := mkGroup(t, r, "G")
	cases := []struct {
		name string
		body string
	}{
		{"missing fields", `{}`},
		{"empty name", fmt.Sprintf(`{"name":"","eventType":"AGENT_OFFLINE","notificationGroupId":%d}`, gid)},
		{"oversized name", fmt.Sprintf(`{"name":"%s","eventType":"AGENT_OFFLINE","notificationGroupId":%d}`, makeStr(101), gid)},
		{"bad event", fmt.Sprintf(`{"name":"R","eventType":"NOPE","notificationGroupId":%d}`, gid)},
		{"bad channel", fmt.Sprintf(`{"name":"R","eventType":"AGENT_OFFLINE","notificationGroupId":%d,"channels":["PIGEON"]}`, gid)},
		{"no channels", fmt.Sprintf(`{"name":"R","eventType":"AGENT_OFFLINE","notificationGroupId":%d,"channels":[]}`, gid)},
		{"bad group", `{"name":"R","eventType":"AGENT_OFFLINE","notificationGroupId":999999}`},
		{"bad condition", fmt.Sprintf(`{"name":"R","eventType":"AGENT_OFFLINE","notificationGroupId":%d,"conditionJson":"{oops"}`, gid)},
		{"bad condition field", fmt.Sprintf(`{"name":"R","eventType":"AGENT_OFFLINE","notificationGroupId":%d,"conditionJson":"{\"field\":\"nope\",\"operator\":\"eq\",\"value\":\"x\"}"}`, gid)},
		{"negative cooldown", fmt.Sprintf(`{"name":"R","eventType":"AGENT_OFFLINE","notificationGroupId":%d,"cooldownSeconds":-5}`, gid)},
		{"huge cooldown", fmt.Sprintf(`{"name":"R","eventType":"AGENT_OFFLINE","notificationGroupId":%d,"cooldownSeconds":99999999}`, gid)},
		{"malformed json", `{"name":`},
	}
	for _, tc := range cases {
		w := ngDoReq(t, r, "POST", "/notification-rules", tc.body)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s must 400, got %d (%s)", tc.name, w.Code, w.Body.String())
		}
	}
}

func makeStr(n int) string {
	s := ""
	for len(s) < n {
		s += "x"
	}
	return s
}

func TestGroupDeletionGuard(t *testing.T) {
	r := rulesSetup(t)
	gid := mkGroup(t, r, "G")
	// Group without rules deletes fine.
	w := ngDoReq(t, r, "DELETE", fmt.Sprintf("/notification-groups/%d", gid), "")
	if w.Code != http.StatusOK {
		t.Fatalf("unreferenced delete: %d", w.Code)
	}
	gid = mkGroup(t, r, "G2")
	w = ngDoReq(t, r, "POST", "/notification-rules",
		fmt.Sprintf(`{"name":"R","eventType":"AGENT_OFFLINE","notificationGroupId":%d}`, gid))
	if w.Code != http.StatusCreated {
		t.Fatalf("rule create: %d %s", w.Code, w.Body.String())
	}
	if w.Code != http.StatusCreated {
		t.Fatalf("rule create: %d", w.Code)
	}
	// Referenced group → 409, no fallback, no delete.
	w = ngDoReq(t, r, "DELETE", fmt.Sprintf("/notification-groups/%d", gid), "")
	if w.Code != http.StatusConflict {
		t.Fatalf("referenced delete must 409, got %d", w.Code)
	}
	w = ngDoReq(t, r, "GET", fmt.Sprintf("/notification-groups/%d", gid), "")
	if w.Code != http.StatusOK {
		t.Fatalf("guarded group must survive, got %d", w.Code)
	}
}
