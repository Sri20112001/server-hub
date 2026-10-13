import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { HardDrive, Plus, RefreshCw, Trash2 } from "lucide-react";
import { dbBackupsApi } from "../../lib/api";
import type { VgStorageInput, VgStorageTarget } from "../../lib/types";
import { ConfirmModal, EmptyState, Modal } from "../../components/ui";
import { useUi } from "../../stores/store";
import { InlineError } from "./shared";
import { ghostBtn, inputCls, labelCls, primaryBtn, useDbBackupsRole } from "./helpers";

function TargetModal({ onClose, onSaved }: { onClose: () => void; onSaved: () => void }) {
  const { pushToast } = useUi();
  const [busy, setBusy] = useState(false);
  const [form, setForm] = useState<VgStorageInput>({ name: "", type: "LOCAL", path: "" });

  const submit = async () => {
    if (!form.name.trim() || !form.path?.trim()) return;
    setBusy(true);
    try {
      await dbBackupsApi.createTarget(form);
      pushToast(`Storage target "${form.name}" created`);
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
      <h3 className="font-head font-bold text-[18px] text-ink dark:text-bone mb-1">New storage target</h3>
      <p className="text-muted dark:text-fog text-[13px] mb-4">
        Only on-machine folders are supported in this phase â€” S3 comes later.
      </p>
      <div className="space-y-3">
        <div>
          <label className={labelCls}>Name *</label>
          <input className={inputCls} value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="Backup disk" />
        </div>
        <div>
          <label className={labelCls}>Folder (absolute path on the agent machine) *</label>
          <input className={`${inputCls} font-mono`} value={form.path ?? ""} onChange={(e) => setForm({ ...form, path: e.target.value })} placeholder="/var/backups" />
        </div>
      </div>
      <div className="flex gap-2 mt-5 justify-end">
        <button onClick={onClose} className={ghostBtn}>Cancel</button>
        <button onClick={submit} disabled={busy || !form.name.trim() || !form.path?.trim()} className={primaryBtn}>
          {busy ? "Creatingâ€¦" : "Create"}
        </button>
      </div>
    </Modal>
  );
}

export function StorageTab() {
  const qc = useQueryClient();
  const { pushToast } = useUi();
  const { canMutate, isAdmin } = useDbBackupsRole();
  const [showCreate, setShowCreate] = useState(false);
  const [deleting, setDeleting] = useState<VgStorageTarget | null>(null);
  const [busy, setBusy] = useState(false);
  const [testingId, setTestingId] = useState<string | null>(null);

  const { data: targets = [], isLoading, isError, error, refetch } = useQuery({
    queryKey: ["vg-targets"],
    queryFn: dbBackupsApi.targets,
    refetchInterval: 30_000,
  });

  const refresh = () => {
    void qc.invalidateQueries({ queryKey: ["vg-targets"] });
    void refetch();
  };

  const testTarget = async (id: string) => {
    setTestingId(id);
    try {
      const res = await dbBackupsApi.testTarget(id);
      pushToast(res.status === "ok" ? `Folder is writable${res.latency_ms != null ? ` (${res.latency_ms}ms)` : ""}` : (res.error ?? "Test failed"), res.status !== "ok");
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
      await dbBackupsApi.deleteTarget(deleting.id);
      pushToast("Storage target deleted");
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
        <p className="text-on-surface-variant text-xs">Folders on agent machines where backups are kept.</p>
        <span className="flex items-center gap-2">
          <button onClick={refresh} aria-label="Refresh"
            className="w-10 h-10 rounded-full bg-surface-container-lowest border border-outline-variant/30 flex items-center justify-center text-on-surface-variant hover:text-on-surface cursor-pointer shadow-sm transition-colors">
            <RefreshCw size={15} />
          </button>
          {canMutate && (
            <button onClick={() => setShowCreate(true)}
              className="flex items-center gap-1.5 px-5 py-2 bg-primary text-on-primary text-sm font-semibold rounded-full cursor-pointer shadow-sm hover:bg-primary/90 transition-all">
              <Plus size={15} /> New target
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
        <InlineError message={error instanceof Error ? error.message : "Could not load storage targets."} onRetry={refresh} />
      )}
      {!isLoading && !isError && targets.length === 0 && (
        <EmptyState title="No storage targets" hint="Add the folder your agents should back up to." />
      )}
      {targets.map((t) => (
        <div key={t.id} className="bg-surface-container-lowest border border-outline-variant/20 rounded-2xl p-4 shadow-sm flex items-start justify-between gap-3">
          <div className="flex items-center gap-3 min-w-0">
            <div className="w-9 h-9 rounded-full bg-surface-container-low flex items-center justify-center text-primary shrink-0">
              <HardDrive size={16} />
            </div>
            <div className="min-w-0">
              <p className="font-semibold text-sm text-on-surface truncate">{t.name}</p>
              <p className="text-xs text-on-surface-variant font-mono truncate mt-0.5">
                {t.type}{t.path ? ` · ${t.path}` : ""}
              </p>
            </div>
          </div>
          <span className="flex items-center gap-1.5 shrink-0">
            {canMutate && (
              <button disabled={testingId === t.id} onClick={() => void testTarget(t.id)}
                className="text-xs px-3.5 py-1.5 rounded-full border border-outline-variant/30 text-on-surface-variant hover:text-on-surface bg-surface-container-low hover:bg-surface-container cursor-pointer disabled:opacity-50 transition-colors">
                {testingId === t.id ? "Testing…" : "Test"}
              </button>
            )}
            {isAdmin && (
              <button onClick={() => setDeleting(t)} className="text-on-surface-variant hover:text-error p-1.5 rounded-full cursor-pointer transition-colors" title="Delete">
                <Trash2 size={14} />
              </button>
            )}
          </span>
        </div>
      ))}
      {showCreate && <TargetModal onClose={() => setShowCreate(false)} onSaved={refresh} />}
      {deleting && (
        <ConfirmModal
          title={`Delete "${deleting.name}"?`}
          body="Jobs using this folder must be reassigned first. Stored backups are left on disk."
          confirmLabel="Delete target"
          busy={busy}
          onClose={() => setDeleting(null)}
          onConfirm={() => void confirmDelete()}
        />
      )}
    </div>
  );
}
