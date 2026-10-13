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
    <main className="w-full max-w-7xl mx-auto px-10 max-md:px-4 pt-24 pb-36">
      <div className="animate-pulse space-y-4">
        <div className="h-8 bg-white dark:bg-panel border border-line dark:border-edge rounded w-48" />
        <div className="h-40 bg-white dark:bg-panel border border-line dark:border-edge rounded-card" />
      </div>
    </main>
  );

  if (!server) return (
    <main className="w-full max-w-7xl mx-auto px-10 max-md:px-4 pt-24 pb-36 text-muted dark:text-fog">
      Server not found.
    </main>
  );

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
    <main className="w-full max-w-5xl mx-auto px-6 pt-12 pb-28">
      {/* Back button */}
      <button
        onClick={() => nav("/servers")}
        className="inline-flex items-center gap-1.5 text-on-surface-variant hover:text-on-surface text-[13px] font-medium mb-6 cursor-pointer border-0 bg-transparent"
        type="button"
      >
        <ArrowLeft size={16} />
        <span>Back to Servers</span>
      </button>

      {/* In-Canvas Title Area */}
      <div className="flex flex-col sm:flex-row sm:items-end justify-between gap-4 mb-6">
        <div>
          <div className="flex items-center gap-2 mb-1.5 flex-wrap">
            <span className={`w-2.5 h-2.5 rounded-full ${STATUS_DOT[server.status] ?? "bg-stone"}`} />
            <span className="text-[11px] font-semibold text-outline uppercase tracking-wider">Node Telemetry</span>
            <span className={`text-[11px] font-mono uppercase tracking-wider font-semibold px-2.5 py-0.5 rounded-full border ${
              server.status === "ONLINE"
                ? "border-secondary/30 bg-secondary/10 text-secondary"
                : server.status === "OFFLINE"
                ? "border-error/30 bg-error/10 text-error"
                : server.status === "WARNING"
                ? "border-tertiary/30 bg-tertiary/10 text-tertiary"
                : "border-outline-variant/30 bg-surface-container-low text-outline"
            }`}>
              {server.status}
            </span>
          </div>
          <h1 className="text-[32px] sm:text-[36px] font-bold text-on-surface tracking-tight leading-tight">{server.name}</h1>
          <div className="flex items-center gap-2 text-on-surface-variant font-mono text-[13px] mt-1 flex-wrap">
            {server.hostname && <span>{server.hostname}</span>}
            {server.ipAddress && <span>· {server.ipAddress}</span>}
            {server.os && <span>· {server.os} {server.osVersion}</span>}
          </div>
        </div>
        <button
          onClick={() => {
            void qc.invalidateQueries({ queryKey: ["server", serverId] });
            void qc.invalidateQueries({ queryKey: ["server-prometheus", serverId] });
          }}
          className="w-10 h-10 rounded-full bg-surface-container-lowest border border-outline-variant/30 flex items-center justify-center text-on-surface-variant hover:text-on-surface shadow-sm cursor-pointer transition-colors"
          aria-label="Refresh"
          type="button"
        >
          <RefreshCw size={15} />
        </button>
      </div>

      {/* Active alerts banner */}
      {serverAlerts.length > 0 && (
        <div className="mb-6 space-y-2">
          {serverAlerts.map((a) => (
            <div
              key={a.id}
              className={`flex items-center justify-between px-5 py-3 rounded-2xl border text-[13px] shadow-sm ${
                a.severity === "CRITICAL"
                  ? "border-error/30 bg-error-container/20 text-error"
                  : "border-tertiary/30 bg-tertiary-container/20 text-tertiary"
              }`}
            >
              <span className="font-semibold">{a.message}</span>
              <button
                onClick={async () => {
                  await api.resolveAlert(a.id);
                  void qc.invalidateQueries({ queryKey: ["alerts"] });
                }}
                className="text-[12px] underline hover:no-underline ml-4 cursor-pointer font-medium border-0 bg-transparent"
                type="button"
              >
                Resolve
              </button>
            </div>
          ))}
        </div>
      )}

      {/* Contextual In-Page Navigation Pills */}
      <div className="flex items-center overflow-x-auto pb-1 gap-1 p-1 bg-surface-container rounded-full max-w-fit shadow-inner mb-6" role="tablist">
        {TABS.map((t) => (
          <button
            key={t.key}
            role="tab"
            aria-selected={tab === t.key}
            onClick={() => setTab(t.key)}
            className={`flex items-center gap-1.5 px-4 py-1.5 rounded-full text-[13px] font-medium transition-all cursor-pointer border-0 whitespace-nowrap ${
              tab === t.key
                ? "bg-surface-container-lowest text-primary font-semibold shadow-sm"
                : "text-on-surface-variant hover:text-on-surface hover:bg-surface-container-high"
            }`}
          >
            {t.icon}
            <span>{t.label}</span>
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
              <div key={item.label} className="bg-surface-container-lowest rounded-2xl p-4 shadow-sm border border-outline-variant/20">
                <div className="text-on-surface-variant text-[11px] uppercase tracking-wider font-semibold">{item.label}</div>
                <div className="text-on-surface font-mono text-[14px] font-semibold mt-1 truncate">{item.value}</div>
              </div>
            ))}
          </div>

          {/* Live metrics */}
          {latest ? (
            <div className="bg-surface-container-lowest rounded-2xl p-6 shadow-sm border border-outline-variant/20">
              <div className="flex items-center justify-between mb-4">
                <h2 className="text-on-surface font-semibold text-[15px]">Live Metrics</h2>
                <span className="text-on-surface-variant text-[12px] font-mono">{new Date(latest.timestamp).toLocaleTimeString()}</span>
              </div>
              <MetricBar label="CPU" pct={latest.cpuUsage} value={`${latest.cpuUsage.toFixed(1)}%`} />
              <MetricBar label="Memory" pct={latest.memoryUsage} value={`${latest.memoryUsage.toFixed(1)}% · ${latest.memoryUsedMB.toFixed(0)}MB`} />
              <MetricBar label="Disk" pct={latest.diskUsage} value={`${latest.diskUsage.toFixed(1)}% · ${latest.diskUsedGB.toFixed(1)}GB`} />
              <div className="grid grid-cols-3 gap-3 mt-4 text-xs">
                {[
                  { label: "Load Avg", value: latest.loadAvg1.toFixed(2) },
                  { label: "Uptime", value: fmtUptime(latest.uptimeSec) },
                  { label: "Agent", value: server.agentStatus },
                ].map((s) => (
                  <div key={s.label} className="bg-surface-container-low border border-outline-variant/20 rounded-xl p-3">
                    <div className="text-on-surface-variant text-[10px] uppercase tracking-wider font-semibold">{s.label}</div>
                    <div className="text-on-surface font-mono font-semibold mt-0.5">{s.value}</div>
                  </div>
                ))}
              </div>
            </div>
          ) : (
            <div className="bg-surface-container-lowest border border-outline-variant/20 rounded-2xl p-8 text-center text-on-surface-variant text-sm shadow-sm">
              No metrics yet. Install and start the agent to begin monitoring.
            </div>
          )}

          {/* Last heartbeat */}
          <div className="bg-surface-container-lowest border border-outline-variant/20 rounded-2xl p-5 text-sm shadow-sm space-y-3">
            <div className="flex justify-between items-center">
              <span className="text-on-surface-variant">Last heartbeat</span>
              <span className="text-on-surface font-mono font-medium">
                {server.lastHeartbeat ? new Date(server.lastHeartbeat).toLocaleString() : "Never"}
              </span>
            </div>
            <div className="flex justify-between items-center border-t border-outline-variant/10 pt-2.5">
              <span className="text-on-surface-variant">Agent status</span>
              <span className={`font-mono font-semibold ${server.agentStatus === "CONNECTED" ? "text-primary" : "text-outline"}`}>
                {server.agentStatus}
              </span>
            </div>
            <div className="flex justify-between items-center border-t border-outline-variant/10 pt-2.5">
              <span className="text-on-surface-variant">Registered</span>
              <span className="text-on-surface font-mono">{new Date(server.createdAt).toLocaleDateString()}</span>
            </div>
          </div>
        </div>
      )}

      {/* ── Monitoring tab ── */}
      {tab === "monitoring" && (
        <div className="space-y-6">
          <div className="flex items-center gap-1 p-1 rounded-full bg-surface-container w-full sm:max-w-fit shadow-inner">
            {RANGES.map((r) => (
              <button key={r} onClick={() => setRange(r)}
                className={`px-3.5 py-1.5 rounded-full text-xs font-mono transition-all cursor-pointer border-0 ${
                  range === r
                    ? "bg-surface-container-lowest text-primary font-semibold shadow-sm"
                    : "text-on-surface-variant hover:text-on-surface hover:bg-surface-container-high"
                }`}>
                {r}
              </button>
            ))}
          </div>

          {points.length === 0 ? (
            <div className="bg-surface-container-lowest border border-outline-variant/20 rounded-2xl p-8 text-center text-on-surface-variant text-sm shadow-sm">
              No metrics for this range. The agent must be running to collect data.
            </div>
          ) : (
            <div className="bg-surface-container-lowest border border-outline-variant/20 rounded-2xl p-6 shadow-sm space-y-6">
              <MetricChart title="CPU Usage" points={points} value={(p) => p.cpuUsage} stroke="#EA580C" fill="#EA580C" unit="%" />
              <MetricChart title="Memory Usage" points={points} value={(p) => p.memoryUsage} stroke="#16A34A" fill="#16A34A" unit="%" />
              <MetricChart title="Disk Usage" points={points} value={(p) => p.diskUsage} stroke="#D97706" fill="#D97706" unit="%" />
              <MetricChart title="Load Average" points={points} value={(p) => p.loadAvg1} stroke="#7C3AED" fill="#7C3AED" unit="" />
            </div>
          )}

          {/* ── Prometheus history (Phase 3C; additive, agent charts above untouched) ── */}
          <div className="pt-2">
            <div className="flex items-center justify-between mb-3">
              <Kicker>Prometheus History</Kicker>
              <span className="font-mono text-[11px] text-on-surface-variant">via Prometheus</span>
            </div>
            {!promSupported ? (
              <div className="bg-surface-container-lowest border border-outline-variant/20 rounded-2xl p-6 text-center text-on-surface-variant text-sm shadow-sm">
                Prometheus history supports ranges up to 7d — switch range for Prometheus charts.
              </div>
            ) : (
              <div className="bg-surface-container-lowest border border-outline-variant/20 rounded-2xl p-6 shadow-sm space-y-6">
                {PROM_METRICS.map((m, i) => {
                  const q = promQueries[i];
                  if (q.isError) {
                    return (
                      <div key={m.key}>
                        <div className="flex items-center justify-between mb-1.5">
                          <Kicker>{m.label}</Kicker>
                        </div>
                        <div className="w-full bg-surface-container-low border border-outline-variant/20 rounded-2xl px-4 py-6 text-center text-on-surface-variant text-xs">
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
                        <div className="w-full h-28 bg-surface-container-low border border-outline-variant/20 rounded-2xl animate-pulse" role="status" aria-label={`${m.label} loading`} />
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
        <div className="bg-surface-container-lowest border border-outline-variant/20 rounded-2xl overflow-hidden shadow-sm">
          <div className="overflow-x-auto">
            <table className="w-full text-sm border-collapse">
              <thead>
                <tr className="bg-surface-container-low/50 border-b border-outline-variant/20">
                  <th className="text-left text-on-surface-variant text-[11px] px-4 py-3 font-semibold uppercase tracking-wider">Time</th>
                  <th className="text-left text-on-surface-variant text-[11px] px-4 py-3 font-semibold uppercase tracking-wider">Actor</th>
                  <th className="text-left text-on-surface-variant text-[11px] px-4 py-3 font-semibold uppercase tracking-wider">Action</th>
                  <th className="text-left text-on-surface-variant text-[11px] px-4 py-3 font-semibold uppercase tracking-wider hidden sm:table-cell">Result</th>
                </tr>
              </thead>
              <tbody>
                {auditLogs.length === 0 ? (
                  <tr><td colSpan={4} className="px-4 py-8 text-center text-on-surface-variant text-sm">No audit records.</td></tr>
                ) : auditLogs.map((log) => (
                  <tr key={log.id} className="border-t border-outline-variant/10 hover:bg-surface-container-low/40 transition-colors">
                    <td className="px-4 py-3 font-mono text-xs text-on-surface-variant whitespace-nowrap">
                      {new Date(log.timestamp).toLocaleString()}
                    </td>
                    <td className="px-4 py-3 font-mono text-xs text-on-surface font-semibold">{log.actor}</td>
                    <td className="px-4 py-3 font-mono text-xs text-on-surface">{log.action} {log.resource}</td>
                    <td className={`px-4 py-3 font-mono text-xs hidden sm:table-cell font-semibold ${log.result === "ok" ? "text-primary" : "text-error"}`}>
                      {log.result}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* ── Tokens tab ── */}
      {tab === "tokens" && (
        <div className="bg-surface-container-lowest border border-outline-variant/20 rounded-2xl p-6 shadow-sm">
          <div className="flex items-center justify-between mb-4">
            <div className="flex items-center gap-2">
              <Key size={16} className="text-primary" />
              <h2 className="text-on-surface font-semibold text-base">Agent Tokens</h2>
            </div>
            <button onClick={() => setShowTokenForm(!showTokenForm)} className="text-xs text-primary hover:underline font-semibold cursor-pointer">
              + Generate Token
            </button>
          </div>

          {showTokenForm && (
            <div className="mb-4 p-4 bg-surface-container-low border border-outline-variant/20 rounded-2xl flex gap-2.5">
              <input className="flex-1 bg-surface-container-lowest border border-outline-variant/30 rounded-xl px-3.5 py-2 text-on-surface text-sm outline-none focus:border-primary font-mono placeholder:text-on-surface-variant/40 transition-colors"
                placeholder="Token label (e.g. prod-primary-agent)" value={tokenLabel} onChange={(e) => setTokenLabel(e.target.value)} />
              <button onClick={createToken} disabled={creatingToken}
                className="px-5 py-2 bg-primary text-on-primary text-sm font-semibold rounded-full hover:bg-primary/90 disabled:opacity-50 cursor-pointer shadow-sm transition-all">
                {creatingToken ? "…" : "Create"}
              </button>
            </div>
          )}

          {newToken && (
            <div className="mb-4 p-4 bg-tertiary-container/15 border border-tertiary/30 rounded-2xl shadow-sm">
              <p className="text-tertiary text-xs mb-2 font-semibold">Copy this token now — it will not be shown again.</p>
              <div className="flex items-center gap-2">
                <code className="text-on-surface text-xs font-mono flex-1 break-all bg-surface-container-lowest px-3.5 py-2 rounded-xl border border-outline-variant/30">{newToken}</code>
                <button onClick={copyToken} className="text-on-surface-variant hover:text-on-surface shrink-0 cursor-pointer p-2 rounded-full bg-surface-container-lowest border border-outline-variant/30 shadow-sm transition-colors"><Copy size={14} /></button>
              </div>
              <button onClick={() => setNewToken(null)} className="text-xs text-on-surface-variant hover:text-on-surface mt-2 underline cursor-pointer">Dismiss</button>
            </div>
          )}

          <div className="divide-y divide-outline-variant/10">
            {tokens.length === 0
              ? <p className="text-on-surface-variant text-xs py-6 text-center">No tokens yet. Generate one to connect an agent.</p>
              : tokens.map((t) => <TokenRow key={t.id} token={t} serverId={serverId} onRevoked={() => void refetchTokens()} />)}
          </div>

          <div className="mt-6 p-4 bg-surface-container-low border border-outline-variant/20 rounded-2xl text-xs text-on-surface-variant font-mono">
            <p className="mb-1 text-on-surface font-semibold">Agent install (Linux):</p>
            <p className="text-[11px] selection:bg-primary/20">go build -o serverhub-agent ./cmd/agent</p>
            <p className="mt-1 text-[11px] selection:bg-primary/20">AGENT_TOKEN=&lt;token&gt; SERVERHUB_URL=http://&lt;host&gt;:4000 ./serverhub-agent</p>
          </div>
        </div>
      )}
    </main>
  );
}
