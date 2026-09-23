package database_test

import (
	"testing"

	"serverhub/internal/database"
	"serverhub/internal/testdb"
)

func TestOpenFreshDBCreatesAllTables(t *testing.T) {
	db := testdb.Open(t)
	for _, tbl := range []string{"users", "projects", "services", "deployments",
		"secrets", "gateway_routes", "audit_logs", "operations", "backups",
		"server_snapshots", "app_logs", "db_servers", "db_registrations",
		"app_settings"} {
		var name string
		if err := db.QueryRow(`SELECT tablename FROM pg_tables WHERE schemaname='public' AND tablename=?`, tbl).Scan(&name); err != nil {
			t.Fatalf("table %s missing: %v", tbl, err)
		}
	}
}

func TestOpenRequiresDatabaseURL(t *testing.T) {
	if _, err := database.OpenDatabase(""); err == nil {
		t.Fatal("expected error for empty DATABASE_URL")
	}
}
