package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"serverhub/internal/audit"
	"serverhub/internal/database"
	"serverhub/internal/middleware"
	"serverhub/internal/notify"
)

// Notification settings live in app_settings (server-side, all operators
// share them). Secrets are stored encrypted; reads only report hasToken/hasPassword.
type NotificationsHandler struct {
	DB *database.DB
}

func notifyAll(db *database.DB) map[string]string {
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

func notifyBool(m map[string]string, key string, def bool) bool {
	v, ok := m[key]
	if !ok {
		return def
	}
	v = strings.ToLower(strings.TrimSpace(v))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func notifySet(db *database.DB, key, value string) error {
	_, err := db.Exec(`INSERT INTO app_settings (key, value) VALUES (?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

type notifyView struct {
	Enabled bool `json:"enabled"`
	Events  struct {
		DeployFailed  bool `json:"deployFailed"`
		Threshold     bool `json:"threshold"`
		BackupFailed  bool `json:"backupFailed"`
		ProjectFailed bool `json:"projectFailed"`
	} `json:"events"`
	Telegram struct {
		Enabled  bool   `json:"enabled"`
		ChatID   string `json:"chatId"`
		HasToken bool   `json:"hasToken"`
	} `json:"telegram"`
	Email struct {
		Enabled     bool   `json:"enabled"`
		Host        string `json:"host"`
		Port        string `json:"port"`
		Username    string `json:"username"`
		From        string `json:"from"`
		To          string `json:"to"`
		TLS         bool   `json:"tls"`
		HasPassword bool   `json:"hasPassword"`
	} `json:"email"`
}

// GET /server-hub/api/settings/notifications
func (h *NotificationsHandler) Get(c *gin.Context) {
	m := notifyAll(h.DB)
	var v notifyView
	v.Enabled = notifyBool(m, notify.KeyEnabled, false)
	v.Events.DeployFailed = notifyBool(m, notify.KeyOnDeploy, true)
	v.Events.Threshold = notifyBool(m, notify.KeyOnThreshold, true)
	v.Events.BackupFailed = notifyBool(m, notify.KeyOnBackup, true)
	v.Events.ProjectFailed = notifyBool(m, notify.KeyOnProject, true)
	v.Telegram.Enabled = notifyBool(m, notify.KeyTgEnabled, false)
	v.Telegram.ChatID = m[notify.KeyTgChat]
	v.Telegram.HasToken = strings.HasPrefix(m[notify.KeyTgToken], "enc:")
	v.Email.Enabled = notifyBool(m, notify.KeySmtpEnabled, false)
	v.Email.Host = m[notify.KeySmtpHost]
	v.Email.Port = m[notify.KeySmtpPort]
	if v.Email.Port == "" {
		v.Email.Port = "587"
	}
	v.Email.Username = m[notify.KeySmtpUser]
	v.Email.From = m[notify.KeySmtpFrom]
	v.Email.To = m[notify.KeySmtpTo]
	v.Email.TLS = notifyBool(m, notify.KeySmtpTLS, true)
	v.Email.HasPassword = strings.HasPrefix(m[notify.KeySmtpPass], "enc:")
	c.JSON(http.StatusOK, v)
}

// PUT /server-hub/api/settings/notifications — empty token/password keeps
// the stored one.
func (h *NotificationsHandler) Update(c *gin.Context) {
	var body struct {
		Enabled *bool `json:"enabled"`
		Events  struct {
			DeployFailed  *bool `json:"deployFailed"`
			Threshold     *bool `json:"threshold"`
			BackupFailed  *bool `json:"backupFailed"`
			ProjectFailed *bool `json:"projectFailed"`
		} `json:"events"`
		Telegram struct {
			Enabled *bool  `json:"enabled"`
			ChatID  string `json:"chatId"`
			Token   string `json:"token"`
		} `json:"telegram"`
		Email struct {
			Enabled  *bool  `json:"enabled"`
			Host     string `json:"host"`
			Port     string `json:"port"`
			Username string `json:"username"`
			Password string `json:"password"`
			From     string `json:"from"`
			To       string `json:"to"`
			TLS      *bool  `json:"tls"`
		} `json:"email"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	setBool := func(key string, v *bool) error {
		if v == nil {
			return nil
		}
		val := "false"
		if *v {
			val = "true"
		}
		return notifySet(h.DB, key, val)
	}
	setStr := func(key, v string) error {
		return notifySet(h.DB, key, strings.TrimSpace(v))
	}
	for _, fn := range []func() error{
		func() error { return setBool(notify.KeyEnabled, body.Enabled) },
		func() error { return setBool(notify.KeyOnDeploy, body.Events.DeployFailed) },
		func() error { return setBool(notify.KeyOnThreshold, body.Events.Threshold) },
		func() error { return setBool(notify.KeyOnBackup, body.Events.BackupFailed) },
		func() error { return setBool(notify.KeyOnProject, body.Events.ProjectFailed) },
		func() error { return setBool(notify.KeyTgEnabled, body.Telegram.Enabled) },
		func() error { return setStr(notify.KeyTgChat, body.Telegram.ChatID) },
		func() error { return setBool(notify.KeySmtpEnabled, body.Email.Enabled) },
		func() error { return setStr(notify.KeySmtpHost, body.Email.Host) },
		func() error { return setStr(notify.KeySmtpUser, body.Email.Username) },
		func() error { return setStr(notify.KeySmtpFrom, body.Email.From) },
		func() error { return setStr(notify.KeySmtpTo, body.Email.To) },
		func() error { return setBool(notify.KeySmtpTLS, body.Email.TLS) },
	} {
		if err := fn(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	if strings.TrimSpace(body.Email.Port) != "" {
		if _, err := strconv.Atoi(strings.TrimSpace(body.Email.Port)); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "smtp port must be numeric"})
			return
		}
		if err := setStr(notify.KeySmtpPort, body.Email.Port); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	if body.Telegram.Token != "" {
		if err := notifySet(h.DB, notify.KeyTgToken, notify.EncryptSecret(body.Telegram.Token)); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	if body.Email.Password != "" {
		if err := notifySet(h.DB, notify.KeySmtpPass, notify.EncryptSecret(body.Email.Password)); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	u, _ := middleware.CurrentUser(c)
	audit.Write(h.DB, u, "update", "notification-settings", "", "ok", "")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// POST /server-hub/api/settings/notifications/test — probe enabled channels.
func (h *NotificationsHandler) Test(c *gin.Context) {
	tg, mail := notify.Test(h.DB)
	c.JSON(http.StatusOK, gin.H{"telegram": tg, "email": mail})
}
