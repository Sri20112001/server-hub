package crypto

import (
	"strings"
	"testing"
)

func TestRoundtrip(t *testing.T) {
	key, err := KeyFromHex("00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff")
	if err != nil {
		t.Fatal(err)
	}
	ct, nonce, err := Encrypt(key, "supersecret-value")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(ct, "supersecret") {
		t.Fatal("ciphertext leaks plaintext")
	}
	pt, err := Decrypt(key, ct, nonce)
	if err != nil {
		t.Fatal(err)
	}
	if pt != "supersecret-value" {
		t.Fatalf("got %q", pt)
	}
}

func TestWrongKeyFails(t *testing.T) {
	k1, _ := KeyFromHex(strings.Repeat("01", 32))
	k2, _ := KeyFromHex(strings.Repeat("02", 32))
	ct, nonce, err := Encrypt(k1, "x")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decrypt(k2, ct, nonce); err == nil {
		t.Fatal("expected decryption failure with wrong key")
	}
}

func TestKeyFromHexValidation(t *testing.T) {
	if _, err := KeyFromHex("tooshort"); err == nil {
		t.Fatal("expected error for short key")
	}
	if _, err := KeyFromHex(strings.Repeat("zz", 32)); err == nil {
		t.Fatal("expected error for non-hex key")
	}
}
