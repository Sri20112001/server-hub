package database

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// DB is the Postgres handle used across ServerHub. GORM is the ORM
// (schema via CreateTable, app_logs writes, aggregator queries) while
// Exec/Query/QueryRow keep the existing raw-SQL call sites working with
// ? placeholders (rebound to $n for Postgres).
type DB struct {
	GDB *gorm.DB
	SQL *sql.DB
}

// OpenDatabase opens the Postgres store at databaseURL (required) and
// ensures schema plus append-only log triggers. There is no embedded
// fallback: DATABASE_URL must be set.
func OpenDatabase(databaseURL string) (*DB, error) {
	if databaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required (postgres-only; set e.g. postgres://serverhub:changeme@localhost:5432/serverhub?sslmode=disable)")
	}
	gdb, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	// Create missing tables only — never rebuild existing ones.
	for _, m := range []any{
		&User{},
		&Project{},
		&Service{},
		&Deployment{},
		&Secret{},
		&GatewayRoute{},
		&AuditLog{},
		&Operation{},
		&Backup{},
		&ServerSnapshot{},
		&AppLog{},
		&DbServer{},
		&DbRegistration{},
	} {
		if gdb.Migrator().HasTable(m) {
			continue
		}
		if err := gdb.Migrator().CreateTable(m); err != nil {
			return nil, fmt.Errorf("create table: %w", err)
		}
	}
	sqldb, err := gdb.DB()
	if err != nil {
		return nil, err
	}
	db := &DB{GDB: gdb, SQL: sqldb}
	// Log tables are append-only: once written, rows can never be updated
	// or deleted. Enforced at the DB layer so no API, prune job, or raw SQL
	// path can tamper with history.
	if err := db.ensureLogImmutability(); err != nil {
		return nil, err
	}
	return db, nil
}

// ensureLogImmutability installs append-only guards on every table that
// carries log/history data:
//
//	app_logs, audit_logs      -> no UPDATE, no DELETE (fully immutable)
//	deployments, backups,
//	operations, server_snapshots -> no DELETE (history cannot be wiped;
//	UPDATE still allowed so running pipelines can progress
//	RUNNING -> SUCCESS/FAILED and record their output)
func (d *DB) ensureLogImmutability() error {
	// Single helper: any guarded mutation raises instead of applying.
	if _, err := d.SQL.Exec(`CREATE OR REPLACE FUNCTION serverhub_reject_log_mutation()
		RETURNS trigger AS $$
		BEGIN
			RAISE EXCEPTION 'Table % is append-only and cannot be modified (operation: %)', TG_TABLE_NAME, TG_OP;
			RETURN NULL;
		END;
		$$ LANGUAGE plpgsql`); err != nil {
		return fmt.Errorf("create reject function: %w", err)
	}
	exec := func(q string) error {
		if _, err := d.SQL.Exec(q); err != nil {
			return err
		}
		return nil
	}
	// Fully immutable log tables: block UPDATE and DELETE.
	for _, tbl := range []string{"app_logs", "audit_logs"} {
		trg := tbl + "_no_update_delete"
		if err := exec(`DROP TRIGGER IF EXISTS ` + trg + ` ON ` + tbl); err != nil {
			return err
		}
		if err := exec(`CREATE TRIGGER ` + trg +
			` BEFORE UPDATE OR DELETE ON ` + tbl +
			` FOR EACH ROW EXECUTE FUNCTION serverhub_reject_log_mutation()`); err != nil {
			return fmt.Errorf("create trigger %s: %w", trg, err)
		}
	}
	// Delete-protected history tables.
	for _, tbl := range []string{"deployments", "backups", "operations", "server_snapshots"} {
		trg := tbl + "_no_delete"
		if err := exec(`DROP TRIGGER IF EXISTS ` + trg + ` ON ` + tbl); err != nil {
			return err
		}
		if err := exec(`CREATE TRIGGER ` + trg +
			` BEFORE DELETE ON ` + tbl +
			` FOR EACH ROW EXECUTE FUNCTION serverhub_reject_log_mutation()`); err != nil {
			return fmt.Errorf("create trigger %s: %w", trg, err)
		}
	}
	return nil
}

// Rebind converts ? placeholders to $n for Postgres.
func (d *DB) Rebind(q string) string {
	if d == nil {
		return q
	}
	var sb strings.Builder
	n := 0
	for i := 0; i < len(q); i++ {
		if q[i] == '?' {
			n++
			sb.WriteString("$")
			sb.WriteString(strconv.Itoa(n))
		} else {
			sb.WriteByte(q[i])
		}
	}
	return sb.String()
}

func (d *DB) Exec(query string, args ...any) (sql.Result, error) {
	return d.SQL.Exec(d.Rebind(query), args...)
}

func (d *DB) Query(query string, args ...any) (*sql.Rows, error) {
	return d.SQL.Query(d.Rebind(query), args...)
}

func (d *DB) QueryRow(query string, args ...any) *sql.Row {
	return d.SQL.QueryRow(d.Rebind(query), args...)
}

// InsertID runs an INSERT and returns the new row id via RETURNING id
// (Postgres has no LastInsertId).
func (d *DB) InsertID(query string, args ...any) (int64, error) {
	var id int64
	if err := d.SQL.QueryRow(d.Rebind(query+" RETURNING id"), args...).Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

func (d *DB) Ping() error { return d.SQL.Ping() }

func (d *DB) Close() error { return d.SQL.Close() }
