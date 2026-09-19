// Package scanfs detects deployable projects by directory signatures
// (package.json, go.mod, Dockerfile, docker-compose.* …) under configured
// server roots like /srv/apps. No code is executed — files are only listed
// and small manifests are parsed.
package scanfs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type FoundProject struct {
	Name         string   `json:"name"`
	Path         string   `json:"path"`
	Stack        []string `json:"stack"`
	ComposeFile  string   `json:"composeFile"`
	Registered   bool     `json:"registered"`
}

const maxDirs = 200

// Scan returns detected projects one level below each root.
func Scan(roots []string) []FoundProject {
	out := []FoundProject{}
	seen := map[string]bool{}
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			dir := filepath.Join(root, e.Name())
			if seen[dir] {
				continue
			}
			seen[dir] = true
			if fp, ok := inspect(dir); ok {
				out = append(out, fp)
			}
			if len(out) >= maxDirs {
				return out
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func inspect(dir string) (FoundProject, bool) {
	fp := FoundProject{Name: filepath.Base(dir), Path: dir, Stack: []string{}}
	files, err := os.ReadDir(dir)
	if err != nil {
		return fp, false
	}
	has := map[string]bool{}
	for _, f := range files {
		if !f.IsDir() {
			has[f.Name()] = true
		}
	}
	add := func(s string) {
		for _, x := range fp.Stack {
			if x == s {
				return
			}
		}
		fp.Stack = append(fp.Stack, s)
	}

	if has["docker-compose.yml"] {
		fp.ComposeFile = "docker-compose.yml"
		add("Compose")
	} else if has["docker-compose.yaml"] {
		fp.ComposeFile = "docker-compose.yaml"
		add("Compose")
	} else if has["compose.yml"] {
		fp.ComposeFile = "compose.yml"
		add("Compose")
	} else if has["compose.yaml"] {
		fp.ComposeFile = "compose.yaml"
		add("Compose")
	}
	if has["Dockerfile"] || has["dockerfile"] {
		add("Docker")
	}
	if has["package.json"] {
		add("Node")
		inspectPackageJSON(filepath.Join(dir, "package.json"), add)
	}
	if has["vite.config.js"] || has["vite.config.ts"] || has["vite.config.mjs"] {
		add("Vite")
	}
	if has["next.config.js"] || has["next.config.mjs"] || has["next.config.ts"] {
		add("Next.js")
	}
	if has["go.mod"] {
		add("Go")
	}
	if has["requirements.txt"] || has["pyproject.toml"] || has["Pipfile"] {
		add("Python")
	}
	if has["Cargo.toml"] {
		add("Rust")
	}
	if has["pom.xml"] || has["build.gradle"] || has["build.gradle.kts"] {
		add("Java")
	}
	if len(fp.Stack) == 0 {
		return fp, false
	}
	return fp, true
}

func inspectPackageJSON(path string, add func(string)) {
	b, err := os.ReadFile(path)
	if err != nil || len(b) > 256*1024 {
		return
	}
	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(b, &pkg); err != nil {
		return
	}
	deps := map[string]bool{}
	for k := range pkg.Dependencies {
		deps[strings.ToLower(k)] = true
	}
	for k := range pkg.DevDependencies {
		deps[strings.ToLower(k)] = true
	}
	if deps["react"] || deps["react-dom"] {
		add("React")
	}
	if deps["vue"] {
		add("Vue")
	}
	if deps["next"] {
		add("Next.js")
	}
	if deps["express"] || deps["fastify"] || deps["koa"] || deps["hapi"] {
		add("Express")
	}
	if deps["typescript"] {
		add("TypeScript")
	}
}

// ParseComposeServices extracts top-level service names under `services:`
// using indentation only (no YAML dependency, tolerant of anchors).
func ParseComposeServices(composePath string) []string {
	b, err := os.ReadFile(composePath)
	if err != nil || len(b) > 512*1024 {
		return nil
	}
	var out []string
	inServices := false
	for _, line := range strings.Split(string(b), "\n") {
		trimmed := strings.TrimRight(line, " \t\r")
		if trimmed == "" || strings.HasPrefix(strings.TrimSpace(trimmed), "#") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		stripped := strings.TrimSpace(trimmed)
		if indent == 0 {
			inServices = stripped == "services:"
			continue
		}
		if !inServices {
			continue
		}
		if indent == 2 && strings.HasSuffix(stripped, ":") {
			name := strings.TrimSuffix(stripped, ":")
			name = strings.Trim(name, `"'`)
			if name != "" && !strings.Contains(name, " ") {
				out = append(out, name)
			}
		}
		if indent < 2 {
			break
		}
	}
	return out
}
