import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, CheckCircle, RefreshCw } from "lucide-react";
import { api } from "../../lib/api";
import type { Alert } from "../../lib/types";
import { useEvents } from "../../lib/useEvents";
import { useUi } from "../../stores/store";

const SEVERITY_STYLE: Record<string, string> = {
  CRITICAL: "border-brick/30 bg-red-50 dark:bg-red-950/30 text-brick dark:text-red-200",
  WARNING: "border-status-amber/30 bg-amber-50 dark:bg-amber-950/30 text-status-amber dark:text-amber-200",
  INFO: "border-line dark:border-edge bg-paper dark:bg-abyss text-muted dark:text-fog",
};

function AlertRow({ alert, serverName, onResolve }: { alert: Alert; serverName?: string; onResolve: () => void }) {
  const { pushToast } = useUi();
  const [busy, setBusy] = useState(false);

  const resolve = async () => {
    setBusy(true);
    try {
      await api.resolveAlert(alert.id);
      pushToast("Alert resolved");
      onResolve();
    } catch (e) {
      pushToast(e instanceof Error ? e.message : "Failed", true);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className={`flex items-start justify-between p-4 rounded-card border mb-2 shadow-sm ${SEVERITY_STYLE[alert.severity] ?? SEVERITY_STYLE.WARNING}`}>
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2 mb-1">
          <span className="text-xs font-mono font-bold uppercase">{alert.severity}</span>
          <span className="text-xs font-mono opacity-60">{alert.condition}</span>
          {serverName && <span className="text-xs font-mono opacity-60">· {serverName}</span>}
        </div>
        <p className="text-sm text-ink dark:text-bone">{alert.message}</p>
        <p className="text-xs text-muted dark:text-fog mt-1 font-mono">
          {new Date(alert.triggeredAt).toLocaleString()}
          {alert.resolvedAt && ` → resolved ${new Date(alert.resolvedAt).toLocaleString()}`}
        </p>
      </div>
      {alert.status === "TRIGGERED" && (
        <button
          onClick={resolve}
          disabled={busy}
          className="ml-4 shrink-0 flex items-center gap-1 text-xs underline text-ink dark:text-bone opacity-70 hover:opacity-100 disabled:opacity-40 cursor-pointer"
        >
          <CheckCircle size={13} /> Resolve
        </button>
      )}
    </div>
  );
}

export function AlertsPage() {
  const qc = useQueryClient();
  const [tab, setTab] = useState<"TRIGGERED" | "RESOLVED" | "">("TRIGGERED");

  const { data: alerts = [], isLoading, refetch } = useQuery({
    queryKey: ["alerts", tab],
    queryFn: () => api.alerts(tab || undefined),
    refetchInterval: 30_000,
  });

  const { data: servers = [] } = useQuery({
    queryKey: ["servers"],
    queryFn: api.servers,
  });
  const serverName = (id: number | null) =>
    servers.find((s) => s.id === id)?.name;

  useEvents((ev) => {
    if (
      ev.type === "alert.triggered" ||
      ev.type === "alert.resolved" ||
      ev.type === "monitoring.alert.firing" ||
      ev.type === "monitoring.alert.resolved"
    ) {
      void qc.invalidateQueries({ queryKey: ["alerts"] });
    }
  });

  const triggered = alerts.filter((a) => a.status === "TRIGGERED");
  const resolved = alerts.filter((a) => a.status === "RESOLVED");
  const shown = tab === "TRIGGERED" ? triggered : tab === "RESOLVED" ? resolved : alerts;

  return (
    <main className="w-full max-w-4xl mx-auto px-4 sm:px-6 pt-24 pb-36">
      <div className="flex items-center justify-between mb-6">
        <div className="flex items-center gap-3">
          <AlertTriangle size={22} className="text-accent dark:text-ember" />
          <h1 className="font-head text-[22px] font-bold text-ink dark:text-bone tracking-[-0.02em]">Alerts</h1>
          {triggered.length > 0 && (
            <span className="bg-brick text-white text-xs font-bold px-2 py-0.5 rounded-full font-mono">
              {triggered.length}
            </span>
          )}
        </div>
        <button
          onClick={() => void refetch()}
          className="w-9 h-9 rounded-full bg-white dark:bg-panel border border-line dark:border-edge flex items-center justify-center text-muted dark:text-fog hover:text-ink dark:hover:text-bone shadow-chrome cursor-pointer"
          aria-label="Refresh"
        >
          <RefreshCw size={15} />
        </button>
      </div>

      <div className="flex gap-2 mb-4">
        {(["TRIGGERED", "RESOLVED", ""] as const).map((t) => (
          <button
            key={t || "all"}
            onClick={() => setTab(t)}
            className={`px-3 py-1 rounded-full text-xs font-mono border transition-colors cursor-pointer ${
              tab === t
                ? "border-accent dark:border-ember text-accent dark:text-ember bg-tint/50 dark:bg-emboss"
                : "border-line dark:border-edge text-muted dark:text-fog hover:text-ink dark:hover:text-bone bg-white dark:bg-panel"
            }`}
          >
            {t || "All"}
          </button>
        ))}
      </div>

      {isLoading && (
        <div className="space-y-2">
          {[1, 2, 3].map((i) => <div key={i} className="h-16 bg-white dark:bg-panel border border-line dark:border-edge rounded-card animate-pulse" />)}
        </div>
      )}

      {!isLoading && shown.length === 0 && (
        <div className="text-center py-16 text-muted dark:text-fog">
          <CheckCircle size={40} className="mx-auto mb-3 opacity-30" />
          <p className="text-sm">No alerts{tab ? ` with status ${tab}` : ""}.</p>
        </div>
      )}

      {shown.map((a) => (
        <AlertRow
          key={a.id}
          alert={a}
          serverName={serverName(a.serverId)}
          onResolve={() => void qc.invalidateQueries({ queryKey: ["alerts"] })}
        />
      ))}
    </main>
  );
}
