package rules

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"serverhub/internal/database"
	"serverhub/internal/testdb"
)

type fakeSender struct {
	mu     sync.Mutex
	emails []fakeMail
	inApps int32
}

type fakeMail struct {
	groupID uint
	subject string
}

func (f *fakeSender) SendEmail(groupID uint, subject, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.emails = append(f.emails, fakeMail{groupID: groupID, subject: subject})
	return nil
}

func (f *fakeSender) SendTelegram(text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.emails = append(f.emails, fakeMail{groupID: 0, subject: "tg:" + text})
	return nil
}

func (f *fakeSender) SendInApp(serverID *uint, alertID *int64, title, body string) error {
	atomic.AddInt32(&f.inApps, 1)
	return nil
}

func (f *fakeSender) emailCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.emails)
}

func testEngine(t *testing.T, db *database.DB, s *fakeSender) *Engine {
	t.Helper()
	now := time.Now().UTC()
	return &Engine{
		DB:      db,
		Sender:  s,
		Enabled: true,
		Now:     func() time.Time { return now },
	}
}

// seedGroup creates a group + returns its id.
func seedGroup(t *testing.T, db *database.DB, name string) uint {
	t.Helper()
	id, err := db.InsertID(`INSERT INTO notification_groups (name,created_at,updated_at)
		VALUES ($1,NOW(),NOW())`, name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO notification_group_members (group_id,email,created_at)
		VALUES ($1,'member@x.y',NOW())`, id); err != nil {
		t.Fatal(err)
	}
	return uint(id)
}

// seedRule inserts a rule row directly, returning its id.
func seedRule(t *testing.T, db *database.DB, name, eventType string, gid uint, mutate func(*ruleSeed)) uint {
	t.Helper()
	s := &ruleSeed{
		name: name, eventType: eventType, groupID: gid,
		channels: "EMAIL", cooldown: 0, recovery: false, enabled: true,
	}
	if mutate != nil {
		mutate(s)
	}
	id, err := db.InsertID(`INSERT INTO notification_rules
		(name,enabled,event_type,severity,condition_json,notification_group_id,
		 channels,cooldown_seconds,notify_on_recovery,created_by,created_at,updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'test',NOW(),NOW())`,
		s.name, s.enabled, s.eventType, s.severity, s.cond, s.groupID,
		s.channels, s.cooldown, s.recovery)
	if err != nil {
		t.Fatal(err)
	}
	return uint(id)
}

type ruleSeed struct {
	name      string
	eventType string
	groupID   uint
	severity  string
	cond      string
	channels  string
	cooldown  int
	recovery  bool
	enabled   bool
}

func baseEvent() Event {
	sid := uint(7)
	return Event{
		Type: EventServerAlert, ServerID: &sid,
		Severity: "WARNING", Condition: "cpu_high", Value: 95,
		Message: "cpu_high reached 95%", Fingerprint: "fp-1",
	}
}

func TestEvaluateDisabledEngine(t *testing.T) {
	db := testdb.Open(t)
	s := &fakeSender{}
	eng := testEngine(t, db, s)
	eng.Enabled = false
	gid := seedGroup(t, db, "G")
	seedRule(t, db, "R", EventServerAlert, gid, nil)
	eng.Evaluate(baseEvent())
	if s.emailCount() != 0 {
		t.Fatal("disabled engine must not send")
	}
}

func TestEvaluateNoRules(t *testing.T) {
	db := testdb.Open(t)
	s := &fakeSender{}
	testEngine(t, db, s).Evaluate(baseEvent())
	if s.emailCount() != 0 {
		t.Fatal("no rules must mean no sends")
	}
}

func TestEvaluateMatchAndMismatch(t *testing.T) {
	db := testdb.Open(t)
	s := &fakeSender{}
	eng := testEngine(t, db, s)
	gid := seedGroup(t, db, "G")
	seedRule(t, db, "Match", EventServerAlert, gid, nil)
	seedRule(t, db, "WrongType", EventAgentOffline, gid, nil)
	seedRule(t, db, "Off", EventServerAlert, gid, func(s *ruleSeed) { s.enabled = false })
	seedRule(t, db, "WrongSev", EventServerAlert, gid, func(s *ruleSeed) { s.severity = "CRITICAL" })
	seedRule(t, db, "CondNoMatch", EventServerAlert, gid,
		func(s *ruleSeed) { s.cond = `{"field":"value","operator":"gt","value":99}` })
	seedRule(t, db, "CondMatch", EventServerAlert, gid,
		func(s *ruleSeed) { s.cond = `{"field":"value","operator":"gt","value":90}` })
	eng.Evaluate(baseEvent())
	if got := s.emailCount(); got != 2 {
		t.Fatalf("expected 2 sends (Match + CondMatch), got %d", got)
	}
	for _, m := range s.emails {
		if m.groupID != gid {
			t.Fatalf("wrong group used: %+v", m)
		}
	}
}

func TestEvaluateCooldown(t *testing.T) {
	db := testdb.Open(t)
	s := &fakeSender{}
	now := time.Now().UTC()
	eng := &Engine{DB: db, Sender: s, Enabled: true, Now: func() time.Time { return now }}
	gid := seedGroup(t, db, "G")
	seedRule(t, db, "Cool", EventServerAlert, gid, func(s *ruleSeed) { s.cooldown = 900 })
	eng.Evaluate(baseEvent())
	eng.Evaluate(baseEvent())
	if got := s.emailCount(); got != 1 {
		t.Fatalf("second event in cooldown must suppress, got %d", got)
	}
	now = now.Add(901 * time.Second)
	eng.Evaluate(baseEvent())
	if got := s.emailCount(); got != 2 {
		t.Fatalf("post-cooldown event must send, got %d", got)
	}
}

func TestEvaluateRecovery(t *testing.T) {
	newSetup := func(t *testing.T, recovery bool) (*database.DB, *fakeSender, *Engine, uint) {
		db := testdb.Open(t)
		s := &fakeSender{}
		now := time.Now().UTC()
		eng := &Engine{DB: db, Sender: s, Enabled: true, Now: func() time.Time { return now }}
		gid := seedGroup(t, db, "G")
		seedRule(t, db, "R", EventServerAlert, gid, func(s *ruleSeed) { s.recovery = recovery })
		return db, s, eng, gid
	}
	mkResolved := func() Event {
		ev := baseEvent()
		ev.Type = EventServerAlertResolved
		return ev
	}
	t.Run("with recovery enabled", func(t *testing.T) {
		_, s, eng, _ := newSetup(t, true)
		eng.Evaluate(baseEvent())
		eng.Evaluate(mkResolved())
		eng.Evaluate(mkResolved()) // repeated recovery must not duplicate
		if got := s.emailCount(); got != 2 {
			t.Fatalf("expected firing+recovery = 2, got %d", got)
		}
	})
	t.Run("recovery disabled", func(t *testing.T) {
		_, s, eng, _ := newSetup(t, false)
		eng.Evaluate(baseEvent())
		eng.Evaluate(mkResolved())
		if got := s.emailCount(); got != 1 {
			t.Fatalf("expected firing only = 1, got %d", got)
		}
	})
	t.Run("recovery without firing", func(t *testing.T) {
		_, s, eng, _ := newSetup(t, true)
		eng.Evaluate(mkResolved())
		if got := s.emailCount(); got != 0 {
			t.Fatalf("lone recovery must not notify, got %d", got)
		}
	})
}

func TestEvaluateBrokenRuleIsolated(t *testing.T) {
	db := testdb.Open(t)
	s := &fakeSender{}
	eng := testEngine(t, db, s)
	gid := seedGroup(t, db, "G")
	seedRule(t, db, "Broken", EventServerAlert, gid, func(s *ruleSeed) { s.cond = `{oops` })
	seedRule(t, db, "Good", EventServerAlert, gid, nil)
	eng.Evaluate(baseEvent()) // must not panic; good rule still sends
	if got := s.emailCount(); got != 1 {
		t.Fatalf("expected 1 send from good rule, got %d", got)
	}
}

func TestEvaluateDualRuleRecovery(t *testing.T) {
	// A firing-type rule (recovery on) plus a dedicated resolved-type rule
	// must together produce exactly one recovery notification.
	db := testdb.Open(t)
	s := &fakeSender{}
	eng := testEngine(t, db, s)
	gid := seedGroup(t, db, "G")
	seedRule(t, db, "Firing", EventServerAlert, gid, func(s *ruleSeed) { s.recovery = true })
	seedRule(t, db, "Resolved", EventServerAlertResolved, gid, func(s *ruleSeed) { s.recovery = true })
	eng.Evaluate(baseEvent())
	ev := baseEvent()
	ev.Type = EventServerAlertResolved
	eng.Evaluate(ev)
	if got := s.emailCount(); got != 2 {
		t.Fatalf("expected firing + single recovery = 2, got %d", got)
	}
}
func TestEvaluateMissingGroup(t *testing.T) {
	db := testdb.Open(t)
	s := &fakeSender{}
	eng := testEngine(t, db, s)
	seedRule(t, db, "Orphan", EventServerAlert, 999999, nil)
	seedRule(t, db, "Good", EventServerAlert, seedGroup(t, db, "G"), nil)
	eng.Evaluate(baseEvent()) // orphan fails safely (fake sender accepts any id though)
	if got := s.emailCount(); got != 2 {
		t.Fatalf("engine must attempt both, got %d", got)
	}
}

func TestEvaluateConcurrent(t *testing.T) {
	db := testdb.Open(t)
	s := &fakeSender{}
	eng := testEngine(t, db, s)
	gid := seedGroup(t, db, "G")
	seedRule(t, db, "Cool", EventServerAlert, gid, func(s *ruleSeed) { s.cooldown = 3600 })
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			eng.Evaluate(baseEvent())
		}()
	}
	wg.Wait()
	if got := s.emailCount(); got != 1 {
		t.Fatalf("concurrent evaluation must send exactly once, got %d", got)
	}
}

func TestFingerprintHelper(t *testing.T) {
	if got := Fingerprint(7, "cpu_high"); got != "server:7:cpu_high" {
		t.Fatalf("bad fingerprint: %q", got)
	}
}

func TestConditionValidation(t *testing.T) {
	valid := []string{
		``,
		`{"field":"severity","operator":"eq","value":"CRITICAL"}`,
		`{"field":"condition","operator":"contains","value":"cpu"}`,
		`{"field":"value","operator":"gte","value":90}`,
		`{"field":"serverId","operator":"in","value":[1,2]}`,
		`{"field":"severity","operator":"in","value":["INFO","WARNING"]}`,
	}
	for _, raw := range valid {
		if _, err := ParseCondition(raw); err != nil {
			t.Fatalf("valid condition rejected %q: %v", raw, err)
		}
	}
	invalid := []string{
		`{oops`,
		`{"field":"nope","operator":"eq","value":"x"}`,
		`{"field":"severity","operator":"gt","value":"x"}`,
		`{"field":"value","operator":"contains","value":"x"}`,
		`{"field":"value","operator":"eq","value":"x"}`,
		`{"field":"severity","operator":"eq","value":"BOGUS"}`,
		`{"field":"severity","operator":"in","value":[]}`,
		`{"field":"severity","operator":"eq"}`,
		`{"field":"nope"}`,
	}
	for _, raw := range invalid {
		if _, err := ParseCondition(raw); err == nil {
			t.Fatalf("invalid condition accepted %q", raw)
		}
	}
}

func TestValidateRule(t *testing.T) {
	if err := ValidateRule("", "", "NOPE", "", "", nil, -1); err == nil {
		t.Fatal("empty rule must fail")
	}
	if err := ValidateRule("n", "", "AGENT_OFFLINE", "", "", []string{"EMAIL"}, 0); err != nil {
		t.Fatalf("minimal rule must pass: %v", err)
	}
	if err := ValidateRule("n", "", "AGENT_OFFLINE", "", "", []string{"PIGEON"}, 0); err == nil {
		t.Fatal("bad channel must fail")
	}
	if err := ValidateRule("n", "", "AGENT_OFFLINE", "", "", []string{"EMAIL"}, MaxCooldownSeconds+1); err == nil {
		t.Fatal("overlong cooldown must fail")
	}
}

// TestDeliverHookRoutesExternalChannels verifies the pipeline hook:
// EMAIL/TELEGRAM go through Deliver (not direct sends), IN_APP stays
// direct, and rule cooldown still gates repeat Deliver calls.
func TestDeliverHookRoutesExternalChannels(t *testing.T) {
	db := testdb.Open(t)
	s := &fakeSender{}
	eng := testEngine(t, db, s)
	var mu sync.Mutex
	var got []DeliveryRequest
	eng.Deliver = func(req DeliveryRequest) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, req)
		return "queued", nil
	}
	gid := seedGroup(t, db, "G-hook")
	seedRule(t, db, "R-hook", EventServerAlert, gid, func(s *ruleSeed) {
		s.channels = "EMAIL,TELEGRAM,IN_APP"
		s.cooldown = 3600
	})
	eng.Evaluate(baseEvent())
	eng.Evaluate(baseEvent()) // cooldown: no second Deliver
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 {
		t.Fatalf("want 1 Deliver call, got %d", len(got))
	}
	req := got[0]
	if len(req.Channels) != 2 || req.GroupID != gid {
		t.Fatalf("channels/group wrong: %v %d", req.Channels, req.GroupID)
	}
	if req.Key == "" || req.Severity == "" {
		t.Fatalf("missing key/severity: %+v", req)
	}
	if atomic.LoadInt32(&s.inApps) != 1 {
		t.Fatalf("IN_APP must stay direct, got %d", s.inApps)
	}
	if s.emailCount() != 0 {
		t.Fatalf("EMAIL must not send directly when Deliver is set, got %d", s.emailCount())
	}
}

func TestTelegramChannelValid(t *testing.T) {
	if err := ValidateRule("n", "", "AGENT_OFFLINE", "", "", []string{"TELEGRAM"}, 0); err != nil {
		t.Fatalf("TELEGRAM must be accepted: %v", err)
	}
	if err := ValidateRuleFull(RuleParams{
		Name: "n", EventType: "AGENT_OFFLINE", Channels: []string{"EMAIL"},
		DigestMode: "digest", DigestIntervalMin: 7,
	}); err == nil {
		t.Fatal("bad digest interval must fail")
	}
	if err := ValidateRuleFull(RuleParams{
		Name: "n", EventType: "AGENT_OFFLINE", Channels: []string{"EMAIL"},
		GroupBy: []string{"nope"},
	}); err == nil {
		t.Fatal("bad group-by key must fail")
	}
}

func TestRuleIndependence(t *testing.T) {
	db := testdb.Open(t)
	s := &fakeSender{}
	eng := testEngine(t, db, s)
	g1 := seedGroup(t, db, "G1")
	g2 := seedGroup(t, db, "G2")
	seedRule(t, db, "R1", EventServerAlert, g1, nil)
	seedRule(t, db, "R2", EventServerAlert, g2, nil)
	eng.Evaluate(baseEvent())
	if got := s.emailCount(); got != 2 {
		t.Fatalf("both rules must fire independently, got %d", got)
	}
	seen := map[uint]bool{}
	for _, m := range s.emails {
		seen[m.groupID] = true
	}
	if !seen[g1] || !seen[g2] {
		t.Fatalf("each rule must use its own group: %v", seen)
	}
	// Different fingerprint notifies again (independent logical alerts).
	ev2 := baseEvent()
	ev2.Fingerprint = "fp-2"
	eng.Evaluate(ev2)
	if got := s.emailCount(); got != 4 {
		t.Fatalf("new fingerprint must notify, got %d", got)
	}
}
