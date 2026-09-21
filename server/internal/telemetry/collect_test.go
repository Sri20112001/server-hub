package telemetry

import (
	"testing"

	"serverhub/internal/testdb"
)

func TestSampleWritesRow(t *testing.T) {
	db := testdb.Open(t)
	var tbl string
	if err := db.QueryRow(`SELECT tablename FROM pg_tables WHERE schemaname='public' AND tablename='server_snapshots'`).Scan(&tbl); err != nil {
		t.Fatalf("table missing: %v", err)
	}
	sample(db)
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM server_snapshots`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 row, got %d", n)
	}
}
