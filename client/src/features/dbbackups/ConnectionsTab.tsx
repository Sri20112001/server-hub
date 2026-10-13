import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Pencil, Plus, RefreshCw, Trash2 } from "lucide-react";
import { dbBackupsApi } from "../../lib/api";
import type { VgConnection, VgConnectionInput } from "../../lib/types";
import { ConfirmModal, EmptyState, Modal } from "../../components/ui";
import { useUi } from "../../stores/store";
import { InlineError } from "./shared";
import { ghostBtn, inputCls, labelCls, primaryBtn, statusDot, timeStr, useDbBackupsRole } from "./helpers";

const TYPES = ["POSTGRES", "MONGODB", "MSSQL"];

function ConnModal({ initial, onClose, onSaved }: {
  initial?: VgConnection;
  onClose: () => void;
  onSaved: () => void;
}) {
  const { pushToast } = useUi();
  const [busy, setBusy] = useState(false);
  const [form, setForm] = useState<VgConnectionInput>(() => initial ? {
    name: initial.name,
    type: initial.type,
    agent_id: initial.agent_id,
    host: initial.host,
    port: initial.port,
    username: initial.username,
  } : { name: "", type: "POSTGRES", agent_id: "", host: "", port: 5432, username: "" });
  const [password, setPassword] = useState("");

  const { data: agents = [] } = useQuery({ queryKey: ["vg-agents"], queryFn: dbBackupsApi.agents });

  const submit = async () => {
    if (!form.name.trim() || !form.agent_id || !form.host.trim()) return;
    setBusy(true);
    try {
      // Password is write-only: sent on create, or only when the field is
      // non-empty on edit (rotating the stored secret). It is never read
      // back, logged, or placed in a URL.
      const body = password ? { ...form, password } : form;
      if (initial) await dbBackupsApi.updateConnection(initial.id, body);
      else await dbBackupsApi.createConnection(body);
      pushToast(initial ? `Connection "${form.name}" updated` : `Connection "${form.name}" created`);
      onSaved();
      onClose();
    } catch (e) {
      pushToast(e instanceof Error ? e.message : "Failed", true);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal onClose={onClose}>
      <h3 className="font-head font-bold text-[18px] text-ink dark:text-bone mb-1">
        {initial ? "Edit connection" : "New connection"}
      </h3>
      <p className="text-muted dark:text-fog text-[13px] mb-4">
        Saved database logins for backup jobs. Passwords are stored encrypted and never shown again.
      </p>
      <div className="space-y-3">
        <div className="grid grid-cols-2 gap-2">
          <div>
            <label className={labelCls}>Name *</label>
            <input className={inputCls} value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="Production DB" />
          </div>
          <div>
            <label className={labelCls}>Type *</label>
            <select className={inputCls} value={form.type} onChange={(e) => setForm({ ...form, type: e.target.value })}>
              {TYPES.map((t) => <option key={t} value={t}>{t}</option>)}
            </select>
          </div>
        </div>
        <div>
          <label className={labelCls}>Agent *</label>
          <select className={inputCls} value={form.agent_id} onChange={(e) => setForm({ ...form, agent_id: e.target.value })}>
            <option value="">Select...</option>
            {agents.map((a) => <option key={a.id} value={a.id}>{a.name}</option>)}
          </select>
        </div>
        <div className="grid grid-cols-3 gap-2">
          <div className="col-span-2">
            <label className={labelCls}>Host *</label>
            <input className={`${inputCls} font-mono`} value={form.host} onChange={(e) => setForm({ ...form, host: e.target.value })} placeholder="db.internal" />
          </div>
          <div>
            <label className={labelCls}>Port</label>
            <input type="number" min={1} max={65535} className={`${inputCls} font-mono`} value={form.port ?? ""} onChange={(e) => setForm({ ...form, port: Number(e.target.value) })} placeholder="5432" />
          </div>
        </div>
        <div className="grid grid-cols-2 gap-2">
          <div>
            <label className={labelCls}>Username</label>
            <input className={`${inputCls} font-mono`} value={form.username ?? ""} onChange={(e) => setForm({ ...form, username: e.target.value })} autoComplete="off" />
          </div>
          <div>
            <label className={labelCls}>{initial ? "New password (leave blank to keep)" : "Password"}</label>
            <input type="password" className={`${inputCls} font-mono`} value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="new-password" />
          </div>
        </div>
      </div>
      <div className="flex gap-2 mt-5 justify-end">
        <button onClick={onClose} className={ghostBtn}>Cancel</button>
        <button onClick={submit} disabled={busy || !form.name.trim() || !form.agent_id || !form.host.trim()} className={primaryBtn}>
          {busy ? "Savingâ€¦" : initial ? "Save" : "Create"}
        </button>
      </div>
    </Modal>
  );
}

export function ConnectionsTab() {
  const qc = useQueryClient();
  const { pushToast } = useUi();
  const { canMutate, isAdmin } = useDbBackupsRole();
  const [showCreate, setShowCreate] = useState(false);
  const [editing, setEditing] = useState<VgConnection | null>(null);
  const [deleting, setDeleting] = useState<VgConnection | null>(null);
  const [busy, setBusy] = useState(false);
  const [testingId, setTestingId] = useState<string | null>(null);

  const { data: conns = [], isLoading, isError, error, refetch } = useQuery({
    queryKey: ["vg-connections"],
    queryFn: dbBackupsApi.connections,
    refetchInterval: 30_000,
  });

  const refresh = () => {
    void qc.invalidateQueries({ queryKey: ["vg-connections"] });
    void refetch();
  };

  const testConn = async (id: string) => {
    setTestingId(id);
    try {
      const res = await dbBackupsApi.testConnection(id);
      const detail = res.databases && res.databases.length > 0
        ? ` Â· ${res.databases.length} database${res.databases.length === 1 ? "" : "s"} found`
        : "";
      pushToast(res.status === "ok" ? `Reachable${res.latency_ms != null ? ` (${res.latency_ms}ms)` : ""}${detail}` : (res.error ?? "Test failed"), res.status !== "ok");
      refresh();
    } catch (e) {
      pushToast(e instanceof Error ? e.message : "Failed", true);
    } finally {
      setTestingId(null);
    }
  };

  const confirmDelete = async () => {
    if (!deleting) return;
    setBusy(true);
    try {
      await dbBackupsApi.deleteConnection(deleting.id);
      pushToast("Connection deleted");
      setDeleting(null);
      refresh();
    } catch (e) {
      pushToast(e instanceof Error ? e.message : "Failed", true);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between flex-wrap gap-2.5">
        <p className="text-on-surface-variant text-xs">
          Saved logins for database jobs. Live schema browsing happens on the agent; the server only checks reachability.
        </p>
        <span className="flex items-center gap-2">
          <button onClick={refresh} aria-label="Refresh"
            className="w-10 h-10 rounded-full bg-surface-container-lowest border border-outline-variant/30 flex items-center justify-center text-on-surface-variant hover:text-on-surface cursor-pointer shadow-sm transition-colors">
            <RefreshCw size={15} />
          </button>
          {canMutate && (
            <button onClick={() => setShowCreate(true)}
              className="flex items-center gap-1.5 px-5 py-2 bg-primary text-on-primary text-sm font-semibold rounded-full cursor-pointer shadow-sm hover:bg-primary/90 transition-all">
              <Plus size={15} /> New connection
            </button>
          )}
        </span>
      </div>

      {isLoading && (
        <div className="space-y-3">
          {[1, 2].map((i) => <div key={i} className="h-20 rounded-2xl bg-surface-container-low border border-outline-variant/20 animate-pulse" />)}
        </div>
      )}
      {!isLoading && isError && (
        <InlineError message={error instanceof Error ? error.message : "Could not load connections."} onRetry={refresh} />
      )}
      {!isLoading && !isError && conns.length === 0 && (
        <EmptyState title="No connections" hint="Save a database login so backup jobs can use it." />
      )}
      {conns.map((c) => (
        <div key={c.id} className="bg-surface-container-lowest border border-outline-variant/20 rounded-2xl p-4 shadow-sm">
          <div className="flex items-start justify-between gap-3">
            <div className="flex items-center gap-2.5 min-w-0">
              <span className={`w-2.5 h-2.5 rounded-full shrink-0 ${statusDot(c.status)}`} />
              <div className="min-w-0">
                <p className="font-semibold text-sm text-on-surface truncate">{c.name}</p>
                <p className="text-xs text-on-surface-variant font-mono truncate mt-0.5">
                  {c.type} · {c.username ? `${c.username}@` : ""}{c.host}:{c.port}
                  {c.database_count > 0 ? ` · ${c.database_count} databases` : ""}
                </p>
              </div>
            </div>
            <span className="flex items-center gap-1.5 shrink-0">
              {canMutate && (
                <>
                  <button disabled={testingId === c.id} onClick={() => void testConn(c.id)}
                    className="text-xs px-3.5 py-1.5 rounded-full border border-outline-variant/30 text-on-surface-variant hover:text-on-surface bg-surface-container-low hover:bg-surface-container cursor-pointer disabled:opacity-50 transition-colors">
                    {testingId === c.id ? "Testing…" : "Test"}
                  </button>
                  <button onClick={() => setEditing(c)} className="text-on-surface-variant hover:text-on-surface p-1.5 rounded-full cursor-pointer transition-colors" title="Edit">
                    <Pencil size={14} />
                  </button>
                </>
              )}
              {isAdmin && (
                <button onClick={() => setDeleting(c)} className="text-on-surface-variant hover:text-error p-1.5 rounded-full cursor-pointer transition-colors" title="Delete">
                  <Trash2 size={14} />
                </button>
              )}
            </span>
          </div>
          {c.last_checked_at && (
            <p className="text-[11px] text-on-surface-variant font-mono mt-1.5">Last checked {timeStr(c.last_checked_at)}</p>
          )}
        </div>
      ))}

      {showCreate && <ConnModal onClose={() => setShowCreate(false)} onSaved={refresh} />}
      {editing && <ConnModal initial={editing} onClose={() => setEditing(null)} onSaved={refresh} />}
      {deleting && (
        <ConfirmModal
          title={`Delete "${deleting.name}"?`}
          body="Jobs using this login must be reassigned first. Saved passwords are destroyed with it."
          confirmLabel="Delete connection"
          busy={busy}
          onClose={() => setDeleting(null)}
          onConfirm={() => void confirmDelete()}
        />
      )}
    </div>
  );
}
