import { useEffect, useState } from "react";
import { useNavigate } from "react-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus, RefreshCw, Server } from "lucide-react";
import { api } from "../../lib/api";
import type { ManagedServer } from "../../lib/types";
import { useEvents } from "../../lib/useEvents";
import { useUi } from "../../stores/store";

const STATUS_COLOR: Record<string, string> = {
  ONLINE: "bg-green-500",
  OFFLINE: "bg-red-500",
  WARNING: "bg-yellow-400",
  UNKNOWN: "bg-zinc-500",
};

const STATUS_TEXT: Record<string, string> = {
  ONLINE: "text-green-400",
  OFFLINE: "text-red-400",
  WARNING: "text-yellow-400",
  UNKNOWN: "text-zinc-400",
};

function ServerCard({ s, onClick }: { s: ManagedServer; onClick: () => void }) {
  return (
    <button
      onClick={onClick}
      className="w-full text-left bg-panel border border-edge rounded-xl p-4 hover:border-ember/50 transition-colors cursor-pointer"
    >
      <div className="flex items-start justify-between mb-3">
        <div className="flex items-center gap-2">
          <span className={`w-2 h-2 rounded-full ${STATUS_COLOR[s.status] ?? "bg-zinc-500"}`} />
          <span className="font-semibold text-bone text-sm">{s.name}</span>
        </div>
        <span className={`text-xs font-mono ${STATUS_TEXT[s.status] ?? "text-zinc-400"}`}>
          {s.status}
        </span>
      </div>
      <div className="text-xs text-fog font-mono space-y-0.5">
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
        <div className="mt-2 text-[10px] text-fog font-mono">
          Last seen: {new Date(s.lastHeartbeat).toLocaleString()}
        </div>
      )}
    </button>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="bg-emboss rounded-lg p-2">
      <div className="text-fog text-[10px] uppercase tracking-wider">{label}</div>
      <div className="text-bone font-mono text-xs mt-0.5 truncate">{value}</div>
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
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4" onClick={onClose}>
      <div className="bg-panel border border-edge rounded-xl p-6 w-full max-w-md" onClick={(e) => e.stopPropagation()}>
        <h2 className="text-bone font-bold text-lg mb-4">Add Server</h2>
        <div className="space-y-3">
          <Field label="Name *" value={name} onChange={setName} placeholder="production-api" />
          <Field label="Hostname" value={hostname} onChange={setHostname} placeholder="api.example.com" />
          <Field label="IP Address" value={ip} onChange={setIp} placeholder="192.168.1.10" />
        </div>
        <p className="text-fog text-xs mt-3">
          After adding, generate an agent token and install the agent on the server.
        </p>
        <div className="flex gap-2 mt-5 justify-end">
          <button onClick={onClose} className="px-4 py-2 text-sm text-fog hover:text-bone border border-edge rounded-lg">
            Cancel
          </button>
          <button
            onClick={submit}
            disabled={busy || !name.trim()}
            className="px-4 py-2 text-sm bg-ember text-black font-semibold rounded-lg disabled:opacity-50"
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
      <label className="text-fog text-xs mb-1 block">{label}</label>
      <input
        className="w-full bg-emboss border border-edge rounded-lg px-3 py-2 text-bone text-sm outline-none focus:border-ember"
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
  };

  return (
    <div className="min-h-screen bg-abyss pt-20 pb-28 px-4 sm:px-6 max-w-6xl mx-auto">
      {/* Header */}
      <div className="flex items-center justify-between mb-6">
        <div className="flex items-center gap-3">
          <Server size={22} className="text-ember" />
          <h1 className="text-bone font-bold text-xl">Servers</h1>
        </div>
        <div className="flex gap-2">
          <button
            onClick={() => void refetch()}
            className="w-9 h-9 rounded-full bg-emboss flex items-center justify-center text-fog hover:text-bone"
            aria-label="Refresh"
          >
            <RefreshCw size={15} />
          </button>
          <button
            onClick={() => setShowAdd(true)}
            className="flex items-center gap-1.5 px-3 py-2 bg-ember text-black text-sm font-semibold rounded-lg"
          >
            <Plus size={15} /> Add Server
          </button>
        </div>
      </div>

      {/* Summary */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 mb-6">
        {[
          { label: "Total", value: counts.total, color: "text-bone" },
          { label: "Online", value: counts.online, color: "text-green-400" },
          { label: "Offline", value: counts.offline, color: "text-red-400" },
          { label: "Warning", value: counts.warning, color: "text-yellow-400" },
        ].map((c) => (
          <div key={c.label} className="bg-panel border border-edge rounded-xl p-4">
            <div className="text-fog text-xs uppercase tracking-wider mb-1">{c.label}</div>
            <div className={`font-bold text-2xl font-mono ${c.color}`}>{c.value}</div>
          </div>
        ))}
      </div>

      {/* Group filter */}
      {groups.length > 0 && (
        <div className="flex gap-2 mb-4 flex-wrap">
          <button
            onClick={() => setGroupFilter(null)}
            className={`px-3 py-1 rounded-full text-xs font-mono border ${groupFilter === null ? "border-ember text-ember" : "border-edge text-fog"}`}
          >
            All
          </button>
          {groups.map((g) => (
            <button
              key={g.id}
              onClick={() => setGroupFilter(g.id)}
              className={`px-3 py-1 rounded-full text-xs font-mono border ${groupFilter === g.id ? "border-ember text-ember" : "border-edge text-fog"}`}
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
            <div key={i} className="bg-panel border border-edge rounded-xl p-4 h-40 animate-pulse" />
          ))}
        </div>
      )}

      {isError && (
        <div className="text-red-400 text-sm p-4 bg-panel border border-edge rounded-xl">
          Failed to load servers. <button onClick={() => void refetch()} className="underline">Retry</button>
        </div>
      )}

      {!isLoading && !isError && filtered.length === 0 && (
        <div className="text-center py-16 text-fog">
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
    </div>
  );
}
