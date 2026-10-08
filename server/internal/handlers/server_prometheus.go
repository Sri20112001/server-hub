package handlers

import (
	"encoding/json"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"serverhub/internal/monitoring"
)

// ── Server-scoped Prometheus metrics (Phase 3C) ─────────────────────────────
//
// Per-server history lives in Prometheus (the TSDB); PostgreSQL keeps only
// the 60s agent snapshots. These endpoints expose a small allowlisted set of
// logical metrics for ONE managed server. The backend builds every PromQL
// expression from the allowlist below — viewers never supply query text,
// label matchers, or selector values. The server identity always comes from
// the :id route parameter after a managed-server existence check, so a
// request can never address another server's series or probe server IDs via
// Prometheus behavior (unknown IDs 404 before any query runs).

// serverPromMetric maps one logical metric to a server-scoped PromQL
// expression. Every expression must collapse to a single aggregate series
// for the target server (avg/sum/single-host gauges pinned to mountpoint="/"
// or the host itself) — never per-core, per-mount (beyond root), or
// per-interface detail.
type serverPromMetric struct {
	// name is the logical metric key and the series name in responses.
	name string
	// doc states the exact semantics for API docs and UI tooltips.
	doc string
	// query builds the instant/range expression for a numeric server id.
	query func(serverID string) string
}

// serverPromMetrics is the complete allowlist. Expressions mirror the
// node-exporter conventions already used by the global monitoring
// endpoints and rules/serverhub.yml.
var serverPromMetrics = map[string]serverPromMetric{
	"cpu_usage": {
		name: "cpu_usage",
		doc:  "100 minus average idle CPU percentage across all cores (5m rate).",
		query: func(serverID string) string {
			return `100 - (avg(rate(node_cpu_seconds_total{mode="idle",server_id="` + serverID + `"}[5m])) * 100)`
		},
	},
	"memory_usage": {
		name: "memory_usage",
		doc:  "100 * (1 - MemAvailable/MemTotal): used-or-unreclaimable memory percent.",
		query: func(serverID string) string {
			return `(1 - (node_memory_MemAvailable_bytes{server_id="` + serverID + `"} / node_memory_MemTotal_bytes{server_id="` + serverID + `"})) * 100`
		},
	},
	"disk_usage": {
		name: "disk_usage",
		doc:  "Root filesystem (mountpoint=\"/\") used percent; pseudo-filesystems excluded by the mountpoint pin.",
		query: func(serverID string) string {
			return `(1 - (node_filesystem_avail_bytes{server_id="` + serverID + `",mountpoint="/"} / node_filesystem_size_bytes{server_id="` + serverID + `",mountpoint="/"})) * 100`
		},
	},
	"load_1m": {
		name: "load_1m",
		doc:  "1-minute system load average (node_load1 gauge, host-level).",
		query: func(serverID string) string {
			return `node_load1{server_id="` + serverID + `"}`
		},
	},
	"network_receive": {
		name: "network_receive",
		doc:  "Total receive rate across non-loopback interfaces (sum, 5m rate, bytes/sec).",
		query: func(serverID string) string {
			return `sum(rate(node_network_receive_bytes_total{server_id="` + serverID + `",device!="lo"}[5m]))`
		},
	},
	"network_transmit": {
		name: "network_transmit",
		doc:  "Total transmit rate across non-loopback interfaces (sum, 5m rate, bytes/sec).",
		query: func(serverID string) string {
			return `sum(rate(node_network_transmit_bytes_total{server_id="` + serverID + `",device!="lo"}[5m]))`
		},
	},
}

// serverPromMetricNames lists allowlisted keys in stable order (errors, docs).
func serverPromMetricNames() []string {
	names := make([]string, 0, len(serverPromMetrics))
	for n := range serverPromMetrics {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// promPoint is one frontend-friendly sample; Value is null for gaps/NaN.
type promPoint struct {
	Timestamp int64    `json:"timestamp"`
	Value     *float64 `json:"value"`
}

// promSeries is one named sample list. Labels are deliberately dropped:
// allowlisted queries are single-aggregate by construction, so identity
// comes from the logical metric name, not exporter internals.
type promSeries struct {
	Name   string      `json:"name"`
	Values []promPoint `json:"values"`
}

// parsePromFloat decodes a Prometheus JSON number, mapping NaN/±Inf to nil
// (strconv.ParseFloat accepts "NaN"/"Inf" — they must not leak into JSON).
func parsePromFloat(s string) *float64 {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "nan", "+inf", "-inf", "inf":
		return nil
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return nil
	}
	return &f
}

// normalizeMatrix converts a Prometheus matrix result into frontend-friendly
// series, skipping malformed samples (never failing the whole response).
func normalizeMatrix(name string, data []byte) []promSeries {
	var envelope struct {
		Result []struct {
			Values [][2]json.RawMessage `json:"values"`
		} `json:"result"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return []promSeries{}
	}
	out := make([]promSeries, 0, len(envelope.Result))
	for _, r := range envelope.Result {
		pts := make([]promPoint, 0, len(r.Values))
		for _, v := range r.Values {
			var ts float64
			var vs string
			if err := json.Unmarshal(v[0], &ts); err != nil {
				continue
			}
			if err := json.Unmarshal(v[1], &vs); err != nil {
				continue
			}
			pts = append(pts, promPoint{Timestamp: int64(ts), Value: parsePromFloat(vs)})
		}
		out = append(out, promSeries{Name: name, Values: pts})
	}
	return out
}

// serverPromContext validates :id (existence first — never probe Prometheus
// for unknown servers), the metric allowlist, and Prometheus availability.
// Returns the numeric server id string (digits only: injection-proof by
// construction) or writes the error response.
func (h *MonitoringHandler) serverPromContext(c *gin.Context, metric string) (string, serverPromMetric, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return "", serverPromMetric{}, false
	}
	var count int
	if err := h.DB.QueryRow(`SELECT COUNT(*) FROM managed_servers WHERE id=$1`, id).Scan(&count); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return "", serverPromMetric{}, false
	}
	if count == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return "", serverPromMetric{}, false
	}
	m, ok := serverPromMetrics[metric]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "unknown metric; use " + strings.Join(serverPromMetricNames(), "|"),
		})
		return "", serverPromMetric{}, false
	}
	if h.Prometheus == nil || !h.Prometheus.Available() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "prometheus not configured"})
		return "", serverPromMetric{}, false
	}
	return strconv.FormatInt(id, 10), m, true
}

// GET /server-hub/api/servers/:id/prometheus/metrics?metric=cpu_usage&range=24h&step=1m
// Range history for one allowlisted metric, scoped to {server_id="<id>"}.
func (h *MonitoringHandler) ServerPrometheusMetrics(c *gin.Context) {
	metric := c.DefaultQuery("metric", "cpu_usage")
	serverID, m, ok := h.serverPromContext(c, metric)
	if !ok {
		return
	}
	rangeStr := c.DefaultQuery("range", "1h")
	dur, ok := monitoring.RangeSeconds(rangeStr)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown range; use 1h|6h|24h|7d"})
		return
	}
	step := c.DefaultQuery("step", "60")
	if err := monitoring.ValidateStep(step, dur); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	now := time.Now().Unix()
	start := strconv.FormatInt(now-dur, 10)
	end := strconv.FormatInt(now, 10)

	ctx, cancel := promCtx(h.Cfg)
	defer cancel()
	data, err := h.Prometheus.QueryRange(ctx, m.query(serverID), start, end, step)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"serverId": serverID,
		"metric":   metric,
		"range":    rangeStr,
		"step":     step,
		"series":   normalizeMatrix(metric, data),
	})
}

// GET /server-hub/api/servers/:id/prometheus/metrics/latest?metric=cpu_usage
// Current value for one allowlisted metric (dashboard tiles without a range
// fetch). Same allowlist, authorization, construction, and limits.
func (h *MonitoringHandler) ServerPrometheusLatest(c *gin.Context) {
	metric := c.DefaultQuery("metric", "cpu_usage")
	serverID, m, ok := h.serverPromContext(c, metric)
	if !ok {
		return
	}

	ctx, cancel := promCtx(h.Cfg)
	defer cancel()
	data, err := h.Prometheus.Query(ctx, m.query(serverID))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	var envelope struct {
		Result []struct {
			Value [2]json.RawMessage `json:"value"`
		} `json:"result"`
	}
	var value *float64
	var ts *int64
	if err := json.Unmarshal(data, &envelope); err == nil && len(envelope.Result) > 0 {
		var tv float64
		var vs string
		if json.Unmarshal(envelope.Result[0].Value[0], &tv) == nil {
			t := int64(tv)
			ts = &t
		}
		if json.Unmarshal(envelope.Result[0].Value[1], &vs) == nil {
			value = parsePromFloat(vs)
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"serverId":  serverID,
		"metric":    metric,
		"value":     value,
		"timestamp": ts,
	})
}
