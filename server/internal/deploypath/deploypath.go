// Package deploypath constrains operator-controlled deployment paths.
//
// deployment_path + compose_file flow from project rows (operator input)
// into `docker compose` executions run by a Docker-socket-enabled backend
// (≈ host root). Without containment that is a host-level execution
// primitive, so every use resolves the path to a canonical absolute form
// and enforces containment within explicitly configured roots:
//
//   - ValidateWrite checks a path at project save time (absolute, inside
//     roots). Symlinks are NOT resolved here — the target may not exist yet.
//   - ResolveExec re-checks at execution time AND resolves symlinks, so a
//     symlink planted after save cannot escape the roots.
//
// An empty roots list means "unconfigured": only the absolute-path rule
// applies (legacy mode, e.g. unit tests without config). Production always
// configures roots (DEPLOY_ROOTS defaults to SCAN_ROOTS).
package deploypath

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidateComposeFile checks that name is a safe bare filename: no path
// separators, no absolute path, no leading dash (flag injection into
// `docker compose -f <name>` style invocations).
func ValidateComposeFile(name string) error {
	if name == "" {
		return fmt.Errorf("compose_file is required")
	}
	if filepath.Base(name) != name || strings.HasPrefix(name, "-") || filepath.IsAbs(name) {
		return fmt.Errorf("compose_file must be a plain filename with no path separators or leading dashes")
	}
	return nil
}

// ValidateWrite checks path for storing on a project row. Empty means
// "unconfigured" and is allowed (callers treat it as no deployment path).
func ValidateWrite(path string, roots []string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", nil
	}
	clean := filepath.Clean(path)
	if !filepath.IsAbs(clean) {
		return "", fmt.Errorf("deployment_path must be an absolute path")
	}
	if len(roots) > 0 && !withinRoots(clean, roots) {
		return "", fmt.Errorf("deployment_path must be inside an allowed deployment root")
	}
	return clean, nil
}

// ResolveExec fully resolves path for execution: absolute + contained,
// then symlinks resolved and containment re-checked on the result.
func ResolveExec(path string, roots []string) (string, error) {
	clean, err := ValidateWrite(path, roots)
	if err != nil {
		return "", err
	}
	if clean == "" {
		return "", fmt.Errorf("no deployment_path configured")
	}
	resolved, err := filepath.EvalSymlinks(clean)
	if err != nil {
		// Nonexistent path: keep the cleaned form so the caller's
		// existence check reports "not found" rather than a symlink error.
		if os.IsNotExist(err) {
			return clean, nil
		}
		return "", fmt.Errorf("cannot resolve deployment_path: %v", err)
	}
	resolved = filepath.Clean(resolved)
	if len(roots) > 0 && !withinRoots(resolved, roots) {
		return "", fmt.Errorf("deployment_path escapes the allowed deployment roots (symlink)")
	}
	return resolved, nil
}

// withinRoots reports whether clean (already filepath.Clean) equals a root
// or lies beneath it. String-prefix matching on uncleaned input would allow
// "/srv/apps-evil"; Clean + separator-boundary matching does not.
func withinRoots(clean string, roots []string) bool {
	for _, r := range roots {
		rc := filepath.Clean(r)
		if rc == "" || !filepath.IsAbs(rc) {
			continue
		}
		if clean == rc || strings.HasPrefix(clean, rc+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
