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
    <main className="w-full max-w-5xl mx-auto px-6 pt-12 pb-28">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-end justify-between gap-4 mb-8">
        <div>
          <div className="flex items-center gap-2 mb-1.5">
            <span className="inline-flex items-center justify-center w-6 h-6 rounded-md bg-primary/10 text-primary">
              <AlertTriangle size={15} />
            </span>
            <span className="text-[11px] font-semibold text-outline uppercase tracking-wider">Signals & Incidents</span>
          </div>
          <div className="flex items-center gap-3">
            <h1 className="text-[32px] sm:text-[36px] font-bold text-on-surface tracking-tight leading-tight">Alerts</h1>
            {triggered.length > 0 && (
              <span className="bg-error text-white text-[12px] font-semibold px-2.5 py-0.5 rounded-full font-mono shadow-sm">
                {triggered.length} active
              </span>
            )}
          </div>
          <p className="text-[15px] text-on-surface-variant max-w-xl mt-1">
            Actionable system warnings, thresholds breached, and historical resolution logs.
          </p>
        </div>
        <button
          onClick={() => void refetch()}
          className="w-10 h-10 rounded-full bg-surface-container-lowest border border-outline-variant/30 flex items-center justify-center text-on-surface-variant hover:text-on-surface shadow-sm cursor-pointer transition-colors"
          aria-label="Refresh"
          type="button"
        >
          <RefreshCw size={15} />
        </button>
      </div>

      {/* Contextual Pill Segments */}
      <div className="flex items-center overflow-x-auto pb-1 gap-1 p-1 bg-surface-container rounded-full max-w-fit shadow-inner mb-6" role="tablist">
        {([
          { key: "TRIGGERED", label: `Active (${triggered.length})` },
          { key: "RESOLVED", label: `Resolved (${resolved.length})` },
          { key: "", label: `All (${alerts.length})` },
        ] as const).map((t) => (
          <button
            key={t.key || "all"}
            role="tab"
            aria-selected={tab === t.key}
            onClick={() => setTab(t.key)}
            className={`px-4 py-1.5 rounded-full text-[13px] font-medium transition-all cursor-pointer border-0 whitespace-nowrap ${
              tab === t.key
                ? "bg-surface-container-lowest text-primary font-semibold shadow-sm"
                : "text-on-surface-variant hover:text-on-surface hover:bg-surface-container-high"
            }`}
          >
            {t.label}
          </button>
        ))}
      </div>

      {isLoading && (
        <div className="space-y-3">
          {[1, 2, 3].map((i) => (
            <div key={i} className="h-24 bg-surface-container-lowest rounded-2xl animate-pulse shadow-sm border border-outline-variant/20" />
          ))}
        </div>
      )}

      {!isLoading && shown.length === 0 && (
        <div className="text-center py-16 bg-surface-container-low rounded-2xl border border-outline-variant/20 px-4">
          <CheckCircle size={40} className="mx-auto mb-3 text-secondary opacity-60" />
          <p className="text-[15px] font-semibold text-on-surface">
            {tab === "TRIGGERED" ? "No active alerts." : tab === "RESOLVED" ? "No resolved alerts yet." : "No alerts at all."}
          </p>
          <p className="text-[13px] text-on-surface-variant mt-1">
            {tab === "RESOLVED"
              ? "Past problems will appear here with duration and timestamps."
              : "All system parameters and watched nodes are operating normally."}
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
