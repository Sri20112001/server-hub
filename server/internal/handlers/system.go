package handlers

import (
	"database/sql"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"serverhub/internal/config"
	"serverhub/internal/database"
	"serverhub/internal/dockerx"
	"serverhub/internal/models"
	"serverhub/internal/serverinfo"
)

type SystemHandler struct {
	DB     *database.DB
	Docker *dockerx.Client
	Cfg    *config.Config
}

// GET /api/health — overall rollup for the dashboard.
func (h *SystemHandler) Health(c *gin.Context) {
	var projects, services, running, healthy, degraded, down int
	_ = h.DB.QueryRow(`SELECT COUNT(*) FROM projects`).Scan(&projects)
	_ = h.DB.QueryRow(`SELECT COUNT(*) FROM services`).Scan(&services)
	_ = h.DB.QueryRow(`SELECT COUNT(*) FROM services WHERE status='HEALTHY'`).Scan(&healthy)
	_ = h.DB.QueryRow(`SELECT COUNT(*) FROM services WHERE status='DEGRADED'`).Scan(&degraded)
	_ = h.DB.QueryRow(`SELECT COUNT(*) FROM services WHERE status='DOWN'`).Scan(&down)
	if h.Docker.Available() {
		ctx, cancel := dockerCtx()
		defer cancel()
		if list, err := h.Docker.ListContainers(ctx); err == nil {
			for _, ctr := range list {
				if ctr.State == "running" {
					running++
				}
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"projects": projects, "services": services, "running": running,
		"healthy": healthy, "degraded": degraded, "down": down,
		"dockerAvailable": h.Docker.Available(),
	})
}

// GET /api/projects/:id/health — project health detail.
func (h *SystemHandler) ProjectHealth(c *gin.Context) {
	pid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project id"})
		return
	}
	var status, healthURL string
	var updated sql.NullString
	err = h.DB.QueryRow(`SELECT status, health_url, updated_at FROM projects WHERE id=?`, pid).
		Scan(&status, &healthURL, &updated)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}
	rows, _ := h.DB.Query(`SELECT id,project_id,name,type,container_name,internal_port,host_port,health_url,docker_service_name,status,last_health,last_health_at,response_time_ms FROM services WHERE project_id=?`, pid)
	services := []models.Service{}
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var s models.Service
			var ip, hp sql.NullInt64
			var rt sql.NullInt64
			var lastHealthAt sql.NullString
			if err := rows.Scan(&s.ID, &s.ProjectID, &s.Name, &s.Type, &s.ContainerName, &ip, &hp, &s.HealthURL, &s.DockerServiceName, &s.Status, &s.LastHealth, &lastHealthAt, &rt); err == nil {
				s.LastHealthAt = nullStr(lastHealthAt)
				if ip.Valid {
					v := int(ip.Int64)
					s.InternalPort = &v
				}
				if hp.Valid {
					v := int(hp.Int64)
					s.HostPort = &v
				}
				if rt.Valid {
					v := rt.Int64
					s.ResponseTimeMs = &v
				}
				services = append(services, s)
			}
		}
	}
	// Live probe of the project health URL (does not persist).
	live := status
	var rtMs *int64
	var lastCheck string
	if healthURL != "" {
		start := time.Now()
		client := &http.Client{Timeout: 8 * time.Second}
		resp, err := client.Get(healthURL)
		ms := time.Since(start).Milliseconds()
		rtMs = &ms
		lastCheck = time.Now().UTC().Format(time.RFC3339)
		if err != nil {
			live = "DOWN"
		} else {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			_ = body
			switch {
			case resp.StatusCode >= 200 && resp.StatusCode < 300:
				live = "HEALTHY"
			case resp.StatusCode >= 500:
				live = "DOWN"
			default:
				live = "DEGRADED"
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"projectId": pid, "status": status, "liveStatus": live,
		"healthUrl": healthURL, "updatedAt": nullStr(updated),
		"responseTimeMs": rtMs, "lastCheck": lastCheck,
		"services": services,
	})
}

// GET /api/server — CPU/RAM/disk + counts, cores and live net rates.
func (h *SystemHandler) Server(c *gin.Context) {
	info := serverinfo.Get()
	var projects, services, deployments int
	_ = h.DB.QueryRow(`SELECT COUNT(*) FROM projects`).Scan(&projects)
	_ = h.DB.QueryRow(`SELECT COUNT(*) FROM services`).Scan(&services)
	_ = h.DB.QueryRow(`SELECT COUNT(*) FROM deployments`).Scan(&deployments)
	containers := -1
	running := -1
	if h.Docker.Available() {
		ctx, cancel := dockerCtx()
		defer cancel()
		if list, err := h.Docker.ListContainers(ctx); err == nil {
			containers = len(list)
			running = 0
			for _, ctr := range list {
				if ctr.State == "running" {
					running++
				}
			}
		}
	}
	cores, rxRate, txRate := h.serverRates()
	c.JSON(http.StatusOK, gin.H{
		"cpu": info.CPUPercent, "memTotalMB": info.MemTotalMB, "memUsedMB": info.MemUsedMB,
		"memPercent": info.MemPercent, "diskTotalGB": info.DiskTotalGB, "diskUsedGB": info.DiskUsedGB,
		"diskPercent": info.DiskPercent, "uptimeSec": info.UptimeSec,
		"cpuCores": cores, "netRxRate": rxRate, "netTxRate": txRate,
		"projects": projects, "services": services, "deployments": deployments,
		"containers": containers, "runningContainers": running,
		"dockerAvailable": h.Docker.Available(),
	})
}

// GET /api/dashboard — single-call overview for the frontend.
func (h *SystemHandler) Dashboard(c *gin.Context) {
	var projects, services, deploymentsCount, healthy, degraded, down int
	_ = h.DB.QueryRow(`SELECT COUNT(*) FROM projects`).Scan(&projects)
	_ = h.DB.QueryRow(`SELECT COUNT(*) FROM services`).Scan(&services)
	_ = h.DB.QueryRow(`SELECT COUNT(*) FROM deployments`).Scan(&deploymentsCount)
	_ = h.DB.QueryRow(`SELECT COUNT(*) FROM projects WHERE status='healthy' OR status='HEALTHY'`).Scan(&healthy)
	_ = h.DB.QueryRow(`SELECT COUNT(*) FROM projects WHERE status='degraded' OR status='DEGRADED'`).Scan(&degraded)
	_ = h.DB.QueryRow(`SELECT COUNT(*) FROM projects WHERE status='down' OR status='DOWN'`).Scan(&down)

	rows, _ := h.DB.Query(`SELECT id,name,description,repository,branch,environment,deployment_path,compose_file,gateway_prefix,health_url,status,created_at,updated_at FROM projects ORDER BY name`)
	plist := []models.Project{}
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var p models.Project
			var created, updated sql.NullString
			if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.Repository, &p.Branch, &p.Environment, &p.DeploymentPath, &p.ComposeFile, &p.GatewayPrefix, &p.HealthURL, &p.Status, &created, &updated); err == nil {
				p.CreatedAt, p.UpdatedAt = nullStr(created), nullStr(updated)
				plist = append(plist, p)
			}
		}
	}
	drows, _ := h.DB.Query(`SELECT id,project_id,commit_sha,branch,trigger,status,started_at,completed_at,duration_sec,logs FROM deployments ORDER BY id DESC LIMIT 10`)
	recent := []models.Deployment{}
	if drows != nil {
		defer drows.Close()
		recent = scanDeployments(drows)
	}
	c.JSON(http.StatusOK, gin.H{
		"server": serverinfo.Get(),
		"counts": gin.H{"projects": projects, "services": services, "deployments": deploymentsCount, "healthy": healthy, "degraded": degraded, "down": down},
		"projects": plist, "recentDeployments": recent,
		"dockerAvailable": h.Docker.Available(),
	})
}

// GET /api/audit — recent audit records (secrets never included).
func (h *SystemHandler) Audit(c *gin.Context) {
	limit := 100
	if v, err := strconv.Atoi(c.DefaultQuery("limit", "100")); err == nil && v > 0 && v <= 500 {
		limit = v
	}
	rows, err := h.DB.Query(`SELECT id,actor,action,resource,resource_id,timestamp,result,metadata FROM audit_logs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []models.AuditLog{}
	for rows.Next() {
		var a models.AuditLog
		var ts sql.NullString
		if err := rows.Scan(&a.ID, &a.Actor, &a.Action, &a.Resource, &a.ResourceID, &ts, &a.Result, &a.Metadata); err == nil {
			a.Timestamp = nullStr(ts)
			out = append(out, a)
		}
	}
	c.JSON(http.StatusOK, out)
}
