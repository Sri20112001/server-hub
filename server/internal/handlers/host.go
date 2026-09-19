package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/gin-gonic/gin"

	"serverhub/internal/serverinfo"
)

func pressure(value, warnAt float64) string {
	switch {
	case value >= warnAt:
		return "High"
	case value >= 70:
		return "Elevated"
	default:
		return "Normal"
	}
}

// GET /server-hub/api/server/detail — full host inventory plus pressure
// states computed from live measurements (not invented scores).
func (h *SystemHandler) Detail(c *gin.Context) {
	d := serverinfo.GetDetail()
	var cpuWarn, ramWarn, diskWarn float64 = 85, 90, 80
	if h.Cfg != nil {
		cpuWarn, ramWarn, diskWarn = h.Cfg.AlertCPU, h.Cfg.AlertRAM, h.Cfg.AlertDisk
	}
	c.JSON(http.StatusOK, gin.H{
		"host":        d.Host,
		"cpu":         d.CPU,
		"memory":      d.Memory,
		"filesystems": d.Filesystems,
		"network":     d.Network,
		"diskIO":      d.DiskIO,
		"processes":   d.Processes,
		"pressure": gin.H{
			"cpu":    loadPressure(d.CPU.Load1, d.CPU.LogicalCores, cpuWarn),
			"memory": pressure(d.Memory.UsedPct, ramWarn),
			"disk":   pressure(maxFsPct(d.Filesystems), diskWarn),
		},
	})
}

// loadPressure grades load-average-per-core: >2x cores is saturated
// regardless of the instant CPU percentage.
func loadPressure(load1 float64, cores int, cpuWarn float64) string {
	if cores < 1 {
		cores = 1
	}
	perCore := load1 / float64(cores)
	switch {
	case perCore >= 2 || (cpuWarn > 0 && perCore*100 >= cpuWarn*2):
		return "High"
	case perCore >= 1:
		return "Elevated"
	default:
		return "Normal"
	}
}

func maxFsPct(fs []serverinfo.Filesystem) float64 {
	m := 0.0
	for _, f := range fs {
		if f.UsedPct > m {
			m = f.UsedPct
		}
	}
	return m
}

type appUsage struct {
	Name    string  `json:"name"`
	Path    string  `json:"path"`
	BytesMB float64 `json:"bytesMB"`
}

// GET /server-hub/api/server/storage — filesystems, per-app disk use and
// Docker disk usage (images/containers/volumes/build cache).
func (h *SystemHandler) Storage(c *gin.Context) {
	d := serverinfo.GetDetail()
	resp := gin.H{"filesystems": d.Filesystems}

	// Applications: immediate subdirectories of the first existing scan root.
	appsRoot := ""
	var roots []string
	if h.Cfg != nil {
		roots = h.Cfg.ScanRoots
	}
	for _, r := range roots {
		if st, err := os.Stat(r); err == nil && st.IsDir() {
			appsRoot = r
			break
		}
	}
	apps := []appUsage{}
	partial := false
	if appsRoot != "" {
		entries, err := os.ReadDir(appsRoot)
		if err == nil {
			deadline := time.Now().Add(20 * time.Second)
			for _, e := range entries {
				if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
					continue
				}
				p := filepath.Join(appsRoot, e.Name())
				n, done := duDir(p, deadline)
				apps = append(apps, appUsage{Name: e.Name(), Path: p, BytesMB: float64(n) / 1024 / 1024})
				if !done {
					partial = true
					break
				}
			}
		}
	}
	resp["appsRoot"] = appsRoot
	resp["apps"] = apps
	resp["appsPartial"] = partial
	resp["docker"] = h.dockerUsage()
	c.JSON(http.StatusOK, resp)
}

// duDir sums regular file sizes under root until the deadline.
func duDir(root string, deadline time.Time) (int64, bool) {
	var total int64
	var files int64
	done := true
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		files++
		if files%4096 == 0 && time.Now().After(deadline) {
			done = false
			return filepath.SkipAll
		}
		if info, err := d.Info(); err == nil && info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	return total, done
}

type imageTop struct {
	Name   string  `json:"name"`
	SizeMB float64 `json:"sizeMB"`
}

func (h *SystemHandler) dockerUsage() interface{} {
	if !h.Docker.Available() {
		return nil
	}
	cli, err := h.Docker.Raw()
	if err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	du, err := cli.DiskUsage(ctx, types.DiskUsageOptions{})
	if err != nil {
		return nil
	}
	var imagesBytes, containersBytes, volumesBytes, cacheBytes int64
	for _, im := range du.Images {
		imagesBytes += im.Size
	}
	for _, co := range du.Containers {
		containersBytes += co.SizeRw
	}
	for _, v := range du.Volumes {
		if v.UsageData != nil {
			volumesBytes += v.UsageData.Size
		}
	}
	for _, bc := range du.BuildCache {
		if bc != nil {
			cacheBytes += bc.Size
		}
	}
	top := []imageTop{}
	for _, im := range du.Images {
		name := "<none>"
		if len(im.RepoTags) > 0 {
			name = im.RepoTags[0]
		}
		top = append(top, imageTop{Name: name, SizeMB: float64(im.Size) / 1024 / 1024})
	}
	sort.Slice(top, func(i, j int) bool { return top[i].SizeMB > top[j].SizeMB })
	if len(top) > 8 {
		top = top[:8]
	}
	return gin.H{
		"imagesMB":     float64(imagesBytes) / 1024 / 1024,
		"containersMB": float64(containersBytes) / 1024 / 1024,
		"volumesMB":    float64(volumesBytes) / 1024 / 1024,
		"buildCacheMB": float64(cacheBytes) / 1024 / 1024,
		"imagesTop":    top,
	}
}

type containerStat struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Image      string  `json:"image"`
	State      string  `json:"state"`
	CPUPercent float64 `json:"cpuPercent"`
	MemMB      float64 `json:"memMB"`
	MemLimitMB float64 `json:"memLimitMB"`
	MemPct     float64 `json:"memPct"`
	NetRxMB    float64 `json:"netRxMB"`
	NetTxMB    float64 `json:"netTxMB"`
	BlockReadMB  float64 `json:"blockReadMB"`
	BlockWriteMB float64 `json:"blockWriteMB"`
	Pids       int     `json:"pids"`
}

// GET /server-hub/api/containers/stats — one-shot resource snapshot for
// every running container (CPU% via precpu deltas, no streaming needed).
func (h *ContainerHandler) StatsAll(c *gin.Context) {
	if !h.Docker.Available() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "docker unavailable"})
		return
	}
	cli, err := h.Docker.Raw()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	ctx, cancel := dockerCtx()
	defer cancel()
	type summary struct {
		ID    string
		Names []string
		Image string
		State string
	}
	type rawSummary = summary
	_ = rawSummary{}
	list, err := h.Docker.ListContainers(ctx)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	out := make([]containerStat, 0, len(list))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for _, ctr := range list {
		if ctr.State != "running" {
			continue
		}
		wg.Add(1)
		go func(id string, names []string, image, state string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			sctx, scancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer scancel()
			resp, err := cli.ContainerStats(sctx, id, false)
			if err != nil {
				return
			}
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				return
			}
			st, ok := summarizeStats(body)
			if !ok {
				return
			}
			st.ID = id
			st.Image = image
			st.State = state
			if len(names) > 0 {
				st.Name = strings.TrimPrefix(names[0], "/")
			}
			mu.Lock()
			out = append(out, st)
			mu.Unlock()
		}(ctr.ID, ctr.Names, ctr.Image, ctr.State)
	}
	wg.Wait()
	c.JSON(http.StatusOK, out)
}

type dockerStats struct {
	CPUStats struct {
		CPUUsage struct {
			TotalUsage  uint64   `json:"total_usage"`
			PercpuUsage []uint64 `json:"percpu_usage"`
		} `json:"cpu_usage"`
		SystemUsage uint64 `json:"system_cpu_usage"`
	} `json:"cpu_stats"`
	PreCPUStats struct {
		CPUUsage struct {
			TotalUsage uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemUsage uint64 `json:"system_cpu_usage"`
	} `json:"precpu_stats"`
	MemoryStats struct {
		Usage uint64 `json:"usage"`
		Limit uint64 `json:"limit"`
	} `json:"memory_stats"`
	Networks map[string]struct {
		RxBytes uint64 `json:"rx_bytes"`
		TxBytes uint64 `json:"tx_bytes"`
	} `json:"networks"`
	BlkioStats struct {
		IoServiceBytesRecursive []struct {
			Op    string `json:"op"`
			Value uint64 `json:"value"`
		} `json:"io_service_bytes_recursive"`
	} `json:"blkio_stats"`
	PidsStats struct {
		Current int `json:"current"`
	} `json:"pids_stats"`
}

func summarizeStats(body []byte) (containerStat, bool) {
	var s dockerStats
	if err := json.Unmarshal(body, &s); err != nil {
		return containerStat{}, false
	}
	var st containerStat
	cpuDelta := float64(s.CPUStats.CPUUsage.TotalUsage) - float64(s.PreCPUStats.CPUUsage.TotalUsage)
	sysDelta := float64(s.CPUStats.SystemUsage) - float64(s.PreCPUStats.SystemUsage)
	cpus := float64(len(s.CPUStats.CPUUsage.PercpuUsage))
	if cpus < 1 {
		cpus = 1
	}
	if sysDelta > 0 && cpuDelta >= 0 {
		st.CPUPercent = cpuDelta / sysDelta * cpus * 100
	}
	st.MemMB = float64(s.MemoryStats.Usage) / 1024 / 1024
	st.MemLimitMB = float64(s.MemoryStats.Limit) / 1024 / 1024
	if s.MemoryStats.Limit > 0 {
		st.MemPct = 100 * float64(s.MemoryStats.Usage) / float64(s.MemoryStats.Limit)
	}
	for _, n := range s.Networks {
		st.NetRxMB += float64(n.RxBytes) / 1024 / 1024
		st.NetTxMB += float64(n.TxBytes) / 1024 / 1024
	}
	for _, b := range s.BlkioStats.IoServiceBytesRecursive {
		switch strings.ToLower(b.Op) {
		case "read":
			st.BlockReadMB += float64(b.Value) / 1024 / 1024
		case "write":
			st.BlockWriteMB += float64(b.Value) / 1024 / 1024
		}
	}
	st.Pids = s.PidsStats.Current
	return st, true
}

// ServerRates augments the summary with cores and live net rates.
func (h *SystemHandler) serverRates() (cores int, rxRate, txRate float64) {
	info := serverinfo.GetDetail()
	cores = info.CPU.LogicalCores
	rows, err := h.DB.Query(`SELECT net_rx, net_tx, ts FROM server_snapshots ORDER BY ts DESC LIMIT 2`)
	if err != nil {
		return cores, 0, 0
	}
	defer rows.Close()
	type pt struct {
		rx, tx uint64
		ts     int64
	}
	var pts []pt
	for rows.Next() {
		var p pt
		var rx, tx sql.NullInt64
		if err := rows.Scan(&rx, &tx, &p.ts); err == nil && rx.Valid && tx.Valid {
			p.rx, p.tx = uint64(rx.Int64), uint64(tx.Int64)
			pts = append(pts, p)
		}
	}
	if len(pts) == 2 && pts[0].ts > pts[1].ts {
		dt := float64(pts[0].ts - pts[1].ts)
		if dt > 0 {
			if pts[0].rx >= pts[1].rx {
				rxRate = float64(pts[0].rx-pts[1].rx) / dt / 1024
			}
			if pts[0].tx >= pts[1].tx {
				txRate = float64(pts[0].tx-pts[1].tx) / dt / 1024
			}
		}
	}
	return cores, rxRate, txRate
}
