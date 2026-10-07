import { useEffect, useState } from "react";
import { useParams, useNavigate } from "react-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, Copy, Key, RefreshCw, Trash2, Activity, ClipboardList, Server } from "lucide-react";
import { api } from "../../lib/api";
import { useEvents } from "../../lib/useEvents";
import { useUi } from "../../stores/store";
import { fmtUptime } from "../../lib/format";
import { Kicker } from "../../components/ui";
import type { AgentToken, ServerMetricPoint } from "../../lib/types";

const RANGES = ["1h", "6h", "24h", "7d", "30d"] as const;
type Tab = "overview" | "monitoring" | "audit" | "tokens";

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
        <span className="font-mono text-[13px]">{last.toFixed(1)}<span className="text-fog text-[11px] ml-1">{unit}</span></span>
      </div>
      <svg viewBox={`0 0 ${W} ${H}`} className="w-full h-28 bg-abyss border border-edge rounded-xl" role="img" aria-label={`${title} chart`}>
        {[0.25, 0.5, 0.75].map((f) => (
          <line key={f} x1="0" x2={W} y1={H * f} y2={H * f} stroke="#27272a" strokeWidth="1" />
        ))}
        <polygon points={`0,${H} ${line} ${W},${H}`} fill={fill} opacity="0.2" />
        <polyline points={line} fill="none" stroke={stroke} strokeWidth="2" strokeLinejoin="round" />
      </svg>
      <div className="flex justify-between mt-1 font-mono text-[10px] text-fog">
        <span>{points.length ? new Date(points[0].timestamp).toLocaleTimeString() : "—"}</span>
        <span>{points.length ? new Date(points[points.length - 1].timestamp).toLocaleTimeString() : "—"}</span>
      </div>
    </div>
  );
}

// ─── Metric bar ───────────────────────────────────────────────────────────────
function MetricBar({ label, pct, value }: { label: string; pct: number; value: string }) {
  const color = pct > 85 ? "bg-red-500" : pct > 70 ? "bg-yellow-400" : "bg-green-500";
  return (
    <div className="mb-3">
      <div className="flex justify-between text-xs mb-1">
        <span className="text-fog">{label}</span>
        <span className="text-bone font-mono">{value}</span>
      </div>
      <div className="h-1.5 bg-emboss rounded-full overflow-hidden">
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
    <div className="flex items-center justify-between py-2 border-b border-edge last:border-0">
      <div>
        <div className="text-bone text-sm">{token.label || `Token #${token.id}`}</div>
        <div className="text-fog text-xs font-mono">
          Created {new Date(token.createdAt).toLocaleDateString()}
          {token.lastUsedAt && ` · Last used ${new Date(token.lastUsedAt).toLocaleDateString()}`}
        </div>
      </div>
      {token.revoked
        ? <span className="text-xs text-red-400 font-mono">revoked</span>
        : <button onClick={revoke} disabled={busy} className="text-fog hover:text-red-400 p-1"><Trash2 size={14} /></button>}
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
    <div className="min-h-screen bg-abyss pt-20 pb-28 px-4 max-w-5xl mx-auto">
      <div className="animate-pulse space-y-4">
        <div className="h-8 bg-panel rounded w-48" />
        <div className="h-40 bg-panel rounded-xl" />
      </div>
    </div>
  );

  if (!server) return (
    <div className="min-h-screen bg-abyss pt-20 pb-28 px-4 max-w-5xl mx-auto text-fog">Server not found.</div>
  );

  const STATUS_COLOR: Record<string, string> = {
    ONLINE: "text-green-400", OFFLINE: "text-red-400", WARNING: "text-yellow-400", UNKNOWN: "text-zinc-400",
  };
  const STATUS_DOT: Record<string, string> = {
    ONLINE: "bg-green-500", OFFLINE: "bg-red-500", WARNING: "bg-yellow-400", UNKNOWN: "bg-zinc-500",
  };

  const points = metricsHistory?.points ?? [];

  const TABS: { key: Tab; label: string; icon: React.ReactNode }[] = [
    { key: "overview", label: "Overview", icon: <Server size={14} /> },
    { key: "monitoring", label: "Monitoring", icon: <Activity size={14} /> },
    { key: "audit", label: "Audit", icon: <ClipboardList size={14} /> },
    { key: "tokens", label: "Tokens", icon: <Key size={14} /> },
  ];

  return (
    <div className="min-h-screen bg-abyss pt-20 pb-28 px-4 sm:px-6 max-w-5xl mx-auto">
      {/* Back */}
      <button onClick={() => nav("/servers")} className="flex items-center gap-1.5 text-fog hover:text-bone text-sm mb-4">
        <ArrowLeft size={15} /> Servers
      </button>

      {/* Header */}
      <div className="flex items-start justify-between mb-4">
        <div>
          <div className="flex items-center gap-2 mb-1">
            <span className={`w-2.5 h-2.5 rounded-full ${STATUS_DOT[server.status] ?? "bg-zinc-500"}`} />
            <h1 className="text-bone font-bold text-xl">{server.name}</h1>
            <span className={`text-sm font-mono ${STATUS_COLOR[server.status] ?? "text-zinc-400"}`}>{server.status}</span>
          </div>
          <div className="flex items-center gap-2 text-fog text-sm flex-wrap">
            {server.hostname && <span>{server.hostname}</span>}
            {server.ipAddress && <span>· {server.ipAddress}</span>}
            {server.os && <span>· {server.os} {server.osVersion}</span>}
          </div>
        </div>
        <button onClick={() => void qc.invalidateQueries({ queryKey: ["server", serverId] })}
          className="w-9 h-9 rounded-full bg-emboss flex items-center justify-center text-fog hover:text-bone">
          <RefreshCw size={15} />
        </button>
      </div>

      {/* Active alerts banner */}
      {serverAlerts.length > 0 && (
        <div className="mb-4 space-y-2">
          {serverAlerts.map((a) => (
            <div key={a.id} className={`flex items-center justify-between px-4 py-2 rounded-lg border text-sm ${
              a.severity === "CRITICAL" ? "border-red-500/40 bg-red-500/10 text-red-300" : "border-yellow-500/40 bg-yellow-500/10 text-yellow-300"
            }`}>
              <span>{a.message}</span>
              <button onClick={async () => { await api.resolveAlert(a.id); void qc.invalidateQueries({ queryKey: ["alerts"] }); }}
                className="text-xs underline opacity-70 hover:opacity-100 ml-4">Resolve</button>
            </div>
          ))}
        </div>
      )}

      {/* Tabs */}
      <div className="flex gap-1 mb-6 bg-panel border border-edge rounded-xl p-1 w-fit">
        {TABS.map((t) => (
          <button key={t.key} onClick={() => setTab(t.key)}
            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-mono transition-colors ${
              tab === t.key ? "bg-ember text-black font-semibold" : "text-fog hover:text-bone"
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
              <div key={item.label} className="bg-panel border border-edge rounded-xl p-3">
                <div className="text-fog text-[10px] uppercase tracking-wider">{item.label}</div>
                <div className="text-bone font-mono text-sm mt-0.5 truncate">{item.value}</div>
              </div>
            ))}
          </div>

          {/* Live metrics */}
          {latest ? (
            <div className="bg-panel border border-edge rounded-xl p-4">
              <div className="flex items-center justify-between mb-4">
                <h2 className="text-bone font-semibold text-sm">Live Metrics</h2>
                <span className="text-fog text-xs font-mono">{new Date(latest.timestamp).toLocaleTimeString()}</span>
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
                  <div key={s.label} className="bg-emboss rounded-lg p-2">
                    <div className="text-fog text-[10px]">{s.label}</div>
                    <div className="text-bone font-mono">{s.value}</div>
                  </div>
                ))}
              </div>
            </div>
          ) : (
            <div className="bg-panel border border-edge rounded-xl p-6 text-center text-fog text-sm">
              No metrics yet. Install and start the agent to begin monitoring.
            </div>
          )}

          {/* Last heartbeat */}
          <div className="bg-panel border border-edge rounded-xl p-4 text-sm">
            <div className="flex justify-between">
              <span className="text-fog">Last heartbeat</span>
              <span className="text-bone font-mono">
                {server.lastHeartbeat ? new Date(server.lastHeartbeat).toLocaleString() : "Never"}
              </span>
            </div>
            <div className="flex justify-between mt-2">
              <span className="text-fog">Agent status</span>
              <span className={`font-mono ${server.agentStatus === "CONNECTED" ? "text-green-400" : "text-zinc-400"}`}>
                {server.agentStatus}
              </span>
            </div>
            <div className="flex justify-between mt-2">
              <span className="text-fog">Registered</span>
              <span className="text-bone font-mono">{new Date(server.createdAt).toLocaleDateString()}</span>
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
                className={`px-2.5 py-1 rounded-lg text-xs font-mono ${range === r ? "bg-ember text-black" : "text-fog hover:text-bone"}`}>
                {r}
              </button>
            ))}
          </div>
          {points.length === 0 ? (
            <div className="bg-panel border border-edge rounded-xl p-8 text-center text-fog text-sm">
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
        </div>
      )}

      {/* ── Audit tab ── */}
      {tab === "audit" && (
        <div className="bg-panel border border-edge rounded-xl overflow-hidden">
          <table className="w-full text-sm border-collapse">
            <thead>
              <tr className="bg-emboss">
                <th className="text-left text-fog text-xs px-4 py-2.5 font-semibold uppercase tracking-wider">Time</th>
                <th className="text-left text-fog text-xs px-4 py-2.5 font-semibold uppercase tracking-wider">Actor</th>
                <th className="text-left text-fog text-xs px-4 py-2.5 font-semibold uppercase tracking-wider">Action</th>
                <th className="text-left text-fog text-xs px-4 py-2.5 font-semibold uppercase tracking-wider hidden sm:table-cell">Result</th>
              </tr>
            </thead>
            <tbody>
              {auditLogs.length === 0 ? (
                <tr><td colSpan={4} className="px-4 py-8 text-center text-fog text-sm">No audit records.</td></tr>
              ) : auditLogs.map((log) => (
                <tr key={log.id} className="border-t border-edge hover:bg-emboss/50">
                  <td className="px-4 py-2.5 font-mono text-xs text-fog whitespace-nowrap">
                    {new Date(log.timestamp).toLocaleString()}
                  </td>
                  <td className="px-4 py-2.5 font-mono text-xs text-bone">{log.actor}</td>
                  <td className="px-4 py-2.5 font-mono text-xs text-bone">{log.action} {log.resource}</td>
                  <td className={`px-4 py-2.5 font-mono text-xs hidden sm:table-cell ${log.result === "ok" ? "text-green-400" : "text-red-400"}`}>
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
        <div className="bg-panel border border-edge rounded-xl p-4">
          <div className="flex items-center justify-between mb-3">
            <div className="flex items-center gap-2">
              <Key size={15} className="text-fog" />
              <h2 className="text-bone font-semibold text-sm">Agent Tokens</h2>
            </div>
            <button onClick={() => setShowTokenForm(!showTokenForm)} className="text-xs text-ember hover:underline">
              + Generate
            </button>
          </div>

          {showTokenForm && (
            <div className="mb-3 flex gap-2">
              <input className="flex-1 bg-emboss border border-edge rounded-lg px-3 py-1.5 text-bone text-sm outline-none focus:border-ember"
                placeholder="Token label (optional)" value={tokenLabel} onChange={(e) => setTokenLabel(e.target.value)} />
              <button onClick={createToken} disabled={creatingToken}
                className="px-3 py-1.5 bg-ember text-black text-sm font-semibold rounded-lg disabled:opacity-50">
                {creatingToken ? "…" : "Create"}
              </button>
            </div>
          )}

          {newToken && (
            <div className="mb-3 p-3 bg-emboss rounded-lg border border-yellow-500/30">
              <p className="text-yellow-300 text-xs mb-2 font-semibold">Copy this token now — it will not be shown again.</p>
              <div className="flex items-center gap-2">
                <code className="text-bone text-xs font-mono flex-1 break-all">{newToken}</code>
                <button onClick={copyToken} className="text-fog hover:text-bone shrink-0"><Copy size={14} /></button>
              </div>
              <button onClick={() => setNewToken(null)} className="text-xs text-fog hover:text-bone mt-2 underline">Dismiss</button>
            </div>
          )}

          {tokens.length === 0
            ? <p className="text-fog text-xs">No tokens yet. Generate one to connect an agent.</p>
            : tokens.map((t) => <TokenRow key={t.id} token={t} serverId={serverId} onRevoked={() => void refetchTokens()} />)}

          <div className="mt-4 p-3 bg-emboss rounded-lg text-xs text-fog font-mono">
            <p className="mb-1 text-bone">Agent install (Linux):</p>
            <p>go build -o serverhub-agent ./cmd/agent</p>
            <p className="mt-1">AGENT_TOKEN=&lt;token&gt; SERVERHUB_URL=http://&lt;host&gt;:4000 ./serverhub-agent</p>
          </div>
        </div>
      )}
    </div>
  );
}
