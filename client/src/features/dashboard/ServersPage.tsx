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

const STATUS_TEXT: Record<string, string> = {
  ONLINE: "text-moss",
  OFFLINE: "text-brick",
  WARNING: "text-status-amber",
  UNKNOWN: "text-stone",
};

function ServerCard({ s, onClick }: { s: ManagedServer; onClick: () => void }) {
  return (
    <button
      onClick={onClick}
      className="w-full text-left bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-4 hover:border-accent dark:hover:border-ember transition-colors cursor-pointer"
    >
      <div className="flex items-start justify-between mb-3">
        <div className="flex items-center gap-2">
          <span className={`w-2 h-2 rounded-full ${STATUS_COLOR[s.status] ?? "bg-stone"}`} />
          <span className="font-semibold text-ink dark:text-bone text-sm">{s.name}</span>
        </div>
        <span className={`text-xs font-mono ${STATUS_TEXT[s.status] ?? "text-muted dark:text-fog"}`}>
          {s.status}
        </span>
      </div>
      <div className="text-xs text-muted dark:text-fog font-mono space-y-0.5">
        {s.hostname && <div>{s.hostname}</div>}
        {s.ipAddress && <div>{s.ipAddress}</div>}
        {s.os && <div>{s.os} {s.osVersion}</div>}
      </div>
      <div className="mt-3 grid grid-cols-3 gap-2 text-xs">
        <Stat label="CPU" value={s.cpuCores > 0 ? `${s.cpuCores} cores` : "—"} />
        <Stat label="RAM" value={s.ramTotal > 0 ? `${Math.round(s.ramTotal / 1024 / 1024 / 1024)}GB` : "—"} />
        <Stat label="Agent" value={s.agentStatus} />
      </div>
      {s.lastHeartbeat && (
        <div className="mt-2 text-[10px] text-muted dark:text-fog font-mono">
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
      <div className="text-ink dark:text-bone font-mono text-xs mt-0.5 truncate">{value}</div>
    </div>
  );
}

function AddServerModal({ onClose, onCreated }: { onClose: () => void; onCreated: () => void }) {
  const { pushToast } = useUi();
  const [name, setName] = useState("");
  const [hostname, setHostname] = useState("");
  const [ip, setIp] = useState("");
  const [busy, setBusy] = useState(false);

  const submit = async () => {
    if (!name.trim()) return;
    setBusy(true);
    try {
      await api.createServer({ name: name.trim(), hostname, ipAddress: ip });
      pushToast(`Server "${name}" added`);
      onCreated();
      onClose();
    } catch (e) {
      pushToast(e instanceof Error ? e.message : "Failed to add server", true);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-[rgba(28,25,23,0.28)] dark:bg-[rgba(0,0,0,0.55)] p-4" onClick={onClose}>
      <div className="bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-6 w-full max-w-md shadow-chrome" onClick={(e) => e.stopPropagation()}>
        <h2 className="text-ink dark:text-bone font-head font-bold text-lg mb-4">Add Server</h2>
        <div className="space-y-3">
          <Field label="Name *" value={name} onChange={setName} placeholder="production-api" />
          <Field label="Hostname" value={hostname} onChange={setHostname} placeholder="api.example.com" />
          <Field label="IP Address" value={ip} onChange={setIp} placeholder="192.168.1.10" />
        </div>
        <p className="text-muted dark:text-fog text-xs mt-3">
          After adding, generate an agent token and install the agent on the server.
        </p>
        <div className="flex gap-2 mt-5 justify-end">
          <button onClick={onClose} className="px-4 py-2 text-sm text-muted dark:text-fog hover:text-ink dark:hover:text-bone border border-line dark:border-edge rounded-input">
            Cancel
          </button>
          <button
            onClick={submit}
            disabled={busy || !name.trim()}
            className="px-4 py-2 text-sm bg-accent dark:bg-ember text-white dark:text-black font-semibold rounded-input hover:bg-accent-hover dark:hover:bg-ember-hover disabled:opacity-50"
          >
            {busy ? "Adding…" : "Add Server"}
          </button>
        </div>
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
        className="w-full bg-white dark:bg-panel border border-line dark:border-edge rounded-input px-3 py-2 text-ink dark:text-bone text-sm outline-none focus:border-accent dark:focus:border-ember placeholder:text-stone dark:placeholder:text-fog font-mono"
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

  const filtered = groupFilter ? servers.filter((s) => s.groupId === groupFilter) : servers;

  const counts = {
    total: servers.length,
    online: servers.filter((s) => s.status === "ONLINE").length,
    offline: servers.filter((s) => s.status === "OFFLINE").length,
    warning: servers.filter((s) => s.status === "WARNING").length,
    unknown: servers.filter((s) => s.status !== "ONLINE" && s.status !== "OFFLINE" && s.status !== "WARNING").length,
  };

  return (
    <main className="w-full max-w-6xl mx-auto px-4 sm:px-6 pt-24 pb-36">
      {/* Header */}
      <div className="flex items-center justify-between mb-6">
        <div className="flex items-center gap-3">
          <Server size={22} className="text-accent dark:text-ember" />
          <h1 className="font-head text-[22px] font-bold text-ink dark:text-bone tracking-[-0.02em]">Servers</h1>
        </div>
        <div className="flex gap-2">
          <button
            onClick={() => void refetch()}
            className="w-9 h-9 rounded-full bg-white dark:bg-panel border border-line dark:border-edge flex items-center justify-center text-muted dark:text-fog hover:text-ink dark:hover:text-bone shadow-chrome"
            aria-label="Refresh"
          >
            <RefreshCw size={15} />
          </button>
          <button
            onClick={() => setShowAdd(true)}
            className="flex items-center gap-1.5 px-3 py-2 bg-accent dark:bg-ember text-white dark:text-black text-sm font-semibold rounded-input hover:bg-accent-hover dark:hover:bg-ember-hover"
          >
            <Plus size={15} /> Add Server
          </button>
        </div>
      </div>

      {/* Summary */}
      <div className="grid grid-cols-2 sm:grid-cols-5 gap-3 mb-6">
        {[
          { label: "Total", value: counts.total, color: "text-ink dark:text-bone" },
          { label: "Online", value: counts.online, color: "text-moss" },
          { label: "Offline", value: counts.offline, color: "text-brick" },
          { label: "Warning", value: counts.warning, color: "text-status-amber" },
          { label: "Unknown", value: counts.unknown, color: "text-stone" },
        ].map((c) => (
          <div key={c.label} className="bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-4 shadow-chrome">
            <div className="text-muted dark:text-fog text-xs uppercase tracking-wider mb-1">{c.label}</div>
            <div className={`font-bold text-2xl font-mono ${c.color}`}>{c.value}</div>
          </div>
        ))}
      </div>

      {/* Group filter */}
      {groups.length > 0 && (
        <div className="flex gap-2 mb-4 flex-wrap">
          <button
            onClick={() => setGroupFilter(null)}
            className={`px-3 py-1 rounded-full text-xs font-mono border transition-colors ${
              groupFilter === null
                ? "border-accent dark:border-ember text-accent dark:text-ember bg-tint/50 dark:bg-emboss"
                : "border-line dark:border-edge text-muted dark:text-fog hover:text-ink dark:hover:text-bone bg-white dark:bg-panel"
            }`}
          >
            All
          </button>
          {groups.map((g) => (
            <button
              key={g.id}
              onClick={() => setGroupFilter(g.id)}
              className={`px-3 py-1 rounded-full text-xs font-mono border transition-colors ${
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

      {/* Server grid */}
      {isLoading && (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
          {[1, 2, 3].map((i) => (
            <div key={i} className="bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-4 h-40 animate-pulse" />
          ))}
        </div>
      )}

      {isError && (
        <div className="text-brick text-sm p-4 bg-red-50 dark:bg-red-950/30 border border-brick/30 rounded-card">
          Failed to load servers. <button onClick={() => void refetch()} className="underline font-medium">Retry</button>
        </div>
      )}

      {!isLoading && !isError && filtered.length === 0 && (
        <div className="text-center py-16 text-muted dark:text-fog">
          <Server size={40} className="mx-auto mb-3 opacity-30" />
          <p className="text-sm">No servers yet. Add one to get started.</p>
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
