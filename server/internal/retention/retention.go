// Package retention bounds the server_metrics history table.
//
// At one sample per server per minute (~1,440 rows/server/day) the table
// grows indefinitely without pruning. Cleanup deletes only rows strictly
// older than the retention cutoff, per server (so the existing
// (server_id, timestamp) composite index serves every statement and no new
// index is needed), in small autocommitted batches. Nothing else is
// touched: alerts, servers, tokens, groups, rules, logs are out of scope.
package retention

import (
	"context"
	"fmt"
	"log"
	"time"

	"serverhub/internal/database"
)

const (
	// DefaultInterval paces cleanup runs (hourly is plenty for daily growth).
	DefaultInterval = time.Hour
	// BatchSize caps rows per DELETE so no statement holds a long lock.
	BatchSize = 5000
	// MaxBatchesPerRun caps total work per run (~600k rows); a larger
	// backlog converges over subsequent hourly runs.
	MaxBatchesPerRun = 120
)

// Cutoff returns the instant before which rows are eligible for deletion.
// Semantics are strict: a row exactly at the cutoff is retained.
func Cutoff(now time.Time, retentionDays int) time.Time {
	return now.UTC().AddDate(0, 0, -retentionDays)
}

// Cleanup deletes server_metrics rows older than the cutoff, server by
// server in bounded batches. Each batch is its own short statement — no
// long transaction, no table lock. Returns total rows deleted.
func Cleanup(db *database.DB, retentionDays int) (int64, error) {
	return cleanupSince(db, retentionDays, time.Now())
}

// cleanupSince is Cleanup with an injectable clock (boundary tests).
func cleanupSince(db *database.DB, retentionDays int, now time.Time) (int64, error) {
	if db == nil {
		return 0, fmt.Errorf("nil database")
	}
	if retentionDays < 1 {
		return 0, fmt.Errorf("retention must be at least 1 day")
	}
	cutoff := Cutoff(now, retentionDays)
	rows, err := db.Query(`SELECT DISTINCT server_id FROM server_metrics WHERE timestamp < $1`, cutoff)
	if err != nil {
		return 0, err
	}
	var serverIDs []uint
	for rows.Next() {
		var id uint
		if err := rows.Scan(&id); err == nil {
			serverIDs = append(serverIDs, id)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	var total int64
	batches := 0
	for _, sid := range serverIDs {
		for {
			if batches >= MaxBatchesPerRun {
				return total, nil
			}
			res, err := db.Exec(`DELETE FROM server_metrics WHERE ctid IN (
				SELECT ctid FROM server_metrics
				WHERE server_id=$1 AND timestamp < $2
				ORDER BY timestamp ASC LIMIT $3)`, sid, cutoff, BatchSize)
			if err != nil {
				return total, err
			}
			n, _ := res.RowsAffected()
			batches++
			total += n
			if n < BatchSize {
				break
			}
		}
	}
	return total, nil
}

// Policy bundles the per-category retention windows. Zero values disable
// that category (except metrics, which requires >= 1 day).
type Policy struct {
	MetricsDays       int
	AppLogDays        int
	HealthResultDays  int
	// DeliveryDays bounds terminal outbox rows, attempts and flushed
	// digests (default 90). Firing delivery_state rows are never pruned.
	DeliveryDays int
}

// CleanupDeliveries prunes delivery history without touching live state:
// terminal outbox rows + attempts older than the cutoff, flushed digest
// batches older than 30 days, and resolved incident states older than the
// cutoff. Audit logs are never touched (separate table, immutable).
func CleanupDeliveries(db *database.DB, retentionDays int) (int64, error) {
	if db == nil {
		return 0, fmt.Errorf("nil database")
	}
	if retentionDays < 1 {
		return 0, fmt.Errorf("retention must be at least 1 day")
	}
	cutoff := Cutoff(time.Now(), retentionDays)
	digestCutoff := Cutoff(time.Now(), 30)
	var total int64
	steps := []struct {
		table string
		query string
		args  []any
	}{
		{"delivery_attempts",
			`DELETE FROM delivery_attempts WHERE ctid IN (
				SELECT ctid FROM delivery_attempts WHERE created_at < $1
				ORDER BY created_at ASC LIMIT $2)`, []any{cutoff, BatchSize}},
		{"delivery_outbox",
			`DELETE FROM delivery_outbox WHERE ctid IN (
				SELECT ctid FROM delivery_outbox
				WHERE status IN ('sent','failed') AND created_at < $1
				ORDER BY created_at ASC LIMIT $2)`, []any{cutoff, BatchSize}},
		{"digest_batches",
			`DELETE FROM digest_batches WHERE ctid IN (
				SELECT ctid FROM digest_batches
				WHERE status='flushed' AND window_end < $1
				ORDER BY window_end ASC LIMIT $2)`, []any{digestCutoff, BatchSize}},
		{"delivery_state",
			`DELETE FROM delivery_state WHERE ctid IN (
				SELECT ctid FROM delivery_state
				WHERE last_state='resolved' AND updated_at < $1
				ORDER BY updated_at ASC LIMIT $2)`, []any{cutoff, BatchSize}},
	}
	for _, s := range steps {
		for batches := 0; batches < MaxBatchesPerRun; batches++ {
			res, err := db.Exec(s.query, s.args...)
			if err != nil {
				// A missing table (very old installs mid-migration) must
				// not break the whole loop; other categories proceed.
				log.Printf("retention: %s: %v", s.table, err)
				break
			}
			n, _ := res.RowsAffected()
			total += n
			if n < BatchSize {
				break
			}
		}
	}
	return total, nil
}

// CleanupAppLogs deletes app_logs rows older than the cutoff in bounded
// batches. The DB trigger (AppLogsPruneFloorDays) independently rejects
// deletes of recent rows, so a misconfigured retention can fail loudly but
// can never wipe fresh history. audit_logs is never touched (fully
// immutable trigger, no exception).
func CleanupAppLogs(db *database.DB, retentionDays int) (int64, error) {
	return cleanupTable(db, "app_logs", retentionDays)
}

// CleanupHealthCheckResults deletes health_check_results rows older than
// the cutoff in bounded batches. The table carries no immutability
// trigger; raw probe history is operational data, not audit evidence.
func CleanupHealthCheckResults(db *database.DB, retentionDays int) (int64, error) {
	return cleanupTable(db, "health_check_results", retentionDays)
}

// cleanupTable prunes timestamped rows older than the cutoff via ctid
// batches (short statements, no long locks). Returns rows deleted.
func cleanupTable(db *database.DB, table string, retentionDays int) (int64, error) {
	if db == nil {
		return 0, fmt.Errorf("nil database")
	}
	if retentionDays < 1 {
		return 0, fmt.Errorf("retention must be at least 1 day")
	}
	switch table {
	case "app_logs", "health_check_results":
	default:
		return 0, fmt.Errorf("table %s is not prunable", table)
	}
	cutoff := Cutoff(time.Now(), retentionDays)
	var total int64
	for batches := 0; batches < MaxBatchesPerRun; batches++ {
		res, err := db.Exec(`DELETE FROM `+table+` WHERE ctid IN (
			SELECT ctid FROM `+table+`
			WHERE timestamp < $1
			ORDER BY timestamp ASC LIMIT $2)`, cutoff, BatchSize)
		if err != nil {
			return total, err
		}
		n, _ := res.RowsAffected()
		total += n
		if n < BatchSize {
			break
		}
	}
	return total, nil
}

// StartPolicyLoop runs every enabled prune job immediately (bounded) and
// then on every interval, logging per-category counts, durations and
// errors. audit_logs, deployments, backups and operations are never
// pruned by any job here.
func StartPolicyLoop(ctx context.Context, db *database.DB, p Policy, interval time.Duration) {
	if interval <= 0 {
		interval = DefaultInterval
	}
	go func() {
		run := func() {
			if p.MetricsDays >= 1 {
				t0 := time.Now()
				deleted, err := Cleanup(db, p.MetricsDays)
				if err != nil {
					log.Printf("retention: metrics: %v", err)
				} else if deleted > 0 {
					log.Printf("retention: pruned %d server_metrics row(s) older than %dd in %s",
						deleted, p.MetricsDays, time.Since(t0).Round(time.Millisecond))
				}
			}
			if p.AppLogDays >= 1 {
				t0 := time.Now()
				deleted, err := CleanupAppLogs(db, p.AppLogDays)
				if err != nil {
					log.Printf("retention: app_logs: %v", err)
				} else if deleted > 0 {
					log.Printf("retention: pruned %d app_logs row(s) older than %dd in %s",
						deleted, p.AppLogDays, time.Since(t0).Round(time.Millisecond))
				}
			}
			if p.HealthResultDays >= 1 {
				t0 := time.Now()
				deleted, err := CleanupHealthCheckResults(db, p.HealthResultDays)
				if err != nil {
					log.Printf("retention: health_check_results: %v", err)
				} else if deleted > 0 {
					log.Printf("retention: pruned %d health_check_results row(s) older than %dd in %s",
						deleted, p.HealthResultDays, time.Since(t0).Round(time.Millisecond))
				}
			}
			if p.DeliveryDays >= 1 {
				t0 := time.Now()
				deleted, err := CleanupDeliveries(db, p.DeliveryDays)
				if err != nil {
					log.Printf("retention: delivery: %v", err)
				} else if deleted > 0 {
					log.Printf("retention: pruned %d delivery history row(s) older than %dd in %s",
						deleted, p.DeliveryDays, time.Since(t0).Round(time.Millisecond))
				}
			}
		}
		run() // converge immediately (bounded); then on interval.
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				run()
			}
		}
	}()
}

// StartLoop runs Cleanup immediately (bounded, converges fast) and then on
// every interval until ctx is cancelled. HTTP handlers never block on it.
func StartLoop(ctx context.Context, db *database.DB, retentionDays int, interval time.Duration) {
	if interval <= 0 {
		interval = DefaultInterval
	}
	go func() {
		run := func() {
			deleted, err := Cleanup(db, retentionDays)
			if err != nil {
				log.Printf("retention: %v", err)
				return
			}
			if deleted > 0 {
				log.Printf("retention: pruned %d server_metrics row(s) older than %dd", deleted, retentionDays)
			}
		}
		run() // converge immediately (bounded); then on interval.
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				run()
			}
		}
	}()
}
