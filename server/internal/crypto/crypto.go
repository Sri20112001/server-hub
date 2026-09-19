package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
)

// KeyFromHex parses a 64-char hex string (32 bytes) into a key.
func KeyFromHex(h string) ([]byte, error) {
	k, err := hex.DecodeString(h)
	if err != nil {
		return nil, err
	}
	if len(k) != 32 {
		return nil, errors.New("encryption key must be 32 bytes hex-encoded (64 chars)")
	}
	return k, nil
}

func Encrypt(key []byte, plaintext string) (cipherHex, nonceHex string, err error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", "", err
	}
	ct := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	return hex.EncodeToString(ct), hex.EncodeToString(nonce), nil
}

func Decrypt(key []byte, cipherHex, nonceHex string) (string, error) {
	ct, err := hex.DecodeString(cipherHex)
	if err != nil {
		return "", err
	}
	nonce, err := hex.DecodeString(nonceHex)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}
