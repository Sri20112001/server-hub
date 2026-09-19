package scanfs

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScanDetectsStacks(t *testing.T) {
	root := t.TempDir()
	node := filepath.Join(root, "shop")
	goapp := filepath.Join(root, "api")
	empty := filepath.Join(root, "notes")
	for _, d := range []string{node, goapp, empty} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write(t, node, "package.json", `{"name":"shop","dependencies":{"react":"18","express":"4"}}`)
	write(t, node, "vite.config.ts", `export default {}`)
	write(t, node, "docker-compose.yml", "services:\n  web:\n    image: x\n")
	write(t, goapp, "go.mod", "module example.com/api\n\ngo 1.22\n")
	write(t, empty, "todo.txt", "buy milk")

	found := Scan([]string{root})
	if len(found) != 2 {
		t.Fatalf("expected 2 projects, got %d: %v", len(found), found)
	}
	byName := map[string]FoundProject{}
	for _, f := range found {
		byName[f.Name] = f
	}
	shop := byName["shop"]
	for _, want := range []string{"Node", "React", "Express", "Vite", "Compose"} {
		ok := false
		for _, s := range shop.Stack {
			if s == want {
				ok = true
			}
		}
		if !ok {
			t.Errorf("shop stack missing %q: %v", want, shop.Stack)
		}
	}
	if shop.ComposeFile != "docker-compose.yml" {
		t.Errorf("compose file = %q", shop.ComposeFile)
	}
	if byName["api"].Stack[0] != "Go" {
		t.Errorf("api stack = %v", byName["api"].Stack)
	}
}

func TestParseComposeServices(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "docker-compose.yml")
	content := `version: "3"
services:
  backend:
    image: api:1
    # a comment
    ports:
      - "4000:4000"
  frontend:
    build: ./web
  db:
    image: postgres:15
networks:
  default:
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got := ParseComposeServices(path)
	want := []string{"backend", "frontend", "db"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
