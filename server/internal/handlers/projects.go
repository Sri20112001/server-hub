package handlers

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"serverhub/internal/audit"
	"serverhub/internal/database"
	"serverhub/internal/middleware"
	"serverhub/internal/models"
)

type ProjectHandler struct {
	DB *database.DB
}

func scanProject(row interface {
	Scan(dest ...interface{}) error
}) (models.Project, error) {
	var p models.Project
	var autoDeploy int
	var created, updated sql.NullString
	err := row.Scan(&p.ID, &p.Name, &p.Description, &p.Repository, &p.Branch,
		&p.Environment, &p.DeploymentPath, &p.ComposeFile, &p.GatewayPrefix,
		&p.HealthURL, &p.Status, &autoDeploy, &created, &updated)
	p.AutoDeploy = autoDeploy == 1
	p.CreatedAt, p.UpdatedAt = nullStr(created), nullStr(updated)
	return p, err
}

func (h *ProjectHandler) List(c *gin.Context) {
	rows, err := h.DB.Query(`SELECT id,name,description,repository,branch,environment,deployment_path,compose_file,gateway_prefix,health_url,status,auto_deploy,created_at,updated_at FROM projects ORDER BY name`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []models.Project{}
	for rows.Next() {
		var p models.Project
		var autoDeploy int
		var created, updated sql.NullString
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.Repository, &p.Branch,
			&p.Environment, &p.DeploymentPath, &p.ComposeFile, &p.GatewayPrefix,
			&p.HealthURL, &p.Status, &autoDeploy, &created, &updated); err == nil {
			p.AutoDeploy = autoDeploy == 1
			p.CreatedAt, p.UpdatedAt = nullStr(created), nullStr(updated)
			out = append(out, p)
		}
	}
	c.JSON(http.StatusOK, out)
}

func (h *ProjectHandler) Create(c *gin.Context) {
	var p models.Project
	if err := c.ShouldBindJSON(&p); err != nil || p.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	if p.Branch == "" {
		p.Branch = "main"
	}
	if p.Environment == "" {
		p.Environment = "production"
	}
	if p.ComposeFile == "" {
		p.ComposeFile = "docker-compose.yml"
	}
	id, err := h.DB.InsertID(`INSERT INTO projects (name,description,repository,branch,environment,deployment_path,compose_file,gateway_prefix,health_url,status,auto_deploy)
		VALUES (?,?,?,?,?,?,?,?,?,'unknown',?)`,
		p.Name, p.Description, p.Repository, p.Branch, p.Environment, p.DeploymentPath, p.ComposeFile, p.GatewayPrefix, p.HealthURL, boolToInt(p.AutoDeploy))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	u, _ := middleware.CurrentUser(c)
	audit.Write(h.DB, u, "create", "project", strconv.FormatInt(id, 10), "ok", p.Name)
	h.GetByID(c, id)
}

func (h *ProjectHandler) Get(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	h.GetByID(c, id)
}

func (h *ProjectHandler) GetByID(c *gin.Context, id int64) {
	p, err := scanProject(h.DB.QueryRow(`SELECT id,name,description,repository,branch,environment,deployment_path,compose_file,gateway_prefix,health_url,status,auto_deploy,created_at,updated_at FROM projects WHERE id=?`, id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}
	c.JSON(http.StatusOK, p)
}

func (h *ProjectHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var p models.Project
	if err := c.ShouldBindJSON(&p); err != nil || p.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	_, err = h.DB.Exec(`UPDATE projects SET name=?,description=?,repository=?,branch=?,environment=?,deployment_path=?,compose_file=?,gateway_prefix=?,health_url=?,auto_deploy=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		p.Name, p.Description, p.Repository, p.Branch, p.Environment, p.DeploymentPath, p.ComposeFile, p.GatewayPrefix, p.HealthURL, boolToInt(p.AutoDeploy), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	u, _ := middleware.CurrentUser(c)
	audit.Write(h.DB, u, "update", "project", strconv.FormatInt(id, 10), "ok", p.Name)
	h.GetByID(c, id)
}

func (h *ProjectHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	res, err := h.DB.Exec(`DELETE FROM projects WHERE id=?`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}
	u, _ := middleware.CurrentUser(c)
	audit.Write(h.DB, u, "delete", "project", strconv.FormatInt(id, 10), "ok", "")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
