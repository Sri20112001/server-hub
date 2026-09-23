package notify

import (
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
	if _, _, err := buildEmail(map[string]string{}, "s", "b"); err == nil {
		t.Fatal("missing host/from/to must fail")
	}
	addr, msg, err := buildEmail(map[string]string{
		KeySmtpHost: "smtp.example.com", KeySmtpFrom: "a@x.y", KeySmtpTo: "b@x.y",
	}, "Hi", "body")
	if err != nil || addr != "smtp.example.com:587" || len(msg) == 0 {
		t.Fatalf("build failed: %v %q %d", err, addr, len(msg))
	}
}

func TestSendDisabledNoop(t *testing.T) {
	db := testdb.Open(t)
	// No settings rows → Send must be a silent no-op (and return fast).
	Send(db, EventDeployFailed, "title", "body")
	Send(db, "bogus-event", "title", "body")
	Send(nil, EventDeployFailed, "title", "body")
}
