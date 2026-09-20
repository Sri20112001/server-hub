package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"serverhub/internal/database"
)

type TelemetryHandler struct {
	DB *database.DB
}

var rangeSeconds = map[string]int64{
	"15m": 15 * 60,
	"1h":  3600,
	"6h":  6 * 3600,
	"24h": 24 * 3600,
}

// GET /server-hub/api/telemetry?range=15m|1h|6h|24h — snapshot history,
// oldest first. Network values are cumulative host counters; the frontend
// derives per-second rates.
func (h *TelemetryHandler) History(c *gin.Context) {
	r := c.DefaultQuery("range", "1h")
	secs, ok := rangeSeconds[r]
	if !ok {
		secs = rangeSeconds["1h"]
		r = "1h"
	}
	since := time.Now().Unix() - secs
	rows, err := h.DB.Query(`SELECT ts,cpu,mem_pct,mem_used_mb,disk_pct,net_rx,net_tx,
		COALESCE(disk_read,0),COALESCE(disk_write,0)
		FROM server_snapshots WHERE ts >= ? ORDER BY ts ASC LIMIT 2000`, since)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	type point struct {
		Ts       int64   `json:"ts"`
		CPU      float64 `json:"cpu"`
		MemPct   float64 `json:"memPct"`
		MemMB    float64 `json:"memMB"`
		DiskPct  float64 `json:"diskPct"`
		NetRx    uint64  `json:"netRx"`
		NetTx    uint64  `json:"netTx"`
		DiskRead uint64  `json:"diskRead"`
		DiskWrite uint64 `json:"diskWrite"`
	}
	out := []point{}
	for rows.Next() {
		var p point
		if err := rows.Scan(&p.Ts, &p.CPU, &p.MemPct, &p.MemMB, &p.DiskPct, &p.NetRx, &p.NetTx, &p.DiskRead, &p.DiskWrite); err == nil {
			out = append(out, p)
		}
	}
	c.JSON(http.StatusOK, gin.H{"range": r, "points": out})
}

// Latest returns the newest snapshot with per-second rates derived from
// the previous sample (or 204 when the sampler hasn't run).
func (h *TelemetryHandler) Latest(c *gin.Context) {
	type row struct {
		ts                              int64
		cpu, memPct, memMB, diskPct     float64
		rx, tx, dread, dwrite           uint64
	}
	rows, err := h.DB.Query(`SELECT ts,cpu,mem_pct,mem_used_mb,disk_pct,net_rx,net_tx,
		COALESCE(disk_read,0),COALESCE(disk_write,0)
		FROM server_snapshots ORDER BY ts DESC LIMIT 2`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	var pts []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.ts, &r.cpu, &r.memPct, &r.memMB, &r.diskPct,
			&r.rx, &r.tx, &r.dread, &r.dwrite); err == nil {
			pts = append(pts, r)
		}
	}
	if len(pts) == 0 {
		c.JSON(http.StatusNoContent, gin.H{})
		return
	}
	rate := func(now, prev uint64, dt int64) float64 {
		if dt <= 0 || now < prev {
			return 0
		}
		return float64(now-prev) / float64(dt)
	}
	latest := pts[0]
	rxRate, txRate, rdRate, wrRate := 0.0, 0.0, 0.0, 0.0
	if len(pts) == 2 {
		dt := latest.ts - pts[1].ts
		rxRate = rate(latest.rx, pts[1].rx, dt)
		txRate = rate(latest.tx, pts[1].tx, dt)
		rdRate = rate(latest.dread, pts[1].dread, dt)
		wrRate = rate(latest.dwrite, pts[1].dwrite, dt)
	}
	c.JSON(http.StatusOK, gin.H{
		"ts": latest.ts, "cpu": latest.cpu, "memPct": latest.memPct, "memMB": latest.memMB,
		"diskPct": latest.diskPct, "netRx": latest.rx, "netTx": latest.tx,
		"rxRate": rxRate, "txRate": txRate, "diskReadRate": rdRate, "diskWriteRate": wrRate,
	})
}
