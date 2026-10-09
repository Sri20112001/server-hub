package deploypath

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// MaxComposeBytes caps the compose file read for reference scanning.
// Compose files are kilobytes; anything larger is refused, not scanned.
const MaxComposeBytes = 4 << 20

// OutsideRef describes one compose-file reference resolving outside the
// allowed deployment roots.
type OutsideRef struct {
	// Where, e.g. `services.web.volumes[0]`, `env_file`, `include[1]`.
	Field string
	// The referenced path as written.
	Value string
}

// CheckComposeRefs scans a compose file for indirect host references that
// resolve outside roots: top-level `include`, service `env_file`,
// `extends.file`, bind-mount `volumes` sources and `build.context` dirs.
//
// It returns the offending references for WARNING (not failure): bind
// mounts like /var/run/docker.sock are legitimate in real stacks, so
// failing closed here would break the product. Callers log + audit the
// result so unexpected escapes are visible. Containment of the compose file
// itself (ResolveExec) remains the enforced boundary.
func CheckComposeRefs(composePath string, roots []string) ([]OutsideRef, error) {
	if len(roots) == 0 {
		return nil, nil // unconfigured: nothing to compare against
	}
	st, err := os.Stat(composePath)
	if err != nil {
		return nil, err
	}
	if st.Size() > MaxComposeBytes {
		return nil, fmt.Errorf("compose file too large to scan (%d bytes)", st.Size())
	}
	raw, err := os.ReadFile(composePath)
	if err != nil {
		return nil, err
	}
	var doc map[string]interface{}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		// Unparseable: compose itself will fail with a proper error;
		// scanning must not invent a second failure mode.
		return nil, nil
	}
	base := filepath.Dir(composePath)
	var out []OutsideRef
	add := func(field, value string) {
		if value == "" || !looksLikeHostPath(value) {
			return
		}
		if resolvedOutsideBase(value, base, roots) {
			out = append(out, OutsideRef{Field: field, Value: value})
		}
	}
	// Top-level include: strings or {path: ...} objects.
	for i, inc := range toList(doc["include"]) {
		add(fmt.Sprintf("include[%d]", i), includePath(inc))
	}
	// build.context is per-service; services map first.
	if svcs, ok := doc["services"].(map[string]interface{}); ok {
		names := make([]string, 0, len(svcs))
		for n := range svcs {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, name := range names {
			svc, ok := svcs[name].(map[string]interface{})
			if !ok {
				continue
			}
			prefix := "services." + name
			for i, e := range toList(svc["env_file"]) {
				add(fmt.Sprintf("%s.env_file[%d]", prefix, i), envFilePath(e))
			}
			if ext, ok := svc["extends"].(map[string]interface{}); ok {
				if f, ok := ext["file"].(string); ok {
					add(prefix+".extends.file", f)
				}
			}
			if b, ok := svc["build"].(map[string]interface{}); ok {
				if ctx, ok := b["context"].(string); ok && ctx != "" && ctx != "." {
					add(prefix+".build.context", ctx)
				}
			}
			for i, v := range toList(svc["volumes"]) {
				add(fmt.Sprintf("%s.volumes[%d]", prefix, i), volumeHostPath(v))
			}
		}
	}
	return out, nil
}

// looksLikeHostPath filters out named volumes and values that cannot be
// host paths (URLs, variables, empty).
func looksLikeHostPath(v string) bool {
	v = strings.TrimSpace(v)
	if v == "" || strings.HasPrefix(v, "$") {
		return false
	}
	if strings.Contains(v, "://") {
		return false
	}
	// Absolute path, ./ or ../ relative, or home-relative.
	return strings.HasPrefix(v, "/") || strings.HasPrefix(v, ".") ||
		strings.HasPrefix(v, "~") || strings.Contains(v, "/")
}

// resolvedOutsideBase resolves value against base and reports whether the
// result (symlinks resolved best-effort) escapes roots.
func resolvedOutsideBase(value, base string, roots []string) bool {
	p := value
	if strings.HasPrefix(p, "~") {
		// Home-relative: cannot resolve reliably — flag for review.
		return true
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(base, p)
	}
	clean := filepath.Clean(p)
	if withinRoots(clean, roots) {
		// Resolved symlinks may still escape (planted after save).
		if res, err := filepath.EvalSymlinks(clean); err == nil {
			return !withinRoots(filepath.Clean(res), roots)
		}
		return false
	}
	return true
}

func toList(v interface{}) []interface{} {
	switch t := v.(type) {
	case nil:
		return nil
	case string:
		return []interface{}{t}
	case []interface{}:
		return t
	default:
		return nil
	}
}

func strField(m map[string]interface{}, key string) string {
	if s, ok := m[key].(string); ok {
		return s
	}
	return ""
}

func includePath(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	if m, ok := v.(map[string]interface{}); ok {
		return strField(m, "path")
	}
	return ""
}

func envFilePath(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	if m, ok := v.(map[string]interface{}); ok {
		return strField(m, "path")
	}
	return ""
}

// volumeHostPath extracts the host side of a volume mount: short syntax
// "host:container[:opts]" or long syntax {type, source}.
func volumeHostPath(v interface{}) string {
	if s, ok := v.(string); ok {
		// Anonymous/named volumes have no host part to check.
		if i := strings.Index(s, ":"); i >= 0 {
			return s[:i]
		}
		return ""
	}
	if m, ok := v.(map[string]interface{}); ok {
		typ, _ := m["type"].(string)
		src, _ := m["source"].(string)
		if typ == "bind" || (typ == "" && looksLikeHostPath(src)) {
			return src
		}
	}
	return ""
}
