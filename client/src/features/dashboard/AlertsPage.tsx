import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, CheckCircle, RefreshCw } from "lucide-react";
import { api } from "../../lib/api";
import type { Alert } from "../../lib/types";
import { useEvents } from "../../lib/useEvents";
import { useUi } from "../../stores/store";

const SEVERITY_STYLE: Record<string, string> = {
  CRITICAL: "border-red-500/40 bg-red-500/10 text-red-300",
  WARNING: "border-yellow-500/40 bg-yellow-500/10 text-yellow-300",
  INFO: "border-blue-500/40 bg-blue-500/10 text-blue-300",
};

function AlertRow({ alert, onResolve }: { alert: Alert; onResolve: () => void }) {
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
    <div className={`flex items-start justify-between p-4 rounded-xl border mb-2 ${SEVERITY_STYLE[alert.severity] ?? SEVERITY_STYLE.WARNING}`}>
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2 mb-1">
          <span className="text-xs font-mono font-bold uppercase">{alert.severity}</span>
          <span className="text-xs font-mono opacity-60">{alert.condition}</span>
        </div>
        <p className="text-sm">{alert.message}</p>
        <p className="text-xs opacity-60 mt-1 font-mono">
          {new Date(alert.triggeredAt).toLocaleString()}
          {alert.resolvedAt && ` → resolved ${new Date(alert.resolvedAt).toLocaleString()}`}
        </p>
      </div>
      {alert.status === "TRIGGERED" && (
        <button
          onClick={resolve}
          disabled={busy}
          className="ml-4 shrink-0 flex items-center gap-1 text-xs underline opacity-70 hover:opacity-100 disabled:opacity-40"
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

  useEvents((ev) => {
    if (ev.type === "alert.triggered" || ev.type === "alert.resolved") {
      void qc.invalidateQueries({ queryKey: ["alerts"] });
    }
  });

  const triggered = alerts.filter((a) => a.status === "TRIGGERED");
  const resolved = alerts.filter((a) => a.status === "RESOLVED");
  const shown = tab === "TRIGGERED" ? triggered : tab === "RESOLVED" ? resolved : alerts;

  return (
    <div className="min-h-screen bg-abyss pt-20 pb-28 px-4 sm:px-6 max-w-4xl mx-auto">
      <div className="flex items-center justify-between mb-6">
        <div className="flex items-center gap-3">
          <AlertTriangle size={22} className="text-ember" />
          <h1 className="text-bone font-bold text-xl">Alerts</h1>
          {triggered.length > 0 && (
            <span className="bg-red-500 text-white text-xs font-bold px-2 py-0.5 rounded-full">
              {triggered.length}
            </span>
          )}
        </div>
        <button
          onClick={() => void refetch()}
          className="w-9 h-9 rounded-full bg-emboss flex items-center justify-center text-fog hover:text-bone"
        >
          <RefreshCw size={15} />
        </button>
      </div>

      <div className="flex gap-2 mb-4">
        {(["TRIGGERED", "RESOLVED", ""] as const).map((t) => (
          <button
            key={t || "all"}
            onClick={() => setTab(t)}
            className={`px-3 py-1 rounded-full text-xs font-mono border ${tab === t ? "border-ember text-ember" : "border-edge text-fog"}`}
          >
            {t || "All"}
          </button>
        ))}
      </div>

      {isLoading && (
        <div className="space-y-2">
          {[1, 2, 3].map((i) => <div key={i} className="h-16 bg-panel rounded-xl animate-pulse" />)}
        </div>
      )}

      {!isLoading && shown.length === 0 && (
        <div className="text-center py-16 text-fog">
          <CheckCircle size={40} className="mx-auto mb-3 opacity-30" />
          <p className="text-sm">No alerts{tab ? ` with status ${tab}` : ""}.</p>
        </div>
      )}

      {shown.map((a) => (
        <AlertRow
          key={a.id}
          alert={a}
          onResolve={() => void qc.invalidateQueries({ queryKey: ["alerts"] })}
        />
      ))}
    </div>
  );
}
