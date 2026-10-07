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
	"math/rand"
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

	// Report immediately on start, then on interval. Each loop tracks its
	// own consecutive failures and backs off (with jitter) instead of
	// hammering an unavailable server — the agent never exits on its own.
	var hbFail, mxFail int
	hbTimer := time.NewTimer(0)
	mxTimer := time.NewTimer(0)
	defer hbTimer.Stop()
	defer mxTimer.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("shutting down")
			return
		case <-hbTimer.C:
			if doHeartbeat(ctx, api) {
				hbFail = 0
				hbTimer.Reset(cfg.HeartbeatInterval)
			} else {
				hbFail++
				hbTimer.Reset(backoff(hbFail, cfg.HeartbeatInterval, 60*time.Second))
			}
		case <-mxTimer.C:
			if doMetrics(ctx, api) {
				mxFail = 0
				mxTimer.Reset(cfg.MetricsInterval)
			} else {
				mxFail++
				mxTimer.Reset(backoff(mxFail, cfg.MetricsInterval, 120*time.Second))
			}
		}
	}
}

// backoff returns interval normally, or an exponentially increasing delay
// (base 5s doubling per consecutive failure, capped) plus up to 1s jitter
// when failures > 0. The heartbeat cap keeps worst-case delay well under the
// server's offline timeout so backoff alone never flaps a server OFFLINE.
func backoff(failures int, interval, cap time.Duration) time.Duration {
	if failures <= 0 {
		return interval
	}
	d := 5 * time.Second
	for i := 1; i < failures; i++ {
		d *= 2
		if d >= cap {
			d = cap
			break
		}
	}
	if d > cap {
		d = cap
	}
	return d + time.Duration(rand.Int63n(int64(time.Second)))
}

func doHeartbeat(ctx context.Context, api *client.Client) bool {
	if err := api.Heartbeat(ctx); err != nil {
		log.Printf("heartbeat failed: %v", err)
		return false
	}
	log.Println("heartbeat ok")
	return true
}

func doMetrics(ctx context.Context, api *client.Client) bool {
	snap, err := metrics.Collect()
	if err != nil {
		log.Printf("collect failed: %v", err)
	}
	if err := api.SendMetrics(ctx, snap); err != nil {
		log.Printf("metrics failed: %v", err)
		return false
	}
	log.Printf("metrics ok (cpu=%.1f%% mem=%.1f%% disk=%.1f%%)",
		snap.CPUUsage, snap.MemoryUsage, snap.DiskUsage)
	return true
}
