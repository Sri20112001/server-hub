// Package healthcheck runs periodic HTTP, TCP, and Ping probes.
package healthcheck

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"serverhub/internal/database"
	"serverhub/internal/events"
)

const runInterval = 30 * time.Second

func StartLoop(db *database.DB, broker *events.Broker) {
	go func() {
		t := time.NewTicker(runInterval)
		defer t.Stop()
		runAll(db, broker)
		for range t.C {
			runAll(db, broker)
		}
	}()
}

func runAll(db *database.DB, broker *events.Broker) {
	rows, err := db.Query(`SELECT id, type, target, timeout, expected_status FROM health_checks WHERE enabled=true`)
	if err != nil {
		log.Printf("healthcheck: query: %v", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id uint
		var typ, target string
		var timeout, expectedStatus int
		if err := rows.Scan(&id, &typ, &target, &timeout, &expectedStatus); err != nil {
			continue
		}
		go probe(db, broker, id, typ, target, timeout, expectedStatus)
	}
}

func probe(db *database.DB, broker *events.Broker, id uint, typ, target string, timeout, expectedStatus int) {
	start := time.Now()
	status, errMsg := runProbe(typ, target, timeout, expectedStatus)
	ms := time.Since(start).Milliseconds()
	now := time.Now().UTC()

	_, _ = db.Exec(`
		INSERT INTO health_check_results (health_check_id, timestamp, status, response_time_ms, error)
		VALUES ($1,$2,$3,$4,$5)`, id, now, status, ms, errMsg)

	_, _ = db.Exec(`
		UPDATE health_checks SET status=$1, response_time_ms=$2, last_checked_at=$3, updated_at=$3
		WHERE id=$4`, status, ms, now, id)

	if broker != nil {
		broker.Publish("healthcheck.result", map[string]interface{}{
			"id": id, "status": status, "responseTimeMs": ms,
		})
	}
}

func runProbe(typ, target string, timeout, expectedStatus int) (status, errMsg string) {
	d := time.Duration(timeout) * time.Second
	switch strings.ToLower(typ) {
	case "http":
		return probeHTTP(target, d, expectedStatus)
	case "tcp":
		return probeTCP(target, d)
	case "ping":
		return probePing(target, d)
	default:
		return "UNKNOWN", fmt.Sprintf("unknown check type: %s", typ)
	}
}

func probeHTTP(url string, timeout time.Duration, expectedStatus int) (string, string) {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		return "DOWN", err.Error()
	}
	defer resp.Body.Close()
	if expectedStatus > 0 && resp.StatusCode != expectedStatus {
		return "DOWN", fmt.Sprintf("got %d, expected %d", resp.StatusCode, expectedStatus)
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return "UP", ""
	}
	return "DOWN", fmt.Sprintf("status %d", resp.StatusCode)
}

func probeTCP(addr string, timeout time.Duration) (string, string) {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return "DOWN", err.Error()
	}
	conn.Close()
	return "UP", ""
}

func probePing(host string, timeout time.Duration) (string, string) {
	var cmd *exec.Cmd
	secs := fmt.Sprintf("%d", int(timeout.Seconds()))
	if runtime.GOOS == "windows" {
		cmd = exec.Command("ping", "-n", "1", "-w", fmt.Sprintf("%d", int(timeout.Milliseconds())), host)
	} else {
		cmd = exec.Command("ping", "-c", "1", "-W", secs, host)
	}
	if err := cmd.Run(); err != nil {
		return "DOWN", err.Error()
	}
	return "UP", ""
}
