import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Activity, Plus, RefreshCw, Trash2, ChevronDown, ChevronUp } from "lucide-react";
import { api } from "../../lib/api";
import type { HealthCheck } from "../../lib/types";
import { useUi } from "../../stores/store";

const STATUS_COLOR: Record<string, string> = {
  UP: "text-green-400", DOWN: "text-red-400", UNKNOWN: "text-zinc-400",
};
const STATUS_DOT: Record<string, string> = {
  UP: "bg-green-500", DOWN: "bg-red-500", UNKNOWN: "bg-zinc-500",
};

function ResultsPanel({ checkId }: { checkId: number }) {
  const { data: results = [] } = useQuery({
    queryKey: ["hc-results", checkId],
    queryFn: () => api.healthCheckResults(checkId),
    refetchInterval: 30_000,
  });
  return (
    <div className="mt-3 border-t border-edge pt-3">
      <div className="text-fog text-xs mb-2">Last 10 results</div>
      <div className="flex gap-1 flex-wrap">
        {results.slice(0, 10).map((r) => (
          <div key={r.id} title={`${r.status} · ${r.responseTimeMs}ms · ${new Date(r.timestamp).toLocaleString()}`}
            className={`w-5 h-5 rounded ${r.status === "UP" ? "bg-green-500/70" : "bg-red-500/70"}`} />
        ))}
        {results.length === 0 && <span className="text-fog text-xs">No results yet.</span>}
      </div>
      {results[0] && (
        <div className="text-fog text-xs mt-1 font-mono">
          Last: {results[0].responseTimeMs}ms · {new Date(results[0].timestamp).toLocaleString()}
          {results[0].error && <span className="text-red-400 ml-2">{results[0].error}</span>}
        </div>
      )}
    </div>
  );
}

function CheckCard({ check, onDelete, onToggle }: {
  check: HealthCheck;
  onDelete: () => void;
  onToggle: () => void;
}) {
  const [expanded, setExpanded] = useState(false);
  return (
    <div className="bg-panel border border-edge rounded-xl p-4">
      <div className="flex items-start justify-between">
        <div className="flex items-center gap-2 flex-1 min-w-0">
          <span className={`w-2 h-2 rounded-full shrink-0 ${STATUS_DOT[check.status] ?? "bg-zinc-500"}`} />
          <div className="min-w-0">
            <div className="text-bone font-semibold text-sm truncate">{check.name}</div>
            <div className="text-fog text-xs font-mono truncate">{check.type.toUpperCase()} · {check.target}</div>
          </div>
        </div>
        <div className="flex items-center gap-2 ml-2 shrink-0">
          <span className={`text-xs font-mono ${STATUS_COLOR[check.status] ?? "text-zinc-400"}`}>{check.status}</span>
          {check.responseTimeMs != null && (
            <span className="text-fog text-xs font-mono">{check.responseTimeMs}ms</span>
          )}
          <button onClick={onToggle}
            className={`text-xs px-2 py-0.5 rounded border font-mono ${check.enabled ? "border-green-500/40 text-green-400" : "border-edge text-fog"}`}>
            {check.enabled ? "ON" : "OFF"}
          </button>
          <button onClick={() => setExpanded(!expanded)} className="text-fog hover:text-bone">
            {expanded ? <ChevronUp size={14} /> : <ChevronDown size={14} />}
          </button>
          <button onClick={onDelete} className="text-fog hover:text-red-400"><Trash2 size={14} /></button>
        </div>
      </div>
      <div className="text-fog text-xs mt-1">
        Every {check.interval}s · timeout {check.timeout}s
        {check.lastCheckedAt && ` · checked ${new Date(check.lastCheckedAt).toLocaleString()}`}
      </div>
      {expanded && <ResultsPanel checkId={check.id} />}
    </div>
  );
}

function AddCheckModal({ onClose, onCreated }: { onClose: () => void; onCreated: () => void }) {
  const { pushToast } = useUi();
  const [form, setForm] = useState<{ name: string; type: "http" | "tcp" | "ping"; target: string; interval: number; timeout: number; expectedStatus: number }>({ name: "", type: "http", target: "", interval: 60, timeout: 10, expectedStatus: 200 });
  const [busy, setBusy] = useState(false);

  const set = (k: string, v: string | number) =>
    setForm((f) => ({ ...f, [k]: v }) as typeof f);

  const submit = async () => {
    if (!form.name || !form.target) return;
    setBusy(true);
    try {
      await api.createHealthCheck(form);
      pushToast(`Health check "${form.name}" created`);
      onCreated();
      onClose();
    } catch (e) {
      pushToast(e instanceof Error ? e.message : "Failed", true);
    } finally { setBusy(false); }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4" onClick={onClose}>
      <div className="bg-panel border border-edge rounded-xl p-6 w-full max-w-md" onClick={(e) => e.stopPropagation()}>
        <h2 className="text-bone font-bold text-lg mb-4">Add Health Check</h2>
        <div className="space-y-3">
          <Field label="Name *" value={form.name} onChange={(v) => set("name", v)} placeholder="API health" />
          <div>
            <label className="text-fog text-xs mb-1 block">Type</label>
            <select className="w-full bg-emboss border border-edge rounded-lg px-3 py-2 text-bone text-sm outline-none focus:border-ember"
              value={form.type} onChange={(e) => set("type", e.target.value as "http" | "tcp" | "ping")}>
              <option value="http">HTTP</option>
              <option value="tcp">TCP</option>
              <option value="ping">Ping</option>
            </select>
          </div>
          <Field label="Target *" value={form.target} onChange={(v) => set("target", v)}
            placeholder={form.type === "http" ? "https://example.com/health" : form.type === "tcp" ? "host:port" : "hostname"} />
          <div className="grid grid-cols-3 gap-2">
            <div>
              <label className="text-fog text-xs mb-1 block">Interval (s)</label>
              <input type="number" className="w-full bg-emboss border border-edge rounded-lg px-3 py-2 text-bone text-sm outline-none focus:border-ember"
                value={form.interval} onChange={(e) => set("interval", Number(e.target.value))} />
            </div>
            <div>
              <label className="text-fog text-xs mb-1 block">Timeout (s)</label>
              <input type="number" className="w-full bg-emboss border border-edge rounded-lg px-3 py-2 text-bone text-sm outline-none focus:border-ember"
                value={form.timeout} onChange={(e) => set("timeout", Number(e.target.value))} />
            </div>
            {form.type === "http" && (
              <div>
                <label className="text-fog text-xs mb-1 block">Expected</label>
                <input type="number" className="w-full bg-emboss border border-edge rounded-lg px-3 py-2 text-bone text-sm outline-none focus:border-ember"
                  value={form.expectedStatus} onChange={(e) => set("expectedStatus", Number(e.target.value))} />
              </div>
            )}
          </div>
        </div>
        <div className="flex gap-2 mt-5 justify-end">
          <button onClick={onClose} className="px-4 py-2 text-sm text-fog hover:text-bone border border-edge rounded-lg">Cancel</button>
          <button onClick={submit} disabled={busy || !form.name || !form.target}
            className="px-4 py-2 text-sm bg-ember text-black font-semibold rounded-lg disabled:opacity-50">
            {busy ? "Creating…" : "Create"}
          </button>
        </div>
      </div>
    </div>
  );
}

function Field({ label, value, onChange, placeholder }: { label: string; value: string; onChange: (v: string) => void; placeholder?: string }) {
  return (
    <div>
      <label className="text-fog text-xs mb-1 block">{label}</label>
      <input className="w-full bg-emboss border border-edge rounded-lg px-3 py-2 text-bone text-sm outline-none focus:border-ember"
        value={value} onChange={(e) => onChange(e.target.value)} placeholder={placeholder} />
    </div>
  );
}

export function HealthChecksPage() {
  const qc = useQueryClient();
  const { pushToast } = useUi();
  const [showAdd, setShowAdd] = useState(false);

  const { data: checks = [], isLoading, refetch } = useQuery({
    queryKey: ["health-checks"],
    queryFn: api.healthChecks,
    refetchInterval: 30_000,
  });

  const deleteCheck = async (id: number) => {
    if (!confirm("Delete this health check?")) return;
    try {
      await api.deleteHealthCheck(id);
      pushToast("Deleted");
      void qc.invalidateQueries({ queryKey: ["health-checks"] });
    } catch (e) { pushToast(e instanceof Error ? e.message : "Failed", true); }
  };

  const toggleCheck = async (check: HealthCheck) => {
    try {
      await api.updateHealthCheck(check.id, { enabled: !check.enabled });
      void qc.invalidateQueries({ queryKey: ["health-checks"] });
    } catch (e) { pushToast(e instanceof Error ? e.message : "Failed", true); }
  };

  const up = checks.filter((c) => c.status === "UP").length;
  const down = checks.filter((c) => c.status === "DOWN").length;

  return (
    <div className="min-h-screen bg-abyss pt-20 pb-28 px-4 sm:px-6 max-w-4xl mx-auto">
      <div className="flex items-center justify-between mb-6">
        <div className="flex items-center gap-3">
          <Activity size={22} className="text-ember" />
          <h1 className="text-bone font-bold text-xl">Health Checks</h1>
          <span className="text-green-400 text-xs font-mono">{up} up</span>
          {down > 0 && <span className="text-red-400 text-xs font-mono">{down} down</span>}
        </div>
        <div className="flex gap-2">
          <button onClick={() => void refetch()}
            className="w-9 h-9 rounded-full bg-emboss flex items-center justify-center text-fog hover:text-bone">
            <RefreshCw size={15} />
          </button>
          <button onClick={() => setShowAdd(true)}
            className="flex items-center gap-1.5 px-3 py-2 bg-ember text-black text-sm font-semibold rounded-lg">
            <Plus size={15} /> Add Check
          </button>
        </div>
      </div>

      {isLoading && (
        <div className="space-y-3">
          {[1, 2, 3].map((i) => <div key={i} className="h-20 bg-panel rounded-xl animate-pulse" />)}
        </div>
      )}

      {!isLoading && checks.length === 0 && (
        <div className="text-center py-16 text-fog">
          <Activity size={40} className="mx-auto mb-3 opacity-30" />
          <p className="text-sm">No health checks yet. Add one to start monitoring.</p>
        </div>
      )}

      <div className="space-y-3">
        {checks.map((c) => (
          <CheckCard key={c.id} check={c}
            onDelete={() => void deleteCheck(c.id)}
            onToggle={() => void toggleCheck(c)} />
        ))}
      </div>

      {showAdd && (
        <AddCheckModal
          onClose={() => setShowAdd(false)}
          onCreated={() => void qc.invalidateQueries({ queryKey: ["health-checks"] })}
        />
      )}
    </div>
  );
}
