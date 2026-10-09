import { useEffect, useState } from "react";
import { useNavigate } from "react-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus, RefreshCw, Server } from "lucide-react";
import { api } from "../../lib/api";
import type { ManagedServer } from "../../lib/types";
import { useEvents } from "../../lib/useEvents";
import { useUi } from "../../stores/store";

const STATUS_COLOR: Record<string, string> = {
  ONLINE: "bg-moss",
  OFFLINE: "bg-brick",
  WARNING: "bg-status-amber",
  UNKNOWN: "bg-stone",
};

function ServerCard({ s, onClick }: { s: ManagedServer; onClick: () => void }) {
  return (
    <button
      onClick={onClick}
      className="w-full text-left bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-5 hover:border-accent dark:hover:border-ember hover:shadow-chrome transition-all cursor-pointer shadow-sm group"
    >
      <div className="flex items-start justify-between mb-3">
        <div className="flex items-center gap-2.5">
          <span className={`w-2 h-2 rounded-full shrink-0 ${STATUS_COLOR[s.status] ?? "bg-stone"}`} />
          <span className="font-semibold text-ink dark:text-bone text-[15px] group-hover:text-accent dark:group-hover:text-ember transition-colors">{s.name}</span>
        </div>
        <span className={`text-[11px] font-mono uppercase tracking-wider font-semibold px-2 py-0.5 rounded border ${
          s.status === "ONLINE"
            ? "border-moss/30 bg-moss/10 text-moss"
            : s.status === "OFFLINE"
            ? "border-brick/30 bg-brick/10 text-brick"
            : s.status === "WARNING"
            ? "border-status-amber/30 bg-status-amber/10 text-status-amber"
            : "border-line dark:border-edge bg-paper dark:bg-abyss text-stone"
        }`}>
          {s.status}
        </span>
      </div>
      <div className="text-xs text-muted dark:text-fog font-mono space-y-1 mb-4">
        {s.hostname && <div className="truncate">{s.hostname}</div>}
        {s.ipAddress && <div>{s.ipAddress}</div>}
        {s.os && <div className="text-[11px] opacity-80">{s.os} {s.osVersion}</div>}
      </div>
      <div className="grid grid-cols-3 gap-2 text-xs pt-1 border-t border-line dark:border-edge">
        <Stat label="CPU" value={s.cpuCores > 0 ? `${s.cpuCores} cores` : "—"} />
        <Stat label="RAM" value={s.ramTotal > 0 ? `${Math.round(s.ramTotal / 1024 / 1024 / 1024)}GB` : "—"} />
        <Stat label="Agent" value={s.agentStatus} />
      </div>
      {s.lastHeartbeat && (
        <div className="mt-3 text-[10px] text-muted dark:text-fog font-mono">
          Last seen: {new Date(s.lastHeartbeat).toLocaleString()}
        </div>
      )}
    </button>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="bg-paper dark:bg-abyss border border-line dark:border-edge rounded-lg p-2">
      <div className="text-muted dark:text-fog text-[10px] uppercase tracking-wider">{label}</div>
      <div className="text-ink dark:text-bone font-mono text-xs font-medium mt-0.5 truncate">{value}</div>
    </div>
  );
}

function AddServerModal({ onClose, onCreated }: { onClose: () => void; onCreated: () => void }) {
  const { pushToast } = useUi();
  const [step, setStep] = useState<1 | 2 | 3 | 4>(1);
  const [name, setName] = useState("");
  const [hostname, setHostname] = useState("");
  const [ip, setIp] = useState("");
  const [osType, setOsType] = useState<"linux" | "docker" | "windows">("linux");
  const [createdServerId, setCreatedServerId] = useState<number | null>(null);
  const [agentToken, setAgentToken] = useState("");
  const [busy, setBusy] = useState(false);
  const [copied, setCopied] = useState(false);
  const [pollStatus, setPollStatus] = useState<"connecting" | "connected" | "timeout">("connecting");
  const [secondsWaiting, setSecondsWaiting] = useState(0);

  // Step 1 -> Step 2 -> Step 3: create server & token
  const handleProceedToSetup = async () => {
    if (!name.trim()) return;
    setBusy(true);
    try {
      let serverId = createdServerId;
      if (!serverId) {
        const created = await api.createServer({ name: name.trim(), hostname: hostname.trim() || undefined, ipAddress: ip.trim() || undefined });
        serverId = created.id;
        setCreatedServerId(serverId);
      }
      if (!agentToken && serverId) {
        const res = await api.createAgentToken(serverId, `${name.trim()}-initial-token`);
        setAgentToken(res.token);
      }
      onCreated();
      setStep(3);
    } catch (e) {
      pushToast(e instanceof Error ? e.message : "Failed to initialize server", true);
    } finally {
      setBusy(false);
    }
  };

  // Step 4: Live connection polling
  useEffect(() => {
    if (step !== 4 || !createdServerId) return;
    setPollStatus("connecting");
    setSecondsWaiting(0);

    const timer = setInterval(() => {
      setSecondsWaiting((s) => s + 1);
    }, 1000);

    const poller = setInterval(async () => {
      try {
        const s = await api.managedServer(createdServerId);
        if (s.agentStatus === "CONNECTED" || s.status === "ONLINE") {
          setPollStatus("connected");
          clearInterval(poller);
          clearInterval(timer);
          onCreated();
        }
      } catch {
        /* ignore polling errors */
      }
    }, 2500);

    const timeout = setTimeout(() => {
      setPollStatus((prev) => (prev === "connected" ? "connected" : "timeout"));
      clearInterval(poller);
      clearInterval(timer);
    }, 60000);

    return () => {
      clearInterval(poller);
      clearInterval(timer);
      clearTimeout(timeout);
    };
  }, [step, createdServerId, onCreated]);

  const apiHost = window.location.origin;

  const command =
    osType === "linux"
      ? `curl -fsSL ${apiHost}/agent/install.sh | sudo AGENT_TOKEN="${agentToken}" SERVERHUB_URL="${apiHost}" bash`
      : osType === "docker"
      ? `docker run -d --name serverhub-agent --restart always \\
  -e AGENT_TOKEN="${agentToken}" \\
  -e SERVERHUB_URL="${apiHost}" \\
  -v /var/run/docker.sock:/var/run/docker.sock \\
  serverhub/agent:latest`
      : `$env:AGENT_TOKEN="${agentToken}"; $env:SERVERHUB_URL="${apiHost}"; .\\serverhub-agent.exe`;

  const copyCommand = async () => {
    try {
      await navigator.clipboard.writeText(command);
      setCopied(true);
      pushToast("Setup command copied to clipboard!");
      setTimeout(() => setCopied(false), 3000);
    } catch {
      pushToast("Failed to copy to clipboard", true);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-[rgba(28,25,23,0.35)] dark:bg-[rgba(0,0,0,0.65)] p-4 backdrop-blur-xs" onClick={onClose}>
      <div className="bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-6 w-full max-w-xl shadow-chrome" onClick={(e) => e.stopPropagation()}>
        {/* Wizard progress header */}
        <div className="flex items-center justify-between border-b border-line dark:border-edge pb-4 mb-5">
          <div>
            <h2 className="text-ink dark:text-bone font-head font-bold text-lg">Connect a New Server</h2>
            <p className="text-muted dark:text-fog text-xs mt-0.5">Quick 4-step guided setup for any host environment</p>
          </div>
          <div className="flex items-center gap-1.5 text-xs font-mono">
            {[1, 2, 3, 4].map((s) => (
              <span
                key={s}
                className={`w-6 h-6 rounded-full flex items-center justify-center font-bold text-[11px] transition-all ${
                  step === s
                    ? "bg-accent dark:bg-ember text-white dark:text-black shadow-sm"
                    : step > s
                    ? "bg-moss/20 text-moss border border-moss/30"
                    : "bg-paper dark:bg-abyss text-stone border border-line dark:border-edge"
                }`}
              >
                {step > s ? "✓" : s}
              </span>
            ))}
          </div>
        </div>

        {/* Step 1: Details */}
        {step === 1 && (
          <div className="space-y-4">
            <div>
              <h3 className="text-sm font-semibold text-ink dark:text-bone mb-1">Step 1: Host Information</h3>
              <p className="text-xs text-muted dark:text-fog mb-3">Provide a recognizable label for this server in your inventory.</p>
            </div>
            <div className="space-y-3">
              <Field label="Server Name *" value={name} onChange={setName} placeholder="e.g. web-production-01, internal-db" />
              <Field label="Hostname / Domain (Optional)" value={hostname} onChange={setHostname} placeholder="e.g. api.yourcompany.com" />
              <Field label="IP Address (Optional)" value={ip} onChange={setIp} placeholder="e.g. 192.168.1.100 or 54.210.12.3" />
            </div>
            <div className="flex justify-end gap-2 pt-4 border-t border-line dark:border-edge">
              <button onClick={onClose} className="px-4 py-2 text-sm text-muted dark:text-fog hover:text-ink dark:hover:text-bone border border-line dark:border-edge rounded-lg cursor-pointer">
                Cancel
              </button>
              <button
                onClick={() => setStep(2)}
                disabled={!name.trim()}
                className="px-4 py-2 text-sm bg-accent dark:bg-ember text-white dark:text-black font-semibold rounded-lg hover:bg-accent-hover dark:hover:bg-ember-hover disabled:opacity-50 cursor-pointer shadow-sm"
              >
                Next: Choose Environment →
              </button>
            </div>
          </div>
        )}

        {/* Step 2: Environment Selection */}
        {step === 2 && (
          <div className="space-y-4">
            <div>
              <h3 className="text-sm font-semibold text-ink dark:text-bone mb-1">Step 2: Server Operating System</h3>
              <p className="text-xs text-muted dark:text-fog mb-3">Select the platform where the ServerHub monitoring agent will run.</p>
            </div>
            <div className="grid grid-cols-3 gap-3">
              {[
                { id: "linux", title: "Linux Host", desc: "Ubuntu, Debian, RHEL, CentOS", icon: "🐧" },
                { id: "docker", title: "Docker Container", desc: "Any Docker daemon host", icon: "🐳" },
                { id: "windows", title: "Windows Server", desc: "PowerShell binary execution", icon: "🪟" },
              ].map((opt) => (
                <button
                  key={opt.id}
                  onClick={() => setOsType(opt.id as any)}
                  className={`p-4 rounded-xl border text-left transition-all cursor-pointer ${
                    osType === opt.id
                      ? "border-accent dark:border-ember bg-tint/40 dark:bg-emboss ring-1 ring-accent dark:ring-ember"
                      : "border-line dark:border-edge hover:border-accent/40 dark:hover:border-ember/40 bg-paper/50 dark:bg-abyss/50"
                  }`}
                >
                  <div className="text-2xl mb-2">{opt.icon}</div>
                  <div className="text-xs font-semibold text-ink dark:text-bone">{opt.title}</div>
                  <div className="text-[11px] text-muted dark:text-fog mt-0.5 leading-snug">{opt.desc}</div>
                </button>
              ))}
            </div>
            <div className="flex justify-between gap-2 pt-4 border-t border-line dark:border-edge">
              <button onClick={() => setStep(1)} className="px-4 py-2 text-sm text-muted dark:text-fog hover:text-ink dark:hover:text-bone border border-line dark:border-edge rounded-lg cursor-pointer">
                ← Back
              </button>
              <button
                onClick={handleProceedToSetup}
                disabled={busy}
                className="px-4 py-2 text-sm bg-accent dark:bg-ember text-white dark:text-black font-semibold rounded-lg hover:bg-accent-hover dark:hover:bg-ember-hover disabled:opacity-50 cursor-pointer shadow-sm"
              >
                {busy ? "Registering…" : "Next: Setup Command →"}
              </button>
            </div>
          </div>
        )}

        {/* Step 3: Copy Command */}
        {step === 3 && (
          <div className="space-y-4">
            <div>
              <h3 className="text-sm font-semibold text-ink dark:text-bone mb-1">Step 3: Run the Setup Command</h3>
              <p className="text-xs text-muted dark:text-fog mb-3">Copy and execute this single command in your server's terminal with root privileges.</p>
            </div>
            <div className="relative bg-paper dark:bg-abyss border border-line dark:border-edge rounded-xl p-4">
              <pre className="font-mono text-xs text-ink dark:text-bone overflow-x-auto whitespace-pre-wrap break-all pr-12 leading-relaxed">
                {command}
              </pre>
              <button
                onClick={copyCommand}
                className="absolute top-3 right-3 px-2.5 py-1.5 text-xs bg-white dark:bg-panel border border-line dark:border-edge rounded-lg shadow-sm font-medium hover:border-accent dark:hover:border-ember text-ink dark:text-bone transition-all cursor-pointer flex items-center gap-1.5"
              >
                {copied ? "✓ Copied" : "Copy"}
              </button>
            </div>
            <div className="p-3 bg-paper dark:bg-abyss/80 border border-line dark:border-edge rounded-lg text-xs text-muted dark:text-fog space-y-1">
              <p className="font-semibold text-ink dark:text-bone">Security & Port Requirements:</p>
              <p>• The agent only initiates outbound HTTP/HTTPS requests to <span className="font-mono text-ink dark:text-bone">{apiHost}</span>.</p>
              <p>• No inbound server ports need to be opened in your firewall.</p>
            </div>
            <div className="flex justify-between gap-2 pt-4 border-t border-line dark:border-edge">
              <button onClick={() => setStep(2)} className="px-4 py-2 text-sm text-muted dark:text-fog hover:text-ink dark:hover:text-bone border border-line dark:border-edge rounded-lg cursor-pointer">
                ← Back
              </button>
              <button
                onClick={() => setStep(4)}
                className="px-4 py-2 text-sm bg-accent dark:bg-ember text-white dark:text-black font-semibold rounded-lg hover:bg-accent-hover dark:hover:bg-ember-hover cursor-pointer shadow-sm"
              >
                I've Run the Command → Verify Connection
              </button>
            </div>
          </div>
        )}

        {/* Step 4: Live Verification */}
        {step === 4 && (
          <div className="space-y-5 text-center py-4">
            {pollStatus === "connecting" && (
              <div className="space-y-4">
                <div className="w-16 h-16 rounded-full border-3 border-accent dark:border-ember border-t-transparent animate-spin mx-auto" />
                <div>
                  <h3 className="text-base font-semibold text-ink dark:text-bone">Listening for Agent Connection…</h3>
                  <p className="text-xs text-muted dark:text-fog mt-1">Waiting for initial heartbeat from "{name}" ({secondsWaiting}s elapsed)</p>
                </div>
                <p className="text-[11px] text-muted dark:text-fog max-w-sm mx-auto">
                  Once your agent process launches, it will report CPU, RAM, and disk telemetry automatically.
                </p>
              </div>
            )}

            {pollStatus === "connected" && (
              <div className="space-y-4">
                <div className="w-16 h-16 rounded-full bg-moss/10 border-2 border-moss text-moss text-2xl flex items-center justify-center mx-auto shadow-sm">
                  ✓
                </div>
                <div>
                  <h3 className="text-base font-bold text-moss">Server Successfully Connected!</h3>
                  <p className="text-xs text-muted dark:text-fog mt-1">Heartbeat received. "{name}" is now online and actively monitored.</p>
                </div>
                <div className="pt-2">
                  <button
                    onClick={onClose}
                    className="px-5 py-2.5 text-sm bg-accent dark:bg-ember text-white dark:text-black font-semibold rounded-lg hover:bg-accent-hover dark:hover:bg-ember-hover cursor-pointer shadow-sm"
                  >
                    Done & View Server Inventory
                  </button>
                </div>
              </div>
            )}

            {pollStatus === "timeout" && (
              <div className="space-y-4 text-left">
                <div className="p-4 bg-status-amber/10 border border-status-amber/30 rounded-xl">
                  <h4 className="text-status-amber font-semibold text-sm mb-1">Agent Has Not Connected Yet</h4>
                  <p className="text-xs text-muted dark:text-fog">
                    We didn't receive a heartbeat within 60 seconds. The server was saved in your inventory, but please check the following:
                  </p>
                  <ul className="text-xs text-ink dark:text-bone space-y-1.5 mt-2.5 list-disc pl-4">
                    <li>Did the installation command finish without errors in your terminal?</li>
                    <li>Can the server reach <span className="font-mono bg-paper dark:bg-abyss px-1 py-0.5 rounded border border-line dark:border-edge">{apiHost}</span> over port 4000/80/443?</li>
                    <li>Verify the agent service status: <span className="font-mono bg-paper dark:bg-abyss px-1 py-0.5 rounded border border-line dark:border-edge">sudo systemctl status serverhub-agent</span></li>
                  </ul>
                </div>
                <div className="flex justify-between items-center pt-2">
                  <button
                    onClick={() => setStep(3)}
                    className="text-xs text-accent dark:text-ember underline cursor-pointer"
                  >
                    ← Review Setup Command
                  </button>
                  <div className="flex gap-2">
                    <button
                      onClick={() => setPollStatus("connecting")}
                      className="px-3.5 py-2 text-xs border border-line dark:border-edge rounded-lg text-ink dark:text-bone hover:bg-paper dark:hover:bg-abyss cursor-pointer"
                    >
                      Retry Listening
                    </button>
                    <button
                      onClick={onClose}
                      className="px-4 py-2 text-xs bg-accent dark:bg-ember text-white dark:text-black font-semibold rounded-lg hover:bg-accent-hover dark:hover:bg-ember-hover cursor-pointer"
                    >
                      Close & Finish Later
                    </button>
                  </div>
                </div>
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}

function Field({ label, value, onChange, placeholder }: {
  label: string; value: string; onChange: (v: string) => void; placeholder?: string;
}) {
  return (
    <div>
      <label className="text-muted dark:text-fog text-xs mb-1 block">{label}</label>
      <input
        className="w-full bg-white dark:bg-panel border border-line dark:border-edge rounded-lg px-3 py-2 text-ink dark:text-bone text-sm outline-none focus:border-accent dark:focus:border-ember placeholder:text-stone dark:placeholder:text-fog font-mono"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
      />
    </div>
  );
}

export function ServersPage({ setOnline }: { setOnline: (v: boolean) => void }) {
  const nav = useNavigate();
  const qc = useQueryClient();
  const [showAdd, setShowAdd] = useState(false);
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState<string | null>(null);
  const [groupFilter, setGroupFilter] = useState<number | null>(null);

  const { data: servers = [], isLoading, isError, refetch, isSuccess } = useQuery({
    queryKey: ["servers"],
    queryFn: api.servers,
    refetchInterval: 30_000,
  });

  useEffect(() => {
    if (isSuccess) setOnline(true);
    else if (isError) setOnline(false);
  }, [isSuccess, isError, setOnline]);

  const { data: groups = [] } = useQuery({ queryKey: ["server-groups"], queryFn: api.serverGroups });

  // Live updates via SSE
  useEvents((ev) => {
    if (ev.type === "server.heartbeat" || ev.type === "server.metrics") {
      void qc.invalidateQueries({ queryKey: ["servers"] });
    }
  });

  const counts = {
    total: servers.length,
    online: servers.filter((s) => s.status === "ONLINE").length,
    offline: servers.filter((s) => s.status === "OFFLINE").length,
    warning: servers.filter((s) => s.status === "WARNING").length,
    unknown: servers.filter((s) => s.status !== "ONLINE" && s.status !== "OFFLINE" && s.status !== "WARNING").length,
  };

  const needle = search.trim().toLowerCase();
  const filtered = servers.filter((s) => {
    if (groupFilter !== null && s.groupId !== groupFilter) return false;
    if (statusFilter !== null && s.status !== statusFilter) return false;
    if (needle) {
      const matchName = s.name.toLowerCase().includes(needle);
      const matchHost = s.hostname?.toLowerCase().includes(needle) ?? false;
      const matchIp = s.ipAddress?.toLowerCase().includes(needle) ?? false;
      const matchOs = s.os?.toLowerCase().includes(needle) ?? false;
      if (!matchName && !matchHost && !matchIp && !matchOs) return false;
    }
    return true;
  });

  return (
    <main className="w-full max-w-6xl mx-auto px-4 sm:px-6 pt-24 pb-36">
      {/* Header */}
      <div className="flex items-center justify-between mb-6 flex-wrap gap-4">
        <div>
          <div className="flex items-center gap-3">
            <Server size={22} className="text-accent dark:text-ember" />
            <h1 className="font-head text-[24px] font-bold text-ink dark:text-bone tracking-[-0.02em]">Servers</h1>
          </div>
          <p className="text-muted dark:text-fog text-xs mt-1 font-mono">
            Managed host inventory · Remote agents
          </p>
        </div>
        <div className="flex gap-2.5">
          <button
            onClick={() => void refetch()}
            className="w-9 h-9 rounded-full bg-white dark:bg-panel border border-line dark:border-edge flex items-center justify-center text-muted dark:text-fog hover:text-ink dark:hover:text-bone shadow-sm cursor-pointer"
            aria-label="Refresh"
          >
            <RefreshCw size={15} />
          </button>
          <button
            onClick={() => setShowAdd(true)}
            className="flex items-center gap-1.5 px-3.5 py-2 bg-accent dark:bg-ember text-white dark:text-black text-sm font-semibold rounded-lg hover:bg-accent-hover dark:hover:bg-ember-hover shadow-sm cursor-pointer"
          >
            <Plus size={15} /> Add Server
          </button>
        </div>
      </div>

      {/* Summary */}
      <div className="grid grid-cols-2 sm:grid-cols-5 gap-3 mb-6">
        {[
          { label: "Total", value: counts.total, color: "text-ink dark:text-bone", filter: null },
          { label: "Online", value: counts.online, color: "text-moss", filter: "ONLINE" },
          { label: "Offline", value: counts.offline, color: "text-brick", filter: "OFFLINE" },
          { label: "Warning", value: counts.warning, color: "text-status-amber", filter: "WARNING" },
          { label: "Unknown", value: counts.unknown, color: "text-stone", filter: "UNKNOWN" },
        ].map((c) => (
          <button
            key={c.label}
            onClick={() => setStatusFilter(statusFilter === c.filter ? null : c.filter)}
            className={`text-left bg-white dark:bg-panel border rounded-card p-4 shadow-sm transition-all cursor-pointer ${
              statusFilter === c.filter
                ? "border-accent dark:border-ember ring-1 ring-accent dark:ring-ember"
                : "border-line dark:border-edge hover:border-accent/40 dark:hover:border-ember/40"
            }`}
          >
            <div className="text-muted dark:text-fog text-[11px] uppercase tracking-wider mb-1 font-medium">{c.label}</div>
            <div className={`font-bold text-2xl font-mono ${c.color}`}>{c.value}</div>
          </button>
        ))}
      </div>

      {/* Search and Filters */}
      <div className="flex flex-col sm:flex-row gap-3 items-stretch sm:items-center justify-between mb-6">
        <div className="relative flex-1 max-w-md">
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search servers by name, hostname, IP..."
            className="w-full bg-white dark:bg-panel border border-line dark:border-edge rounded-lg px-3 py-2 text-ink dark:text-bone text-sm outline-none focus:border-accent dark:focus:border-ember placeholder:text-stone dark:placeholder:text-fog font-mono shadow-sm"
          />
          {search && (
            <button
              onClick={() => setSearch("")}
              className="absolute right-2.5 top-1/2 -translate-y-1/2 text-muted dark:text-fog hover:text-ink dark:hover:text-bone text-xs font-mono cursor-pointer"
            >
              Clear
            </button>
          )}
        </div>

        {/* Group filter & Status clear */}
        <div className="flex gap-1.5 flex-wrap items-center">
          {statusFilter !== null && (
            <button
              onClick={() => setStatusFilter(null)}
              className="px-2.5 py-1 rounded-full text-xs font-mono bg-paper dark:bg-abyss border border-line dark:border-edge text-muted dark:text-fog hover:text-ink dark:hover:text-bone cursor-pointer"
            >
              Clear status: {statusFilter} ×
            </button>
          )}

          {groups.length > 0 && (
            <div className="flex gap-1.5 flex-wrap">
              <button
                onClick={() => setGroupFilter(null)}
                className={`px-3 py-1 rounded-full text-xs font-mono border transition-colors cursor-pointer ${
                  groupFilter === null
                    ? "border-accent dark:border-ember text-accent dark:text-ember bg-tint/50 dark:bg-emboss"
                    : "border-line dark:border-edge text-muted dark:text-fog hover:text-ink dark:hover:text-bone bg-white dark:bg-panel"
                }`}
              >
                All groups
              </button>
              {groups.map((g) => (
                <button
                  key={g.id}
                  onClick={() => setGroupFilter(g.id)}
                  className={`px-3 py-1 rounded-full text-xs font-mono border transition-colors cursor-pointer ${
                    groupFilter === g.id
                      ? "border-accent dark:border-ember text-accent dark:text-ember bg-tint/50 dark:bg-emboss"
                      : "border-line dark:border-edge text-muted dark:text-fog hover:text-ink dark:hover:text-bone bg-white dark:bg-panel"
                  }`}
                >
                  {g.name}
                </button>
              ))}
            </div>
          )}
        </div>
      </div>

      {/* Server grid */}
      {isLoading && (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
          {[1, 2, 3].map((i) => (
            <div key={i} className="bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-4 h-48 animate-pulse" />
          ))}
        </div>
      )}

      {isError && (
        <div className="text-brick text-sm p-4 bg-red-50 dark:bg-red-950/30 border border-brick/30 rounded-card">
          Failed to load servers. <button onClick={() => void refetch()} className="underline font-medium cursor-pointer">Retry</button>
        </div>
      )}

      {!isLoading && !isError && filtered.length === 0 && (
        <div className="text-center py-16 bg-paper dark:bg-abyss border border-line dark:border-edge rounded-card px-4">
          <Server size={40} className="mx-auto mb-3 opacity-30 text-muted dark:text-fog" />
          <p className="text-sm font-medium text-ink dark:text-bone">
            {search || statusFilter || groupFilter ? "No servers match current filter." : "No servers yet. Add one to get started."}
          </p>
          {(search || statusFilter || groupFilter) && (
            <button
              onClick={() => { setSearch(""); setStatusFilter(null); setGroupFilter(null); }}
              className="mt-3 text-xs font-mono text-accent dark:text-ember hover:underline cursor-pointer"
            >
              Reset all filters
            </button>
          )}
        </div>
      )}

      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
        {filtered.map((s) => (
          <ServerCard key={s.id} s={s} onClick={() => nav(`/servers/${s.id}`)} />
        ))}
      </div>

      {showAdd && (
        <AddServerModal
          onClose={() => setShowAdd(false)}
          onCreated={() => void qc.invalidateQueries({ queryKey: ["servers"] })}
        />
      )}
    </main>
  );
}
