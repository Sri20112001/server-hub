package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"serverhub/internal/applog"
)

// Login rate limiting.
//
// Two independent throttles protect the password endpoints:
//   - per-IP sliding window: ipMaxAttempts attempts per ipWindow.
//   - per-account: acctMaxFailures consecutive failures lock the account for
//     a doubling backoff (baseLockout, capped at maxLockout). A successful
//     login clears the account's failure streak.
//
// IP-only limiting is bypassable with rotating addresses, so the account
// throttle is the real brute-force defense; the IP window stops casual
// credential stuffing and spreads the cost.
const (
	ipMaxAttempts   = 5
	ipWindow        = time.Minute
	acctMaxFailures = 10
	baseLockout     = time.Minute
	maxLockout      = 30 * time.Minute
	maxTrackedIPs   = 10000
	maxTrackedAccts = 10000
)

type acctState struct {
	failures     int
	lockoutUntil time.Time
	lockLevel    int // doublings applied, caps the backoff
}

// LoginLimiter is safe for concurrent use. It is pure in-memory state:
// restarting the server resets all counters (fail-closed only while running,
// which is the correct trade-off for a single-instance control plane).
type LoginLimiter struct {
	mu       sync.Mutex
	ipHits   map[string][]time.Time
	accounts map[string]*acctState
}

// NewLoginLimiter returns an empty limiter.
func NewLoginLimiter() *LoginLimiter {
	return &LoginLimiter{
		ipHits:   map[string][]time.Time{},
		accounts: map[string]*acctState{},
	}
}

func (l *LoginLimiter) pruneLocked(now time.Time, ip string) []time.Time {
	hits := l.ipHits[ip]
	kept := hits[:0]
	for _, t := range hits {
		if now.Sub(t) < ipWindow {
			kept = append(kept, t)
		}
	}
	return kept
}

// blocked reports whether the IP or account is currently throttled, with the
// time the caller must wait before retrying.
func (l *LoginLimiter) blocked(ip, user string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if hits := l.pruneLocked(now, ip); len(hits) >= ipMaxAttempts {
		oldest := hits[0]
		for _, t := range hits[1:] {
			if t.Before(oldest) {
				oldest = t
			}
		}
		return true, ipWindow - now.Sub(oldest)
	}
	if user != "" {
		if st, ok := l.accounts[user]; ok && now.Before(st.lockoutUntil) {
			return true, time.Until(st.lockoutUntil)
		}
	}
	return false, 0
}

// recordFailure registers a failed authentication for the IP and account.
func (l *LoginLimiter) recordFailure(ip, user string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if len(l.ipHits) >= maxTrackedIPs {
		l.ipHits = map[string][]time.Time{}
	}
	l.ipHits[ip] = append(l.pruneLocked(now, ip), now)
	if user == "" {
		return
	}
	if len(l.accounts) >= maxTrackedAccts {
		l.accounts = map[string]*acctState{}
	}
	st, ok := l.accounts[user]
	if !ok {
		st = &acctState{}
		l.accounts[user] = st
	}
	st.failures++
	if st.failures >= acctMaxFailures && now.After(st.lockoutUntil) {
		backoff := baseLockout << st.lockLevel
		if backoff > maxLockout || backoff <= 0 {
			backoff = maxLockout
		}
		st.lockoutUntil = now.Add(backoff)
		if backoff < maxLockout {
			st.lockLevel++
		}
	}
}

// recordSuccess clears the account's failure streak (the IP sliding window
// is left intact — a burst of logins from one address stays throttled).
func (l *LoginLimiter) recordSuccess(user string) {
	if user == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.accounts, user)
}

// peekUsername best-effort extracts the login username from the request body
// without consuming it (the handler still needs to bind the body).
func peekUsername(c *gin.Context) string {
	if c.Request.Body == nil {
		return ""
	}
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return ""
	}
	_ = c.Request.Body.Close()
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	var v struct {
		Username string `json:"username"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		return ""
	}
	return v.Username
}

// LoginRateLimit throttles brute force against the login/token endpoints.
// Successful and failed attempts are recorded by inspecting the response
// status after the handler runs (200 = success, 401 = failure); lockouts
// are audit-logged to the append-only store.
func LoginRateLimit(gdb *gorm.DB, l *LoginLimiter) gin.HandlerFunc {
	if l == nil {
		l = NewLoginLimiter()
	}
	return func(c *gin.Context) {
		ip := c.ClientIP()
		user := peekUsername(c)
		if blocked, wait := l.blocked(ip, user); blocked {
			secs := int(wait.Seconds()) + 1
			c.Header("Retry-After", itoa(secs))
			applog.Warn(gdb, "auth", "login throttled ip="+ip+" user="+user)
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "too many login attempts, retry later"})
			return
		}
		c.Next()
		switch c.Writer.Status() {
		case http.StatusOK:
			l.recordSuccess(user)
		case http.StatusUnauthorized:
			l.recordFailure(ip, user)
			if blocked, _ := l.blocked(ip, user); blocked {
				applog.Warn(gdb, "auth", "login lockout engaged user="+user+" ip="+ip)
			}
		}
	}
}

func itoa(n int) string {
	if n < 0 {
		n = 0
	}
	var buf [16]byte
	i := len(buf)
	if n == 0 {
		i--
		buf[i] = '0'
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
