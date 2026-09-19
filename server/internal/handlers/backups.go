package handlers

import (
	"archive/tar"
	"compress/gzip"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"serverhub/internal/audit"
	"serverhub/internal/events"
	"serverhub/internal/middleware"
	"serverhub/internal/ops"
)

// BackupsHandler snapshots project deployment directories as .tar.gz
// archives plus a metadata manifest, and restores them on demand.
// Restore overwrites live files: HIGH RISK, confirmation-gated.
type BackupsHandler struct {
	DB     *sql.DB
	Broker *events.Broker
	Dir    string
}

type backupRow struct {
	ID        int64  `json:"id"`
	ProjectID int64  `json:"projectId"`
	Kind      string `json:"kind"`
	Path      string `json:"-"`
	SizeBytes int64  `json:"sizeBytes"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
}

func (h *BackupsHandler) dir() string {
	if h.Dir != "" {
		return h.Dir
	}
	return "./data/backups"
}

func (h *BackupsHandler) emit(t string, data interface{}) {
	if h.Broker != nil {
		h.Broker.Publish(t, data)
	}
}

// GET /server-hub/api/projects/:id/backups
func (h *BackupsHandler) List(c *gin.Context) {
	pid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project id"})
		return
	}
	rows, err := h.DB.Query(`SELECT id,project_id,kind,path,size_bytes,status,created_at
		FROM backups WHERE project_id=? ORDER BY id DESC LIMIT 50`, pid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []backupRow{}
	for rows.Next() {
		var b backupRow
		if err := rows.Scan(&b.ID, &b.ProjectID, &b.Kind, &b.Path, &b.SizeBytes, &b.Status, &b.CreatedAt); err == nil {
			b.Path = filepath.Base(b.Path)
			out = append(out, b)
		}
	}
	c.JSON(http.StatusOK, out)
}

// POST /server-hub/api/projects/:id/backups — snapshot now (async op).
func (h *BackupsHandler) Create(c *gin.Context) {
	pid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project id"})
		return
	}
	var name, deployPath string
	if err := h.DB.QueryRow(`SELECT name, deployment_path FROM projects WHERE id=?`, pid).
		Scan(&name, &deployPath); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}
	u, _ := middleware.CurrentUser(c)
	op, err := ops.Create(h.DB, "backup", "project", strconv.FormatInt(pid, 10), u,
		[]string{"Snapshot files", "Write manifest"})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not track operation"})
		return
	}
	go h.runBackup(op.ID, pid, name, deployPath, u)
	c.JSON(http.StatusAccepted, gin.H{"ok": true, "operationId": op.ID, "projectId": pid})
}

func (h *BackupsHandler) runBackup(opID string, pid int64, name, deployPath, actor string) {
	_ = ops.Start(h.DB, opID)
	fail := func(msg string) {
		_, _ = h.DB.Exec(`INSERT INTO backups (project_id,kind,path,size_bytes,status,logs)
			VALUES (?, 'snapshot', '', 0, 'FAILED', ?)`, pid, msg)
		_ = ops.Finish(h.DB, opID, "FAILED", msg)
		audit.Write(h.DB, actor, "backup", "project", strconv.FormatInt(pid, 10), "failed", msg)
		h.emit("backup.failed", map[string]interface{}{
			"projectId": pid, "project": name, "operationId": opID, "error": msg,
		})
	}
	if deployPath == "" {
		fail("no deployment_path configured")
		return
	}
	if _, err := os.Stat(deployPath); err != nil {
		fail("deployment_path not found: " + deployPath)
		return
	}
	if err := os.MkdirAll(h.dir(), 0o755); err != nil {
		fail("cannot create backup dir: " + err.Error())
		return
	}
	stamp := time.Now().UTC().Format("20060102-150405")
	archive := filepath.Join(h.dir(), fmt.Sprintf("%s-%s.tar.gz", sanitize(name), stamp))

	_ = ops.SetStage(h.DB, opID, 0, false, "")
	f, err := os.Create(archive)
	if err != nil {
		fail("cannot create archive: " + err.Error())
		return
	}
	size, werr := writeTarGz(f, deployPath)
	cerr := f.Close()
	if werr != nil || cerr != nil {
		_ = os.Remove(archive)
		msg := "snapshot failed"
		if werr != nil {
			msg += ": " + werr.Error()
		}
		fail(msg)
		return
	}

	_ = ops.SetStage(h.DB, opID, 1, false, "")
	meta, _ := json.Marshal(h.manifest(pid, name, deployPath))
	_ = meta // stored inside logs for operator visibility
	logs := fmt.Sprintf("snapshot %s (%d bytes)\nmanifest: %s", filepath.Base(archive), size, string(meta))
	res, err := h.DB.Exec(`INSERT INTO backups (project_id,kind,path,size_bytes,status,logs)
		VALUES (?, 'snapshot', ?, ?, 'SUCCESS', ?)`, pid, archive, size, logs)
	if err != nil {
		_ = os.Remove(archive)
		fail("cannot record backup: " + err.Error())
		return
	}
	bid, _ := res.LastInsertId()
	_ = ops.Finish(h.DB, opID, "SUCCESS", "")
	audit.Write(h.DB, actor, "backup", "backup", strconv.FormatInt(bid, 10), "ok",
		fmt.Sprintf("project=%s size=%d", name, size))
	h.emit("backup.created", map[string]interface{}{
		"projectId": pid, "project": name, "backupId": bid,
		"sizeBytes": size, "operationId": opID,
	})
}

func (h *BackupsHandler) manifest(pid int64, name, deployPath string) map[string]interface{} {
	var svcCount, depCount, secretCount int
	_ = h.DB.QueryRow(`SELECT COUNT(*) FROM services WHERE project_id=?`, pid).Scan(&svcCount)
	_ = h.DB.QueryRow(`SELECT COUNT(*) FROM deployments WHERE project_id=?`, pid).Scan(&depCount)
	_ = h.DB.QueryRow(`SELECT COUNT(*) FROM secrets WHERE project_id=?`, pid).Scan(&secretCount)
	return map[string]interface{}{
		"serverhub": "backup-v1", "project": name, "projectId": pid,
		"deploymentPath": deployPath, "services": svcCount,
		"deployments": depCount, "secrets": secretCount,
		"createdAt": time.Now().UTC().Format(time.RFC3339),
	}
}

// GET /server-hub/api/backups/:id — one backup's metadata.
func (h *BackupsHandler) Get(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var b backupRow
	if err := h.DB.QueryRow(`SELECT id,project_id,kind,path,size_bytes,status,created_at
		FROM backups WHERE id=?`, id).
		Scan(&b.ID, &b.ProjectID, &b.Kind, &b.Path, &b.SizeBytes, &b.Status, &b.CreatedAt); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "backup not found"})
		return
	}
	b.Path = filepath.Base(b.Path)
	c.JSON(http.StatusOK, b)
}

// DELETE /server-hub/api/backups/:id
func (h *BackupsHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var path string
	var pid int64
	if err := h.DB.QueryRow(`SELECT project_id, path FROM backups WHERE id=?`, id).
		Scan(&pid, &path); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "backup not found"})
		return
	}
	_ = os.Remove(path)
	_, _ = h.DB.Exec(`DELETE FROM backups WHERE id=?`, id)
	u, _ := middleware.CurrentUser(c)
	audit.Write(h.DB, u, "delete", "backup", strconv.FormatInt(id, 10), "ok", "")
	h.emit("backup.deleted", map[string]interface{}{"projectId": pid, "backupId": id})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// POST /server-hub/api/backups/:id/restore?confirm=true — HIGH RISK.
func (h *BackupsHandler) Restore(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	u, _ := middleware.CurrentUser(c)
	if !confirmed(c) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "restoring overwrites live files: retry with ?confirm=true or body {\"confirm\": true}"})
		return
	}
	var path string
	var pid int64
	var name, deployPath string
	err = h.DB.QueryRow(`SELECT b.project_id, b.path, p.name, p.deployment_path
		FROM backups b JOIN projects p ON p.id = b.project_id WHERE b.id=?`, id).
		Scan(&pid, &path, &name, &deployPath)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "backup not found"})
		return
	}
	op, err := ops.Create(h.DB, "restore", "backup", strconv.FormatInt(id, 10), u,
		[]string{"Extract archive"})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not track operation"})
		return
	}
	go func() {
		_ = ops.Start(h.DB, op.ID)
		_ = ops.SetStage(h.DB, op.ID, 0, false, "")
		rerr := extractTarGz(path, deployPath)
		if rerr != nil {
			_ = ops.SetStage(h.DB, op.ID, 0, true, "")
			_ = ops.Finish(h.DB, op.ID, "FAILED", rerr.Error())
			audit.Write(h.DB, u, "restore", "backup", strconv.FormatInt(id, 10), "failed", rerr.Error())
			h.emit("backup.restoreFailed", map[string]interface{}{
				"projectId": pid, "backupId": id, "operationId": op.ID, "error": rerr.Error(),
			})
			return
		}
		_ = ops.Finish(h.DB, op.ID, "SUCCESS", "")
		audit.Write(h.DB, u, "restore", "backup", strconv.FormatInt(id, 10), "ok",
			fmt.Sprintf("project=%s", name))
		h.emit("backup.restored", map[string]interface{}{
			"projectId": pid, "project": name, "backupId": id, "operationId": op.ID,
		})
	}()
	c.JSON(http.StatusAccepted, gin.H{"ok": true, "operationId": op.ID, "backupId": id})
}

func sanitize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "project"
	}
	return out
}

func writeTarGz(w io.Writer, srcDir string) (int64, error) {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	var total int64
	err := filepath.WalkDir(srcDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if path == srcDir {
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if !info.Mode().IsRegular() && !info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
			return nil // skip sockets, pipes, devices
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return nil
			}
			hdr.Linkname = target
		}
		hdr.Name = filepath.ToSlash(rel)
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			f, err := os.Open(path)
			if err != nil {
				return nil
			}
			n, err := io.Copy(tw, f)
			f.Close()
			if err != nil {
				return err
			}
			total += n
		}
		return nil
	})
	if err != nil {
		tw.Close()
		gz.Close()
		return total, err
	}
	if err := tw.Close(); err != nil {
		gz.Close()
		return total, err
	}
	return total, gz.Close()
}

func extractTarGz(archive, destDir string) error {
	if destDir == "" {
		return fmt.Errorf("no deployment_path configured")
	}
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		// ZipSlip protection: reject absolute paths and .. escapes.
		clean := filepath.Clean(hdr.Name)
		if filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") {
			continue
		}
		target := filepath.Join(destDir, clean)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		case tar.TypeSymlink, tar.TypeLink:
			_ = os.Remove(target)
			linkTarget := filepath.Clean(hdr.Linkname)
			if filepath.IsAbs(linkTarget) || strings.HasPrefix(linkTarget, "..") {
				continue
			}
			_ = os.Symlink(hdr.Linkname, target)
		}
	}
	return nil
}
