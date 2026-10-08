import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Activity, Plus, RefreshCw, Trash2, ChevronDown, ChevronUp } from "lucide-react";
import { api } from "../../lib/api";
import type { HealthCheck } from "../../lib/types";
import { useUi } from "../../stores/store";

const STATUS_DOT: Record<string, string> = {
  UP: "bg-moss", DOWN: "bg-brick", UNKNOWN: "bg-stone",
};

function ResultsPanel({ checkId }: { checkId: number }) {
  const { data: results = [] } = useQuery({
    queryKey: ["hc-results", checkId],
    queryFn: () => api.healthCheckResults(checkId),
    refetchInterval: 30_000,
  });
  return (
    <div className="mt-3 border-t border-line dark:border-edge pt-3">
      <div className="text-muted dark:text-fog text-xs mb-2">Last 10 results</div>
      <div className="flex gap-1 flex-wrap">
        {results.slice(0, 10).map((r) => (
          <div key={r.id} title={`${r.status} · ${r.responseTimeMs}ms · ${new Date(r.timestamp).toLocaleString()}`}
            className={`w-5 h-5 rounded ${r.status === "UP" ? "bg-moss/70" : "bg-brick/70"}`} />
        ))}
        {results.length === 0 && <span className="text-muted dark:text-fog text-xs">No results yet.</span>}
      </div>
      {results[0] && (
        <div className="text-muted dark:text-fog text-xs mt-1 font-mono">
          Last: {results[0].responseTimeMs}ms · {new Date(results[0].timestamp).toLocaleString()}
          {results[0].error && <span className="text-brick ml-2">{results[0].error}</span>}
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
    <div className="bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-5 shadow-sm hover:shadow-chrome transition-all">
      <div className="flex items-start justify-between">
        <div className="flex items-center gap-2.5 flex-1 min-w-0">
          <span className={`w-2.5 h-2.5 rounded-full shrink-0 ${STATUS_DOT[check.status] ?? "bg-stone"}`} />
          <div className="min-w-0">
            <div className="text-ink dark:text-bone font-semibold text-sm truncate">{check.name}</div>
            <div className="text-muted dark:text-fog text-xs font-mono truncate mt-0.5">{check.type.toUpperCase()} · {check.target}</div>
          </div>
        </div>
        <div className="flex items-center gap-2.5 ml-2 shrink-0">
          <span className={`text-[11px] font-mono font-bold px-2 py-0.5 rounded border ${
            check.status === "UP" ? "bg-moss/10 border-moss/30 text-moss" : check.status === "DOWN" ? "bg-brick/10 border-brick/30 text-brick" : "bg-paper dark:bg-abyss border-line dark:border-edge text-stone"
          }`}>{check.status}</span>
          {check.responseTimeMs != null && (
            <span className="text-muted dark:text-fog text-xs font-mono font-medium">{check.responseTimeMs}ms</span>
          )}
          <button onClick={onToggle}
            className={`text-xs px-2.5 py-1 rounded-lg border font-mono font-semibold transition-colors cursor-pointer ${check.enabled ? "border-moss/40 text-moss bg-moss/10" : "border-line dark:border-edge text-muted dark:text-fog bg-paper dark:bg-abyss"}`}>
            {check.enabled ? "ON" : "OFF"}
          </button>
          <button onClick={() => setExpanded(!expanded)} className="text-muted dark:text-fog hover:text-ink dark:hover:text-bone p-1 cursor-pointer">
            {expanded ? <ChevronUp size={15} /> : <ChevronDown size={15} />}
          </button>
          <button onClick={onDelete} className="text-muted dark:text-fog hover:text-brick p-1 cursor-pointer"><Trash2 size={15} /></button>
        </div>
      </div>
      <div className="text-muted dark:text-fog text-xs mt-2 font-mono">
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
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-[rgba(28,25,23,0.28)] dark:bg-[rgba(0,0,0,0.55)] p-4" onClick={onClose}>
      <div className="bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-6 w-full max-w-md shadow-chrome" onClick={(e) => e.stopPropagation()}>
        <h2 className="text-ink dark:text-bone font-head font-bold text-lg mb-4">Add Health Check</h2>
        <div className="space-y-3">
          <Field label="Name *" value={form.name} onChange={(v) => set("name", v)} placeholder="API health" />
          <div>
            <label className="text-muted dark:text-fog text-xs mb-1 block">Type</label>
            <select className="w-full bg-white dark:bg-panel border border-line dark:border-edge rounded-lg px-3 py-2 text-ink dark:text-bone text-sm outline-none focus:border-accent dark:focus:border-ember"
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
              <label className="text-muted dark:text-fog text-xs mb-1 block">Interval (s)</label>
              <input type="number" className="w-full bg-white dark:bg-panel border border-line dark:border-edge rounded-lg px-3 py-2 text-ink dark:text-bone text-sm outline-none focus:border-accent dark:focus:border-ember font-mono"
                value={form.interval} onChange={(e) => set("interval", Number(e.target.value))} />
            </div>
            <div>
              <label className="text-muted dark:text-fog text-xs mb-1 block">Timeout (s)</label>
              <input type="number" className="w-full bg-white dark:bg-panel border border-line dark:border-edge rounded-lg px-3 py-2 text-ink dark:text-bone text-sm outline-none focus:border-accent dark:focus:border-ember font-mono"
                value={form.timeout} onChange={(e) => set("timeout", Number(e.target.value))} />
            </div>
            {form.type === "http" && (
              <div>
                <label className="text-muted dark:text-fog text-xs mb-1 block">Expected</label>
                <input type="number" className="w-full bg-white dark:bg-panel border border-line dark:border-edge rounded-lg px-3 py-2 text-ink dark:text-bone text-sm outline-none focus:border-accent dark:focus:border-ember font-mono"
                  value={form.expectedStatus} onChange={(e) => set("expectedStatus", Number(e.target.value))} />
              </div>
            )}
          </div>
        </div>
        <div className="flex gap-2 mt-5 justify-end">
          <button onClick={onClose} className="px-4 py-2 text-sm text-muted dark:text-fog hover:text-ink dark:hover:text-bone border border-line dark:border-edge rounded-lg cursor-pointer">Cancel</button>
          <button onClick={submit} disabled={busy || !form.name || !form.target}
            className="px-4 py-2 text-sm bg-accent dark:bg-ember text-white dark:text-black font-semibold rounded-lg disabled:opacity-50 hover:bg-accent-hover dark:hover:bg-ember-hover transition-colors cursor-pointer">
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
      <label className="text-muted dark:text-fog text-xs mb-1 block">{label}</label>
      <input className="w-full bg-white dark:bg-panel border border-line dark:border-edge rounded-lg px-3 py-2 text-ink dark:text-bone text-sm outline-none focus:border-accent dark:focus:border-ember font-mono"
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
    <main className="w-full max-w-4xl mx-auto px-4 sm:px-6 pt-24 pb-36">
      <div className="flex items-center justify-between mb-6 flex-wrap gap-4">
        <div>
          <div className="flex items-center gap-3">
            <Activity size={22} className="text-accent dark:text-ember" />
            <h1 className="text-ink dark:text-bone font-head font-bold text-[24px] tracking-[-0.02em]">Health Checks</h1>
            <span className="text-moss text-xs font-mono font-semibold px-2 py-0.5 rounded border border-moss/30 bg-moss/10">{up} up</span>
            {down > 0 && <span className="text-brick text-xs font-mono font-semibold px-2 py-0.5 rounded border border-brick/30 bg-brick/10">{down} down</span>}
          </div>
          <p className="text-muted dark:text-fog text-xs mt-1 font-mono">
            Synthetic uptime monitoring · Probe intervals
          </p>
        </div>
        <div className="flex gap-2.5">
          <button onClick={() => void refetch()}
            className="w-9 h-9 rounded-full bg-white dark:bg-panel border border-line dark:border-edge flex items-center justify-center text-muted dark:text-fog hover:text-ink dark:hover:text-bone transition-colors shadow-sm cursor-pointer"
            aria-label="Refresh"
          >
            <RefreshCw size={15} />
          </button>
          <button onClick={() => setShowAdd(true)}
            className="flex items-center gap-1.5 px-3.5 py-2 bg-accent dark:bg-ember text-white dark:text-black text-sm font-semibold rounded-lg hover:bg-accent-hover dark:hover:bg-ember-hover transition-colors shadow-sm cursor-pointer">
            <Plus size={15} /> Add Check
          </button>
        </div>
      </div>

      {isLoading && (
        <div className="space-y-3">
          {[1, 2, 3].map((i) => <div key={i} className="h-24 bg-white dark:bg-panel border border-line dark:border-edge rounded-card animate-pulse shadow-sm" />)}
        </div>
      )}

      {!isLoading && checks.length === 0 && (
        <div className="text-center py-16 bg-paper dark:bg-abyss border border-line dark:border-edge rounded-card px-4">
          <Activity size={40} className="mx-auto mb-3 opacity-30 text-accent dark:text-ember" />
          <p className="text-sm font-medium text-ink dark:text-bone">No health checks yet. Add one to start monitoring.</p>
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
    </main>
  );
}
