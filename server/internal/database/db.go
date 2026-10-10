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
		&AppSetting{},
		&RefreshToken{},
		&ServerGroup{},
		&ManagedServer{},
		&AgentToken{},
		&ServerMetric{},
		&Alert{},
		&InAppNotification{},
		&HealthCheck{},
		&HealthCheckResult{},
		&NotificationGroup{},
		&NotificationGroupMember{},
		&NotificationRule{},
		&NotificationRuleState{},
		&DeliveryPolicy{},
		&DeliveryState{},
		&DeliveryOutbox{},
		&DeliveryAttempt{},
		&ProviderState{},
		&DigestBatch{},
		&MaintenanceWindow{},
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
	// Idempotent schema upgrades for pre-existing installs (CreateTable
	// above only creates missing tables, never alters them).
	if err := db.ensureNotificationSchema(); err != nil {
		return nil, err
	}
	if err := db.ensureMonitoringSchema(); err != nil {
		return nil, err
	}
	// Log tables are append-only: once written, rows can never be updated
	// or deleted. Enforced at the DB layer so no API, prune job, or raw SQL
	// path can tamper with history.
	if err := db.ensureLogImmutability(); err != nil {
		return nil, err
	}
	return db, nil
}

// ensureMonitoringSchema applies idempotent upgrades for the monitoring
// tables on installs that predate them. Every statement is safe to re-run.
func (d *DB) ensureMonitoringSchema() error {
	stmts := []string{
		`ALTER TABLE alerts ADD COLUMN IF NOT EXISTS fingerprint TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE alerts ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT ''`,
		`CREATE INDEX IF NOT EXISTS idx_alerts_fingerprint ON alerts(fingerprint)`,
		`CREATE INDEX IF NOT EXISTS idx_alerts_server_cond_status ON alerts(server_id, condition, status)`,
		`CREATE INDEX IF NOT EXISTS idx_sm_server_ts ON server_metrics(server_id, timestamp)`,
	}
	for _, q := range stmts {
		if _, err := d.SQL.Exec(q); err != nil {
			return fmt.Errorf("monitoring schema upgrade: %w", err)
		}
	}
	return nil
}

// AppLogsPruneFloorDays is the reviewed exception to app_logs
// immutability: DELETE is permitted only for rows at least this old.
// Anything recent is still tamper-proof (an attacker cannot wipe fresh
// history to cover tracks). audit_logs has NO such exception and stays
// fully immutable. The operational prune job (internal/retention) must use
// a retention >= this floor.
const AppLogsPruneFloorDays = 30

// ensureLogImmutability installs append-only guards on every table that
// carries log/history data:
//
//	audit_logs              -> no UPDATE, no DELETE (fully immutable)
//	app_logs                -> no UPDATE; DELETE only for rows older than
//	                           AppLogsPruneFloorDays (reviewed operational
//	                           retention exception; the audit trail itself
//	                           lives in audit_logs and is untouched)
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
	// app_logs guard: UPDATE always rejected; DELETE allowed only past
	// the prune floor so the retention job can work but recent history
	// cannot be tampered with.
	if _, err := d.SQL.Exec(fmt.Sprintf(`CREATE OR REPLACE FUNCTION serverhub_app_logs_guard()
		RETURNS trigger AS $$
		BEGIN
			IF TG_OP = 'UPDATE' THEN
				RAISE EXCEPTION 'Table app_logs is append-only and cannot be modified (operation: %%)', TG_OP;
			END IF;
			IF OLD.timestamp >= CURRENT_TIMESTAMP - INTERVAL '%d days' THEN
				RAISE EXCEPTION 'Table app_logs rows newer than %d days cannot be deleted';
			END IF;
			RETURN OLD;
		END;
		$$ LANGUAGE plpgsql`, AppLogsPruneFloorDays, AppLogsPruneFloorDays)); err != nil {
		return fmt.Errorf("create app_logs guard: %w", err)
	}
	exec := func(q string) error {
		if _, err := d.SQL.Exec(q); err != nil {
			return err
		}
		return nil
	}
	// audit_logs stays fully immutable: block UPDATE and DELETE.
	for _, tbl := range []string{"audit_logs"} {
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
	// app_logs uses the age-floor guard instead of the blanket reject.
	if err := exec(`DROP TRIGGER IF EXISTS app_logs_no_update_delete ON app_logs`); err != nil {
		return err
	}
	if err := exec(`DROP TRIGGER IF EXISTS app_logs_guarded ON app_logs`); err != nil {
		return err
	}
	if err := exec(`CREATE TRIGGER app_logs_guarded BEFORE UPDATE OR DELETE ON app_logs FOR EACH ROW EXECUTE FUNCTION serverhub_app_logs_guard()`); err != nil {
		return fmt.Errorf("create trigger app_logs_guarded: %w", err)
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

// ensureNotificationSchema applies idempotent upgrades for the centralized
// delivery policy: new columns on groups/rules, the singleton policy row
// with safe defaults, and indexes the dispatcher/worker rely on.
func (d *DB) ensureNotificationSchema() error {
	stmts := []string{
		`ALTER TABLE notification_groups ADD COLUMN IF NOT EXISTS quiet_start TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE notification_groups ADD COLUMN IF NOT EXISTS quiet_end TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE notification_groups ADD COLUMN IF NOT EXISTS quiet_tz TEXT NOT NULL DEFAULT 'UTC'`,
		`ALTER TABLE notification_groups ADD COLUMN IF NOT EXISTS quiet_allow_critical BOOLEAN NOT NULL DEFAULT TRUE`,
		`ALTER TABLE notification_rules ADD COLUMN IF NOT EXISTS repeat_interval_sec INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE notification_rules ADD COLUMN IF NOT EXISTS max_repeats INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE notification_rules ADD COLUMN IF NOT EXISTS digest_mode TEXT NOT NULL DEFAULT 'immediate'`,
		`ALTER TABLE notification_rules ADD COLUMN IF NOT EXISTS digest_interval_min INTEGER NOT NULL DEFAULT 60`,
		`ALTER TABLE notification_rules ADD COLUMN IF NOT EXISTS group_by TEXT NOT NULL DEFAULT ''`,
		`CREATE INDEX IF NOT EXISTS idx_delivery_outbox_due ON delivery_outbox(status, next_retry_at)`,
		`CREATE INDEX IF NOT EXISTS idx_delivery_outbox_key ON delivery_outbox(dedupe_key)`,
		`CREATE INDEX IF NOT EXISTS idx_delivery_attempts_outbox ON delivery_attempts(outbox_id)`,
		`CREATE INDEX IF NOT EXISTS idx_delivery_attempts_ts ON delivery_attempts(created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_digest_due ON digest_batches(status, window_end)`,
		`CREATE INDEX IF NOT EXISTS idx_maint_windows ON maintenance_windows(enabled, starts_at, ends_at)`,
		`INSERT INTO notification_policy (id) VALUES (1) ON CONFLICT (id) DO NOTHING`,
	}
	for _, q := range stmts {
		if _, err := d.SQL.Exec(q); err != nil {
			return fmt.Errorf("notification schema upgrade: %w", err)
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
