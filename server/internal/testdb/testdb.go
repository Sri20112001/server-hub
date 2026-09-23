// Package testdb opens an isolated Postgres database per test.
//
// The server is Postgres-only, so tests need a live Postgres. The connection
// is taken from TEST_DATABASE_URL, falling back to DATABASE_URL, then to a
// local default. Each call creates a fresh serverhub_test_* database (migrated
// via database.OpenDatabase, triggers included) and drops it on cleanup, so
// tests never touch real data and stay isolated from each other.
//
// When no Postgres is reachable, tests skip instead of failing — set
// TEST_DATABASE_URL to run the DB-backed suite (CI provides it).
package testdb

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/url"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"

	"serverhub/internal/database"
)

func baseDSN() string {
	for _, key := range []string{"TEST_DATABASE_URL", "DATABASE_URL"} {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	return "postgres://serverhub:changeme@localhost:5432/postgres?sslmode=disable"
}

// dsnWithDatabase returns base but pointed at the given database name.
// NOTE: pgx's Config.ConnString() returns the *original* connection string
// and ignores later field mutations (it caches connString), so we must
// rebuild the DSN ourselves — mutating cfg.Database then calling
// ConnString() silently keeps the old dbname and all tests end up sharing
// the maintenance database.
func dsnWithDatabase(base, dbName string) string {
	u, err := url.Parse(base)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return base
	}
	u.Path = "/" + dbName
	return u.String()
}

// Open creates a fresh isolated test database and returns an open handle.
// The database is dropped when the test finishes.
func Open(t *testing.T) *database.DB {
	t.Helper()

	base := baseDSN()
	if _, err := url.Parse(base); err != nil {
		t.Fatalf("parse postgres DSN: %v", err)
	}
	maintenanceDSN := dsnWithDatabase(base, "postgres") // for CREATE/DROP DATABASE

	admin, err := sql.Open("pgx", maintenanceDSN)
	if err != nil {
		t.Skipf("postgres driver unavailable: %v", err)
	}
	defer admin.Close()
	if err := admin.Ping(); err != nil {
		t.Skipf("postgres not reachable (%v); set TEST_DATABASE_URL to run DB tests", err)
	}

	name := "serverhub_test_" + randHex(4)
	if _, err := admin.Exec(`CREATE DATABASE "` + name + `"`); err != nil {
		t.Fatalf("create test database: %v", err)
	}

	db, err := database.OpenDatabase(dsnWithDatabase(base, name))
	if err != nil {
		_, _ = admin.Exec(`DROP DATABASE "` + name + `"`)
		t.Fatalf("open test database: %v", err)
	}

	// Sanity check: fail fast if isolation broke and we landed back on a
	// shared database instead of the fresh one.
	var got string
	if err := db.QueryRow(`SELECT current_database()`).Scan(&got); err != nil || got != name {
		_, _ = admin.Exec(`DROP DATABASE "` + name + `"`)
		t.Fatalf("test isolation broken: connected to %q, want %q (err=%v)", got, name, err)
	}

	t.Cleanup(func() {
		_ = db.Close()
		drop, err := sql.Open("pgx", maintenanceDSN)
		if err != nil {
			return
		}
		defer drop.Close()
		_, _ = drop.Exec(`DROP DATABASE "` + name + `"`)
	})
	return db
}

func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
