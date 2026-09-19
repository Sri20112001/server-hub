// Package telemetry samples host resource usage into server_snapshots
// (one row per minute, pruned after 7 days) for the telemetry timeline,
// and fires edge-triggered threshold events for the live signal bus.
package telemetry

import (
	"database/sql"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"

	"serverhub/internal/audit"
	"serverhub/internal/events"
)

const (
	interval    = 60 * time.Second
	retention   = 7 * 24 * time.Hour
	pruneEveryN = 10
)

// Thresholds configures resource alert levels (percent).
type Thresholds struct {
	CPU  float64
	RAM  float64
	Disk float64
}

func StartLoop(db *sql.DB, broker *events.Broker, th Thresholds) {
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		ticks := 0
		above := map[string]bool{}
		checkThresholds(db, broker, th, sample(db), above, true)
		for range t.C {
			ticks++
			checkThresholds(db, broker, th, sample(db), above, false)
			if ticks%pruneEveryN == 0 {
				_, _ = db.Exec(`DELETE FROM server_snapshots WHERE ts < ?`,
					time.Now().Add(-retention).Unix())
			}
		}
	}()
}

type reading struct {
	cpu, memPct, memUsedMB, diskPct float64
	rx, tx                          uint64
	diskRead, diskWrite             uint64
}

func checkThresholds(db *sql.DB, broker *events.Broker, th Thresholds, r reading, above map[string]bool, first bool) {
	levels := map[string]struct {
		value float64
		limit float64
	}{
		"cpu":  {r.cpu, th.CPU},
		"ram":  {r.memPct, th.RAM},
		"disk": {r.diskPct, th.Disk},
	}
	for name, l := range levels {
		if l.limit <= 0 {
			continue
		}
		isAbove := l.value > l.limit
		wasAbove := above[name]
		above[name] = isAbove
		if first || !isAbove || wasAbove {
			continue // arm silently; only rising edges fire
		}
		msg := fmt.Sprintf("host %s reached %.1f%% (threshold %.0f%%)", name, l.value, l.limit)
		audit.Write(db, "monitor", "threshold", "server", name, "firing", msg)
		if broker != nil {
			broker.Publish("telemetry.threshold", map[string]interface{}{
				"resource": name, "value": l.value, "threshold": l.limit,
			})
		}
	}
}

func sample(db *sql.DB) reading {
	var r reading
	if p, err := cpu.Percent(time.Second, false); err == nil && len(p) > 0 {
		r.cpu = p[0]
	}
	if m, err := mem.VirtualMemory(); err == nil {
		r.memPct = m.UsedPercent
		r.memUsedMB = float64(m.Used) / 1024 / 1024
	}
	if d, err := disk.Usage("/"); err == nil {
		r.diskPct = d.UsedPercent
	}
	if counters, err := net.IOCounters(false); err == nil {
		for _, c := range counters {
			if c.Name == "lo" || c.Name == "Loopback*" {
				continue
			}
			r.rx += c.BytesRecv
			r.tx += c.BytesSent
		}
	}
	if ioCounters, err := disk.IOCounters(); err == nil {
		for name, c := range ioCounters {
			if strings.HasPrefix(name, "loop") {
				continue
			}
			r.diskRead += c.ReadBytes
			r.diskWrite += c.WriteBytes
		}
	}
	_, _ = db.Exec(`INSERT OR REPLACE INTO server_snapshots
		(ts, cpu, mem_pct, mem_used_mb, disk_pct, net_rx, net_tx, disk_read, disk_write)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		time.Now().Unix(), r.cpu, r.memPct, r.memUsedMB, r.diskPct, clampU64(r.rx),
		clampU64(r.tx), clampU64(r.diskRead), clampU64(r.diskWrite))
	return r
}

// clampU64 saturates at MaxInt64: database/sql rejects uint64 values with
// the high bit set, and some interfaces report near-max counters.
func clampU64(v uint64) int64 {
	if v > uint64(math.MaxInt64) {
		return math.MaxInt64
	}
	return int64(v)
}
