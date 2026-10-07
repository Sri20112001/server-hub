package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"serverhub/internal/testdb"
)

func groupsSetup(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testdb.Open(t)
	r := gin.New()
	ngH := &NotificationGroupsHandler{DB: db}
	ntH := &NotificationsHandler{DB: db}
	r.GET("/notification-groups", ngH.List)
	r.GET("/notification-groups/:id", ngH.Get)
	r.POST("/notification-groups", ngH.Create)
	r.PATCH("/notification-groups/:id", ngH.Update)
	r.POST("/notification-groups/:id/members", ngH.AddMember)
	r.DELETE("/notification-groups/:id", ngH.Delete)
	r.DELETE("/notification-groups/:id/members/:memberId", ngH.RemoveMember)
	r.GET("/settings/notifications", ntH.Get)
	r.PUT("/settings/notifications", ntH.Update)
	return r
}

func doNgReq(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestGroupCRUD(t *testing.T) {
	r := groupsSetup(t)
	w := doNgReq(t, r, "POST", "/notification-groups", `{"name":"Infra","description":"d"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var created struct {
		ID   uint   `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil || created.ID == 0 {
		t.Fatalf("bad create response: %v %s", err, w.Body.String())
	}
	// Duplicate name → 409.
	w = doNgReq(t, r, "POST", "/notification-groups", `{"name":"Infra"}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("duplicate name must 409, got %d", w.Code)
	}
	// Empty name → 400.
	w = doNgReq(t, r, "POST", "/notification-groups", `{"name":"  "}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("empty name must 400, got %d", w.Code)
	}
	// Get with members.
	w = doNgReq(t, r, "GET", fmt.Sprintf("/notification-groups/%d", created.ID), "")
	if w.Code != http.StatusOK {
		t.Fatalf("get: %d", w.Code)
	}
	// Rename.
	w = doNgReq(t, r, "PATCH", fmt.Sprintf("/notification-groups/%d", created.ID), `{"name":"Infra2"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("patch: %d %s", w.Code, w.Body.String())
	}
	// Unknown ids → 404.
	for _, tc := range [][2]string{
		{"GET", "/notification-groups/999999"},
		{"PATCH", "/notification-groups/999999"},
		{"DELETE", "/notification-groups/999999"},
	} {
		w = doNgReq(t, r, tc[0], tc[1], `{"name":"x"}`)
		if w.Code != http.StatusNotFound {
			t.Fatalf("%s %s must 404, got %d", tc[0], tc[1], w.Code)
		}
	}
	// Delete.
	w = doNgReq(t, r, "DELETE", fmt.Sprintf("/notification-groups/%d", created.ID), "")
	if w.Code != http.StatusOK {
		t.Fatalf("delete: %d", w.Code)
	}
	w = doNgReq(t, r, "GET", fmt.Sprintf("/notification-groups/%d", created.ID), "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("deleted group must 404, got %d", w.Code)
	}
}

func TestGroupMembers(t *testing.T) {
	r := groupsSetup(t)
	w := doNgReq(t, r, "POST", "/notification-groups", `{"name":"Team"}`)
	var created struct {
		ID uint `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	base := fmt.Sprintf("/notification-groups/%d/members", created.ID)

	// Invalid + empty emails → 400.
	for _, bad := range []string{`{"email":""}`, `{"email":"not-an-email"}`, `{}`} {
		w = doNgReq(t, r, "POST", base, bad)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("bad email %s must 400, got %d", bad, w.Code)
		}
	}
	// Valid add (normalized to lowercase).
	w = doNgReq(t, r, "POST", base, `{"email":"Admin@X.Y"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("add: %d %s", w.Code, w.Body.String())
	}
	var m struct {
		ID    uint   `json:"id"`
		Email string `json:"email"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil || m.Email != "admin@x.y" {
		t.Fatalf("email must normalize lowercase: %v %s", err, w.Body.String())
	}
	// Case-insensitive duplicate → 409.
	w = doNgReq(t, r, "POST", base, `{"email":"ADMIN@x.y"}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("duplicate member must 409, got %d", w.Code)
	}
	// Fill to cap (10) then overflow → 400.
	for i := 0; i < 9; i++ {
		w = doNgReq(t, r, "POST", base, fmt.Sprintf(`{"email":"m%d@x.y"}`, i))
		if w.Code != http.StatusCreated {
			t.Fatalf("fill %d: %d %s", i, w.Code, w.Body.String())
		}
	}
	w = doNgReq(t, r, "POST", base, `{"email":"overflow@x.y"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("11th member must 400, got %d", w.Code)
	}
	// Remove unknown member → 404.
	w = doNgReq(t, r, "DELETE", base+"/999999", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown member must 404, got %d", w.Code)
	}
	// Remove real member.
	w = doNgReq(t, r, "DELETE", fmt.Sprintf("%s/%d", base, m.ID), "")
	if w.Code != http.StatusOK {
		t.Fatalf("remove: %d", w.Code)
	}
	// Member on unknown group → 404.
	w = doNgReq(t, r, "POST", "/notification-groups/999999/members", `{"email":"a@x.y"}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown group must 404, got %d", w.Code)
	}
	// Delete group cascades members: group gone.
	w = doNgReq(t, r, "DELETE", fmt.Sprintf("/notification-groups/%d", created.ID), "")
	if w.Code != http.StatusOK {
		t.Fatalf("delete: %d", w.Code)
	}
}

func TestGroupSelection(t *testing.T) {
	r := groupsSetup(t)
	w := doNgReq(t, r, "POST", "/notification-groups", `{"name":"G"}`)
	var created struct {
		ID uint `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	// Select via settings PUT.
	w = doNgReq(t, r, "PUT", "/settings/notifications",
		fmt.Sprintf(`{"email":{"emailGroupId":%d}}`, created.ID))
	if w.Code != http.StatusOK {
		t.Fatalf("select: %d %s", w.Code, w.Body.String())
	}
	w = doNgReq(t, r, "GET", "/settings/notifications", "")
	var out struct {
		Email struct {
			EmailGroupID *uint `json:"emailGroupId"`
		} `json:"email"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Email.EmailGroupID == nil || *out.Email.EmailGroupID != created.ID {
		t.Fatalf("group id not persisted: %s", w.Body.String())
	}
	// Unknown group → 400.
	w = doNgReq(t, r, "PUT", "/settings/notifications", `{"email":{"emailGroupId":999999}}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("unknown group must 400, got %d", w.Code)
	}
	// Explicit null clears back to legacy fallback.
	w = doNgReq(t, r, "PUT", "/settings/notifications", `{"email":{"emailGroupId":null}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("clear: %d %s", w.Code, w.Body.String())
	}
	w = doNgReq(t, r, "GET", "/settings/notifications", "")
	out.Email.EmailGroupID = nil
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Email.EmailGroupID != nil {
		t.Fatalf("clear must null the selector: %s", w.Body.String())
	}
	// Partial PUT still must not wipe sibling string fields.
	w = doNgReq(t, r, "PUT", "/settings/notifications",
		`{"email":{"host":"smtp.example.com","from":"a@x.y","to":"b@x.y"}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("seed: %d", w.Code)
	}
	w = doNgReq(t, r, "PUT", "/settings/notifications", `{"enabled":true}`)
	if w.Code != http.StatusOK {
		t.Fatalf("partial: %d", w.Code)
	}
	w = doNgReq(t, r, "GET", "/settings/notifications", "")
	var out2 struct {
		Email struct {
			Host string `json:"host"`
			To   string `json:"to"`
		} `json:"email"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out2); err != nil {
		t.Fatal(err)
	}
	if out2.Email.Host != "smtp.example.com" || out2.Email.To != "b@x.y" {
		t.Fatalf("partial PUT wiped strings: %s", w.Body.String())
	}
}
