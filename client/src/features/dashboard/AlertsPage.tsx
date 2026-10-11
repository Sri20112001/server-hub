import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, CheckCircle, RefreshCw } from "lucide-react";
import { api } from "../../lib/api";
import { describeAlert } from "../../lib/types";
import type { Alert } from "../../lib/types";
import { fmtDuration } from "../../lib/format";
import { PageError } from "../../components/ui";
import { useEvents } from "../../lib/useEvents";
import { useUi } from "../../stores/store";

const SEVERITY_STYLE: Record<string, string> = {
  CRITICAL: "border-brick/30 bg-brick/10 text-brick",
  WARNING: "border-status-amber/30 bg-status-amber/10 text-status-amber",
  INFO: "border-line dark:border-edge bg-paper dark:bg-abyss text-muted dark:text-fog",
};

const SEVERITY_BADGE: Record<string, string> = {
  CRITICAL: "border-brick/30 bg-brick/15 text-brick",
  WARNING: "border-status-amber/30 bg-status-amber/15 text-status-amber",
  INFO: "border-line dark:border-edge bg-paper dark:bg-abyss text-stone",
};

function alertDuration(a: Alert): string | null {
  if (!a.resolvedAt) return null;
  const ms = new Date(a.resolvedAt).getTime() - new Date(a.triggeredAt).getTime();
  if (!Number.isFinite(ms) || ms < 0) return null;
  return fmtDuration(Math.round(ms / 1000));
}

function AlertRow({ alert, serverName, onResolve }: { alert: Alert; serverName?: string; onResolve: () => void }) {
  const { pushToast } = useUi();
  const [busy, setBusy] = useState(false);
  const desc = describeAlert({
    condition: alert.condition,
    message: alert.message,
    severity: alert.severity,
    source: alert.source,
    threshold: alert.threshold,
  });
  const lasted = alertDuration(alert);

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
    <div className={`flex items-start justify-between p-5 rounded-card border mb-3 shadow-sm transition-all ${SEVERITY_STYLE[alert.severity] ?? SEVERITY_STYLE.WARNING}`}>
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2 mb-1.5 flex-wrap">
          <span className={`text-[10px] font-mono font-bold uppercase tracking-wider px-2 py-0.5 rounded border ${SEVERITY_BADGE[alert.severity] ?? SEVERITY_BADGE.WARNING}`}>
            {alert.severity}
          </span>
          <span className="text-xs font-mono opacity-75 font-medium">{alert.condition}</span>
          {serverName && (
            <span className="text-xs font-mono bg-white/50 dark:bg-black/30 px-2 py-0.5 rounded border border-line dark:border-edge">
              {serverName}
            </span>
          )}
        </div>
        <p className="text-sm font-medium text-ink dark:text-bone">{desc.what}</p>
        <p className="text-xs text-muted dark:text-fog mt-1">{desc.next}</p>
        <p className="text-xs text-muted dark:text-fog mt-1.5 font-mono">
          Started {new Date(alert.triggeredAt).toLocaleString()}
          {alert.resolvedAt && ` · Resolved ${new Date(alert.resolvedAt).toLocaleString()}`}
          {lasted && ` · Lasted ${lasted}`}
          {alert.message && alert.message !== desc.what && ` · ${alert.message}`}
        </p>
      </div>
      {alert.status === "TRIGGERED" && (
        <button
          onClick={resolve}
          disabled={busy}
          className="ml-4 shrink-0 flex items-center gap-1.5 text-xs font-medium px-3 py-1.5 rounded-lg border border-line dark:border-edge bg-white dark:bg-panel text-ink dark:text-bone hover:border-accent dark:hover:border-ember cursor-pointer shadow-sm disabled:opacity-40 transition-colors"
        >
          <CheckCircle size={14} className="text-moss" /> Resolve
        </button>
      )}
    </div>
  );
}

export function AlertsPage() {
  const qc = useQueryClient();
  const [tab, setTab] = useState<"TRIGGERED" | "RESOLVED" | "">("TRIGGERED");

  const { data: alerts = [], isLoading, isError, error, refetch } = useQuery({
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

  // A failed fetch is an error with retry — never an all-clear.
  if (isError && !isLoading && alerts.length === 0) {
    return (
      <PageError
        title="Couldn't load alerts"
        detail={error instanceof Error ? error.message : undefined}
        onRetry={() => void refetch()}
      />
    );
  }

  return (
    <main className="w-full max-w-7xl mx-auto px-10 max-md:px-4 pt-24 pb-36">
      <div className="flex items-center justify-between mb-6 flex-wrap gap-4">
        <div>
          <div className="flex items-center gap-3">
            <AlertTriangle size={22} className="text-accent dark:text-ember" />
            <h1 className="font-head text-[24px] font-bold text-ink dark:text-bone tracking-[-0.02em]">Alerts</h1>
            {triggered.length > 0 && (
              <span className="bg-brick text-white text-xs font-bold px-2.5 py-0.5 rounded-full font-mono shadow-sm">
                {triggered.length} active
              </span>
            )}
          </div>
          <p className="text-muted dark:text-fog text-xs mt-1 font-mono">
            Problems that need attention, plus what happened before
          </p>
        </div>
        <button
          onClick={() => void refetch()}
          className="w-9 h-9 rounded-full bg-white dark:bg-panel border border-line dark:border-edge flex items-center justify-center text-muted dark:text-fog hover:text-ink dark:hover:text-bone shadow-sm cursor-pointer"
          aria-label="Refresh"
        >
          <RefreshCw size={15} />
        </button>
      </div>

      <div className="grid grid-cols-3 gap-1 p-1 rounded-card bg-paper dark:bg-abyss border border-line dark:border-edge mb-6 w-full sm:max-w-xs">
        {([
          { key: "TRIGGERED", label: "Active" },
          { key: "RESOLVED", label: "Resolved" },
          { key: "", label: "All" },
        ] as const).map((t) => (
          <button
            key={t.key || "all"}
            onClick={() => setTab(t.key)}
            className={`px-3 py-1.5 rounded-lg text-xs font-mono transition-colors cursor-pointer border ${
              tab === t.key
                ? "bg-white dark:bg-panel border-line dark:border-edge text-ink dark:text-bone font-semibold shadow-sm"
                : "bg-transparent border-transparent text-muted dark:text-fog hover:text-ink dark:hover:text-bone"
            }`}
          >
            {t.label}
          </button>
        ))}
      </div>

      {isLoading && (
        <div className="space-y-3">
          {[1, 2, 3].map((i) => <div key={i} className="h-24 bg-white dark:bg-panel border border-line dark:border-edge rounded-card animate-pulse shadow-sm" />)}
        </div>
      )}

      {!isLoading && shown.length === 0 && (
        <div className="text-center py-16 bg-paper dark:bg-abyss border border-line dark:border-edge rounded-card px-4">
          <CheckCircle size={40} className="mx-auto mb-3 opacity-30 text-moss" />
          <p className="text-sm font-medium text-ink dark:text-bone">
            {tab === "TRIGGERED" ? "No active alerts." : tab === "RESOLVED" ? "No resolved alerts yet." : "No alerts at all."}
          </p>
          <p className="text-xs text-muted dark:text-fog mt-1">
            {tab === "RESOLVED"
              ? "Past problems will appear here with how long they lasted."
              : "Everything being watched is working normally."}
          </p>
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
