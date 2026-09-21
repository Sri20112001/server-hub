// Package telemetry samples host resource usage into server_snapshots
// (one row per minute, pruned after 7 days) for the telemetry timeline,
// and fires edge-triggered threshold events for the live signal bus.
package telemetry

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"

	"serverhub/internal/audit"
	"serverhub/internal/database"
	"serverhub/internal/events"
)

const (
	interval = 60 * time.Second
	// Telemetry snapshots are history and append-only like all other logs:
	// retained forever, never pruned, never updated in place.
)

// Thresholds configures resource alert levels (percent).
type Thresholds struct {
	CPU  float64
	RAM  float64
	Disk float64
}

func StartLoop(db *database.DB, broker *events.Broker, th Thresholds) {
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		above := map[string]bool{}
		checkThresholds(db, broker, th, sample(db), above, true)
		for range t.C {
			checkThresholds(db, broker, th, sample(db), above, false)
		}
	}()
}

type reading struct {
	cpu, memPct, memUsedMB, diskPct float64
	rx, tx                          uint64
	diskRead, diskWrite             uint64
}

func checkThresholds(db *database.DB, broker *events.Broker, th Thresholds, r reading, above map[string]bool, first bool) {
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

func sample(db *database.DB) reading {
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
	// Append-only: first write wins. ON CONFLICT DO NOTHING so a retry
	// never overwrites history (UPDATE is not used).
	_, _ = db.Exec(`INSERT INTO server_snapshots
		(ts, cpu, mem_pct, mem_used_mb, disk_pct, net_rx, net_tx, disk_read, disk_write)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (ts) DO NOTHING`,
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
