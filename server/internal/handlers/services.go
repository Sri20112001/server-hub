package handlers

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"serverhub/internal/audit"
	"serverhub/internal/middleware"
	"serverhub/internal/models"
)

type ServiceHandler struct {
	DB *sql.DB
}

var allowedServiceTypes = map[string]bool{
	"frontend": true, "backend": true, "worker": true, "database": true,
	"cache": true, "proxy": true, "other": true,
}

func scanService(row interface{ Scan(dest ...interface{}) error }) (models.Service, error) {
	var s models.Service
	var internalPort, hostPort sql.NullInt64
	var responseTime sql.NullInt64
	err := row.Scan(&s.ID, &s.ProjectID, &s.Name, &s.Type, &s.ContainerName,
		&internalPort, &hostPort, &s.HealthURL, &s.DockerServiceName, &s.Status,
		&s.LastHealth, &s.LastHealthAt, &responseTime)
	if internalPort.Valid {
		v := int(internalPort.Int64)
		s.InternalPort = &v
	}
	if hostPort.Valid {
		v := int(hostPort.Int64)
		s.HostPort = &v
	}
	if responseTime.Valid {
		v := responseTime.Int64
		s.ResponseTimeMs = &v
	}
	return s, err
}

func (h *ServiceHandler) ListByProject(c *gin.Context) {
	pid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project id"})
		return
	}
	rows, err := h.DB.Query(`SELECT id,project_id,name,type,container_name,internal_port,host_port,health_url,docker_service_name,status,last_health,last_health_at,response_time_ms FROM services WHERE project_id=? ORDER BY name`, pid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []models.Service{}
	for rows.Next() {
		var s models.Service
		var ip, hp sql.NullInt64
		var rt sql.NullInt64
		if err := rows.Scan(&s.ID, &s.ProjectID, &s.Name, &s.Type, &s.ContainerName, &ip, &hp, &s.HealthURL, &s.DockerServiceName, &s.Status, &s.LastHealth, &s.LastHealthAt, &rt); err == nil {
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
			out = append(out, s)
		}
	}
	c.JSON(http.StatusOK, out)
}

func (h *ServiceHandler) Create(c *gin.Context) {
	pid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project id"})
		return
	}
	var s models.Service
	if err := c.ShouldBindJSON(&s); err != nil || s.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	if s.Type == "" {
		s.Type = "other"
	}
	if !allowedServiceTypes[s.Type] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid service type"})
		return
	}
	var ip, hp interface{}
	if s.InternalPort != nil {
		ip = *s.InternalPort
	}
	if s.HostPort != nil {
		hp = *s.HostPort
	}
	res, err := h.DB.Exec(`INSERT INTO services (project_id,name,type,container_name,internal_port,host_port,health_url,docker_service_name,status)
		VALUES (?,?,?,?,?,?,?,?, 'unknown')`,
		pid, s.Name, s.Type, s.ContainerName, ip, hp, s.HealthURL, s.DockerServiceName)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	u, _ := middleware.CurrentUser(c)
	audit.Write(h.DB, u, "create", "service", strconv.FormatInt(id, 10), "ok", s.Name)
	h.getByID(c, id)
}

func (h *ServiceHandler) getByID(c *gin.Context, id int64) {
	s, err := scanService(h.DB.QueryRow(`SELECT id,project_id,name,type,container_name,internal_port,host_port,health_url,docker_service_name,status,last_health,last_health_at,response_time_ms FROM services WHERE id=?`, id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "service not found"})
		return
	}
	c.JSON(http.StatusOK, s)
}

func (h *ServiceHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var s models.Service
	if err := c.ShouldBindJSON(&s); err != nil || s.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	if s.Type != "" && !allowedServiceTypes[s.Type] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid service type"})
		return
	}
	var ip, hp interface{}
	if s.InternalPort != nil {
		ip = *s.InternalPort
	}
	if s.HostPort != nil {
		hp = *s.HostPort
	}
	_, err = h.DB.Exec(`UPDATE services SET name=?,type=?,container_name=?,internal_port=?,host_port=?,health_url=?,docker_service_name=? WHERE id=?`,
		s.Name, s.Type, s.ContainerName, ip, hp, s.HealthURL, s.DockerServiceName, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	u, _ := middleware.CurrentUser(c)
	audit.Write(h.DB, u, "update", "service", strconv.FormatInt(id, 10), "ok", s.Name)
	h.getByID(c, id)
}

func (h *ServiceHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	res, err := h.DB.Exec(`DELETE FROM services WHERE id=?`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "service not found"})
		return
	}
	u, _ := middleware.CurrentUser(c)
	audit.Write(h.DB, u, "delete", "service", strconv.FormatInt(id, 10), "ok", "")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
