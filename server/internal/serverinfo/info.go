package serverinfo

import (
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
)

type Summary struct {
	CPUPercent float64 `json:"cpuPercent"`
	MemTotalMB float64 `json:"memTotalMB"`
	MemUsedMB  float64 `json:"memUsedMB"`
	MemPercent float64 `json:"memPercent"`
	DiskTotalGB float64 `json:"diskTotalGB"`
	DiskUsedGB  float64 `json:"diskUsedGB"`
	DiskPercent float64 `json:"diskPercent"`
	UptimeSec   uint64  `json:"uptimeSec"`
	BootTime    uint64  `json:"bootTime"`
}

func Get() Summary {
	s := Summary{}
	if p, err := cpu.Percent(time.Second, false); err == nil && len(p) > 0 {
		s.CPUPercent = p[0]
	}
	if m, err := mem.VirtualMemory(); err == nil {
		s.MemTotalMB = float64(m.Total) / 1024 / 1024
		s.MemUsedMB = float64(m.Used) / 1024 / 1024
		s.MemPercent = m.UsedPercent
	}
	if d, err := disk.Usage("/"); err == nil {
		s.DiskTotalGB = float64(d.Total) / 1024 / 1024 / 1024
		s.DiskUsedGB = float64(d.Used) / 1024 / 1024 / 1024
		s.DiskPercent = d.UsedPercent
	}
	if h, err := host.Info(); err == nil {
		s.UptimeSec = h.Uptime
		s.BootTime = h.BootTime
	}
	return s
}
