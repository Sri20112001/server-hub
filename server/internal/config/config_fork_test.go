package config

// Subprocess tests for production fail-fast startup behavior. Load() calls
// os.Exit on misconfiguration, so each scenario runs in a child test
// process (classic TestHelperProcess pattern) and the parent asserts the
// exit code.

import (
	"os"
	"os/exec"
	"testing"
)

const validTestKey = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	Load()
	os.Exit(0) // reached only when Load did not fail fast
}

var forkControlledKeys = []string{
	"SERVERHUB_ENV", "APP_ENV", "GO_ENV",
	"SERVERHUB_ENCRYPTION_KEY", "JWT_SECRET", "ADMIN_PASSWORD",
	"COOKIE_SECURE", "DATABASE_URL", "PORT",
}

func runLoadChild(t *testing.T, extra map[string]string) int {
	t.Helper()
	env := []string{}
outer:
	for _, kv := range os.Environ() {
		for _, k := range forkControlledKeys {
			if len(kv) > len(k) && kv[:len(k)] == k && kv[len(k)] == '=' {
				continue outer
			}
		}
		env = append(env, kv)
	}
	for k, v := range extra {
		env = append(env, k+"="+v)
	}
	env = append(env, "GO_WANT_HELPER_PROCESS=1")
	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcess")
	cmd.Env = env
	if err := cmd.Run(); err == nil {
		return 0
	} else if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	} else {
		t.Fatalf("child: %v", err)
		return -1
	}
}

func baseValidEnv() map[string]string {
	return map[string]string{
		"JWT_SECRET":    "test-secret-that-is-long-enough-32",
		"ADMIN_PASSWORD": "test-password",
	}
}

func TestProductionFailFast(t *testing.T) {
	with := func(over map[string]string) map[string]string {
		m := baseValidEnv()
		for k, v := range over {
			m[k] = v
		}
		return m
	}
	cases := []struct {
		name string
		env  map[string]string
		want int // 0 = starts, non-zero = refuses
	}{
		{"prod missing key refuses", with(map[string]string{
			"SERVERHUB_ENV": "production", "COOKIE_SECURE": "true",
		}), 1},
		{"prod malformed key refuses", with(map[string]string{
			"SERVERHUB_ENV": "production", "COOKIE_SECURE": "true",
			"SERVERHUB_ENCRYPTION_KEY": "too-short",
		}), 1},
		{"prod insecure cookie refuses", with(map[string]string{
			"SERVERHUB_ENV": "production", "COOKIE_SECURE": "false",
			"SERVERHUB_ENCRYPTION_KEY": validTestKey,
		}), 1},
		{"prod valid starts", with(map[string]string{
			"SERVERHUB_ENV": "production", "COOKIE_SECURE": "true",
			"SERVERHUB_ENCRYPTION_KEY": validTestKey,
		}), 0},
		{"dev missing key starts with warning", with(map[string]string{
			"SERVERHUB_ENCRYPTION_KEY": "",
		}), 0},
		{"app_env production fallback refuses", with(map[string]string{
			"APP_ENV": "production", "COOKIE_SECURE": "true",
		}), 1},
		{"serverhub_env wins over app_env", with(map[string]string{
			"SERVERHUB_ENV": "development", "APP_ENV": "production",
		}), 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if code := runLoadChild(t, tc.env); (code == 0) != (tc.want == 0) {
				t.Fatalf("exit = %d, want %d", code, tc.want)
			}
		})
	}
}

func TestResolveEnvPrecedence(t *testing.T) {
	cases := []struct {
		serverhub, app, goenv string
		wantEnv               string
		wantProd              bool
	}{
		{"", "", "", "development", false},
		{"production", "", "", "production", true},
		{"", "production", "", "production", true},
		{"", "", "production", "production", true},
		{"development", "production", "production", "development", false},
		{"PROD", "", "", "prod", true},
	}
	for _, tc := range cases {
		t.Setenv("SERVERHUB_ENV", tc.serverhub)
		t.Setenv("APP_ENV", tc.app)
		t.Setenv("GO_ENV", tc.goenv)
		env, prod := resolveEnv()
		if env != tc.wantEnv || prod != tc.wantProd {
			t.Fatalf("env=(%q,%q,%q): got (%q,%v), want (%q,%v)",
				tc.serverhub, tc.app, tc.goenv, env, prod, tc.wantEnv, tc.wantProd)
		}
	}
}
