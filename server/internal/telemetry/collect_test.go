package telemetry

import (
	"testing"

	"serverhub/internal/database"
)

func TestSampleWritesRow(t *testing.T) {
	db, err := database.OpenDatabase("", t.TempDir()+"/tel.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var tbl string
	if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE name='server_snapshots'`).Scan(&tbl); err != nil {
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
