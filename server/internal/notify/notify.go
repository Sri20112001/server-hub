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
	"net/mail"
	"net/smtp"
	"os"
	"strconv"
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
	KeyEnabled     = "notify_enabled"
	KeyOnDeploy    = "notify_on_deploy_failed"
	KeyOnThreshold = "notify_on_threshold"
	KeyOnBackup    = "notify_on_backup_failed"
	KeyOnProject   = "notify_on_project_failed"
	KeyTgEnabled   = "notify_tg_enabled"
	KeyTgToken     = "notify_tg_token" // encrypted, "enc:" prefixed
	KeyTgChat      = "notify_tg_chat_id"
	KeySmtpEnabled = "notify_smtp_enabled"
	KeySmtpHost    = "notify_smtp_host"
	KeySmtpPort    = "notify_smtp_port"
	KeySmtpUser    = "notify_smtp_user"
	KeySmtpPass    = "notify_smtp_pass" // encrypted, "enc:" prefixed
	KeySmtpFrom    = "notify_smtp_from"
	KeySmtpTo      = "notify_smtp_to"
	KeySmtpTLS     = "notify_smtp_tls" // "true" (default) | "false"
	// KeyEmailGroupID optionally points email delivery at a notification
	// group (row id in notification_groups). When unset/empty/invalid, the
	// legacy notify_smtp_to address is used (backward compatible).
	KeyEmailGroupID = "notify_email_group_id"
)

// DefaultGroupName is seeded by EnsureDefaultGroup during migration.
const DefaultGroupName = "Default Notifications"

// MaxGroupMembers caps recipients per group (backend-enforced).
const MaxGroupMembers = 10

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

// EnsureDefaultGroup migrates a legacy single-recipient setup to a group.
// Idempotent and safe on every startup: if the legacy notify_smtp_to is set
// and no notification group exists yet, it creates DefaultGroupName with that
// address as its sole member. The legacy To value is left untouched as the
// fallback, so existing installs never lose their recipient.
func EnsureDefaultGroup(db *database.DB) {
	if db == nil {
		return
	}
	m := settings(db)
	legacy := strings.TrimSpace(m[KeySmtpTo])
	if legacy == "" {
		return
	}
	var groups int
	if err := db.QueryRow(`SELECT COUNT(*) FROM notification_groups`).Scan(&groups); err != nil || groups > 0 {
		return
	}
	id, err := db.InsertID(`INSERT INTO notification_groups (name,description,created_at,updated_at)
		VALUES ($1,'Migrated from email settings',NOW(),NOW())`, DefaultGroupName)
	if err != nil {
		return
	}
	email := NormalizeMemberEmail(legacy)
	if email == "" {
		return
	}
	_, _ = db.Exec(`INSERT INTO notification_group_members (group_id,email,created_at)
		VALUES ($1,$2,NOW()) ON CONFLICT DO NOTHING`, id, email)
	_, _ = db.Exec(`INSERT INTO app_settings (key, value) VALUES ($1,$2)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, KeyEmailGroupID, itoa(id))
}

// NormalizeMemberEmail trims + lowercases; "" when unparsable as an address.
func NormalizeMemberEmail(raw string) string {
	email := strings.ToLower(strings.TrimSpace(raw))
	if email == "" {
		return ""
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return ""
	}
	return email
}

func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}

// emailRecipients resolves who gets mail: members of the configured default
// group (up to MaxGroupMembers, in join order) when it exists and is
// non-empty, else the legacy single To address. Every address is
// re-normalized, re-validated, and deduplicated at send time — group rows
// predate validation rules and the legacy field is free-form — so a bad row
// can never poison or duplicate a delivery. Returns nil when nothing valid
// remains (the send then fails cleanly instead of sending nowhere).
func emailRecipients(db *database.DB, m map[string]string) []string {
	if db != nil {
		if gid, err := strconv.ParseInt(strings.TrimSpace(m[KeyEmailGroupID]), 10, 64); err == nil && gid > 0 {
			rows, err := db.Query(`SELECT email FROM notification_group_members
				WHERE group_id=$1 ORDER BY id ASC LIMIT $2`, gid, MaxGroupMembers)
			if err == nil {
				var raw []string
				for rows.Next() {
					var e string
					if err := rows.Scan(&e); err == nil {
						raw = append(raw, e)
					}
				}
				rows.Close()
				if out := cleanEmails(raw); len(out) > 0 {
					return out
				}
			} else if rows != nil {
				rows.Close()
			}
		}
	}
	return cleanEmails([]string{strings.TrimSpace(m[KeySmtpTo])})
}

// cleanEmails normalizes, validates, and dedupes addresses, preserving
// first-seen order. Invalid/blank entries are dropped.
func cleanEmails(in []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, raw := range in {
		e := NormalizeMemberEmail(raw)
		if e == "" || seen[e] {
			continue
		}
		seen[e] = true
		out = append(out, e)
	}
	return out
}

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
			if err := sendEmail(db, m, title, text); err != nil {
				applog.Warn(db.GDB, "system", "notify: email failed: "+err.Error())
			} else {
				// Success marker (recipient count only — never addresses):
				// lets operators answer "did the alert go out?" from app_logs.
				// Failures are logged above; per-recipient receipts are out of
				// scope (see docs/api-notes.md).
				applog.Info(db.GDB, "system", fmt.Sprintf("notify: email sent (%s, %d recipient(s))", event, len(emailRecipients(db, m))))
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
	} else if err := sendEmail(db, m, "ServerHub test signal", "ServerHub test signal — notifications are wired up."); err != nil {
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

func buildEmail(m map[string]string, to []string, subject, text string) (addr string, msg []byte, err error) {
	host := strings.TrimSpace(m[KeySmtpHost])
	port := strings.TrimSpace(m[KeySmtpPort])
	from := strings.TrimSpace(m[KeySmtpFrom])
	if host == "" || from == "" || len(to) == 0 {
		return "", nil, fmt.Errorf("smtp host/from/to required")
	}
	if port == "" {
		port = "587"
	}
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s\r\n",
		from, strings.Join(to, ", "), subject, text)
	return netJoinHostPort(host, port), buf.Bytes(), nil
}

func netJoinHostPort(host, port string) string { return host + ":" + port }

func sendEmail(db *database.DB, m map[string]string, subject, text string) error {
	to := emailRecipients(db, m)
	return deliverEmail(m, to, subject, text)
}

// SendEmailTo delivers to an explicit recipient list (rules engine path).
// The global master + SMTP switches still apply; the list itself is assumed
// pre-resolved (group members) and is cleaned again defensively.
func SendEmailTo(db *database.DB, to []string, subject, text string) error {
	m := settings(db)
	if !on(m, KeyEnabled) {
		return fmt.Errorf("notifications disabled")
	}
	if !on(m, KeySmtpEnabled) {
		return fmt.Errorf("email channel disabled")
	}
	return deliverEmail(m, cleanEmails(to), subject, text)
}

// GroupMemberEmails returns up to MaxGroupMembers addresses for a group,
// cleaned and deduplicated. Empty when the group is missing/empty.
func GroupMemberEmails(db *database.DB, groupID uint) []string {
	if db == nil || groupID == 0 {
		return nil
	}
	rows, err := db.Query(`SELECT email FROM notification_group_members
		WHERE group_id=$1 ORDER BY id ASC LIMIT $2`, groupID, MaxGroupMembers)
	if err != nil {
		return nil
	}
	var raw []string
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err == nil {
			raw = append(raw, e)
		}
	}
	rows.Close()
	return cleanEmails(raw)
}

// NotifyUsers inserts an in-app notification for every user, optionally
// linked to a server and an alert row. Used by the rules engine (IN_APP)
// and mirrored by the legacy per-alert helpers.
func NotifyUsers(db *database.DB, serverID *uint, alertID *int64, title, body, category string) {
	if db == nil {
		return
	}
	rows, err := db.Query(`SELECT username FROM users`)
	if err != nil {
		return
	}
	defer rows.Close()
	var sid, aid any
	if serverID != nil {
		sid = *serverID
	}
	if alertID != nil {
		aid = *alertID
	}
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			continue
		}
		_, _ = db.Exec(`
			INSERT INTO in_app_notifications (username,title,body,category,read,server_id,alert_id,created_at)
			VALUES ($1,$2,$3,$4,false,$5,$6,NOW())`,
			u, title, body, category, sid, aid)
	}
}

func deliverEmail(m map[string]string, to []string, subject, text string) error {
	addr, msg, err := buildEmail(m, to, subject, text)
	if err != nil {
		return err
	}
	host := addr[:strings.LastIndex(addr, ":")]
	user := strings.TrimSpace(m[KeySmtpUser])
	pass := secret(m, KeySmtpPass)
	from := strings.TrimSpace(m[KeySmtpFrom])
	var auth smtp.Auth
	if user != "" {
		auth = smtp.PlainAuth("", user, pass, host)
	}
	if strings.ToLower(strings.TrimSpace(m[KeySmtpTLS])) == "false" {
		return smtp.SendMail(addr, auth, from, to, msg)
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
		for _, rcpt := range to {
			if err := client.Rcpt(rcpt); err != nil {
				return err
			}
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
	return smtp.SendMail(addr, auth, from, to, msg)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…(truncated)"
}
