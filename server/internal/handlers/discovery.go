package handlers

import (
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"serverhub/internal/audit"
	"serverhub/internal/config"
	"serverhub/internal/database"
	"serverhub/internal/dockerx"
	"serverhub/internal/events"
	"serverhub/internal/middleware"
	"serverhub/internal/scanfs"
)

// DiscoveryHandler scans the Docker shipyard and configured server roots
// and proposes project/service rows, so fleets can be imported instead of
// typed by hand.
type DiscoveryHandler struct {
	DB     *database.DB
	Docker *dockerx.Client
	Cfg    *config.Config
	Broker *events.Broker
}

type discoveredService struct {
	Name      string `json:"name"`
	Container string `json:"container"`
	Image     string `json:"image"`
	State     string `json:"state"`
	Type      string `json:"type"`
}

type discoveredProject struct {
	Name           string              `json:"name"`
	DeploymentPath string              `json:"deploymentPath"`
	ComposeFile    string              `json:"composeFile"`
	Registered     bool                `json:"registered"`
	Services       []discoveredService `json:"services"`
}

// guessType maps a compose service / image name to a service type.
func guessType(name, image string) string {
	hay := strings.ToLower(name + " " + image)
	switch {
	case strings.Contains(hay, "postgres"), strings.Contains(hay, "mysql"),
		strings.Contains(hay, "mariadb"), strings.Contains(hay, "mongo"),
		strings.Contains(hay, "sqlite"), strings.Contains(hay, "/db"),
		strings.HasSuffix(hay, "db"), strings.Contains(hay, "db:"):
		return "database"
	case strings.Contains(hay, "redis"), strings.Contains(hay, "memcached"),
		strings.Contains(hay, "cache"):
		return "cache"
	case strings.Contains(hay, "worker"), strings.Contains(hay, "queue"),
		strings.Contains(hay, "celery"):
		return "worker"
	case strings.Contains(hay, "api"), strings.Contains(hay, "backend"),
		strings.Contains(hay, "server"):
		return "backend"
	case strings.Contains(hay, "front"), strings.Contains(hay, "web"),
		strings.Contains(hay, "ui"), strings.Contains(hay, "client"),
		strings.Contains(hay, "nginx"), strings.Contains(hay, "caddy"):
		return "frontend"
	default:
		return "other"
	}
}

// GET /server-hub/api/discovery — Docker compose groups plus filesystem
// signature scan results.
func (h *DiscoveryHandler) Scan(c *gin.Context) {
	dockerProjects := h.scanDocker()
	fsProjects := h.scanFS()
	c.JSON(http.StatusOK, gin.H{
		"dockerAvailable": h.Docker.Available(),
		"projects":        dockerProjects,
		"filesystem":      fsProjects,
	})
}

func (h *DiscoveryHandler) scanFS() []scanfs.FoundProject {
	var roots []string
	if h.Cfg != nil {
		roots = h.Cfg.ScanRoots
	}
	found := scanfs.Scan(roots)
	for i := range found {
		var id int64
		err := h.DB.QueryRow(`SELECT id FROM projects WHERE name=? OR deployment_path=?`,
			found[i].Name, found[i].Path).Scan(&id)
		found[i].Registered = err == nil
	}
	if found == nil {
		found = []scanfs.FoundProject{}
	}
	return found
}

// scanDocker groups live containers by compose project.
func (h *DiscoveryHandler) scanDocker() []discoveredProject {
	if !h.Docker.Available() {
		return []discoveredProject{}
	}
	ctx, cancel := dockerCtx()
	defer cancel()
	list, err := h.Docker.ListContainers(ctx)
	if err != nil {
		return []discoveredProject{}
	}
	groups := map[string]*discoveredProject{}
	order := []string{}
	add := func(key string) *discoveredProject {
		if g, ok := groups[key]; ok {
			return g
		}
		g := &discoveredProject{Name: key, Services: []discoveredService{}}
		groups[key] = g
		order = append(order, key)
		return g
	}
	for _, ctr := range list {
		name := ""
		for _, n := range ctr.Names {
			name = strings.TrimPrefix(n, "/")
			break
		}
		project := ""
		service := ""
		workdir := ""
		configs := ""
		if ctr.Labels != nil {
			project = ctr.Labels["com.docker.compose.project"]
			service = ctr.Labels["com.docker.compose.service"]
			workdir = ctr.Labels["com.docker.compose.project.working_dir"]
			configs = ctr.Labels["com.docker.compose.project.config_files"]
		}
		var g *discoveredProject
		if project != "" {
			g = add(project)
			if g.DeploymentPath == "" {
				if workdir != "" {
					g.DeploymentPath = workdir
				} else if configs != "" {
					first := strings.Split(configs, ",")[0]
					g.DeploymentPath = filepath.Dir(first)
					g.ComposeFile = filepath.Base(first)
				}
			}
			if g.ComposeFile == "" {
				g.ComposeFile = "docker-compose.yml"
			}
		} else {
			// Standalone container: its own one-ship group.
			g = add(name)
			if g.ComposeFile == "" {
				g.ComposeFile = "docker-compose.yml"
			}
		}
		if service == "" {
			service = name
		}
		g.Services = append(g.Services, discoveredService{
			Name:      service,
			Container: name,
			Image:     ctr.Image,
			State:     ctr.State,
			Type:      guessType(service, ctr.Image),
		})
	}
	out := make([]discoveredProject, 0, len(order))
	for _, key := range order {
		g := groups[key]
		var id int64
		err := h.DB.QueryRow(`SELECT id FROM projects WHERE name=?`, g.Name).Scan(&id)
		g.Registered = err == nil
		out = append(out, *g)
	}
	return out
}

// POST /server-hub/api/discovery/import — create project + service rows
// from a scan result. Idempotent: re-importing only adds missing services.
func (h *DiscoveryHandler) Import(c *gin.Context) {
	var body struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.Name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	name := strings.TrimSpace(body.Name)
	u, _ := middleware.CurrentUser(c)

	// Re-run the scan server-side so the client can't inject arbitrary rows.
	// Filesystem finds are tried first (exact path match wins on servers
	// where compose labels and directory names agree).
	var deployPath, composeFile string
	var services []discoveredService
	if fp := h.detectFS(name); fp != nil {
		deployPath, composeFile = fp.Path, fp.ComposeFile
		if composeFile == "" {
			composeFile = "docker-compose.yml"
		}
		if fp.ComposeFile != "" {
			for _, s := range scanfs.ParseComposeServices(filepath.Join(fp.Path, fp.ComposeFile)) {
				services = append(services, discoveredService{Name: s, Type: guessType(s, "")})
			}
		}
		if len(services) == 0 {
			services = append(services, discoveredService{Name: name, Type: "other"})
		}
	} else {
		detected, err := h.detect(name)
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
			return
		}
		if detected == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "no such project in the shipyard scan"})
			return
		}
		deployPath, composeFile, services = detected.DeploymentPath, detected.ComposeFile, detected.Services
	}

	var pid int64
	err := h.DB.QueryRow(`SELECT id FROM projects WHERE name=?`, name).Scan(&pid)
	if err != nil {
		newID, err := h.DB.InsertID(`INSERT INTO projects (name,branch,environment,deployment_path,compose_file,status)
			VALUES (?, 'main', 'production', ?, ?, 'unknown')`,
			name, deployPath, composeFile)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		pid = newID
		audit.Write(h.DB, u, "import", "project", strconv.FormatInt(pid, 10), "ok", name)
	}
	added := 0
	for _, s := range services {
		var sid int64
		err := h.DB.QueryRow(`SELECT id FROM services WHERE project_id=? AND name=?`, pid, s.Name).Scan(&sid)
		if err == nil {
			continue
		}
		_, err = h.DB.Exec(`INSERT INTO services (project_id,name,type,container_name,status)
			VALUES (?,?,?,?,'unknown')`, pid, s.Name, s.Type, s.Container)
		if err == nil {
			added++
		}
	}
	audit.Write(h.DB, u, "import", "services", strconv.FormatInt(pid, 10), "ok",
		strconv.Itoa(added)+" added")
	if h.Broker != nil {
		h.Broker.Publish("discovery.completed", map[string]interface{}{
			"projectId": pid, "project": name, "servicesAdded": added,
		})
	}
	var branch, env, status string
	_ = h.DB.QueryRow(`SELECT branch,environment,deployment_path,compose_file,status FROM projects WHERE id=?`, pid).
		Scan(&branch, &env, &deployPath, &composeFile, &status)
	c.JSON(http.StatusOK, gin.H{
		"id": pid, "name": name, "branch": branch, "environment": env,
		"deploymentPath": deployPath, "composeFile": composeFile,
		"status": status, "servicesAdded": added,
	})
}

func (h *DiscoveryHandler) detectFS(name string) *scanfs.FoundProject {
	var roots []string
	if h.Cfg != nil {
		roots = h.Cfg.ScanRoots
	}
	for _, fp := range scanfs.Scan(roots) {
		if fp.Name == name {
			cp := fp
			return &cp
		}
	}
	return nil
}

func (h *DiscoveryHandler) detect(name string) (*discoveredProject, error) {
	if !h.Docker.Available() {
		return nil, dockerx.ErrUnavailable
	}
	ctx, cancel := dockerCtx()
	defer cancel()
	list, err := h.Docker.ListContainers(ctx)
	if err != nil {
		return nil, err
	}
	var g *discoveredProject
	for _, ctr := range list {
		cname := ""
		for _, n := range ctr.Names {
			cname = strings.TrimPrefix(n, "/")
			break
		}
		project := ""
		service := ""
		workdir := ""
		configs := ""
		if ctr.Labels != nil {
			project = ctr.Labels["com.docker.compose.project"]
			service = ctr.Labels["com.docker.compose.service"]
			workdir = ctr.Labels["com.docker.compose.project.working_dir"]
			configs = ctr.Labels["com.docker.compose.project.config_files"]
		}
		key := project
		if key == "" {
			key = cname
		}
		if key != name {
			continue
		}
		if g == nil {
			g = &discoveredProject{Name: name, Services: []discoveredService{}}
		}
		if g.DeploymentPath == "" {
			if workdir != "" {
				g.DeploymentPath = workdir
			} else if configs != "" {
				first := strings.Split(configs, ",")[0]
				g.DeploymentPath = filepath.Dir(first)
				g.ComposeFile = filepath.Base(first)
			}
		}
		if g.ComposeFile == "" {
			g.ComposeFile = "docker-compose.yml"
		}
		if service == "" {
			service = cname
		}
		g.Services = append(g.Services, discoveredService{
			Name:      service,
			Container: cname,
			Image:     ctr.Image,
			State:     ctr.State,
			Type:      guessType(service, ctr.Image),
		})
	}
	return g, nil
}
