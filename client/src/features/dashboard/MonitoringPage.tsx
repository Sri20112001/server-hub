import { useCallback, useEffect, useState } from "react";
import { Activity, AlertTriangle, CheckCircle, RefreshCw, Server, Wifi, WifiOff, XCircle } from "lucide-react";
import { api } from "../../lib/api";
import type { AmAlert, AmSilence, MonitoringOverview, PromRangeResult, PromTarget, PromTargetsData } from "../../lib/types";
import { Kicker, Modal, Field } from "../../components/ui";
import { useEvents } from "../../lib/useEvents";
import type { BusEvent } from "../../lib/types";

type Tab = "dashboard" | "alerts" | "targets" | "silences" | "metrics";
const TABS: { id: Tab; label: string }[] = [
  { id: "dashboard", label: "Dashboard" },
  { id: "alerts", label: "Alerts" },
  { id: "targets", label: "Targets" },
  { id: "silences", label: "Silences" },
  { id: "metrics", label: "Metrics" },
];

function StatusDot({ ok, label }: { ok: boolean | undefined; label: string }) {
  if (ok === undefined) return (
    <span className="inline-flex items-center gap-1.5 font-mono text-[12px] text-muted dark:text-fog">
      <span className="w-2 h-2 rounded-full bg-stone" />{label}
    </span>
  );
  return (
    <span className="inline-flex items-center gap-1.5 font-mono text-[12px]">
      <span className={`w-2 h-2 rounded-full ${ok ? "bg-moss" : "bg-brick"}`} />
      {label}
    </span>
  );
}

function MetricBar({ label, value }: { label: string; value: number | null }) {
  if (value === null || value < 0) return (
    <div className="my-2">
      <div className="flex justify-between items-baseline text-xs mb-1">
        <span>{label}</span><span className="font-mono text-muted dark:text-fog">—</span>
      </div>
      <div className="h-1.5 rounded-full bg-paper dark:bg-abyss border border-line dark:border-edge" />
    </div>
  );
  const pct = Math.max(0, Math.min(100, value));
  const tone = pct >= 90 ? "bg-brick" : pct >= 70 ? "bg-accent dark:bg-ember" : "bg-ink dark:bg-bone";
  const w = Math.round(pct / 5) * 5;
  return (
    <div className="my-2">
      <div className="flex justify-between items-baseline text-xs mb-1">
        <span>{label}</span><span className="font-mono">{pct.toFixed(1)}%</span>
      </div>
      <div className="h-1.5 rounded-full bg-paper dark:bg-abyss border border-line dark:border-edge overflow-hidden">
        <div className={`h-full rounded-full ${tone}`} style={{ width: `${w}%` }} />
      </div>
    </div>
  );
}

function fmtBytes(b: number | null): string {
  if (b === null || b < 0) return "—";
  if (b < 1024) return `${b.toFixed(0)} B/s`;
  if (b < 1024 * 1024) return `${(b / 1024).toFixed(1)} KB/s`;
  return `${(b / 1024 / 1024).toFixed(1)} MB/s`;
}

function fmtDuration(start: string): string {
  const ms = Date.now() - new Date(start).getTime();
  const s = Math.floor(ms / 1000);
  if (s < 60) return `${s}s`;
  if (s < 3600) return `${Math.floor(s / 60)}m`;
  return `${Math.floor(s / 3600)}h`;
}

function severityColor(sev: string | undefined): string {
  switch ((sev ?? "").toLowerCase()) {
    case "critical": return "text-brick";
    case "warning": return "text-status-amber";
    default: return "text-muted dark:text-fog";
  }
}

function SvgChart({ data, color }: { data: PromRangeResult | null; color: string }) {
  if (!data || !data.result?.length) return (
    <div className="h-28 bg-paper dark:bg-abyss border border-line dark:border-edge rounded-xl flex items-center justify-center text-muted dark:text-fog text-[12px]">
      No data
    </div>
  );
  const series = data.result[0].values;
  if (!series.length) return null;
  const W = 560; const H = 112;
  const vals = series.map(([, v]) => parseFloat(v));
  const hi = Math.max(...vals, 1);
  const step = series.length > 1 ? W / (series.length - 1) : 0;
  const pts = vals.map((v, i) => {
    const x = Math.round(i * step);
    const y = Math.round(H - 8 - (Math.min(v, hi) / hi) * (H - 16));
    return `${x},${y}`;
  }).join(" ");
  return (
    <svg viewBox={`0 0 ${W} ${H}`} className="w-full h-28 bg-paper dark:bg-abyss border border-line dark:border-edge rounded-xl">
      {[0.25, 0.5, 0.75].map(f => (
        <line key={f} x1="0" x2={W} y1={H * f} y2={H * f} className="stroke-line dark:stroke-edge" strokeWidth="1" />
      ))}
      <polygon points={`0,${H} ${pts} ${W},${H}`} fill={color} opacity="0.2" />
      <polyline points={pts} fill="none" stroke={color} strokeWidth="2" strokeLinejoin="round" />
    </svg>
  );
}

// ── Dashboard tab ─────────────────────────────────────────────────────────────

function DashboardTab() {
  const [overview, setOverview] = useState<MonitoringOverview | null>(null);
  const [promOk, setPromOk] = useState<boolean | undefined>(undefined);
  const [amOk, setAmOk] = useState<boolean | undefined>(undefined);
  const [alerts, setAlerts] = useState<AmAlert[]>([]);
  const [targets, setTargets] = useState<PromTargetsData | null>(null);
  const [loading, setLoading] = useState(true);

  const load = useCallback(() => {
    setLoading(true);
    Promise.allSettled([
      api.monitoringOverview().then(setOverview),
      api.prometheusStatus().then(r => setPromOk(r.healthy ?? r.available)),
      api.alertmanagerStatus().then(r => setAmOk(r.healthy ?? r.available)),
      api.monitoringAlerts().then(setAlerts).catch(() => setAlerts([])),
      api.prometheusTargets().then(r => setTargets(r.data)).catch(() => {}),
    ]).finally(() => setLoading(false));
  }, []);

  useEffect(() => { load(); }, [load]);

  useEvents((ev: BusEvent) => {
    if (ev.type === "monitoring.alert.firing" || ev.type === "monitoring.alert.resolved") {
      api.monitoringAlerts().then(setAlerts).catch(() => {});
    }
  });

  const firing = alerts.filter(a => a.status.state === "active");
  const upTargets = targets ? targets.activeTargets.filter(t => t.health === "up").length : null;
  const totalTargets = targets ? targets.activeTargets.length : null;

  return (
    <div className="flex flex-col gap-6">
      {/* Status row */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <div className="bg-white dark:bg-panel border border-line dark:border-edge rounded-xl p-4">
          <Kicker>Prometheus</Kicker>
          <div className="mt-2 flex items-center gap-2">
            {promOk === undefined ? <WifiOff size={16} className="text-muted dark:text-fog" /> :
              promOk ? <Wifi size={16} className="text-moss" /> : <WifiOff size={16} className="text-brick" />}
            <StatusDot ok={promOk} label={promOk === undefined ? "Checking…" : promOk ? "Connected" : "Unavailable"} />
          </div>
          {targets !== null && (
            <div className="mt-2 font-mono text-[12px] text-muted dark:text-fog">
              Targets: {upTargets} / {totalTargets} up
            </div>
          )}
        </div>
        <div className="bg-white dark:bg-panel border border-line dark:border-edge rounded-xl p-4">
          <Kicker>Alertmanager</Kicker>
          <div className="mt-2 flex items-center gap-2">
            {amOk === undefined ? <WifiOff size={16} className="text-muted dark:text-fog" /> :
              amOk ? <Wifi size={16} className="text-moss" /> : <WifiOff size={16} className="text-brick" />}
            <StatusDot ok={amOk} label={amOk === undefined ? "Checking…" : amOk ? "Connected" : "Unavailable"} />
          </div>
          <div className="mt-2 font-mono text-[12px] text-muted dark:text-fog">
            Active alerts: {firing.length}
          </div>
        </div>
      </div>

      {/* Resource overview */}
      {loading ? (
        <div className="text-muted dark:text-fog text-[13px]">Loading…</div>
      ) : overview && overview.available ? (
        <div className="bg-white dark:bg-panel border border-line dark:border-edge rounded-xl p-4">
          <Kicker>Resources</Kicker>
          <div className="mt-3 grid grid-cols-1 gap-1 sm:grid-cols-2">
            <MetricBar label="CPU" value={overview.cpu} />
            <MetricBar label="Memory" value={overview.memory} />
            <MetricBar label="Disk" value={overview.disk} />
            <div className="my-2">
              <div className="flex justify-between items-baseline text-xs mb-1">
                <span>Network ↓</span>
                <span className="font-mono">{fmtBytes(overview.networkRx)}</span>
              </div>
              <div className="flex justify-between items-baseline text-xs mb-1">
                <span>Network ↑</span>
                <span className="font-mono">{fmtBytes(overview.networkTx)}</span>
              </div>
            </div>
          </div>
        </div>
      ) : (
        <div className="bg-paper dark:bg-abyss border border-line dark:border-edge rounded-xl px-4 py-6 text-center text-muted dark:text-fog text-[13px]">
          Prometheus not configured — set PROMETHEUS_URL to enable metrics.
        </div>
      )}

      {/* Active alerts */}
      {firing.length > 0 && (
        <div className="bg-white dark:bg-panel border border-line dark:border-edge rounded-xl p-4">
          <Kicker>Active Alerts ({firing.length})</Kicker>
          <div className="mt-3 flex flex-col gap-2">
            {firing.slice(0, 10).map(a => (
              <div key={a.fingerprint} className="flex items-start gap-2.5 py-2 border-t border-line dark:border-edge first:border-0">
                <AlertTriangle size={14} className={`mt-0.5 shrink-0 ${severityColor(a.labels.severity)}`} />
                <div className="min-w-0">
                  <div className="font-medium text-[13px]">{a.labels.alertname ?? "Alert"}</div>
                  <div className="font-mono text-[11px] text-muted dark:text-fog truncate">
                    {a.labels.instance ?? a.labels.job ?? ""} · {fmtDuration(a.startsAt)} ago
                  </div>
                  {a.annotations.summary && (
                    <div className="text-[12px] text-muted dark:text-fog mt-0.5">{a.annotations.summary}</div>
                  )}
                </div>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

// ── Alerts tab ────────────────────────────────────────────────────────────────

function AlertsTab() {
  const [alerts, setAlerts] = useState<AmAlert[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [silencing, setSilencing] = useState<AmAlert | null>(null);
  const [silenceDur, setSilenceDur] = useState("1h");
  const [silenceComment, setSilenceComment] = useState("");
  const [busy, setBusy] = useState(false);

  const load = useCallback(() => {
    setLoading(true);
    api.monitoringAlerts()
      .then(setAlerts)
      .catch(e => setError(e.message))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => { load(); }, [load]);

  useEvents((ev: BusEvent) => {
    if (ev.type === "monitoring.alert.firing" || ev.type === "monitoring.alert.resolved") load();
  });

  const doSilence = async () => {
    if (!silencing) return;
    setBusy(true);
    const durs: Record<string, number> = { "30m": 30, "1h": 60, "4h": 240, "custom": 60 };
    const mins = durs[silenceDur] ?? 60;
    const now = new Date();
    const end = new Date(now.getTime() + mins * 60000);
    const matchers = Object.entries(silencing.labels).map(([name, value]) => ({
      name, value, isRegex: false, isEqual: true,
    }));
    try {
      await api.createSilence({
        matchers,
        startsAt: now.toISOString(),
        endsAt: end.toISOString(),
        createdBy: "serverhub",
        comment: silenceComment || `Silenced via ServerHub for ${silenceDur}`,
      });
      setSilencing(null);
      setSilenceComment("");
    } catch (e: unknown) {
      alert((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  if (loading) return <div className="text-muted dark:text-fog text-[13px]">Loading…</div>;
  if (error) return (
    <div className="bg-paper dark:bg-abyss border border-line dark:border-edge rounded-xl px-4 py-6 text-center">
      <div className="text-brick font-medium mb-1">Alertmanager unavailable</div>
      <div className="text-muted dark:text-fog text-[13px]">{error}</div>
    </div>
  );

  const firing = alerts.filter(a => a.status.state === "active");
  const suppressed = alerts.filter(a => a.status.state === "suppressed");

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between gap-4">
        <div className="font-mono text-[12px] text-muted dark:text-fog">
          {firing.length} firing · {suppressed.length} suppressed · {alerts.length} total
        </div>
        <button onClick={load} className="inline-flex items-center gap-1.5 text-[12px] text-muted dark:text-fog hover:text-ink dark:hover:text-bone cursor-pointer bg-transparent border-0">
          <RefreshCw size={13} /> Refresh
        </button>
      </div>

      {alerts.length === 0 ? (
        <div className="bg-paper dark:bg-abyss border border-line dark:border-edge rounded-xl px-4 py-8 text-center">
          <CheckCircle size={24} className="text-moss mx-auto mb-2" />
          <div className="font-medium">No active alerts</div>
          <div className="text-muted dark:text-fog text-[13px] mt-1">All systems nominal.</div>
        </div>
      ) : (
        <div className="bg-white dark:bg-panel border border-line dark:border-edge rounded-xl overflow-hidden">
          {alerts.map((a, i) => (
            <div key={a.fingerprint} className={`px-4 py-3.5 flex items-start gap-3 ${i > 0 ? "border-t border-line dark:border-edge" : ""}`}>
              <div className="mt-0.5 shrink-0">
                {a.status.state === "active"
                  ? <XCircle size={15} className={severityColor(a.labels.severity)} />
                  : <CheckCircle size={15} className="text-muted dark:text-fog" />}
              </div>
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2 flex-wrap">
                  <span className="font-medium text-[13px]">{a.labels.alertname ?? "Alert"}</span>
                  <span className={`font-mono text-[11px] uppercase ${severityColor(a.labels.severity)}`}>
                    {a.labels.severity ?? ""}
                  </span>
                  <span className="font-mono text-[11px] text-muted dark:text-fog">
                    {a.status.state}
                  </span>
                </div>
                {a.labels.instance && (
                  <div className="font-mono text-[11px] text-muted dark:text-fog mt-0.5">{a.labels.instance}</div>
                )}
                {a.annotations.summary && (
                  <div className="text-[12px] text-muted dark:text-fog mt-0.5">{a.annotations.summary}</div>
                )}
                <div className="font-mono text-[11px] text-muted dark:text-fog mt-1">
                  Started {fmtDuration(a.startsAt)} ago
                </div>
              </div>
              {a.status.state === "active" && (
                <button
                  onClick={() => setSilencing(a)}
                  className="shrink-0 text-[12px] px-2.5 py-1 rounded-lg border border-line dark:border-edge bg-paper dark:bg-abyss hover:bg-white dark:hover:bg-panel cursor-pointer"
                >
                  Silence
                </button>
              )}
            </div>
          ))}
        </div>
      )}

      {silencing && (
        <Modal onClose={() => setSilencing(null)}>
          <h3 className="text-[18px] mb-1">Silence alert</h3>
          <p className="text-muted dark:text-fog text-[13px] mb-4">
            {silencing.labels.alertname} · {silencing.labels.instance ?? ""}
          </p>
          <Field label="Duration">
            <div className="grid grid-cols-4 gap-1 p-1 rounded-xl bg-paper dark:bg-abyss border border-line dark:border-edge">
              {["30m", "1h", "4h", "custom"].map(d => (
                <button key={d} onClick={() => setSilenceDur(d)}
                  className={`px-2 py-1.5 rounded-lg font-mono text-[12px] cursor-pointer border transition-colors ${silenceDur === d ? "bg-white dark:bg-panel border-line dark:border-edge" : "bg-transparent border-transparent text-muted dark:text-fog"}`}>
                  {d}
                </button>
              ))}
            </div>
          </Field>
          <div className="mt-3">
            <Field label="Comment (optional)">
              <input
                className="w-full bg-white dark:bg-panel border border-line dark:border-edge rounded-lg text-[13px] px-3 py-2 outline-none focus:border-accent dark:focus:border-ember"
                value={silenceComment}
                onChange={e => setSilenceComment(e.target.value)}
                placeholder="Reason for silence…"
              />
            </Field>
          </div>
          <div className="flex gap-2 justify-end mt-4">
            <button onClick={() => setSilencing(null)} className="px-4 py-2 text-[13px] rounded-lg border border-line dark:border-edge bg-white dark:bg-panel cursor-pointer">Cancel</button>
            <button onClick={doSilence} disabled={busy} className="px-4 py-2 text-[13px] rounded-lg bg-accent-deep dark:bg-ember text-white dark:text-black cursor-pointer disabled:opacity-50">
              {busy ? "Creating…" : "Create silence"}
            </button>
          </div>
        </Modal>
      )}
    </div>
  );
}

// ── Targets tab ───────────────────────────────────────────────────────────────

function TargetsTab() {
  const [targets, setTargets] = useState<PromTargetsData | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const load = useCallback(() => {
    api.prometheusTargets()
      .then(r => { setTargets(r.data); setError(""); })
      .catch(e => setError(e.message))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => { load(); }, [load]);

  useEvents((ev: BusEvent) => {
    if (ev.type === "monitoring.alert.firing" || ev.type === "monitoring.alert.resolved") {
      load();
    }
  });

  if (loading) return <div className="text-muted dark:text-fog text-[13px]">Loading…</div>;
  if (error) return (
    <div className="bg-paper dark:bg-abyss border border-line dark:border-edge rounded-xl px-4 py-6 text-center">
      <div className="text-brick font-medium mb-1">Prometheus unavailable</div>
      <div className="text-muted dark:text-fog text-[13px]">{error}</div>
    </div>
  );
  if (!targets) return null;

  const all: (PromTarget & { pool: string })[] = targets.activeTargets.map(t => ({ ...t, pool: t.scrapePool }));
  const up = all.filter(t => t.health === "up").length;

  return (
    <div className="flex flex-col gap-4">
      <div className="font-mono text-[12px] text-muted dark:text-fog">
        {up} / {all.length} targets up
      </div>
      <div className="bg-white dark:bg-panel border border-line dark:border-edge rounded-xl overflow-hidden">
        <table className="w-full border-collapse text-[13px]">
          <thead>
            <tr>
              {["Job", "Instance", "Health", "Last scrape", "Duration", "Error"].map(h => (
                <th key={h} className="text-left text-[11px] font-semibold tracking-[0.08em] uppercase text-muted dark:text-fog px-4 py-2.5 bg-paper dark:bg-abyss">{h}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {all.map((t, i) => (
              <tr key={i} className="group">
                <td className="px-4 py-2.5 border-t border-line dark:border-edge font-mono">{t.labels.job ?? t.pool}</td>
                <td className="px-4 py-2.5 border-t border-line dark:border-edge font-mono text-[12px]">{t.labels.instance ?? "—"}</td>
                <td className="px-4 py-2.5 border-t border-line dark:border-edge">
                  <span className={`inline-flex items-center gap-1.5 font-mono text-[11px]`}>
                    <span className={`w-1.5 h-1.5 rounded-full ${t.health === "up" ? "bg-moss" : "bg-brick"}`} />
                    {t.health.toUpperCase()}
                  </span>
                </td>
                <td className="px-4 py-2.5 border-t border-line dark:border-edge font-mono text-[12px] text-muted dark:text-fog">
                  {t.lastScrape ? `${Math.round((Date.now() - new Date(t.lastScrape).getTime()) / 1000)}s ago` : "—"}
                </td>
                <td className="px-4 py-2.5 border-t border-line dark:border-edge font-mono text-[12px] text-muted dark:text-fog">
                  {t.lastScrapeDuration ? `${(t.lastScrapeDuration * 1000).toFixed(0)}ms` : "—"}
                </td>
                <td className="px-4 py-2.5 border-t border-line dark:border-edge font-mono text-[11px] text-brick max-w-[200px] truncate group-hover:bg-paper dark:group-hover:bg-emboss">
                  {t.lastError || "—"}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

// ── Silences tab ──────────────────────────────────────────────────────────────

function SilencesTab() {
  const [silences, setSilences] = useState<AmSilence[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [deleting, setDeleting] = useState<string | null>(null);

  const load = useCallback(() => {
    setLoading(true);
    api.monitoringSilences()
      .then(setSilences)
      .catch(e => setError(e.message))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => { load(); }, [load]);

  useEvents((ev: BusEvent) => {
    if (ev.type === "monitoring.alert.firing" || ev.type === "monitoring.alert.resolved") {
      load();
    }
  });

  const doDelete = async (id: string) => {
    setDeleting(id);
    try {
      await api.deleteSilence(id);
      load();
    } catch (e: unknown) {
      alert((e as Error).message);
    } finally {
      setDeleting(null);
    }
  };

  if (loading) return <div className="text-muted dark:text-fog text-[13px]">Loading…</div>;
  if (error) return (
    <div className="bg-paper dark:bg-abyss border border-line dark:border-edge rounded-xl px-4 py-6 text-center">
      <div className="text-brick font-medium mb-1">Alertmanager unavailable</div>
      <div className="text-muted dark:text-fog text-[13px]">{error}</div>
    </div>
  );

  const active = silences.filter(s => s.status.state === "active");
  const expired = silences.filter(s => s.status.state !== "active");

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between gap-4">
        <div className="font-mono text-[12px] text-muted dark:text-fog">{active.length} active · {expired.length} expired</div>
        <button onClick={load} className="inline-flex items-center gap-1.5 text-[12px] text-muted dark:text-fog hover:text-ink dark:hover:text-bone cursor-pointer bg-transparent border-0">
          <RefreshCw size={13} /> Refresh
        </button>
      </div>

      {silences.length === 0 ? (
        <div className="bg-paper dark:bg-abyss border border-line dark:border-edge rounded-xl px-4 py-8 text-center text-muted dark:text-fog text-[13px]">
          No silences configured.
        </div>
      ) : (
        <div className="bg-white dark:bg-panel border border-line dark:border-edge rounded-xl overflow-hidden">
          {silences.map((s, i) => (
            <div key={s.id} className={`px-4 py-3.5 flex items-start gap-3 ${i > 0 ? "border-t border-line dark:border-edge" : ""}`}>
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2 flex-wrap">
                  <span className={`font-mono text-[11px] uppercase ${s.status.state === "active" ? "text-moss" : "text-muted dark:text-fog"}`}>
                    {s.status.state}
                  </span>
                  <span className="font-mono text-[11px] text-muted dark:text-fog">by {s.createdBy}</span>
                </div>
                <div className="text-[13px] mt-0.5">{s.comment || "—"}</div>
                <div className="flex flex-wrap gap-1.5 mt-1.5">
                  {s.matchers.map((m, j) => (
                    <span key={j} className="font-mono text-[11px] bg-paper dark:bg-abyss border border-line dark:border-edge rounded px-1.5 py-0.5">
                      {m.name}{m.isRegex ? "=~" : "="}{m.value}
                    </span>
                  ))}
                </div>
                <div className="font-mono text-[11px] text-muted dark:text-fog mt-1">
                  {new Date(s.startsAt).toLocaleString()} → {new Date(s.endsAt).toLocaleString()}
                </div>
              </div>
              {s.status.state === "active" && (
                <button
                  onClick={() => doDelete(s.id)}
                  disabled={deleting === s.id}
                  className="shrink-0 text-[12px] px-2.5 py-1 rounded-lg border border-brick text-brick hover:bg-red-50 dark:hover:bg-red-950 cursor-pointer disabled:opacity-50"
                >
                  {deleting === s.id ? "…" : "Expire"}
                </button>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

// ── Metrics tab ───────────────────────────────────────────────────────────────

const METRIC_OPTIONS = [
  { id: "cpu", label: "CPU", color: "#EA580C" },
  { id: "memory", label: "Memory", color: "#16A34A" },
  { id: "disk", label: "Disk", color: "#D97706" },
  { id: "networkRx", label: "Network ↓", color: "#0E7490" },
  { id: "networkTx", label: "Network ↑", color: "#7C3AED" },
] as const;

const RANGES = ["1h", "6h", "24h", "7d"] as const;

function MetricsTab() {
  const [metric, setMetric] = useState<string>("cpu");
  const [range, setRange] = useState<string>("1h");
  const [data, setData] = useState<PromRangeResult | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    setLoading(true);
    setError("");
    api.monitoringMetrics(metric, range)
      .then(r => setData(r.data))
      .catch(e => { setError(e.message); setData(null); })
      .finally(() => setLoading(false));
  }, [metric, range]);

  const color = METRIC_OPTIONS.find(m => m.id === metric)?.color ?? "#EA580C";
  const lastVal = data?.result?.[0]?.values?.at(-1)?.[1];
  const isPercent = metric === "cpu" || metric === "memory" || metric === "disk";

  return (
    <div className="flex flex-col gap-5">
      <div className="flex items-center gap-3 flex-wrap">
        <div className="grid grid-cols-5 gap-1 p-1 rounded-xl bg-paper dark:bg-abyss border border-line dark:border-edge">
          {METRIC_OPTIONS.map(m => (
            <button key={m.id} onClick={() => setMetric(m.id)}
              className={`px-3 py-1.5 rounded-lg font-mono text-[12px] cursor-pointer border transition-colors ${metric === m.id ? "bg-white dark:bg-panel border-line dark:border-edge" : "bg-transparent border-transparent text-muted dark:text-fog"}`}>
              {m.label}
            </button>
          ))}
        </div>
        <div className="grid grid-cols-4 gap-1 p-1 rounded-xl bg-paper dark:bg-abyss border border-line dark:border-edge">
          {RANGES.map(r => (
            <button key={r} onClick={() => setRange(r)}
              className={`px-3 py-1.5 rounded-lg font-mono text-[12px] cursor-pointer border transition-colors ${range === r ? "bg-white dark:bg-panel border-line dark:border-edge" : "bg-transparent border-transparent text-muted dark:text-fog"}`}>
              {r}
            </button>
          ))}
        </div>
      </div>

      {error ? (
        <div className="bg-paper dark:bg-abyss border border-line dark:border-edge rounded-xl px-4 py-6 text-center">
          <div className="text-brick font-medium mb-1">Prometheus unavailable</div>
          <div className="text-muted dark:text-fog text-[13px]">{error}</div>
        </div>
      ) : (
        <div>
          <div className="flex items-center justify-between gap-4 mb-1.5">
            <Kicker>{METRIC_OPTIONS.find(m => m.id === metric)?.label ?? metric}</Kicker>
            {lastVal && (
              <span className="font-mono text-[13px]">
                {isPercent ? `${parseFloat(lastVal).toFixed(1)}%` : fmtBytes(parseFloat(lastVal))}
              </span>
            )}
          </div>
          {loading ? (
            <div className="h-28 bg-paper dark:bg-abyss border border-line dark:border-edge rounded-xl flex items-center justify-center text-muted dark:text-fog text-[12px]">
              Loading…
            </div>
          ) : (
            <SvgChart data={data} color={color} />
          )}
        </div>
      )}
    </div>
  );
}

// ── Main page ─────────────────────────────────────────────────────────────────

export function MonitoringPage() {
  const [tab, setTab] = useState<Tab>("dashboard");

  return (
    <main className="w-full max-w-7xl mx-auto px-10 max-md:px-4 pt-24 pb-36">
      <div className="flex items-end justify-between gap-4 flex-wrap mb-8">
        <div>
          <h1 className="font-head text-[32px] font-bold tracking-[-0.03em] leading-[1.2] max-md:text-[26px]">
            Monitoring.
          </h1>
          <p className="text-muted dark:text-fog mt-1.5">
            Prometheus metrics · Alertmanager alerts
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Server size={14} className="text-muted dark:text-fog" />
          <Activity size={14} className="text-muted dark:text-fog" />
        </div>
      </div>

      <div className="flex items-center gap-1 mb-6 border-b border-line dark:border-edge">
        {TABS.map(t => (
          <button
            key={t.id}
            onClick={() => setTab(t.id)}
            className={`px-4 py-2.5 text-[13px] font-medium border-b-2 -mb-px cursor-pointer bg-transparent transition-colors ${
              tab === t.id
                ? "border-accent dark:border-ember text-ink dark:text-bone"
                : "border-transparent text-muted dark:text-fog hover:text-ink dark:hover:text-bone"
            }`}
          >
            {t.label}
          </button>
        ))}
      </div>

      {tab === "dashboard" && <DashboardTab />}
      {tab === "alerts" && <AlertsTab />}
      {tab === "targets" && <TargetsTab />}
      {tab === "silences" && <SilencesTab />}
      {tab === "metrics" && <MetricsTab />}
    </main>
  );
}
