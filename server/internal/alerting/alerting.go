// Package alerting runs background loops that detect offline servers and
// metric threshold crossings, maintaining TRIGGERED/RESOLVED state.
//
// Notification-flood semantics (Phase 1, no rules engine): each helper only
// notifies on a TRIGGERED transition — fireAlert/notifyAllUsers run only
// when no TRIGGERED row exists for (server_id, condition), and the
// Alertmanager webhook path dedupes on fingerprint the same way. A flapping
// signal (TRIGGERED→RESOLVED→TRIGGERED) notifies once per cycle, which is
// the correct per-incident behavior. True cooldown/grouping/throttling
// semantics belong to the Phase 2 notification rules engine.
package alerting

import (
	"fmt"
	"log"
	"time"

	"serverhub/internal/database"
	"serverhub/internal/events"
	"serverhub/internal/rules"
)

const checkInterval = 60 * time.Second

// defaultOfflineThreshold applies when no timeout is configured.
const defaultOfflineThreshold = 3 * time.Minute

type Thresholds struct {
	CPU  float64
	RAM  float64
	Disk float64
	// OfflineAfter marks a server OFFLINE when its last heartbeat is older
	// than this. Values < 30s fall back to defaultOfflineThreshold.
	OfflineAfter time.Duration
}

func offlineThreshold(th Thresholds) time.Duration {
	if th.OfflineAfter >= 30*time.Second {
		return th.OfflineAfter
	}
	return defaultOfflineThreshold
}

func StartLoop(db *database.DB, broker *events.Broker, th Thresholds, eng *rules.Engine) {
	go func() {
		// Sweep immediately so a restart converges without waiting a tick.
		if err := runChecks(db, broker, th, eng); err != nil {
			log.Printf("alerting: %v", err)
		}
		t := time.NewTicker(checkInterval)
		defer t.Stop()
		for range t.C {
			if err := runChecks(db, broker, th, eng); err != nil {
				log.Printf("alerting: %v", err)
			}
		}
	}()
}

func runChecks(db *database.DB, broker *events.Broker, th Thresholds, eng *rules.Engine) error {
	checkOffline(db, broker, th, eng)
	checkMetricThresholds(db, broker, th, eng)
	return nil
}

func checkOffline(db *database.DB, broker *events.Broker, th Thresholds, eng *rules.Engine) {
	cutoff := time.Now().UTC().Add(-offlineThreshold(th))
	rows, err := db.Query(`
		SELECT id, name FROM managed_servers
		WHERE status='ONLINE' AND (last_heartbeat IS NULL OR last_heartbeat < $1)`, cutoff)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id uint
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			continue
		}
		_, _ = db.Exec(`UPDATE managed_servers SET status='OFFLINE', agent_status='DISCONNECTED', updated_at=NOW() WHERE id=$1`, id)
		fireAlert(db, broker, eng, id, "offline", 0, 0, "CRITICAL",
			fmt.Sprintf("Server %s is offline (no heartbeat)", name))
	}

	onlineRows, err := db.Query(`SELECT id FROM managed_servers WHERE status='ONLINE'`)
	if err != nil {
		return
	}
	defer onlineRows.Close()
	for onlineRows.Next() {
		var id uint
		if err := onlineRows.Scan(&id); err != nil {
			continue
		}
		if resolveAlert(db, broker, id, "offline") && eng != nil {
			sid := id
			eng.Evaluate(rules.Event{
				Type: rules.EventAgentOnline, ServerID: &sid,
				Severity: "INFO", Condition: "offline",
				Message:    fmt.Sprintf("Server %d is back online", id),
				Fingerprint: rules.Fingerprint(id, "offline"),
			})
		}
	}
}

func checkMetricThresholds(db *database.DB, broker *events.Broker, th Thresholds, eng *rules.Engine) {
	// Only evaluate fresh snapshots: a dead server's last high reading must
	// not keep firing threshold alerts (offline detection owns that case).
	freshSince := time.Now().UTC().Add(-offlineThreshold(th))
	rows, err := db.Query(`
		SELECT DISTINCT ON (server_id) server_id, cpu_usage, memory_usage, disk_usage
		FROM server_metrics WHERE timestamp >= $1 ORDER BY server_id, timestamp DESC`, freshSince)
	if err != nil {
		return
	}
	defer rows.Close()

	type check struct {
		condition string
		value     float64
		threshold float64
		severity  string
	}

	for rows.Next() {
		var sid uint
		var cpu, mem, disk float64
		if err := rows.Scan(&sid, &cpu, &mem, &disk); err != nil {
			continue
		}
		checks := []check{
			{"cpu_high", cpu, th.CPU, "WARNING"},
			{"ram_high", mem, th.RAM, "WARNING"},
			{"disk_high", disk, th.Disk, "CRITICAL"},
		}
		for _, ch := range checks {
			if ch.threshold <= 0 {
				continue
			}
			if ch.value > ch.threshold {
				msg := fmt.Sprintf("%s reached %.1f%% (threshold %.0f%%)", ch.condition, ch.value, ch.threshold)
				fireAlert(db, broker, eng, sid, ch.condition, ch.value, ch.threshold, ch.severity, msg)
			} else if resolveAlert(db, broker, sid, ch.condition) && eng != nil {
				id := sid
				eng.Evaluate(rules.Event{
					Type: rules.EventServerAlertResolved, ServerID: &id,
					Severity: ch.severity, Condition: ch.condition,
					Value: ch.value, Threshold: ch.threshold,
					Message:    fmt.Sprintf("%s recovered to %.1f%% (threshold %.0f%%)", ch.condition, ch.value, ch.threshold),
					Fingerprint: rules.Fingerprint(sid, ch.condition),
				})
			}
		}
	}
	rows.Close()

	// Servers whose snapshots went stale keep no live threshold state:
	// resolve their metric alerts so only the offline alert stays firing.
	stale, err := db.Query(`
		SELECT server_id FROM server_metrics
		GROUP BY server_id HAVING MAX(timestamp) < $1`, freshSince)
	if err != nil {
		return
	}
	defer stale.Close()
	for stale.Next() {
		var sid uint
		if err := stale.Scan(&sid); err != nil {
			continue
		}
		resolveAlert(db, broker, sid, "cpu_high")
		resolveAlert(db, broker, sid, "ram_high")
		resolveAlert(db, broker, sid, "disk_high")
	}
}

func fireAlert(db *database.DB, broker *events.Broker, eng *rules.Engine, serverID uint, condition string, value, threshold float64, severity, message string) {
	var count int
	_ = db.QueryRow(`
		SELECT COUNT(*) FROM alerts WHERE server_id=$1 AND condition=$2 AND status='TRIGGERED'`,
		serverID, condition).Scan(&count)
	if count > 0 {
		return
	}
	_, err := db.Exec(`
		INSERT INTO alerts (server_id,condition,threshold,severity,status,message,triggered_at)
		VALUES ($1,$2,$3,$4,'TRIGGERED',$5,NOW())`,
		serverID, condition, threshold, severity, message)
	if err != nil {
		return
	}
	notifyAllUsers(db, serverID, severity+": "+message, message, "alert")
	if broker != nil {
		broker.Publish("alert.triggered", map[string]interface{}{
			"serverId": serverID, "condition": condition, "severity": severity, "message": message,
		})
	}
	// Rules engine hook (additive): the legacy in-app + SSE path above is
	// untouched; rules decide on extra EMAIL/IN_APP delivery.
	if eng != nil {
		evType := rules.EventServerAlert
		if condition == "offline" {
			evType = rules.EventAgentOffline
		}
		eng.Evaluate(rules.Event{
			Type: evType, ServerID: &serverID,
			Severity: severity, Condition: condition,
			Value: value, Threshold: threshold,
			Message:     message,
			Fingerprint: rules.Fingerprint(serverID, condition),
		})
	}
}

// resolveAlert closes the TRIGGERED row, reporting whether a live alert was
// actually resolved (transition-only, like the SSE publish below).
func resolveAlert(db *database.DB, broker *events.Broker, serverID uint, condition string) bool {
	res, err := db.Exec(`
		UPDATE alerts SET status='RESOLVED', resolved_at=NOW()
		WHERE server_id=$1 AND condition=$2 AND status='TRIGGERED'`,
		serverID, condition)
	if err != nil {
		return false
	}
	n, _ := res.RowsAffected()
	if n > 0 && broker != nil {
		broker.Publish("alert.resolved", map[string]interface{}{
			"serverId": serverID, "condition": condition,
		})
	}
	return n > 0
}

func notifyAllUsers(db *database.DB, serverID uint, title, body, category string) {
	rows, err := db.Query(`SELECT username FROM users`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			continue
		}
		_, _ = db.Exec(`
			INSERT INTO in_app_notifications (username,title,body,category,read,server_id,created_at)
			VALUES ($1,$2,$3,$4,false,$5,NOW())`,
			u, title, body, category, serverID)
	}
}
