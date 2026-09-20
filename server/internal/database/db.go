package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// DB is the dialect-aware handle used across ServerHub. GORM is the ORM
// (schema via AutoMigrate, app_logs writes, future aggregator queries) while
// Exec/Query/QueryRow keep the existing raw-SQL call sites working on both
// SQLite (local dev / tests) and Postgres (production, DATABASE_URL set).
type DB struct {
	GDB     *gorm.DB
	SQL     *sql.DB
	Dialect string // "sqlite" | "postgres"
}

// OpenDatabase opens the store. databaseURL set => Postgres; else SQLite at
// dbPath (pure Go, no CGO). AutoMigrate is non-destructive on existing DBs.
func OpenDatabase(databaseURL, dbPath string) (*DB, error) {
	dialect := "sqlite"
	var (
		gdb *gorm.DB
		err error
	)
	if databaseURL != "" {
		dialect = "postgres"
		gdb, err = gorm.Open(postgres.Open(databaseURL), &gorm.Config{
			TranslateError: true,
		})
		if err != nil {
			return nil, fmt.Errorf("open postgres: %w", err)
		}
	} else {
		dir := filepath.Dir(dbPath)
		if dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, fmt.Errorf("mkdir data dir: %w", err)
			}
		}
		gdb, err = gorm.Open(sqlite.Open(dbPath), &gorm.Config{
			TranslateError: true,
		})
		if err != nil {
			return nil, fmt.Errorf("open sqlite: %w", err)
		}
	}
	// Create missing tables only — never rebuild existing ones. Plain
	// AutoMigrate recreates legacy SQLite tables (e.g. users) via a temp
	// table and drops NOT NULL columns in the copy, destroying data.
	// Existing tables already match what the raw-SQL layer expects.
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
	if dialect == "sqlite" {
		sqldb.SetMaxOpenConns(1)
		if _, err := sqldb.Exec("PRAGMA journal_mode=WAL;"); err != nil {
			return nil, err
		}
		if _, err := sqldb.Exec("PRAGMA foreign_keys=ON;"); err != nil {
			return nil, err
		}
	}
	return &DB{GDB: gdb, SQL: sqldb, Dialect: dialect}, nil
}

// Rebind converts ? placeholders to $n for Postgres; no-op for SQLite.
func (d *DB) Rebind(q string) string {
	if d == nil || d.Dialect != "postgres" {
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

// InsertID runs an INSERT and returns the new row id. Postgres has no
// LastInsertId, so RETURNING id is used there.
func (d *DB) InsertID(query string, args ...any) (int64, error) {
	if d.Dialect == "postgres" {
		var id int64
		if err := d.SQL.QueryRow(d.Rebind(query+" RETURNING id"), args...).Scan(&id); err != nil {
			return 0, err
		}
		return id, nil
	}
	res, err := d.SQL.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (d *DB) Ping() error { return d.SQL.Ping() }

func (d *DB) Close() error { return d.SQL.Close() }
