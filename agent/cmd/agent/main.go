// Command agent is the ServerHub monitoring agent: a single native binary
// with no runtime dependencies on the monitored machine.
//
// Configuration (env vars or .env next to the binary / in cwd):
//
//	AGENT_TOKEN        required — token from ServerHub web UI
//	SERVERHUB_URL      required — base URL of the API, e.g. http://host:4000
//	METRICS_INTERVAL   optional — seconds between reports (default 60)
//	HEARTBEAT_INTERVAL optional — seconds between heartbeats (default 30)
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"serverhub-agent/internal/client"
	"serverhub-agent/internal/config"
	"serverhub-agent/internal/metrics"
)

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	api := client.New(cfg.BaseURL, cfg.Token)

	log.Printf("ServerHub Agent starting — %s", cfg.BaseURL)
	log.Printf("Heartbeat every %s, metrics every %s", cfg.HeartbeatInterval, cfg.MetricsInterval)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Report immediately on start, then on interval.
	doHeartbeat(ctx, api)
	doMetrics(ctx, api)

	hb := time.NewTicker(cfg.HeartbeatInterval)
	mx := time.NewTicker(cfg.MetricsInterval)
	defer hb.Stop()
	defer mx.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("shutting down")
			return
		case <-hb.C:
			doHeartbeat(ctx, api)
		case <-mx.C:
			doMetrics(ctx, api)
		}
	}
}

func doHeartbeat(ctx context.Context, api *client.Client) {
	if err := api.Heartbeat(ctx); err != nil {
		log.Printf("heartbeat failed: %v", err)
		return
	}
	log.Println("heartbeat ok")
}

func doMetrics(ctx context.Context, api *client.Client) {
	snap, err := metrics.Collect()
	if err != nil {
		log.Printf("collect failed: %v", err)
	}
	if err := api.SendMetrics(ctx, snap); err != nil {
		log.Printf("metrics failed: %v", err)
		return
	}
	log.Printf("metrics ok (cpu=%.1f%% mem=%.1f%% disk=%.1f%%)",
		snap.CPUUsage, snap.MemoryUsage, snap.DiskUsage)
}
