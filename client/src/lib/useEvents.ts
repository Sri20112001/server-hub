import { useEffect, useRef } from "react";
import { API_BASE } from "./api";
import type { BusEvent } from "./types";

const TYPES = [
  "deployment.started",
  "deployment.completed",
  "deployment.failed",
  "container.start",
  "container.stop",
  "container.restart",
  "container.exec",
  "project.start",
  "project.stop",
  "project.restart",
  "gateway.reloaded",
  "discovery.completed",
  "backup.created",
  "backup.failed",
  "backup.restored",
  "backup.restoreFailed",
  "backup.deleted",
  "telemetry.threshold",
] as const;

/** Subscribes to the live signal bus (SSE). Reconnects on drop. */
export function useEvents(onEvent: (ev: BusEvent) => void) {
  const ref = useRef(onEvent);
  ref.current = onEvent;

  useEffect(() => {
    let es: EventSource | null = null;
    let closed = false;
    let timer = 0;

    const connect = () => {
      if (closed) return;
      try {
        es = new EventSource(`${API_BASE}/server-hub/api/events`, {
          withCredentials: true,
        });
      } catch {
        timer = window.setTimeout(connect, 5000);
        return;
      }
      for (const t of TYPES) {
        es.addEventListener(t, (e) => {
          try {
            const body = JSON.parse((e as MessageEvent).data) as {
              data?: Record<string, unknown>;
              timestamp?: string;
            };
            ref.current({ type: t, data: body.data, timestamp: body.timestamp ?? "" });
          } catch {
            /* malformed frame — ignore */
          }
        });
      }
      es.onerror = () => {
        es?.close();
        if (!closed) timer = window.setTimeout(connect, 5000);
      };
    };

    connect();
    return () => {
      closed = true;
      window.clearTimeout(timer);
      es?.close();
    };
  }, []);
}
