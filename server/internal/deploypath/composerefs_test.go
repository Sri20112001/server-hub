package deploypath

import (
	"os"
	"path/filepath"
	"testing"
)

func writeCompose(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "docker-compose.yml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCheckComposeRefsClean(t *testing.T) {
	root := t.TempDir()
	p := writeCompose(t, `
services:
  web:
    image: nginx
    env_file: ./app.env
    volumes:
      - appdata:/data
      - ./local:/srv
volumes:
  appdata:
`)
	// resolve against the compose file's own dir: use it as a root.
	dir := filepath.Dir(p)
	refs, err := CheckComposeRefs(p, []string{dir, root})
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 0 {
		t.Fatalf("clean compose flagged: %+v", refs)
	}
}

func TestCheckComposeRefsFlagsEscapes(t *testing.T) {
	p := writeCompose(t, `
include:
  - ../shared/common.yml
services:
  web:
    image: nginx
    env_file:
      - /etc/app.env
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - type: bind
        source: ../../escape
        target: /x
  worker:
    extends:
      file: /opt/other/compose.yml
      service: base
`)
	refs, err := CheckComposeRefs(p, []string{"/srv/apps"})
	if err != nil {
		t.Fatal(err)
	}
	fields := map[string]bool{}
	for _, r := range refs {
		fields[r.Field] = true
	}
	for _, want := range []string{
		"include[0]",
		"services.web.env_file[0]",
		"services.web.volumes[0]",
		"services.web.volumes[1]",
		"services.worker.extends.file",
	} {
		if !fields[want] {
			t.Fatalf("missing flag for %s (got %+v)", want, refs)
		}
	}
}

func TestCheckComposeRefsUnparseable(t *testing.T) {
	p := writeCompose(t, "{{{not yaml")
	if _, err := CheckComposeRefs(p, []string{"/srv/apps"}); err != nil {
		t.Fatalf("unparseable compose must not error the scan: %v", err)
	}
}
