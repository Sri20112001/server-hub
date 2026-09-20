package health

import (
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"serverhub/internal/audit"
	"serverhub/internal/database"
	"serverhub/internal/events"
)

// StartLoop periodically checks every service/project health_url and
// updates services.status / projects.status.
// States: HEALTHY | DEGRADED | DOWN | UNKNOWN
func StartLoop(db *database.DB, broker *events.Broker, intervalSec int) {
	if intervalSec <= 0 {
		intervalSec = 60
	}
	go func() {
		t := time.NewTicker(time.Duration(intervalSec) * time.Second)
		defer t.Stop()
		check(db, broker)
		for range t.C {
			check(db, broker)
		}
	}()
}

// transition records a monitor event when a status genuinely changes.
// Transitions FROM unknown are discovery noise, not events.
func transition(db *database.DB, broker *events.Broker, resource, id, name, before, after string) {
	before, after = strings.ToUpper(before), strings.ToUpper(after)
	if before == after || before == "" || before == "UNKNOWN" {
		return
	}
	audit.Write(db, "monitor", "health-transition", resource, id, after,
		name+": "+before+" → "+after)
	if broker != nil {
		broker.Publish("health.changed", map[string]interface{}{
			"resource": resource, "resourceId": id, "name": name,
			"before": before, "after": after,
		})
	}
}

func check(db *database.DB, broker *events.Broker) {
	client := &http.Client{Timeout: 8 * time.Second}
	// Services
	rows, err := db.Query(`SELECT id, name, health_url, last_health FROM services WHERE health_url <> ''`)
	if err != nil {
		return
	}
	type item struct {
		id     int64
		name   string
		url    string
		before string
	}
	var svcs []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.id, &it.name, &it.url, &it.before); err == nil {
			svcs = append(svcs, it)
		}
	}
	rows.Close()
	for _, s := range svcs {
		state, rt := probe(client, s.url)
		_, _ = db.Exec(`UPDATE services SET status=?, last_health=?, last_health_at=CURRENT_TIMESTAMP, response_time_ms=? WHERE id=?`,
			state, state, rt, s.id)
		transition(db, broker, "service", strconv.FormatInt(s.id, 10), s.name, s.before, state)
	}
	// Projects with their own health_url
	prows, err := db.Query(`SELECT id, name, health_url, status FROM projects WHERE health_url <> ''`)
	if err != nil {
		return
	}
	var projs []item
	for prows.Next() {
		var it item
		if err := prows.Scan(&it.id, &it.name, &it.url, &it.before); err == nil {
			projs = append(projs, it)
		}
	}
	prows.Close()
	for _, p := range projs {
		state, _ := probe(client, p.url)
		_, _ = db.Exec(`UPDATE projects SET status=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`, state, p.id)
		transition(db, broker, "project", strconv.FormatInt(p.id, 10), p.name, p.before, state)
	}
	// Snapshot aggregate statuses before recomputing them.
	before := map[int64]string{}
	brows, err := db.Query(`SELECT id, status FROM projects WHERE health_url = '' OR health_url IS NULL`)
	if err == nil {
		for brows.Next() {
			var id int64
			var st string
			if err := brows.Scan(&id, &st); err == nil {
				before[id] = st
			}
		}
		brows.Close()
	}
	// Aggregate project status from services when project has no direct health_url
	_, _ = db.Exec(`
		UPDATE projects SET status = (
			SELECT CASE
				WHEN COUNT(*) = 0 THEN 'unknown'
				WHEN SUM(CASE WHEN s.status='DOWN' THEN 1 ELSE 0 END) > 0 THEN 'down'
				WHEN SUM(CASE WHEN s.status='DEGRADED' THEN 1 ELSE 0 END) > 0 THEN 'degraded'
				WHEN SUM(CASE WHEN s.status='HEALTHY' THEN 1 ELSE 0 END) = COUNT(*) THEN 'healthy'
				ELSE 'unknown'
			END
			FROM services s WHERE s.project_id = projects.id
		), updated_at=CURRENT_TIMESTAMP
		WHERE health_url = '' OR health_url IS NULL
	`)
	arows, err := db.Query(`SELECT id, name, status FROM projects WHERE health_url = '' OR health_url IS NULL`)
	if err == nil {
		for arows.Next() {
			var id int64
			var name, after string
			if err := arows.Scan(&id, &name, &after); err == nil {
				transition(db, broker, "project", strconv.FormatInt(id, 10), name, before[id], after)
			}
		}
		arows.Close()
	}
	_ = log.Default()
}

func probe(client *http.Client, url string) (string, int64) {
	start := time.Now()
	resp, err := client.Get(url)
	rt := time.Since(start).Milliseconds()
	if err != nil {
		return "DOWN", rt
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return "HEALTHY", rt
	}
	if resp.StatusCode >= 500 {
		return "DOWN", rt
	}
	return "DEGRADED", rt
}
