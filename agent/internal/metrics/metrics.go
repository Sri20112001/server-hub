// Package metrics collects host metrics via gopsutil. The Snapshot JSON
// shape matches POST /server-hub/api/agent/metrics on the Go/Gin backend
// (see server/internal/handlers/servers.go IngestMetrics) byte-for-byte.
package metrics

import (
	"fmt"
	"math"
	"runtime"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
)

// Snapshot is one metric report. Sizes mirror the Node agent:
// RAM in bytes, disk in bytes — the backend stores/derives display units.
type Snapshot struct {
	CPUUsage     float64 `json:"cpuUsage"`
	MemoryUsage  float64 `json:"memoryUsage"`
	MemoryUsedMB float64 `json:"memoryUsedMB"`
	DiskUsage    float64 `json:"diskUsage"`
	DiskUsedGB   float64 `json:"diskUsedGB"`
	NetRx        int64   `json:"netRx"`
	NetTx        int64   `json:"netTx"`
	LoadAvg1     float64 `json:"loadAvg1"`
	UptimeSec    int64   `json:"uptimeSec"`

	Hostname  string `json:"hostname"`
	OS        string `json:"os"`
	OSVersion string `json:"osVersion"`
	Arch      string `json:"arch"`
	CPUInfo   string `json:"cpuInfo"`
	CPUCores  int    `json:"cpuCores"`
	RAMTotal  int64  `json:"ramTotal"`
	DiskTotal int64  `json:"diskTotal"`
}

// Collect gathers a snapshot. Independent probes fail soft: a failed probe
// leaves its fields at zero instead of dropping the whole report.
func Collect() (Snapshot, error) {
	var s Snapshot

	// CPU percent over a 1s window (single call).
	if pct, err := cpu.Percent(time.Second, false); err == nil && len(pct) > 0 {
		s.CPUUsage = pct[0]
	}

	if vm, err := mem.VirtualMemory(); err == nil {
		s.MemoryUsage = vm.UsedPercent
		s.MemoryUsedMB = float64(vm.Used) / 1024 / 1024
		s.RAMTotal = int64(vm.Total)
	}

	if du, err := rootDiskUsage(); err == nil {
		s.DiskUsage = du.UsedPercent
		s.DiskUsedGB = float64(du.Used) / 1024 / 1024 / 1024
		s.DiskTotal = int64(du.Total)
	}

	// Cumulative interface counters, excluding loopback.
	if counters, err := net.IOCounters(false); err == nil {
		for _, c := range counters {
			if isLoopback(c.Name) {
				continue
			}
			s.NetRx += int64(c.BytesRecv)
			s.NetTx += int64(c.BytesSent)
		}
	}

	if avg, err := load.Avg(); err == nil {
		s.LoadAvg1 = sanitizeLoad(avg.Load1)
	}

	info, err := host.Info()
	if err != nil {
		return s, fmt.Errorf("host info: %w", err)
	}
	// Real system uptime for the dashboard.
	s.UptimeSec = int64(info.Uptime)
	s.Hostname = info.Hostname
	s.OS = info.Platform
	if s.OS == "" {
		s.OS = info.OS
	}
	s.OSVersion = info.PlatformVersion
	if s.OSVersion == "" {
		s.OSVersion = info.KernelVersion
	}
	s.Arch = info.KernelArch
	if s.Arch == "" {
		s.Arch = runtime.GOARCH
	}

	if ci, err := cpu.Info(); err == nil && len(ci) > 0 {
		s.CPUInfo = strings.TrimSpace(ci[0].ModelName)
		s.CPUCores = int(ci[0].Cores)
	}
	if s.CPUCores <= 0 {
		if n, err := cpu.Counts(true); err == nil {
			s.CPUCores = n
		}
	}

	return s, nil
}

// rootDiskUsage returns usage for "/" on Unix, or the largest local volume
// on platforms without a "/" mount (e.g. Windows drive letters).
func rootDiskUsage() (*disk.UsageStat, error) {
	if du, err := disk.Usage("/"); err == nil {
		return du, nil
	}
	parts, err := disk.Partitions(false)
	if err != nil {
		return nil, err
	}
	var best *disk.UsageStat
	for _, p := range parts {
		du, err := disk.Usage(p.Mountpoint)
		if err != nil || du.Total == 0 {
			continue
		}
		if best == nil || du.Total > best.Total {
			best = du
		}
	}
	if best == nil {
		return nil, fmt.Errorf("no usable filesystem found")
	}
	return best, nil
}

func isLoopback(name string) bool {
	n := strings.ToLower(name)
	return n == "lo" || strings.HasPrefix(n, "loopback")
}

// sanitizeLoad drops NaN/Inf/negative/denormal readings (some platforms
// report garbage for unsupported load averages) to a clean zero.
func sanitizeLoad(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || (v > 0 && v < 1e-9) {
		return 0
	}
	return v
}
