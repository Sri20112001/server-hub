// Package serverinfo exposes host inventory: memory/swap detail,
// filesystems, CPU facts, per-interface network, disk I/O and a
// best-effort top-process list. Everything degrades to zero values
// when the host does not expose a metric.
package serverinfo

import (
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
)

type MemDetail struct {
	TotalMB     float64 `json:"totalMB"`
	UsedMB      float64 `json:"usedMB"`
	AvailableMB float64 `json:"availableMB"`
	FreeMB      float64 `json:"freeMB"`
	CachedMB    float64 `json:"cachedMB"`
	BuffersMB   float64 `json:"buffersMB"`
	UsedPct     float64 `json:"usedPct"`
	SwapTotalMB float64 `json:"swapTotalMB"`
	SwapUsedMB  float64 `json:"swapUsedMB"`
	SwapFreeMB  float64 `json:"swapFreeMB"`
	SwapPct     float64 `json:"swapPct"`
}

type Filesystem struct {
	Device   string  `json:"device"`
	Mount    string  `json:"mount"`
	Fstype   string  `json:"fstype"`
	TotalGB  float64 `json:"totalGB"`
	UsedGB   float64 `json:"usedGB"`
	FreeGB   float64 `json:"freeGB"`
	UsedPct  float64 `json:"usedPct"`
}

type CPUDetail struct {
	LogicalCores  int       `json:"logicalCores"`
	PhysicalCores int       `json:"physicalCores"`
	Model         string    `json:"model"`
	Mhz           float64   `json:"mhz"`
	Load1         float64   `json:"load1"`
	Load5         float64   `json:"load5"`
	Load15        float64   `json:"load15"`
	TimesUserPct  float64   `json:"timesUserPct"`
	TimesSysPct   float64   `json:"timesSysPct"`
	TimesIdlePct  float64   `json:"timesIdlePct"`
	TimesIowaitPct float64  `json:"timesIowaitPct"`
}

type NetIface struct {
	Name      string  `json:"name"`
	RxBytes   uint64  `json:"rxBytes"`
	TxBytes   uint64  `json:"txBytes"`
	RxErrors  uint64  `json:"rxErrors"`
	TxErrors  uint64  `json:"txErrors"`
}

type DiskIO struct {
	Device   string  `json:"device"`
	ReadMB   float64 `json:"readMB"`
	WriteMB  float64 `json:"writeMB"`
	ReadOps  uint64  `json:"readOps"`
	WriteOps uint64  `json:"writeOps"`
}

type HostFacts struct {
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
	Platform string `json:"platform"`
	Kernel   string `json:"kernel"`
	Arch     string `json:"arch"`
	UptimeSec uint64 `json:"uptimeSec"`
	GoVersion string `json:"goVersion"`
}

type ProcInfo struct {
	Pid    int32   `json:"pid"`
	Name   string  `json:"name"`
	Status string  `json:"status"`
	MemMB  float64 `json:"memMB"`
	MemPct float32 `json:"memPct"`
}

type ProcSummary struct {
	Total    int         `json:"total"`
	Running  int         `json:"running"`
	Sleeping int         `json:"sleeping"`
	Other    int         `json:"other"`
	Zombie   int         `json:"zombie"`
	Top      []ProcInfo  `json:"top"`
}

type Detail struct {
	Host       HostFacts   `json:"host"`
	CPU        CPUDetail   `json:"cpu"`
	Memory     MemDetail   `json:"memory"`
	Filesystems []Filesystem `json:"filesystems"`
	Network    []NetIface  `json:"network"`
	DiskIO     []DiskIO    `json:"diskIO"`
	Processes  ProcSummary `json:"processes"`
}

func b2mb(b uint64) float64 { return float64(b) / 1024 / 1024 }
func b2gb(b uint64) float64 { return float64(b) / 1024 / 1024 / 1024 }

func GetDetail() Detail {
	d := Detail{}
	if h, err := host.Info(); err == nil {
		d.Host = HostFacts{
			Hostname: h.Hostname, OS: h.OS, Platform: h.Platform,
			Kernel: h.KernelVersion, Arch: runtime.GOARCH,
			UptimeSec: h.Uptime, GoVersion: runtime.Version(),
		}
	}
	if n, err := cpu.Counts(true); err == nil {
		d.CPU.LogicalCores = n
	}
	if n, err := cpu.Counts(false); err == nil {
		d.CPU.PhysicalCores = n
	}
	if infos, err := cpu.Info(); err == nil && len(infos) > 0 {
		d.CPU.Model = strings.TrimSpace(infos[0].ModelName)
		d.CPU.Mhz = infos[0].Mhz
	}
	if avg, err := load.Avg(); err == nil {
		d.CPU.Load1, d.CPU.Load5, d.CPU.Load15 = avg.Load1, avg.Load5, avg.Load15
	}
	if times, err := cpu.Times(false); err == nil && len(times) > 0 {
		t := times[0]
		total := t.User + t.System + t.Idle + t.Nice + t.Iowait + t.Irq + t.Softirq + t.Steal
		if total > 0 {
			d.CPU.TimesUserPct = 100 * t.User / total
			d.CPU.TimesSysPct = 100 * t.System / total
			d.CPU.TimesIdlePct = 100 * t.Idle / total
			d.CPU.TimesIowaitPct = 100 * t.Iowait / total
		}
	}
	if m, err := mem.VirtualMemory(); err == nil {
		d.Memory = MemDetail{
			TotalMB: b2mb(m.Total), UsedMB: b2mb(m.Used),
			AvailableMB: b2mb(m.Available), FreeMB: b2mb(m.Free),
			CachedMB: b2mb(m.Cached), BuffersMB: b2mb(m.Buffers),
			UsedPct: m.UsedPercent,
			SwapTotalMB: b2mb(m.SwapTotal), SwapUsedMB: b2mb(m.SwapTotal-m.SwapFree),
			SwapFreeMB: b2mb(m.SwapFree),
		}
		if m.SwapTotal > 0 {
			d.Memory.SwapPct = 100 * float64(m.SwapTotal-m.SwapFree) / float64(m.SwapTotal)
		}
	}
	d.Filesystems = filesystems()
	d.Network = interfaces()
	d.DiskIO = diskIO()
	d.Processes = processes()
	_ = time.Now()
	return d
}

func filesystems() []Filesystem {
	out := []Filesystem{}
	parts, err := disk.Partitions(false)
	if err != nil {
		return out
	}
	skipFstype := map[string]bool{
		"squashfs": true, "tmpfs": true, "devtmpfs": true, "overlay": true,
		"nsfs": true, "cgroup": true, "cgroup2": true, "proc": true, "sysfs": true,
	}
	for _, p := range parts {
		if skipFstype[p.Fstype] {
			continue
		}
		if strings.HasPrefix(p.Mountpoint, "/var/lib/docker") ||
			strings.HasPrefix(p.Mountpoint, "/snap") ||
			strings.HasPrefix(p.Mountpoint, "/run") {
			continue
		}
		u, err := disk.Usage(p.Mountpoint)
		if err != nil || u.Total == 0 {
			continue
		}
		out = append(out, Filesystem{
			Device: p.Device, Mount: p.Mountpoint, Fstype: p.Fstype,
			TotalGB: b2gb(u.Total), UsedGB: b2gb(u.Used), FreeGB: b2gb(u.Free),
			UsedPct: u.UsedPercent,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Mount < out[j].Mount })
	return out
}

func interfaces() []NetIface {
	out := []NetIface{}
	counters, err := net.IOCounters(true)
	if err != nil {
		return out
	}
	for _, c := range counters {
		if c.Name == "lo" || strings.HasPrefix(c.Name, "Loopback") {
			continue
		}
		out = append(out, NetIface{
			Name: c.Name, RxBytes: c.BytesRecv, TxBytes: c.BytesSent,
			RxErrors: c.Errin, TxErrors: c.Errout,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func diskIO() []DiskIO {
	out := []DiskIO{}
	counters, err := disk.IOCounters()
	if err != nil {
		return out
	}
	for name, c := range counters {
		if strings.HasPrefix(name, "loop") {
			continue
		}
		out = append(out, DiskIO{
			Device: name, ReadMB: b2mb(c.ReadBytes), WriteMB: b2mb(c.WriteBytes),
			ReadOps: c.ReadCount, WriteOps: c.WriteCount,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ReadMB+out[i].WriteMB > out[j].ReadMB+out[j].WriteMB
	})
	if len(out) > 8 {
		out = out[:8]
	}
	return out
}

func procStatusLabel(s string) string {
	switch strings.ToUpper(s) {
	case "R":
		return "running"
	case "S", "D", "I":
		return "sleeping"
	case "Z":
		return "zombie"
	case "T", "X", "W", "P":
		return "other"
	default:
		low := strings.ToLower(s)
		switch low {
		case "running":
			return "running"
		case "sleeping", "idle", "disk-sleep":
			return "sleeping"
		case "zombie":
			return "zombie"
		default:
			return "other"
		}
	}
}

func processes() ProcSummary {
	sum := ProcSummary{Top: []ProcInfo{}}
	pids, err := process.Pids()
	if err != nil {
		return sum
	}
	sum.Total = len(pids)
	all := make([]ProcInfo, 0, len(pids))
	for _, pid := range pids {
		p, err := process.NewProcess(pid)
		if err != nil {
			continue
		}
		name, _ := p.Name()
		if name == "" {
			continue
		}
		statuses, _ := p.Status()
		label := "other"
		if len(statuses) > 0 {
			label = procStatusLabel(statuses[0])
		}
		switch label {
		case "running":
			sum.Running++
		case "sleeping":
			sum.Sleeping++
		case "zombie":
			sum.Zombie++
		default:
			sum.Other++
		}
		var memMB float64
		var memPct float32
		if mi, err := p.MemoryInfo(); err == nil && mi != nil {
			memMB = b2mb(mi.RSS)
		}
		if pct, err := p.MemoryPercent(); err == nil {
			memPct = pct
		}
		all = append(all, ProcInfo{Pid: pid, Name: name, Status: label, MemMB: memMB, MemPct: memPct})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].MemMB > all[j].MemMB })
	if len(all) > 8 {
		all = all[:8]
	}
	sum.Top = all
	return sum
}
