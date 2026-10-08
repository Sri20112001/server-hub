import { useEffect, useState } from "react";
import { useParams, useNavigate } from "react-router";
import { useQueries, useQuery, useQueryClient, keepPreviousData } from "@tanstack/react-query";
import { ArrowLeft, Copy, Key, RefreshCw, Trash2, Activity, ClipboardList, Server } from "lucide-react";
import { api, ApiError } from "../../lib/api";
import { useEvents } from "../../lib/useEvents";
import { useUi } from "../../stores/store";
import { fmtRate, fmtUptime } from "../../lib/format";
import { Kicker } from "../../components/ui";
import type { AgentToken, ServerMetricPoint } from "../../lib/types";
import type { ServerPromSeries } from "../../lib/api";

const RANGES = ["1h", "6h", "24h", "7d", "30d"] as const;
type Tab = "overview" | "monitoring" | "audit" | "tokens";

// ─── Prometheus history config (Phase 3C; backend builds the PromQL) ─────────
// Step per range stays well inside the backend bounds (15s ≤ step ≤ window).
const PROM_RANGES = ["1h", "6h", "24h", "7d"] as const;
const PROM_STEP: Record<string, string> = { "1h": "1m", "6h": "5m", "24h": "15m", "7d": "1h" };

interface PromMetricDef {
  key: string;
  label: string;
  unit: string;
  fixedMax: number | null;
  stroke: string;
  fill: string;
  format: (v: number) => string;
}

const PROM_METRICS: PromMetricDef[] = [
  { key: "cpu_usage", label: "CPU Usage", unit: "%", fixedMax: 100, stroke: "#EA580C", fill: "#EA580C", format: (v) => `${v.toFixed(1)}%` },
  { key: "memory_usage", label: "Memory Usage", unit: "%", fixedMax: 100, stroke: "#16A34A", fill: "#16A34A", format: (v) => `${v.toFixed(1)}%` },
  { key: "disk_usage", label: "Disk Usage", unit: "%", fixedMax: 100, stroke: "#D97706", fill: "#D97706", format: (v) => `${v.toFixed(1)}%` },
  { key: "load_1m", label: "Load Average", unit: "", fixedMax: null, stroke: "#7C3AED", fill: "#7C3AED", format: (v) => v.toFixed(2) },
  { key: "network_receive", label: "Network Receive", unit: "B/s", fixedMax: null, stroke: "#38BDF8", fill: "#38BDF8", format: (v) => fmtRate(v / 1024) },
  { key: "network_transmit", label: "Network Transmit", unit: "B/s", fixedMax: null, stroke: "#F472B6", fill: "#F472B6", format: (v) => fmtRate(v / 1024) },
];

// ─── Gap-aware SVG chart for Prometheus series (null = gap, never 0) ─────────
function PromChart({
  title, series, range, fixedMax, stroke, fill, unit, formatValue,
}: {
  title: string;
  series: ServerPromSeries[];
  range: string;
  fixedMax: number | null;
  stroke: string;
  fill: string;
  unit: string;
  formatValue: (v: number) => string;
}) {
  const W = 560; const H = 120;
  // Merge all series by timestamp; null values split the line into segments.
  const byTime = new Map<number, number | null>();
  for (const s of series) {
    for (const p of s.values) {
      if (!byTime.has(p.timestamp) || p.value != null) byTime.set(p.timestamp, p.value);
    }
  }
  const times = [...byTime.keys()].sort((a, b) => a - b);
  const valid = times.map((t) => byTime.get(t)).filter((v): v is number => v != null);
  if (valid.length === 0) {
    return (
      <div>
        <div className="flex items-center justify-between mb-1.5">
          <Kicker>{title}</Kicker>
          <span className="font-mono text-[13px] text-muted dark:text-fog">—</span>
        </div>
        <div className="w-full h-28 bg-paper dark:bg-abyss border border-line dark:border-edge rounded-xl flex items-center justify-center text-muted dark:text-fog text-xs">
          No monitoring data available for this range.
        </div>
      </div>
    );
  }
  const hi = fixedMax ?? Math.max(...valid, 1);
  const t0 = times[0]; const t1 = times[times.length - 1];
  const x = (t: number) => (t1 > t0 ? Math.round(((t - t0) / (t1 - t0)) * W) : 0);
  const y = (v: number) => Math.round(H - 10 - (Math.min(v, hi) / hi) * (H - 20));
  // Contiguous non-null runs become separate polylines (gaps stay gaps).
  const segments: string[] = [];
  let current: string[] = [];
  const flush = () => { if (current.length > 1) segments.push(current.join(" ")); else if (current.length === 1) segments.push(`${current[0]} ${current[0]}`); current = []; };
  for (const t of times) {
    const v = byTime.get(t);
    if (v == null) { flush(); continue; }
    current.push(`${x(t)},${y(v)}`);
  }
  flush();
  const area = segments.map((pts) => {
    const first = pts.split(" ")[0].split(",")[0];
    const last = pts.split(" ").at(-1)!.split(",")[0];
    return `${first},${H} ${pts} ${last},${H}`;
  });
  const last = valid[valid.length - 1];
  const fmtAxis = (t: number) => {
    const d = new Date(t * 1000);
    return range === "7d" ? `${d.toLocaleDateString()} ${d.toLocaleTimeString()}` : d.toLocaleTimeString();
  };
  return (
    <div>
      <div className="flex items-center justify-between mb-1.5">
        <Kicker>{title}</Kicker>
        <span className="font-mono text-[13px] text-ink dark:text-bone">{formatValue(last)}<span className="text-muted dark:text-fog text-[11px] ml-1">{unit}</span></span>
      </div>
      <svg viewBox={`0 0 ${W} ${H}`} className="w-full h-28 bg-paper dark:bg-abyss border border-line dark:border-edge rounded-xl" role="img" aria-label={`${title} chart`}>
        {[0.25, 0.5, 0.75].map((f) => (
          <line key={f} x1="0" x2={W} y1={H * f} y2={H * f} className="stroke-line dark:stroke-edge" strokeWidth="1" />
        ))}
        {segments.map((_pts, i) => (
          <polygon key={`a${i}`} points={area[i]} fill={fill} opacity="0.2" />
        ))}
        {segments.map((pts, i) => (
          <polyline key={`l${i}`} points={pts} fill="none" stroke={stroke} strokeWidth="2" strokeLinejoin="round" />
        ))}
      </svg>
      <div className="flex justify-between mt-1 font-mono text-[10px] text-muted dark:text-fog">
        <span>{fmtAxis(t0)}</span>
        <span>{fmtAxis(t1)}</span>
      </div>
    </div>
  );
}

function promErrorMessage(e: unknown): string {
  if (e instanceof ApiError) {
    if (e.status === 503) return "Prometheus monitoring is disabled.";
    if (e.status === 502 || e.status >= 500) return "Monitoring data is currently unavailable.";
  }
  return e instanceof Error ? e.message : "Failed to load monitoring data.";
}

// ─── Reusable SVG sparkline chart (same pattern as TelemetryPage) ─────────────
function MetricChart({
  title, points, value, stroke, fill, unit,
}: {
  title: string;
  points: ServerMetricPoint[];
  value: (p: ServerMetricPoint) => number;
  stroke: string;
  fill: string;
  unit: string;
}) {
  const W = 560; const H = 120;
  const vals = points.map(value);
  const hi = Math.max(...vals, 1);
  const step = points.length > 1 ? W / (points.length - 1) : 0;
  const line = vals.map((v, i) => {
    const x = Math.round(i * step);
    const y = Math.round(H - 10 - (Math.min(v, hi) / hi) * (H - 20));
    return `${x},${y}`;
  }).join(" ");
  const last = vals.length ? vals[vals.length - 1] : 0;
  return (
    <div>
      <div className="flex items-center justify-between mb-1.5">
        <Kicker>{title}</Kicker>
        <span className="font-mono text-[13px] text-ink dark:text-bone">{last.toFixed(1)}<span className="text-muted dark:text-fog text-[11px] ml-1">{unit}</span></span>
      </div>
      <svg viewBox={`0 0 ${W} ${H}`} className="w-full h-28 bg-paper dark:bg-abyss border border-line dark:border-edge rounded-xl" role="img" aria-label={`${title} chart`}>
        {[0.25, 0.5, 0.75].map((f) => (
          <line key={f} x1="0" x2={W} y1={H * f} y2={H * f} className="stroke-line dark:stroke-edge" strokeWidth="1" />
        ))}
        <polygon points={`0,${H} ${line} ${W},${H}`} fill={fill} opacity="0.2" />
        <polyline points={line} fill="none" stroke={stroke} strokeWidth="2" strokeLinejoin="round" />
      </svg>
      <div className="flex justify-between mt-1 font-mono text-[10px] text-muted dark:text-fog">
        <span>{points.length ? new Date(points[0].timestamp).toLocaleTimeString() : "—"}</span>
        <span>{points.length ? new Date(points[points.length - 1].timestamp).toLocaleTimeString() : "—"}</span>
      </div>
    </div>
  );
}

// ─── Metric bar ───────────────────────────────────────────────────────────────
function MetricBar({ label, pct, value }: { label: string; pct: number; value: string }) {
  const color = pct > 85 ? "bg-brick" : pct > 70 ? "bg-status-amber" : "bg-moss";
  return (
    <div className="mb-3">
      <div className="flex justify-between text-xs mb-1">
        <span className="text-muted dark:text-fog">{label}</span>
        <span className="text-ink dark:text-bone font-mono">{value}</span>
      </div>
      <div className="h-1.5 bg-paper dark:bg-abyss border border-line dark:border-edge rounded-full overflow-hidden">
        <div className={`h-full rounded-full transition-all ${color}`} style={{ width: `${Math.min(pct, 100)}%` }} />
      </div>
    </div>
  );
}

// ─── Token row ────────────────────────────────────────────────────────────────
function TokenRow({ token, serverId, onRevoked }: { token: AgentToken; serverId: number; onRevoked: () => void }) {
  const { pushToast } = useUi();
  const [busy, setBusy] = useState(false);
  const revoke = async () => {
    if (!confirm(`Revoke token "${token.label || token.id}"?`)) return;
    setBusy(true);
    try {
      await api.revokeAgentToken(serverId, token.id);
      pushToast("Token revoked");
      onRevoked();
    } catch (e) { pushToast(e instanceof Error ? e.message : "Failed", true); }
    finally { setBusy(false); }
  };
  return (
    <div className="flex items-center justify-between py-2 border-b border-line dark:border-edge last:border-0">
      <div>
        <div className="text-ink dark:text-bone text-sm font-medium">{token.label || `Token #${token.id}`}</div>
        <div className="text-muted dark:text-fog text-xs font-mono">
          Created {new Date(token.createdAt).toLocaleDateString()}
          {token.lastUsedAt && ` · Last used ${new Date(token.lastUsedAt).toLocaleDateString()}`}
        </div>
      </div>
      {token.revoked
        ? <span className="text-xs text-brick font-mono">revoked</span>
        : <button onClick={revoke} disabled={busy} className="text-muted dark:text-fog hover:text-brick p-1"><Trash2 size={14} /></button>}
    </div>
  );
}

// ─── Main page ────────────────────────────────────────────────────────────────
export function ServerDetailPage({ setOnline }: { setOnline: (v: boolean) => void }) {
  const { id } = useParams<{ id: string }>();
  const nav = useNavigate();
  const qc = useQueryClient();
  const { pushToast } = useUi();
  const serverId = Number(id);
  const [tab, setTab] = useState<Tab>("overview");
  const [range, setRange] = useState<string>("1h");
  const [newToken, setNewToken] = useState<string | null>(null);
  const [tokenLabel, setTokenLabel] = useState("");
  const [showTokenForm, setShowTokenForm] = useState(false);
  const [creatingToken, setCreatingToken] = useState(false);

  const { data: server, isLoading, isSuccess, isError } = useQuery({
    queryKey: ["server", serverId],
    queryFn: () => api.managedServer(serverId),
  });

  useEffect(() => {
    if (isSuccess) setOnline(true);
    else if (isError) setOnline(false);
  }, [isSuccess, isError, setOnline]);

  const { data: latest } = useQuery({
    queryKey: ["server-metrics-latest", serverId],
    queryFn: () => api.serverMetricsLatest(serverId),
    refetchInterval: 30_000,
  });

  const { data: metricsHistory } = useQuery({
    queryKey: ["server-metrics", serverId, range],
    queryFn: () => api.serverMetrics(serverId, range),
    enabled: tab === "monitoring",
  });

  // Prometheus history (Phase 3C): parallel per-metric range queries sharing
  // the page range selector. Enabled only on the monitoring tab, for ranges
  // Prometheus supports (30d stays agent-history only). Moderate stale time,
  // previous data kept across range switches, no aggressive polling.
  const promSupported = (PROM_RANGES as readonly string[]).includes(range);
  const promQueries = useQueries({
    queries: PROM_METRICS.map((m) => ({
      queryKey: ["server-prometheus", serverId, m.key, range, PROM_STEP[range]],
      queryFn: () => api.serverPrometheusMetrics(serverId, m.key, range, PROM_STEP[range]),
      enabled: tab === "monitoring" && promSupported && !!server && Number.isFinite(serverId),
      staleTime: 60_000,
      placeholderData: keepPreviousData,
    })),
  });

  const { data: tokens = [], refetch: refetchTokens } = useQuery({
    queryKey: ["agent-tokens", serverId],
    queryFn: () => api.agentTokens(serverId),
    enabled: tab === "tokens",
  });

  const { data: auditLogs = [] } = useQuery({
    queryKey: ["audit-server", serverId],
    queryFn: () => api.audit(50),
    enabled: tab === "audit",
  });

  const { data: alerts = [] } = useQuery({
    queryKey: ["alerts", "TRIGGERED"],
    queryFn: () => api.alerts("TRIGGERED"),
    refetchInterval: 30_000,
  });

  const serverAlerts = alerts.filter((a) => a.serverId === serverId);

  useEvents((ev) => {
    if ((ev.type === "server.heartbeat" || ev.type === "server.metrics") &&
      (ev.data as { serverId?: number })?.serverId === serverId) {
      void qc.invalidateQueries({ queryKey: ["server", serverId] });
      void qc.invalidateQueries({ queryKey: ["server-metrics-latest", serverId] });
    }
    if (ev.type === "alert.triggered" || ev.type === "alert.resolved") {
      void qc.invalidateQueries({ queryKey: ["alerts"] });
    }
  });

  const createToken = async () => {
    setCreatingToken(true);
    try {
      const res = await api.createAgentToken(serverId, tokenLabel);
      setNewToken(res.token);
      setTokenLabel("");
      setShowTokenForm(false);
      void refetchTokens();
    } catch (e) { pushToast(e instanceof Error ? e.message : "Failed", true); }
    finally { setCreatingToken(false); }
  };

  const copyToken = () => {
    if (newToken) { void navigator.clipboard.writeText(newToken); pushToast("Token copied"); }
  };

  if (isLoading) return (
    <main className="w-full max-w-5xl mx-auto px-4 sm:px-6 pt-24 pb-36">
      <div className="animate-pulse space-y-4">
        <div className="h-8 bg-white dark:bg-panel border border-line dark:border-edge rounded w-48" />
        <div className="h-40 bg-white dark:bg-panel border border-line dark:border-edge rounded-card" />
      </div>
    </main>
  );

  if (!server) return (
    <main className="w-full max-w-5xl mx-auto px-4 sm:px-6 pt-24 pb-36 text-muted dark:text-fog">
      Server not found.
    </main>
  );

  const STATUS_COLOR: Record<string, string> = {
    ONLINE: "text-moss", OFFLINE: "text-brick", WARNING: "text-status-amber", UNKNOWN: "text-stone",
  };
  const STATUS_DOT: Record<string, string> = {
    ONLINE: "bg-moss", OFFLINE: "bg-brick", WARNING: "bg-status-amber", UNKNOWN: "bg-stone",
  };

  const points = metricsHistory?.points ?? [];

  const TABS: { key: Tab; label: string; icon: React.ReactNode }[] = [
    { key: "overview", label: "Overview", icon: <Server size={14} /> },
    { key: "monitoring", label: "Monitoring", icon: <Activity size={14} /> },
    { key: "audit", label: "Audit", icon: <ClipboardList size={14} /> },
    { key: "tokens", label: "Tokens", icon: <Key size={14} /> },
  ];

  return (
    <main className="w-full max-w-5xl mx-auto px-4 sm:px-6 pt-24 pb-36">
      {/* Back */}
      <button onClick={() => nav("/servers")} className="flex items-center gap-1.5 text-muted dark:text-fog hover:text-ink dark:hover:text-bone text-sm mb-4 cursor-pointer">
        <ArrowLeft size={15} /> Servers
      </button>

      {/* Header */}
      <div className="flex items-start justify-between mb-4">
        <div>
          <div className="flex items-center gap-2 mb-1">
            <span className={`w-2.5 h-2.5 rounded-full ${STATUS_DOT[server.status] ?? "bg-stone"}`} />
            <h1 className="text-ink dark:text-bone font-head font-bold text-xl">{server.name}</h1>
            <span className={`text-sm font-mono ${STATUS_COLOR[server.status] ?? "text-muted dark:text-fog"}`}>{server.status}</span>
          </div>
          <div className="flex items-center gap-2 text-muted dark:text-fog text-sm flex-wrap">
            {server.hostname && <span>{server.hostname}</span>}
            {server.ipAddress && <span>· {server.ipAddress}</span>}
            {server.os && <span>· {server.os} {server.osVersion}</span>}
          </div>
        </div>
        <button onClick={() => {
          void qc.invalidateQueries({ queryKey: ["server", serverId] });
          void qc.invalidateQueries({ queryKey: ["server-prometheus", serverId] });
        }}
          className="w-9 h-9 rounded-full bg-white dark:bg-panel border border-line dark:border-edge flex items-center justify-center text-muted dark:text-fog hover:text-ink dark:hover:text-bone shadow-chrome cursor-pointer"
          aria-label="Refresh"
        >
          <RefreshCw size={15} />
        </button>
      </div>

      {/* Active alerts banner */}
      {serverAlerts.length > 0 && (
        <div className="mb-4 space-y-2">
          {serverAlerts.map((a) => (
            <div key={a.id} className={`flex items-center justify-between px-4 py-2 rounded-card border text-sm ${
              a.severity === "CRITICAL" ? "border-brick/30 bg-red-50 dark:bg-red-950/30 text-brick dark:text-red-200" : "border-status-amber/30 bg-amber-50 dark:bg-amber-950/30 text-status-amber dark:text-amber-200"
            }`}>
              <span>{a.message}</span>
              <button onClick={async () => { await api.resolveAlert(a.id); void qc.invalidateQueries({ queryKey: ["alerts"] }); }}
                className="text-xs underline opacity-70 hover:opacity-100 ml-4 cursor-pointer">Resolve</button>
            </div>
          ))}
        </div>
      )}

      {/* Tabs */}
      <div className="flex gap-1 mb-6 bg-paper dark:bg-abyss border border-line dark:border-edge rounded-xl p-1 w-fit">
        {TABS.map((t) => (
          <button key={t.key} onClick={() => setTab(t.key)}
            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-mono transition-colors cursor-pointer ${
              tab === t.key ? "bg-white dark:bg-panel border border-line dark:border-edge text-ink dark:text-bone font-semibold shadow-sm" : "text-muted dark:text-fog hover:text-ink dark:hover:text-bone"
            }`}>
            {t.icon}{t.label}
          </button>
        ))}
      </div>

      {/* ── Overview tab ── */}
      {tab === "overview" && (
        <div className="space-y-4">
          {/* System info grid */}
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
            {[
              { label: "OS", value: server.os ? `${server.os} ${server.osVersion}` : "—" },
              { label: "Arch", value: server.arch || "—" },
              { label: "CPU", value: server.cpuCores > 0 ? `${server.cpuCores} cores` : "—" },
              { label: "RAM", value: server.ramTotal > 0 ? `${Math.round(server.ramTotal / 1024 / 1024 / 1024)}GB` : "—" },
            ].map((item) => (
              <div key={item.label} className="bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-3 shadow-chrome">
                <div className="text-muted dark:text-fog text-[10px] uppercase tracking-wider">{item.label}</div>
                <div className="text-ink dark:text-bone font-mono text-sm mt-0.5 truncate">{item.value}</div>
              </div>
            ))}
          </div>

          {/* Live metrics */}
          {latest ? (
            <div className="bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-4 shadow-chrome">
              <div className="flex items-center justify-between mb-4">
                <h2 className="text-ink dark:text-bone font-semibold text-sm">Live Metrics</h2>
                <span className="text-muted dark:text-fog text-xs font-mono">{new Date(latest.timestamp).toLocaleTimeString()}</span>
              </div>
              <MetricBar label="CPU" pct={latest.cpuUsage} value={`${latest.cpuUsage.toFixed(1)}%`} />
              <MetricBar label="Memory" pct={latest.memoryUsage} value={`${latest.memoryUsage.toFixed(1)}% · ${latest.memoryUsedMB.toFixed(0)}MB`} />
              <MetricBar label="Disk" pct={latest.diskUsage} value={`${latest.diskUsage.toFixed(1)}% · ${latest.diskUsedGB.toFixed(1)}GB`} />
              <div className="grid grid-cols-3 gap-3 mt-3 text-xs">
                {[
                  { label: "Load Avg", value: latest.loadAvg1.toFixed(2) },
                  { label: "Uptime", value: fmtUptime(latest.uptimeSec) },
                  { label: "Agent", value: server.agentStatus },
                ].map((s) => (
                  <div key={s.label} className="bg-paper dark:bg-abyss border border-line dark:border-edge rounded-lg p-2">
                    <div className="text-muted dark:text-fog text-[10px]">{s.label}</div>
                    <div className="text-ink dark:text-bone font-mono">{s.value}</div>
                  </div>
                ))}
              </div>
            </div>
          ) : (
            <div className="bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-6 text-center text-muted dark:text-fog text-sm">
              No metrics yet. Install and start the agent to begin monitoring.
            </div>
          )}

          {/* Last heartbeat */}
          <div className="bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-4 text-sm shadow-chrome">
            <div className="flex justify-between">
              <span className="text-muted dark:text-fog">Last heartbeat</span>
              <span className="text-ink dark:text-bone font-mono">
                {server.lastHeartbeat ? new Date(server.lastHeartbeat).toLocaleString() : "Never"}
              </span>
            </div>
            <div className="flex justify-between mt-2">
              <span className="text-muted dark:text-fog">Agent status</span>
              <span className={`font-mono ${server.agentStatus === "CONNECTED" ? "text-moss" : "text-stone"}`}>
                {server.agentStatus}
              </span>
            </div>
            <div className="flex justify-between mt-2">
              <span className="text-muted dark:text-fog">Registered</span>
              <span className="text-ink dark:text-bone font-mono">{new Date(server.createdAt).toLocaleDateString()}</span>
            </div>
          </div>
        </div>
      )}

      {/* ── Monitoring tab ── */}
      {tab === "monitoring" && (
        <div className="space-y-5">
          <div className="flex gap-1">
            {RANGES.map((r) => (
              <button key={r} onClick={() => setRange(r)}
                className={`px-2.5 py-1 rounded-lg text-xs font-mono transition-colors cursor-pointer border ${
                  range === r
                    ? "bg-accent dark:bg-ember text-white dark:text-black font-semibold border-accent dark:border-ember"
                    : "bg-white dark:bg-panel border-line dark:border-edge text-muted dark:text-fog hover:text-ink dark:hover:text-bone"
                }`}>
                {r}
              </button>
            ))}
          </div>
          {points.length === 0 ? (
            <div className="bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-8 text-center text-muted dark:text-fog text-sm">
              No metrics for this range. The agent must be running to collect data.
            </div>
          ) : (
            <>
              <MetricChart title="CPU Usage" points={points} value={(p) => p.cpuUsage} stroke="#EA580C" fill="#EA580C" unit="%" />
              <MetricChart title="Memory Usage" points={points} value={(p) => p.memoryUsage} stroke="#16A34A" fill="#16A34A" unit="%" />
              <MetricChart title="Disk Usage" points={points} value={(p) => p.diskUsage} stroke="#D97706" fill="#D97706" unit="%" />
              <MetricChart title="Load Average" points={points} value={(p) => p.loadAvg1} stroke="#7C3AED" fill="#7C3AED" unit="" />
            </>
          )}

          {/* ── Prometheus history (Phase 3C; additive, agent charts above untouched) ── */}
          <div className="pt-2">
            <div className="flex items-center justify-between mb-3">
              <Kicker>Prometheus History</Kicker>
              <span className="font-mono text-[11px] text-muted dark:text-fog">via Prometheus</span>
            </div>
            {!promSupported ? (
              <div className="bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-6 text-center text-muted dark:text-fog text-sm">
                Prometheus history supports ranges up to 7d — switch range for Prometheus charts.
              </div>
            ) : (
              <div className="space-y-5">
                {PROM_METRICS.map((m, i) => {
                  const q = promQueries[i];
                  if (q.isError) {
                    return (
                      <div key={m.key}>
                        <div className="flex items-center justify-between mb-1.5">
                          <Kicker>{m.label}</Kicker>
                        </div>
                        <div className="w-full bg-white dark:bg-panel border border-line dark:border-edge rounded-card px-4 py-6 text-center text-muted dark:text-fog text-xs">
                          {promErrorMessage(q.error)}
                        </div>
                      </div>
                    );
                  }
                  if (q.isPending) {
                    return (
                      <div key={m.key}>
                        <div className="flex items-center justify-between mb-1.5">
                          <Kicker>{m.label}</Kicker>
                        </div>
                        <div className="w-full h-28 bg-white dark:bg-panel border border-line dark:border-edge rounded-card animate-pulse" role="status" aria-label={`${m.label} loading`} />
                      </div>
                    );
                  }
                  return (
                    <PromChart
                      key={m.key}
                      title={m.label}
                      series={q.data?.series ?? []}
                      range={range}
                      fixedMax={m.fixedMax}
                      stroke={m.stroke}
                      fill={m.fill}
                      unit={m.unit}
                      formatValue={m.format}
                    />
                  );
                })}
              </div>
            )}
          </div>
        </div>
      )}

      {/* ── Audit tab ── */}
      {tab === "audit" && (
        <div className="bg-white dark:bg-panel border border-line dark:border-edge rounded-card overflow-hidden shadow-chrome">
          <table className="w-full text-sm border-collapse">
            <thead>
              <tr className="bg-paper dark:bg-abyss">
                <th className="text-left text-muted dark:text-fog text-xs px-4 py-2.5 font-semibold uppercase tracking-wider">Time</th>
                <th className="text-left text-muted dark:text-fog text-xs px-4 py-2.5 font-semibold uppercase tracking-wider">Actor</th>
                <th className="text-left text-muted dark:text-fog text-xs px-4 py-2.5 font-semibold uppercase tracking-wider">Action</th>
                <th className="text-left text-muted dark:text-fog text-xs px-4 py-2.5 font-semibold uppercase tracking-wider hidden sm:table-cell">Result</th>
              </tr>
            </thead>
            <tbody>
              {auditLogs.length === 0 ? (
                <tr><td colSpan={4} className="px-4 py-8 text-center text-muted dark:text-fog text-sm">No audit records.</td></tr>
              ) : auditLogs.map((log) => (
                <tr key={log.id} className="border-t border-line dark:border-edge hover:bg-paper dark:hover:bg-emboss/50">
                  <td className="px-4 py-2.5 font-mono text-xs text-muted dark:text-fog whitespace-nowrap">
                    {new Date(log.timestamp).toLocaleString()}
                  </td>
                  <td className="px-4 py-2.5 font-mono text-xs text-ink dark:text-bone">{log.actor}</td>
                  <td className="px-4 py-2.5 font-mono text-xs text-ink dark:text-bone">{log.action} {log.resource}</td>
                  <td className={`px-4 py-2.5 font-mono text-xs hidden sm:table-cell ${log.result === "ok" ? "text-moss" : "text-brick"}`}>
                    {log.result}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* ── Tokens tab ── */}
      {tab === "tokens" && (
        <div className="bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-4 shadow-chrome">
          <div className="flex items-center justify-between mb-3">
            <div className="flex items-center gap-2">
              <Key size={15} className="text-muted dark:text-fog" />
              <h2 className="text-ink dark:text-bone font-semibold text-sm">Agent Tokens</h2>
            </div>
            <button onClick={() => setShowTokenForm(!showTokenForm)} className="text-xs text-accent dark:text-ember hover:underline font-medium cursor-pointer">
              + Generate
            </button>
          </div>

          {showTokenForm && (
            <div className="mb-3 flex gap-2">
              <input className="flex-1 bg-white dark:bg-panel border border-line dark:border-edge rounded-input px-3 py-1.5 text-ink dark:text-bone text-sm outline-none focus:border-accent dark:focus:border-ember font-mono placeholder:text-stone dark:placeholder:text-fog"
                placeholder="Token label (optional)" value={tokenLabel} onChange={(e) => setTokenLabel(e.target.value)} />
              <button onClick={createToken} disabled={creatingToken}
                className="px-3 py-1.5 bg-accent dark:bg-ember text-white dark:text-black text-sm font-semibold rounded-input hover:bg-accent-hover dark:hover:bg-ember-hover disabled:opacity-50 cursor-pointer">
                {creatingToken ? "…" : "Create"}
              </button>
            </div>
          )}

          {newToken && (
            <div className="mb-3 p-3 bg-amber-50 dark:bg-amber-950/20 border border-status-amber/40 rounded-card">
              <p className="text-status-amber text-xs mb-2 font-semibold">Copy this token now — it will not be shown again.</p>
              <div className="flex items-center gap-2">
                <code className="text-ink dark:text-bone text-xs font-mono flex-1 break-all bg-white dark:bg-panel px-2 py-1 rounded border border-line dark:border-edge">{newToken}</code>
                <button onClick={copyToken} className="text-muted dark:text-fog hover:text-ink dark:hover:text-bone shrink-0 cursor-pointer p-1"><Copy size={14} /></button>
              </div>
              <button onClick={() => setNewToken(null)} className="text-xs text-muted dark:text-fog hover:text-ink dark:hover:text-bone mt-2 underline cursor-pointer">Dismiss</button>
            </div>
          )}

          {tokens.length === 0
            ? <p className="text-muted dark:text-fog text-xs">No tokens yet. Generate one to connect an agent.</p>
            : tokens.map((t) => <TokenRow key={t.id} token={t} serverId={serverId} onRevoked={() => void refetchTokens()} />)}

          <div className="mt-4 p-3 bg-paper dark:bg-abyss border border-line dark:border-edge rounded-xl text-xs text-muted dark:text-fog font-mono">
            <p className="mb-1 text-ink dark:text-bone font-medium">Agent install (Linux):</p>
            <p>go build -o serverhub-agent ./cmd/agent</p>
            <p className="mt-1">AGENT_TOKEN=&lt;token&gt; SERVERHUB_URL=http://&lt;host&gt;:4000 ./serverhub-agent</p>
          </div>
        </div>
      )}
    </main>
  );
}
