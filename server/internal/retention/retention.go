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
