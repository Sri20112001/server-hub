package handlers

// DatabasesHandler auto-discovers database servers and lets the user pick
// which databases they need.
//
// Sources (no credentials needed to detect):
//   - Docker containers whose image is postgres/mysql/redis/mongo (and kin)
//   - Host ports 5432/3306/6379/27017 (TCP probe + banner sniff, best-effort)
//   - Fleet services already typed database/cache
//
// Flows:
//   - Browse lists the databases inside a server. Docker Postgres/MySQL/
//     Redis/Mongo shells work without credentials; everything else uses the
//     saved connection (Connect) or inline username/password.
//   - Connect verifies + stores the credential (AES-GCM, same key as secrets).
//   - Register creates service rows for the ticked databases under a project
//     and records db_registrations so re-scans show them as registered.

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/gin-gonic/gin"
	mysqlcfg "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"

	"serverhub/internal/audit"
	"serverhub/internal/config"
	"serverhub/internal/crypto"
	"serverhub/internal/database"
	"serverhub/internal/dockerx"
	"serverhub/internal/middleware"
)

type DatabasesHandler struct {
	DB     *database.DB
	Docker *dockerx.Client
	Cfg    *config.Config
}

type dbServerInfo struct {
	Key        string   `json:"key"`
	Engine     string   `json:"engine"` // postgres | mysql | redis | mongo | unknown
	Source     string   `json:"source"` // docker | host | project
	Name       string   `json:"name"`
	Host       string   `json:"host"`
	Port       int      `json:"port"`
	Container  string   `json:"container"`
	Version    string   `json:"version"`
	State      string   `json:"state"`
	Verified   bool     `json:"verified"` // engine confirmed (not just port-guess)
	HasCreds   bool     `json:"hasCreds"`
	Registered []string `json:"registered"`
}

type dbInfo struct {
	Name       string `json:"name"`
	SizeBytes  int64  `json:"sizeBytes"` // -1 when unknown
	Registered bool   `json:"registered"`
	System     bool   `json:"system"`
}

type dbTarget struct {
	Engine    string `json:"engine"`
	Source    string `json:"source"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Container string `json:"container"`
	Username  string `json:"username"`
	Password  string `json:"password"`
}

var dbEngines = map[string]bool{
	"postgres": true, "mysql": true, "redis": true, "mongo": true,
}

func dbDefaultPort(engine string) int {
	switch engine {
	case "postgres":
		return 5432
	case "mysql":
		return 3306
	case "redis":
		return 6379
	case "mongo":
		return 27017
	}
	return 0
}

// dbEngineFromImage maps a container image to an engine + tag version.
func dbEngineFromImage(image string) (engine, version string, ok bool) {
	lower := strings.ToLower(image)
	if i := strings.Index(lower, "@sha256:"); i >= 0 {
		lower = lower[:i]
	}
	name, tag := lower, ""
	if i := strings.LastIndex(lower, ":"); i >= 0 && !strings.Contains(lower[i:], "/") {
		name, tag = lower[:i], lower[i+1:]
	}
	switch {
	case strings.Contains(name, "postgres"), strings.Contains(name, "postgis"),
		strings.Contains(name, "timescale"):
		return "postgres", tag, true
	case strings.Contains(name, "mysql"), strings.Contains(name, "mariadb"):
		return "mysql", tag, true
	case strings.Contains(name, "redis"), strings.Contains(name, "valkey"),
		strings.Contains(name, "keydb"):
		return "redis", tag, true
	case strings.Contains(name, "mongo"):
		return "mongo", tag, true
	}
	return "", "", false
}

// dbServerKey is the stable identity shared by detection, saved connections
// and registrations.
func dbServerKey(source, engine, host string, port int, cont string) string {
	return strings.Join([]string{
		strings.ToLower(strings.TrimSpace(source)),
		strings.ToLower(strings.TrimSpace(engine)),
		strings.ToLower(strings.TrimSpace(host)),
		strconv.Itoa(port),
		strings.TrimSpace(cont),
	}, "|")
}

// GET /server-hub/api/databases/servers — every detected server merged with
// saved credentials and already-registered databases.
func (h *DatabasesHandler) Servers(c *gin.Context) {
	servers := h.detect()
	c.JSON(200, gin.H{"servers": servers, "dockerAvailable": h.Docker.Available()})
}

func (h *DatabasesHandler) detect() []dbServerInfo {
	out := []dbServerInfo{}
	seen := map[string]bool{}
	add := func(s dbServerInfo) {
		if s.Key == "" || seen[s.Key] {
			return
		}
		seen[s.Key] = true
		out = append(out, s)
	}

	// 1. Docker containers running a database image.
	dockerNames := map[string]bool{}
	if h.Docker.Available() {
		ctx, cancel := dockerCtx()
		list, err := h.Docker.ListContainers(ctx)
		cancel()
		if err == nil {
			for _, ctr := range list {
				if !strings.EqualFold(ctr.State, "running") {
					continue
				}
				engine, version, ok := dbEngineFromImage(ctr.Image)
				if !ok {
					continue
				}
				name := ""
				for _, n := range ctr.Names {
					name = strings.TrimPrefix(n, "/")
					break
				}
				if name == "" {
					name = ctr.ID[:12]
				}
				dockerNames[name] = true
				add(dbServerInfo{
					Key:       dbServerKey("docker", engine, "", dbDefaultPort(engine), name),
					Engine:    engine,
					Source:    "docker",
					Name:      name,
					Port:      dbDefaultPort(engine),
					Container: name,
					Version:   version,
					State:     ctr.State,
					Verified:  true,
				})
			}
		}
	}

	// 2. Host ports. Skipped when a Docker DB container already publishes
	// the same host port (same server, no duplicate entry).
	published := map[int]bool{}
	if h.Docker.Available() {
		ctx, cancel := dockerCtx()
		if list, err := h.Docker.ListContainers(ctx); err == nil {
			for _, ctr := range list {
				if _, _, ok := dbEngineFromImage(ctr.Image); !ok {
					continue
				}
				for _, p := range ctr.Ports {
					if p.PublicPort > 0 {
						published[int(p.PublicPort)] = true
					}
				}
			}
		}
		cancel()
	}
	for _, probe := range []struct {
		engine string
		port   int
	}{{"postgres", 5432}, {"mysql", 3306}, {"redis", 6379}, {"mongo", 27017}} {
		if published[probe.port] {
			continue
		}
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", probe.port), 700*time.Millisecond)
		if err != nil {
			continue
		}
		version := sniffDBBanner(conn, probe.engine)
		_ = conn.Close()
		add(dbServerInfo{
			Key:      dbServerKey("host", probe.engine, "127.0.0.1", probe.port, ""),
			Engine:   probe.engine,
			Source:   "host",
			Name:     fmt.Sprintf("127.0.0.1:%d", probe.port),
			Host:     "127.0.0.1",
			Port:     probe.port,
			Version:  version,
			State:    "open",
			Verified: version != "",
		})
	}

	// 3. Fleet services typed database/cache not backed by a live container.
	rows, err := h.DB.Query(`SELECT name,type,container_name FROM services WHERE type IN ('database','cache') ORDER BY name`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var name, typ, cont string
			if err := rows.Scan(&name, &typ, &cont); err != nil {
				continue
			}
			if dockerNames[cont] {
				continue // covered by the Docker entry above
			}
			engine := "unknown"
			if typ == "cache" {
				engine = "redis"
			}
			add(dbServerInfo{
				Key:       dbServerKey("project", engine, "", 0, name),
				Engine:    engine,
				Source:    "project",
				Name:      name,
				Container: cont,
				State:     "fleet",
			})
		}
	}

	// Merge saved credentials + registrations.
	creds := map[string]bool{}
	if r, err := h.DB.Query(`SELECT key, username, encrypted_value FROM db_servers`); err == nil {
		defer r.Close()
		for r.Next() {
			var k, u, enc string
			if err := r.Scan(&k, &u, &enc); err == nil {
				creds[k] = u != "" || enc != ""
			}
		}
	}
	regs := map[string][]string{}
	if r, err := h.DB.Query(`SELECT server_key, database FROM db_registrations ORDER BY database`); err == nil {
		defer r.Close()
		for r.Next() {
			var k, d string
			if err := r.Scan(&k, &d); err == nil {
				regs[k] = append(regs[k], d)
			}
		}
	}
	for i := range out {
		out[i].HasCreds = creds[out[i].Key]
		out[i].Registered = regs[out[i].Key]
		if out[i].Registered == nil {
			out[i].Registered = []string{}
		}
	}
	if out == nil {
		out = []dbServerInfo{}
	}
	return out
}

// sniffDBBanner tries to confirm the engine behind an open host port.
// Best-effort: "" means "open, engine unconfirmed".
func sniffDBBanner(conn net.Conn, engine string) string {
	_ = conn.SetDeadline(time.Now().Add(1200 * time.Millisecond))
	buf := make([]byte, 256)
	switch engine {
	case "mysql":
		// Server greeting: len(3) + seq + 0x0a + version NUL-terminated.
		n, err := conn.Read(buf)
		if err != nil || n < 8 || buf[4] != 0x0a {
			return ""
		}
		end := bytes.IndexByte(buf[5:n], 0)
		if end < 0 {
			return ""
		}
		return string(buf[5 : 5+end])
	case "redis":
		if _, err := conn.Write([]byte("PING\r\n")); err != nil {
			return ""
		}
		n, err := conn.Read(buf)
		if err != nil || n == 0 {
			return ""
		}
		line := strings.TrimSpace(string(buf[:n]))
		switch {
		case strings.HasPrefix(line, "+PONG"):
			return "open (no auth)"
		case strings.Contains(line, "NOAUTH"):
			return "auth required"
		}
	}
	return ""
}

// POST /server-hub/api/databases/browse — list databases in a server.
// Body: {engine, source, host, port, container, username?, password?}.
// Missing credentials fall back to the saved connection; Docker shells
// work credential-free where the image allows it.
func (h *DatabasesHandler) Browse(c *gin.Context) {
	var t dbTarget
	if err := c.ShouldBindJSON(&t); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}
	t.Engine = strings.ToLower(strings.TrimSpace(t.Engine))
	if !dbEngines[t.Engine] {
		c.JSON(400, gin.H{"error": "unknown engine (want postgres, mysql, redis or mongo)"})
		return
	}
	if t.Port == 0 {
		t.Port = dbDefaultPort(t.Engine)
	}
	user, pass := t.Username, t.Password
	if user == "" && pass == "" {
		if u, p, ok := h.savedCreds(t); ok {
			user, pass = u, p
		}
	}
	dbs, authRequired, errMsg := h.browse(t, user, pass)
	if errMsg != "" {
		code := 502
		if authRequired {
			code = 401
		}
		c.JSON(code, gin.H{"error": errMsg, "authRequired": authRequired})
		return
	}
	// Flag already-registered rows.
	reg := map[string]bool{}
	if r, err := h.DB.Query(`SELECT database FROM db_registrations WHERE server_key=?`,
		dbServerKey(t.Source, t.Engine, t.Host, t.Port, t.Container)); err == nil {
		defer r.Close()
		for r.Next() {
			var d string
			if err := r.Scan(&d); err == nil {
				reg[d] = true
			}
		}
	}
	for i := range dbs {
		dbs[i].Registered = reg[dbs[i].Name]
	}
	c.JSON(200, gin.H{"databases": dbs, "authRequired": false})
}

// POST /server-hub/api/databases/connect — verify + save a connection.
// Body: {engine, source, host, port, container, username?, password?}.
func (h *DatabasesHandler) Connect(c *gin.Context) {
	var t dbTarget
	if err := c.ShouldBindJSON(&t); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}
	t.Engine = strings.ToLower(strings.TrimSpace(t.Engine))
	if !dbEngines[t.Engine] {
		c.JSON(400, gin.H{"error": "unknown engine (want postgres, mysql, redis or mongo)"})
		return
	}
	if t.Source == "" {
		t.Source = "host"
	}
	if t.Port == 0 {
		t.Port = dbDefaultPort(t.Engine)
	}
	if t.Source == "docker" && strings.TrimSpace(t.Container) == "" {
		c.JSON(400, gin.H{"error": "container is required for docker sources"})
		return
	}
	if t.Source != "docker" && strings.TrimSpace(t.Host) == "" {
		c.JSON(400, gin.H{"error": "host is required"})
		return
	}
	key := dbServerKey(t.Source, t.Engine, t.Host, t.Port, t.Container)
	user, pass := strings.TrimSpace(t.Username), t.Password
	if user == "" && pass == "" {
		if u, p, ok := h.savedCreds(t); ok {
			user, pass = u, p
		}
	}
	// Verify before storing anything.
	dbs, authRequired, errMsg := h.browse(t, user, pass)
	if errMsg != "" {
		code := 502
		if authRequired {
			code = 401
		}
		c.JSON(code, gin.H{"error": errMsg, "authRequired": authRequired})
		return
	}
	enc, nonce := "", ""
	if pass != "" {
		k, err := crypto.KeyFromHex(h.Cfg.EncryptionKey)
		if err != nil {
			c.JSON(500, gin.H{"error": "server encryption misconfigured"})
			return
		}
		var err2 error
		enc, nonce, err2 = crypto.Encrypt(k, pass)
		if err2 != nil {
			c.JSON(500, gin.H{"error": "encryption failed"})
			return
		}
	}
	// Re-read stored row to preserve its password when none was supplied.
	keepEnc, keepNonce := "", ""
	if pass == "" {
		var e, n string
		if err := h.DB.QueryRow(`SELECT encrypted_value, nonce FROM db_servers WHERE key=?`, key).Scan(&e, &n); err == nil {
			keepEnc, keepNonce = e, n
		}
	}
	if enc == "" {
		enc, nonce = keepEnc, keepNonce
	}
	if _, err := h.DB.Exec(`INSERT INTO db_servers (key,engine,source,host,port,container,username,encrypted_value,nonce,default_db,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,? ,CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET engine=excluded.engine, source=excluded.source, host=excluded.host,
		port=excluded.port, container=excluded.container, username=excluded.username,
		encrypted_value=excluded.encrypted_value, nonce=excluded.nonce, updated_at=CURRENT_TIMESTAMP`,
		key, t.Engine, t.Source, t.Host, t.Port, t.Container, user, enc, nonce, ""); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	u, _ := middleware.CurrentUser(c)
	audit.Write(h.DB, u, "connect", "database", key, "ok", t.Engine)
	c.JSON(200, gin.H{"ok": true, "key": key, "hasCreds": user != "" || enc != "", "databases": len(dbs)})
}

// POST /server-hub/api/databases/register — register ticked databases as
// services under a project. Body: {projectId, server:{...}, databases:[]}.
func (h *DatabasesHandler) Register(c *gin.Context) {
	var body struct {
		ProjectID int64    `json:"projectId"`
		Server    dbTarget `json:"server"`
		Databases []string `json:"databases"`
		Username  string   `json:"username"`
		Password  string   `json:"password"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}
	var pname string
	if err := h.DB.QueryRow(`SELECT name FROM projects WHERE id=?`, body.ProjectID).Scan(&pname); err != nil {
		c.JSON(404, gin.H{"error": "project not found"})
		return
	}
	t := body.Server
	t.Engine = strings.ToLower(strings.TrimSpace(t.Engine))
	if !dbEngines[t.Engine] {
		c.JSON(400, gin.H{"error": "unknown engine (want postgres, mysql, redis or mongo)"})
		return
	}
	if t.Port == 0 {
		t.Port = dbDefaultPort(t.Engine)
	}
	// If credentials ride along, verify + persist them like Connect.
	if strings.TrimSpace(body.Username) != "" || body.Password != "" {
		t.Username, t.Password = body.Username, body.Password
		if _, authRequired, errMsg := h.browse(t, strings.TrimSpace(body.Username), body.Password); errMsg != "" {
			code := 502
			if authRequired {
				code = 401
			}
			c.JSON(code, gin.H{"error": errMsg, "authRequired": authRequired})
			return
		}
		h.saveCreds(t, strings.TrimSpace(body.Username), body.Password)
	}
	key := dbServerKey(t.Source, t.Engine, t.Host, t.Port, t.Container)
	svcType := "database"
	if t.Engine == "redis" {
		svcType = "cache"
	}
	u, _ := middleware.CurrentUser(c)
	registered := []gin.H{}
	skipped := []string{}
	seenNames := map[string]bool{}
	for _, name := range body.Databases {
		name = strings.TrimSpace(name)
		if name == "" || seenNames[name] {
			continue
		}
		seenNames[name] = true
		var rid int64
		if err := h.DB.QueryRow(`SELECT id FROM db_registrations WHERE server_key=? AND database=?`, key, name).Scan(&rid); err == nil {
			skipped = append(skipped, name)
			continue
		}
		var sid int64
		if err := h.DB.QueryRow(`SELECT id FROM services WHERE project_id=? AND name=?`, body.ProjectID, name).Scan(&sid); err != nil {
			newID, err := h.DB.InsertID(`INSERT INTO services (project_id,name,type,container_name,status)
				VALUES (?,?,?,?,'unknown')`, body.ProjectID, name, svcType, t.Container)
			if err != nil {
				skipped = append(skipped, name)
				continue
			}
			sid = newID
			audit.Write(h.DB, u, "create", "service", strconv.FormatInt(sid, 10), "ok", name)
		}
		if _, err := h.DB.Exec(`INSERT INTO db_registrations (server_key,database,project_id,service_id) VALUES (?,?,?,?)
			ON CONFLICT(server_key, database) DO NOTHING`, key, name, body.ProjectID, sid); err != nil {
			skipped = append(skipped, name)
			continue
		}
		registered = append(registered, gin.H{"database": name, "serviceId": sid})
	}
	audit.Write(h.DB, u, "register-databases", "database", key, "ok",
		"project="+strconv.FormatInt(body.ProjectID, 10)+" count="+strconv.Itoa(len(registered)))
	c.JSON(200, gin.H{"ok": true, "registered": registered, "skipped": skipped})
}

// savedCreds returns the stored username/password for a server target.
func (h *DatabasesHandler) savedCreds(t dbTarget) (user, pass string, ok bool) {
	var enc, nonce string
	key := dbServerKey(t.Source, t.Engine, t.Host, t.Port, t.Container)
	if err := h.DB.QueryRow(`SELECT username, encrypted_value, nonce FROM db_servers WHERE key=?`, key).
		Scan(&user, &enc, &nonce); err != nil {
		return "", "", false
	}
	if enc == "" {
		return user, "", true
	}
	k, err := crypto.KeyFromHex(h.Cfg.EncryptionKey)
	if err != nil {
		return "", "", false
	}
	pt, err := crypto.Decrypt(k, enc, nonce)
	if err != nil {
		return "", "", false
	}
	return user, pt, true
}

// saveCreds persists a verified credential (best-effort; failures swallowed).
func (h *DatabasesHandler) saveCreds(t dbTarget, user, pass string) {
	key := dbServerKey(t.Source, t.Engine, t.Host, t.Port, t.Container)
	enc, nonce := "", ""
	if pass != "" {
		k, err := crypto.KeyFromHex(h.Cfg.EncryptionKey)
		if err != nil {
			return
		}
		var err2 error
		enc, nonce, err2 = crypto.Encrypt(k, pass)
		if err2 != nil {
			return
		}
	} else {
		var e, n string
		if err := h.DB.QueryRow(`SELECT encrypted_value, nonce FROM db_servers WHERE key=?`, key).Scan(&e, &n); err == nil {
			enc, nonce = e, n
		}
	}
	_, _ = h.DB.Exec(`INSERT INTO db_servers (key,engine,source,host,port,container,username,encrypted_value,nonce,default_db,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,'',CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET engine=excluded.engine, source=excluded.source, host=excluded.host,
		port=excluded.port, container=excluded.container, username=excluded.username,
		encrypted_value=excluded.encrypted_value, nonce=excluded.nonce, updated_at=CURRENT_TIMESTAMP`,
		key, t.Engine, t.Source, t.Host, t.Port, t.Container, user, enc, nonce)
}

// browse lists databases for a target using explicit credentials.
// Returns (rows, authRequired, errMsg).
func (h *DatabasesHandler) browse(t dbTarget, user, pass string) ([]dbInfo, bool, string) {
	switch t.Engine {
	case "postgres":
		if t.Source == "docker" && strings.TrimSpace(t.Container) != "" {
			if user == "" {
				user = h.pgContainerUser(t.Container)
			}
			return h.listPostgresExec(t.Container, user, pass)
		}
		if strings.TrimSpace(t.Host) == "" {
			return nil, false, "host is required for non-docker sources"
		}
		return h.listPostgresDirect(t.Host, t.Port, user, pass)
	case "mysql":
		if t.Source == "docker" && strings.TrimSpace(t.Container) != "" {
			if user == "" {
				user = "root"
			}
			return h.listMysqlExec(t.Container, user, pass)
		}
		if strings.TrimSpace(t.Host) == "" {
			return nil, false, "host is required for non-docker sources"
		}
		if user == "" {
			return nil, true, "username and password are required"
		}
		return h.listMysqlDirect(t.Host, t.Port, user, pass)
	case "redis":
		if t.Source == "docker" && strings.TrimSpace(t.Container) != "" {
			return h.listRedisExec(t.Container, pass)
		}
		if strings.TrimSpace(t.Host) == "" {
			return nil, false, "host is required for non-docker sources"
		}
		return h.listRedisDirect(t.Host, t.Port, pass)
	case "mongo":
		if t.Source == "docker" && strings.TrimSpace(t.Container) != "" {
			return h.listMongoExec(t.Container, user, pass)
		}
		return nil, false, "direct MongoDB connections are not supported yet — run it as a container or save a connection first"
	}
	return nil, false, "unknown engine"
}

func pgSystemDb(name string) bool { return name == "postgres" }

func mysqlSystemDb(name string) bool {
	switch name {
	case "information_schema", "performance_schema", "mysql", "sys":
		return true
	}
	return false
}

// execInContainer runs a command and captures output (non-interactive).
func (h *DatabasesHandler) execInContainer(cont string, env []string, cmd ...string) (string, string, int, error) {
	raw, err := h.Docker.Raw()
	if err != nil {
		return "", "", 0, err
	}
	ctx, cancel := dockerCtx()
	defer cancel()
	resp, err := raw.ContainerExecCreate(ctx, cont, container.ExecOptions{
		AttachStdout: true, AttachStderr: true, Env: env, Cmd: cmd,
	})
	if err != nil {
		return "", "", 0, err
	}
	attach, err := raw.ContainerExecAttach(ctx, resp.ID, container.ExecAttachOptions{})
	if err != nil {
		return "", "", 0, err
	}
	var outB, errB bytes.Buffer
	_, copyErr := stdcopy.StdCopy(&outB, &errB, attach.Reader)
	attach.Close()
	insp, err := raw.ContainerExecInspect(ctx, resp.ID)
	if err != nil {
		return outB.String(), errB.String(), 0, err
	}
	if copyErr != nil {
		return outB.String(), errB.String(), insp.ExitCode, copyErr
	}
	return outB.String(), errB.String(), insp.ExitCode, nil
}

// pgContainerUser reads POSTGRES_USER from the container env (default postgres).
func (h *DatabasesHandler) pgContainerUser(cont string) string {
	ctx, cancel := dockerCtx()
	defer cancel()
	info, err := h.Docker.InspectContainer(ctx, cont)
	if err != nil || info.Config == nil {
		return "postgres"
	}
	for _, e := range info.Config.Env {
		if v, ok := strings.CutPrefix(e, "POSTGRES_USER="); ok && strings.TrimSpace(v) != "" {
			return v
		}
	}
	return "postgres"
}

func (h *DatabasesHandler) listPostgresExec(cont, user, pass string) ([]dbInfo, bool, string) {
	env := []string{}
	if pass != "" {
		env = append(env, "PGPASSWORD="+pass)
	}
	if user == "" {
		user = "postgres"
	}
	out, errOut, code, err := h.execInContainer(cont, env,
		"psql", "-U", user, "-tA",
		"-c", "SELECT datname||'|'||pg_database_size(datname) FROM pg_database WHERE datistemplate=false ORDER BY 1")
	if err != nil {
		return nil, false, "exec failed: " + err.Error()
	}
	if code != 0 {
		msg := strings.TrimSpace(errOut + " " + out)
		if strings.Contains(msg, "password authentication failed") || strings.Contains(msg, "no password supplied") {
			return nil, true, "database password required"
		}
		if strings.Contains(msg, "role") && strings.Contains(msg, "does not exist") {
			return nil, true, "database user required: " + msg
		}
		return nil, false, "psql: " + msg
	}
	return parsePgSizes(out), false, ""
}

func parsePgSizes(out string) []dbInfo {
	res := []dbInfo{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		name, sizeStr, _ := strings.Cut(line, "|")
		sz, _ := strconv.ParseInt(strings.TrimSpace(sizeStr), 10, 64)
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		res = append(res, dbInfo{Name: name, SizeBytes: sz, System: pgSystemDb(name)})
	}
	return res
}

func (h *DatabasesHandler) listPostgresDirect(host string, port int, user, pass string) ([]dbInfo, bool, string) {
	_ = h
	if user == "" {
		return nil, true, "username and password are required"
	}
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, pass),
		Host:   fmt.Sprintf("%s:%d", host, port),
		Path:   "/postgres",
	}
	q := u.Query()
	q.Set("sslmode", "disable")
	q.Set("connect_timeout", "8")
	u.RawQuery = q.Encode()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, u.String())
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "28P01") || strings.Contains(msg, "password authentication failed") || strings.Contains(msg, "no password supplied") {
			return nil, true, "invalid username or password"
		}
		return nil, false, "connect failed: " + msg
	}
	defer conn.Close(context.Background())
	rows, err := conn.Query(ctx, `SELECT datname||'|'||pg_database_size(datname) FROM pg_database WHERE datistemplate=false ORDER BY 1`)
	if err != nil {
		return nil, false, "query failed: " + err.Error()
	}
	defer rows.Close()
	var sb strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err == nil {
			sb.WriteString(line + "\n")
		}
	}
	return parsePgSizes(sb.String()), false, ""
}

func parseMysqlNames(out string) []string {
	names := []string{}
	for _, line := range strings.Split(out, "\n") {
		if n := strings.TrimSpace(line); n != "" {
			names = append(names, n)
		}
	}
	return names
}

func (h *DatabasesHandler) listMysqlExec(cont, user, pass string) ([]dbInfo, bool, string) {
	if user == "" {
		user = "root"
	}
	env := []string{}
	if pass != "" {
		env = append(env, "MYSQL_PWD="+pass)
	}
	out, errOut, code, err := h.execInContainer(cont, env,
		"mysql", "-u", user, "-N", "-e", "SELECT schema_name FROM information_schema.schemata ORDER BY 1")
	if err != nil {
		return nil, false, "exec failed: " + err.Error()
	}
	if code != 0 {
		msg := strings.TrimSpace(errOut + " " + out)
		if strings.Contains(msg, "Access denied") {
			return nil, true, "database username/password required"
		}
		return nil, false, "mysql: " + msg
	}
	names := parseMysqlNames(out)
	// Best-effort sizes (needs broader privileges; names alone still count).
	sizes := map[string]int64{}
	sout, _, scode, _ := h.execInContainer(cont, env,
		"mysql", "-u", user, "-N", "-e",
		"SELECT table_schema, COALESCE(SUM(data_length+index_length),0) FROM information_schema.tables GROUP BY 1")
	if scode == 0 {
		for _, line := range strings.Split(sout, "\n") {
			parts := strings.Split(strings.TrimSpace(line), "\t")
			if len(parts) != 2 {
				parts = strings.Fields(strings.TrimSpace(line))
			}
			if len(parts) == 2 {
				if v, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
					sizes[parts[0]] = v
				}
			}
		}
	}
	res := make([]dbInfo, 0, len(names))
	for _, n := range names {
		sz, ok := sizes[n]
		if !ok {
			sz = -1
		}
		res = append(res, dbInfo{Name: n, SizeBytes: sz, System: mysqlSystemDb(n)})
	}
	return res, false, ""
}

func (h *DatabasesHandler) listMysqlDirect(host string, port int, user, pass string) ([]dbInfo, bool, string) {
	mc := mysqlcfg.NewConfig()
	mc.Net = "tcp"
	mc.Addr = fmt.Sprintf("%s:%d", host, port)
	mc.User = user
	mc.Passwd = pass
	mc.DBName = "information_schema"
	mc.Timeout = 8 * time.Second
	mc.ReadTimeout = 8 * time.Second
	mc.WriteTimeout = 8 * time.Second
	mc.ParseTime = true
	db, err := sql.Open("mysql", mc.FormatDSN())
	if err != nil {
		return nil, false, "connect failed: " + err.Error()
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rows, err := db.QueryContext(ctx, `SELECT schema_name FROM information_schema.schemata ORDER BY 1`)
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "Access denied") {
			return nil, true, "invalid username or password"
		}
		return nil, false, "query failed: " + msg
	}
	defer rows.Close()
	sizes := map[string]int64{}
	if szRows, err := db.QueryContext(ctx, `SELECT table_schema, COALESCE(SUM(data_length+index_length),0) FROM information_schema.tables GROUP BY 1`); err == nil {
		defer szRows.Close()
		for szRows.Next() {
			var n string
			var v int64
			if err := szRows.Scan(&n, &v); err == nil {
				sizes[n] = v
			}
		}
	}
	res := []dbInfo{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err == nil {
			sz, ok := sizes[n]
			if !ok {
				sz = -1
			}
			res = append(res, dbInfo{Name: n, SizeBytes: sz, System: mysqlSystemDb(n)})
		}
	}
	return res, false, ""
}

// redisConn is a minimal RESP client (PING/AUTH/INFO only, no new deps).
type redisConn struct {
	c net.Conn
}

func dialRedis(host string, port int) (*redisConn, error) {
	c, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), 5*time.Second)
	if err != nil {
		return nil, err
	}
	_ = c.SetDeadline(time.Now().Add(8 * time.Second))
	return &redisConn{c: c}, nil
}

func (r *redisConn) close() { _ = r.c.Close() }

func (r *redisConn) cmd(args ...string) (string, error) {
	var sb strings.Builder
	fmt.Fprintf(&sb, "*%d\r\n", len(args))
	for _, a := range args {
		fmt.Fprintf(&sb, "$%d\r\n%s\r\n", len(a), a)
	}
	if _, err := r.c.Write([]byte(sb.String())); err != nil {
		return "", err
	}
	return r.readReply()
}

func (r *redisConn) readLine() (string, error) {
	var line strings.Builder
	b := make([]byte, 1)
	for {
		n, err := r.c.Read(b)
		if err != nil {
			return "", err
		}
		if n == 0 {
			continue
		}
		if b[0] == '\n' {
			break
		}
		if b[0] != '\r' {
			line.WriteByte(b[0])
		}
		if line.Len() > 1<<20 {
			return "", fmt.Errorf("reply too large")
		}
	}
	return line.String(), nil
}

func (r *redisConn) readReply() (string, error) {
	line, err := r.readLine()
	if err != nil || line == "" {
		return "", err
	}
	switch line[0] {
	case '+', ':', '-':
		if line[0] == '-' {
			return "", fmt.Errorf("%s", line[1:])
		}
		return line[1:], nil
	case '$':
		n, err := strconv.Atoi(line[1:])
		if err != nil || n < 0 {
			return "", err
		}
		buf := make([]byte, n+2)
		read := 0
		for read < len(buf) {
			m, err := r.c.Read(buf[read:])
			if err != nil {
				return "", err
			}
			read += m
		}
		return string(buf[:n]), nil
	}
	return "", fmt.Errorf("unexpected reply: %q", line)
}

func parseRedisKeyspace(info string) map[string]int64 {
	// Lines like: db0:keys=12,expires=0,avg_ttl=0
	out := map[string]int64{}
	for _, line := range strings.Split(info, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, rest, ok := strings.Cut(line, ":")
		if !ok || !strings.HasPrefix(name, "db") {
			continue
		}
		for _, kv := range strings.Split(rest, ",") {
			if k, v, ok := strings.Cut(kv, "="); ok && k == "keys" {
				if n, err := strconv.ParseInt(v, 10, 64); err == nil {
					out[name] = n
				}
			}
		}
	}
	return out
}

func (h *DatabasesHandler) listRedisExec(cont, pass string) ([]dbInfo, bool, string) {
	args := []string{"redis-cli"}
	if pass != "" {
		args = append(args, "-a", pass)
	}
	args = append(args, "INFO", "keyspace")
	out, errOut, code, err := h.execInContainer(cont, nil, args...)
	if err != nil {
		return nil, false, "exec failed: " + err.Error()
	}
	msg := strings.TrimSpace(errOut + " " + out)
	if code != 0 {
		if strings.Contains(msg, "NOAUTH") || strings.Contains(msg, "Authentication required") {
			return nil, true, "redis password required"
		}
		if strings.Contains(msg, "redis-cli: not found") || strings.Contains(msg, "executable file not found") {
			return nil, false, "redis-cli not available in container"
		}
		return nil, false, "redis-cli: " + msg
	}
	ks := parseRedisKeyspace(out)
	res := []dbInfo{}
	for i := 0; i < 16; i++ {
		name := fmt.Sprintf("db%d", i)
		if keys, ok := ks[name]; ok {
			res = append(res, dbInfo{Name: name, SizeBytes: keys})
		}
	}
	return res, false, ""
}

func (h *DatabasesHandler) listRedisDirect(host string, port int, pass string) ([]dbInfo, bool, string) {
	_ = h
	rc, err := dialRedis(host, port)
	if err != nil {
		return nil, false, "connect failed: " + err.Error()
	}
	defer rc.close()
	if pass != "" {
		if _, err := rc.cmd("AUTH", pass); err != nil {
			return nil, true, "invalid redis password"
		}
	}
	info, err := rc.cmd("INFO", "keyspace")
	if err != nil {
		if strings.Contains(err.Error(), "NOAUTH") {
			return nil, true, "redis password required"
		}
		return nil, false, "INFO failed: " + err.Error()
	}
	ks := parseRedisKeyspace(info)
	res := []dbInfo{}
	for i := 0; i < 16; i++ {
		name := fmt.Sprintf("db%d", i)
		if keys, ok := ks[name]; ok {
			res = append(res, dbInfo{Name: name, SizeBytes: keys})
		}
	}
	return res, false, ""
}

func (h *DatabasesHandler) listMongoExec(cont, user, pass string) ([]dbInfo, bool, string) {
	auth := []string{}
	if user != "" {
		auth = append(auth, "-u", user)
		if pass != "" {
			auth = append(auth, "-p", pass)
		}
		auth = append(auth, "--authenticationDatabase", "admin")
	}
	eval := `JSON.stringify(db.adminCommand({listDatabases: 1}))`
	for _, shell := range [][]string{
		append([]string{"mongosh", "--quiet", "--eval", eval}, auth...),
		append([]string{"mongo", "--quiet", "--eval", eval}, auth...),
	} {
		out, errOut, code, err := h.execInContainer(cont, nil, shell...)
		if err != nil {
			return nil, false, "exec failed: " + err.Error()
		}
		msg := strings.TrimSpace(errOut + " " + out)
		if code != 0 {
			if strings.Contains(msg, "not found") || strings.Contains(msg, "executable file not found") {
				continue // try the other shell
			}
			if strings.Contains(msg, "auth") || strings.Contains(msg, "Authentication failed") || strings.Contains(msg, "Unauthorized") {
				return nil, true, "mongo username/password required"
			}
			return nil, false, "mongo: " + msg
		}
		var parsed struct {
			Databases []struct {
				Name       string  `json:"name"`
				SizeOnDisk float64 `json:"sizeOnDisk"`
			} `json:"databases"`
		}
		// mongosh may wrap output; find the JSON object.
		payload := strings.TrimSpace(out)
		if i := strings.Index(payload, "{"); i >= 0 {
			payload = payload[i:]
		}
		if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
			return nil, false, "could not parse mongo output"
		}
		res := []dbInfo{}
		for _, d := range parsed.Databases {
			res = append(res, dbInfo{
				Name:      d.Name,
				SizeBytes: int64(d.SizeOnDisk),
				System:    d.Name == "admin" || d.Name == "local" || d.Name == "config",
			})
		}
		return res, false, ""
	}
	return nil, false, "no mongo shell (mongosh/mongo) in container"
}