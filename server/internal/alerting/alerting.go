// Package alerting runs background loops that detect offline servers and
// metric threshold crossings, maintaining TRIGGERED/RESOLVED state.
package alerting

import (
	"fmt"
	"log"
	"time"

	"serverhub/internal/database"
	"serverhub/internal/events"
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

func StartLoop(db *database.DB, broker *events.Broker, th Thresholds) {
	go func() {
		// Sweep immediately so a restart converges without waiting a tick.
		if err := runChecks(db, broker, th); err != nil {
			log.Printf("alerting: %v", err)
		}
		t := time.NewTicker(checkInterval)
		defer t.Stop()
		for range t.C {
			if err := runChecks(db, broker, th); err != nil {
				log.Printf("alerting: %v", err)
			}
		}
	}()
}

func runChecks(db *database.DB, broker *events.Broker, th Thresholds) error {
	checkOffline(db, broker, th)
	checkMetricThresholds(db, broker, th)
	return nil
}

func checkOffline(db *database.DB, broker *events.Broker, th Thresholds) {
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
		fireAlert(db, broker, id, "offline", 0, "CRITICAL",
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
		resolveAlert(db, broker, id, "offline")
	}
}

func checkMetricThresholds(db *database.DB, broker *events.Broker, th Thresholds) {
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
				fireAlert(db, broker, sid, ch.condition, ch.threshold, ch.severity, msg)
			} else {
				resolveAlert(db, broker, sid, ch.condition)
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

func fireAlert(db *database.DB, broker *events.Broker, serverID uint, condition string, threshold float64, severity, message string) {
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
}

func resolveAlert(db *database.DB, broker *events.Broker, serverID uint, condition string) {
	res, err := db.Exec(`
		UPDATE alerts SET status='RESOLVED', resolved_at=NOW()
		WHERE server_id=$1 AND condition=$2 AND status='TRIGGERED'`,
		serverID, condition)
	if err != nil {
		return
	}
	if n, _ := res.RowsAffected(); n > 0 && broker != nil {
		broker.Publish("alert.resolved", map[string]interface{}{
			"serverId": serverID, "condition": condition,
		})
	}
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
