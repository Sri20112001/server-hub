package notify

import (
	"strings"
	"testing"

	"serverhub/internal/testdb"
)

const testKey = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"

func TestEncryptSecretRoundtrip(t *testing.T) {
	t.Setenv("SERVERHUB_ENCRYPTION_KEY", testKey)
	if got := EncryptSecret(""); got != "" {
		t.Fatalf("empty must stay empty, got %q", got)
	}
	stored := EncryptSecret("s3cret-token")
	if stored == "" || len(stored) < 5 || stored[:4] != "enc:" {
		t.Fatalf("bad stored form: %q", stored)
	}
	m := map[string]string{"k": stored}
	if got := secret(m, "k"); got != "s3cret-token" {
		t.Fatalf("roundtrip failed: %q", got)
	}
	if got := secret(map[string]string{"k": "plain"}, "k"); got != "" {
		t.Fatalf("plaintext must not decrypt: %q", got)
	}
}

func TestBuildEmailValidation(t *testing.T) {
	if _, _, err := buildEmail(map[string]string{}, nil, "s", "b"); err == nil {
		t.Fatal("missing host/from/to must fail")
	}
	addr, msg, err := buildEmail(map[string]string{
		KeySmtpHost: "smtp.example.com", KeySmtpFrom: "a@x.y",
	}, []string{"b@x.y", "c@x.y"}, "Hi", "body")
	if err != nil || addr != "smtp.example.com:587" || len(msg) == 0 {
		t.Fatalf("build failed: %v %q %d", err, addr, len(msg))
	}
	if !strings.Contains(string(msg), "To: b@x.y, c@x.y\r\n") {
		t.Fatalf("multi-recipient To header missing: %q", msg)
	}
}

func TestSendDisabledNoop(t *testing.T) {
	db := testdb.Open(t)
	// No settings rows → Send must be a silent no-op (and return fast).
	Send(db, EventDeployFailed, "title", "body")
	Send(db, "bogus-event", "title", "body")
	Send(nil, EventDeployFailed, "title", "body")
}

func TestEmailRecipientsLegacyFallback(t *testing.T) {
	db := testdb.Open(t)
	m := map[string]string{KeySmtpTo: "solo@x.y"}
	got := emailRecipients(db, m)
	if len(got) != 1 || got[0] != "solo@x.y" {
		t.Fatalf("legacy fallback broken: %v", got)
	}
	if got := emailRecipients(db, map[string]string{}); len(got) != 0 {
		t.Fatalf("empty settings must yield no recipients: %v", got)
	}
}

func TestEnsureDefaultGroupIdempotent(t *testing.T) {
	db := testdb.Open(t)
	if _, err := db.Exec(`INSERT INTO app_settings (key, value) VALUES (?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, KeySmtpTo, "Legacy@x.y"); err != nil {
		t.Fatal(err)
	}
	EnsureDefaultGroup(db)
	EnsureDefaultGroup(db) // second run must not duplicate
	var groups int
	if err := db.QueryRow(`SELECT COUNT(*) FROM notification_groups`).Scan(&groups); err != nil || groups != 1 {
		t.Fatalf("expected exactly 1 group, got %d (%v)", groups, err)
	}
	var members int
	if err := db.QueryRow(`SELECT COUNT(*) FROM notification_group_members`).Scan(&members); err != nil || members != 1 {
		t.Fatalf("expected exactly 1 member, got %d (%v)", members, err)
	}
	var email, gid string
	if err := db.QueryRow(`SELECT email FROM notification_group_members`).Scan(&email); err != nil || email != "legacy@x.y" {
		t.Fatalf("member email must be normalized lowercase: %q (%v)", email, err)
	}
	if err := db.QueryRow(`SELECT value FROM app_settings WHERE key=$1`, KeyEmailGroupID).Scan(&gid); err != nil || gid == "" {
		t.Fatalf("default group id must be stored: %q (%v)", gid, err)
	}
	// Recipients now resolve from the group, not the legacy field.
	got := emailRecipients(db, settings(db))
	if len(got) != 1 || got[0] != "legacy@x.y" {
		t.Fatalf("group resolution broken: %v", got)
	}
}

func TestNormalizeMemberEmail(t *testing.T) {
	if got := NormalizeMemberEmail("  Admin@X.Y "); got != "admin@x.y" {
		t.Fatalf("normalize broken: %q", got)
	}
	for _, bad := range []string{"", "not-an-email", "a@b@c@d"} {
		if got := NormalizeMemberEmail(bad); got != "" {
			t.Fatalf("invalid %q must normalize to empty, got %q", bad, got)
		}
	}
}

func TestEnsureDefaultGroupSkipsWhenGroupsExist(t *testing.T) {
	db := testdb.Open(t)
	if _, err := db.Exec(`INSERT INTO notification_groups (name,created_at,updated_at)
		VALUES ('Personal',NOW(),NOW())`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO app_settings (key, value) VALUES (?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, KeySmtpTo, "keep@x.y"); err != nil {
		t.Fatal(err)
	}
	EnsureDefaultGroup(db)
	var groups int
	if err := db.QueryRow(`SELECT COUNT(*) FROM notification_groups`).Scan(&groups); err != nil || groups != 1 {
		t.Fatalf("user group must be preserved untouched, got %d (%v)", groups, err)
	}
	var members int
	if err := db.QueryRow(`SELECT COUNT(*) FROM notification_group_members`).Scan(&members); err != nil || members != 0 {
		t.Fatalf("no members must be added, got %d (%v)", members, err)
	}
	var gid string
	if err := db.QueryRow(`SELECT value FROM app_settings WHERE key=$1`, KeyEmailGroupID).Scan(&gid); err == nil && gid != "" {
		t.Fatalf("group selector must stay unset, got %q", gid)
	}
}

func TestEmailRecipientsDedupeAndValidate(t *testing.T) {
	db := testdb.Open(t)
	if _, err := db.Exec(`INSERT INTO notification_groups (name,created_at,updated_at)
		VALUES ('G',NOW(),NOW())`); err != nil {
		t.Fatal(err)
	}
	var gid int64
	if err := db.QueryRow(`SELECT id FROM notification_groups WHERE name='G'`).Scan(&gid); err != nil {
		t.Fatal(err)
	}
	for _, e := range []string{"A@x.y", "a@x.y ", "bad-address", "b@x.y"} {
		_, _ = db.Exec(`INSERT INTO notification_group_members (group_id,email,created_at)
			VALUES ($1,$2,NOW()) ON CONFLICT DO NOTHING`, gid, e)
	}
	m := map[string]string{KeyEmailGroupID: itoa(gid), KeySmtpTo: "fallback@x.y"}
	got := emailRecipients(db, m)
	if len(got) != 2 {
		t.Fatalf("expected 2 clean recipients (case-dup merged, invalid dropped), got %v", got)
	}
	seen := map[string]bool{}
	for _, e := range got {
		if seen[e] {
			t.Fatalf("duplicate recipient %q", e)
		}
		seen[e] = true
	}
	if !seen["a@x.y"] || !seen["b@x.y"] {
		t.Fatalf("unexpected set: %v", got)
	}
}
