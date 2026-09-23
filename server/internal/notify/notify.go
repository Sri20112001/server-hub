// Package notify delivers failure/resource signals to external channels.
//
// Configuration lives in the app_settings table (edited via the Settings UI),
// so no restart is needed to change channels. Sends are fire-and-forget:
// callers invoke Send and it fans out in a goroutine with timeouts, so event
// paths (deploys, backups, telemetry) never block on the network. Delivery
// failures are recorded in app_logs, never raised to the caller.
package notify

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/smtp"
	"os"
	"strings"
	"time"

	"serverhub/internal/applog"
	"serverhub/internal/crypto"
	"serverhub/internal/database"
)

// Events. Each has an app_settings toggle notify_on_<event>.
const (
	EventDeployFailed  = "deploy_failed"
	EventThreshold     = "threshold"
	EventBackupFailed  = "backup_failed"
	EventProjectFailed = "project_failed"
)

// Setting keys.
const (
	KeyEnabled      = "notify_enabled"
	KeyOnDeploy     = "notify_on_deploy_failed"
	KeyOnThreshold  = "notify_on_threshold"
	KeyOnBackup     = "notify_on_backup_failed"
	KeyOnProject    = "notify_on_project_failed"
	KeyTgEnabled    = "notify_tg_enabled"
	KeyTgToken      = "notify_tg_token" // encrypted, "enc:" prefixed
	KeyTgChat       = "notify_tg_chat_id"
	KeySmtpEnabled  = "notify_smtp_enabled"
	KeySmtpHost     = "notify_smtp_host"
	KeySmtpPort     = "notify_smtp_port"
	KeySmtpUser     = "notify_smtp_user"
	KeySmtpPass     = "notify_smtp_pass" // encrypted, "enc:" prefixed
	KeySmtpFrom     = "notify_smtp_from"
	KeySmtpTo       = "notify_smtp_to"
	KeySmtpTLS      = "notify_smtp_tls" // "true" (default) | "false"
)

func eventKey(event string) string {
	switch event {
	case EventDeployFailed:
		return KeyOnDeploy
	case EventThreshold:
		return KeyOnThreshold
	case EventBackupFailed:
		return KeyOnBackup
	case EventProjectFailed:
		return KeyOnProject
	}
	return ""
}

func settings(db *database.DB) map[string]string {
	m := map[string]string{}
	if db == nil {
		return m
	}
	rows, err := db.Query(`SELECT key, value FROM app_settings`)
	if err != nil {
		return m
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err == nil {
			m[k] = v
		}
	}
	return m
}

func on(m map[string]string, key string) bool {
	v := strings.ToLower(strings.TrimSpace(m[key]))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

// secret decrypts "enc:"-prefixed values with the server encryption key.
func secret(m map[string]string, key string) string {
	v := m[key]
	enc, ok := strings.CutPrefix(v, "enc:")
	if !ok {
		return ""
	}
	ct, nonce, ok := strings.Cut(enc, ".")
	if !ok {
		return ""
	}
	k, err := crypto.KeyFromHex(os.Getenv("SERVERHUB_ENCRYPTION_KEY"))
	if err != nil {
		return ""
	}
	pt, err := crypto.Decrypt(k, ct, nonce)
	if err != nil {
		return ""
	}
	return pt
}

// EncryptSecret wraps a plaintext secret for storage ("" stays "").
func EncryptSecret(plain string) string {
	if plain == "" {
		return ""
	}
	k, err := crypto.KeyFromHex(os.Getenv("SERVERHUB_ENCRYPTION_KEY"))
	if err != nil {
		return ""
	}
	ct, nonce, err := crypto.Encrypt(k, plain)
	if err != nil {
		return ""
	}
	return "enc:" + ct + "." + nonce
}

// Send delivers title/body to every enabled channel for the event.
// Never blocks the caller; never returns delivery errors (they go to app_logs).
func Send(db *database.DB, event, title, body string) {
	if db == nil || eventKey(event) == "" {
		return
	}
	go func() {
		m := settings(db)
		if !on(m, KeyEnabled) || !on(m, eventKey(event)) {
			return
		}
		text := title
		if strings.TrimSpace(body) != "" {
			text += "\n" + truncate(body, 1500)
		}
		if on(m, KeyTgEnabled) {
			if err := sendTelegram(m, text); err != nil {
				applog.Warn(db.GDB, "system", "notify: telegram failed: "+err.Error())
			}
		}
		if on(m, KeySmtpEnabled) {
			if err := sendEmail(m, title, text); err != nil {
				applog.Warn(db.GDB, "system", "notify: email failed: "+err.Error())
			}
		}
	}()
}

// Test sends a probe message via each enabled channel and reports per-channel
// results synchronously (used by the Settings "Send test" button).
func Test(db *database.DB) (telegram, email string) {
	m := settings(db)
	if !on(m, KeyEnabled) {
		return "disabled", "disabled"
	}
	if !on(m, KeyTgEnabled) {
		telegram = "disabled"
	} else if err := sendTelegram(m, "ServerHub test signal — notifications are wired up."); err != nil {
		telegram = "failed: " + err.Error()
	} else {
		telegram = "sent"
	}
	if !on(m, KeySmtpEnabled) {
		email = "disabled"
	} else if err := sendEmail(m, "ServerHub test signal", "ServerHub test signal — notifications are wired up."); err != nil {
		email = "failed: " + err.Error()
	} else {
		email = "sent"
	}
	return telegram, email
}

func sendTelegram(m map[string]string, text string) error {
	token := secret(m, KeyTgToken)
	chat := strings.TrimSpace(m[KeyTgChat])
	if token == "" || chat == "" {
		return fmt.Errorf("bot token or chat id missing")
	}
	payload, _ := json.Marshal(map[string]string{"chat_id": chat, "text": text})
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post("https://api.telegram.org/bot"+token+"/sendMessage",
		"application/json", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("telegram api: %s", resp.Status)
	}
	return nil
}

func buildEmail(m map[string]string, subject, text string) (addr string, msg []byte, err error) {
	host := strings.TrimSpace(m[KeySmtpHost])
	port := strings.TrimSpace(m[KeySmtpPort])
	from := strings.TrimSpace(m[KeySmtpFrom])
	to := strings.TrimSpace(m[KeySmtpTo])
	if host == "" || from == "" || to == "" {
		return "", nil, fmt.Errorf("smtp host/from/to required")
	}
	if port == "" {
		port = "587"
	}
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s\r\n",
		from, to, subject, text)
	return netJoinHostPort(host, port), buf.Bytes(), nil
}

func netJoinHostPort(host, port string) string { return host + ":" + port }

func sendEmail(m map[string]string, subject, text string) error {
	addr, msg, err := buildEmail(m, subject, text)
	if err != nil {
		return err
	}
	host := addr[:strings.LastIndex(addr, ":")]
	user := strings.TrimSpace(m[KeySmtpUser])
	pass := secret(m, KeySmtpPass)
	from := strings.TrimSpace(m[KeySmtpFrom])
	to := strings.TrimSpace(m[KeySmtpTo])
	var auth smtp.Auth
	if user != "" {
		auth = smtp.PlainAuth("", user, pass, host)
	}
	if strings.ToLower(strings.TrimSpace(m[KeySmtpTLS])) == "false" {
		return smtp.SendMail(addr, auth, from, []string{to}, msg)
	}
	// Implicit TLS (port 465 style): dial TLS first.
	if strings.HasSuffix(addr, ":465") {
		conn, err := tls.DialWithDialer(
			&net.Dialer{Timeout: 10 * time.Second},
			"tcp", addr, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
		if err != nil {
			return err
		}
		defer conn.Close()
		client, err := smtp.NewClient(conn, host)
		if err != nil {
			return err
		}
		defer client.Close()
		if auth != nil {
			if ok, _ := client.Extension("AUTH"); ok {
				if err := client.Auth(auth); err != nil {
					return err
				}
			}
		}
		if err := client.Mail(from); err != nil {
			return err
		}
		if err := client.Rcpt(to); err != nil {
			return err
		}
		w, err := client.Data()
		if err != nil {
			return err
		}
		if _, err := w.Write(msg); err != nil {
			return err
		}
		if err := w.Close(); err != nil {
			return err
		}
		return client.Quit()
	}
	return smtp.SendMail(addr, auth, from, []string{to}, msg)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…(truncated)"
}
