import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Pencil, Plus, RefreshCw, Trash2 } from "lucide-react";
import { dbBackupsApi } from "../../lib/api";
import type { VgBackupPolicy, VgPolicyInput } from "../../lib/types";
import { ConfirmModal, EmptyState, Modal } from "../../components/ui";
import { useUi } from "../../stores/store";
import { InlineError } from "./shared";
import { ghostBtn, inputCls, labelCls, primaryBtn, useDbBackupsRole } from "./helpers";

function PolicyModal({ initial, onClose, onSaved }: {
  initial?: VgBackupPolicy;
  onClose: () => void;
  onSaved: () => void;
}) {
  const { pushToast } = useUi();
  const [busy, setBusy] = useState(false);
  const [form, setForm] = useState<VgPolicyInput>(() => ({
    name: initial?.name ?? "",
    strategy: initial?.strategy ?? "FULL",
    cron_expr: initial?.cron_expr ?? "",
    timezone: initial?.timezone ?? "UTC",
    enabled: initial?.enabled ?? true,
    mode: initial?.mode ?? "NORMAL",
    encrypted: initial?.encrypted ?? false,
    retention_days: initial?.retention_days ?? 30,
    verification_enabled: initial?.verification_enabled ?? true,
    max_retries: initial?.max_retries ?? 0,
    retry_delay_seconds: initial?.retry_delay_seconds ?? 300,
    sla_target_minutes: initial?.sla_target_minutes ?? 0,
    rpo_target_minutes: initial?.rpo_target_minutes ?? 0,
    rto_target_minutes: initial?.rto_target_minutes ?? 0,
  }));

  const set = (k: keyof VgPolicyInput, v: string | number | boolean) =>
    setForm((f) => ({ ...f, [k]: v }));

  const submit = async () => {
    if (!form.name.trim()) return;
    setBusy(true);
    try {
      if (initial) await dbBackupsApi.updatePolicy(initial.id, form);
      else await dbBackupsApi.createPolicy(form);
      pushToast(initial ? `Policy "${form.name}" updated` : `Policy "${form.name}" created`);
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
        {initial ? "Edit policy" : "New policy"}
      </h3>
      <p className="text-muted dark:text-fog text-[13px] mb-4">
        Reusable backup rules. Jobs linked to a policy follow it; unlinked jobs use their own settings.
      </p>
      <div className="space-y-3">
        <div className="grid grid-cols-2 gap-2">
          <div>
            <label className={labelCls}>Name *</label>
            <input className={inputCls} value={form.name} onChange={(e) => set("name", e.target.value)} placeholder="Nightly standard" />
          </div>
          <div>
            <label className={labelCls}>Strategy</label>
            <select className={inputCls} value={form.strategy ?? "FULL"} onChange={(e) => set("strategy", e.target.value)}>
              <option value="FULL">Full (only supported kind)</option>
            </select>
          </div>
        </div>
        <div className="grid grid-cols-2 gap-2">
          <div>
            <label className={labelCls}>Schedule (cron, empty = manual)</label>
            <input className={`${inputCls} font-mono`} value={form.cron_expr ?? ""} onChange={(e) => set("cron_expr", e.target.value)} placeholder="0 2 * * *" />
          </div>
          <div>
            <label className={labelCls}>Timezone</label>
            <input className={inputCls} value={form.timezone ?? "UTC"} onChange={(e) => set("timezone", e.target.value)} />
          </div>
        </div>
        <div className="grid grid-cols-3 gap-2">
          <div>
            <label className={labelCls}>Mode</label>
            <select className={inputCls} value={form.mode ?? "NORMAL"} onChange={(e) => set("mode", e.target.value)}>
              <option value="NORMAL">Normal</option>
              <option value="COMPRESSED">Compressed</option>
            </select>
          </div>
          <div>
            <label className={labelCls}>Keep (days)</label>
            <input type="number" min={0} max={3650} className={inputCls} value={form.retention_days ?? 30} onChange={(e) => set("retention_days", Number(e.target.value))} />
          </div>
          <div>
            <label className={labelCls}>Retries (0â€“10)</label>
            <input type="number" min={0} max={10} className={inputCls} value={form.max_retries ?? 0} onChange={(e) => set("max_retries", Number(e.target.value))} />
          </div>
        </div>
        <div className="grid grid-cols-3 gap-2">
          <div>
            <label className={labelCls}>Retry wait (s)</label>
            <input type="number" min={0} max={86400} className={inputCls} value={form.retry_delay_seconds ?? 300} onChange={(e) => set("retry_delay_seconds", Number(e.target.value))} />
          </div>
          <div>
            <label className={labelCls}>SLA (min, 0 = off — breaches on Overview)</label>
            <input type="number" min={0} max={525600} className={inputCls} value={form.sla_target_minutes ?? 0} onChange={(e) => set("sla_target_minutes", Number(e.target.value))} />
          </div>
          <div>
            <label className={labelCls}>RPO (min, 0 = off — breaches on Overview)</label>
            <input type="number" min={0} max={525600} className={inputCls} value={form.rpo_target_minutes ?? 0} onChange={(e) => set("rpo_target_minutes", Number(e.target.value))} />
          </div>
        </div>
        <p className="text-muted dark:text-fog text-xs">
          RTO is recorded for planning only — it is not enforced. Size baselines are tracked automatically for future anomaly detection.
        </p>
        <div className="flex gap-4 flex-wrap">
          <label className="inline-flex items-center gap-2 text-sm text-ink dark:text-bone cursor-pointer">
            <input type="checkbox" checked={!!form.enabled} onChange={(e) => set("enabled", e.target.checked)} className="w-4 h-4" /> Enabled
          </label>
          <label className="inline-flex items-center gap-2 text-sm text-ink dark:text-bone cursor-pointer">
            <input type="checkbox" checked={!!form.encrypted} onChange={(e) => set("encrypted", e.target.checked)} className="w-4 h-4" /> Encrypted
          </label>
          <label className="inline-flex items-center gap-2 text-sm text-ink dark:text-bone cursor-pointer">
            <input type="checkbox" checked={form.verification_enabled ?? true} onChange={(e) => set("verification_enabled", e.target.checked)} className="w-4 h-4" /> Verify
          </label>
        </div>
      </div>
      <div className="flex gap-2 mt-5 justify-end">
        <button onClick={onClose} className={ghostBtn}>Cancel</button>
        <button onClick={submit} disabled={busy || !form.name.trim()} className={primaryBtn}>
          {busy ? "Savingâ€¦" : initial ? "Save" : "Create"}
        </button>
      </div>
    </Modal>
  );
}

export function PoliciesTab() {
  const qc = useQueryClient();
  const { pushToast } = useUi();
  const { canMutate, isAdmin } = useDbBackupsRole();
  const [showCreate, setShowCreate] = useState(false);
  const [editing, setEditing] = useState<VgBackupPolicy | null>(null);
  const [deleting, setDeleting] = useState<VgBackupPolicy | null>(null);
  const [busy, setBusy] = useState(false);

  const { data: policies = [], isLoading, isError, error, refetch } = useQuery({
    queryKey: ["vg-policies"],
    queryFn: dbBackupsApi.policies,
    refetchInterval: 30_000,
  });

  const refresh = () => {
    void qc.invalidateQueries({ queryKey: ["vg-policies"] });
    void refetch();
  };

  const confirmDelete = async () => {
    if (!deleting) return;
    setBusy(true);
    try {
      await dbBackupsApi.deletePolicy(deleting.id);
      pushToast("Policy deleted â€” linked jobs fall back to their own settings");
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
        <p className="text-on-surface-variant text-xs">Reusable rules: schedule, keeping time, retries, and targets. Jobs without a policy use inline settings.</p>
        <span className="flex items-center gap-2">
          <button onClick={refresh} aria-label="Refresh"
            className="w-10 h-10 rounded-full bg-surface-container-lowest border border-outline-variant/30 flex items-center justify-center text-on-surface-variant hover:text-on-surface cursor-pointer shadow-sm transition-colors">
            <RefreshCw size={15} />
          </button>
          {canMutate && (
            <button onClick={() => setShowCreate(true)}
              className="flex items-center gap-1.5 px-5 py-2 bg-primary text-on-primary text-sm font-semibold rounded-full cursor-pointer shadow-sm hover:bg-primary/90 transition-all">
              <Plus size={15} /> New policy
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
        <InlineError message={error instanceof Error ? error.message : "Could not load policies."} onRetry={refresh} />
      )}
      {!isLoading && !isError && policies.length === 0 && (
        <EmptyState title="No policies" hint="Policies are optional — jobs work fine with inline settings." />
      )}
      {policies.map((p) => (
        <div key={p.id} className="bg-surface-container-lowest border border-outline-variant/20 rounded-2xl p-4 shadow-sm flex items-start justify-between gap-3">
          <div className="min-w-0">
            <p className="font-semibold text-sm text-on-surface truncate">
              {p.name}
              {!p.enabled && <span className="ml-2 text-[11px] font-mono text-on-surface-variant px-2 py-0.5 rounded-full bg-surface-container-low border border-outline-variant/30">paused</span>}
            </p>
            <p className="text-xs text-on-surface-variant font-mono mt-0.5">
              {p.cron_expr || "manual"}{p.timezone ? ` · ${p.timezone}` : ""} · keep {p.retention_days}d · retries {p.max_retries}
            </p>
          </div>
          {canMutate && (
            <span className="flex items-center gap-1.5 shrink-0">
              <button onClick={() => setEditing(p)} className="text-on-surface-variant hover:text-on-surface p-1.5 rounded-full cursor-pointer transition-colors" title="Edit">
                <Pencil size={14} />
              </button>
              {isAdmin && (
                <button onClick={() => setDeleting(p)} className="text-on-surface-variant hover:text-error p-1.5 rounded-full cursor-pointer transition-colors" title="Delete">
                  <Trash2 size={14} />
                </button>
              )}
            </span>
          )}
        </div>
      ))}

      {showCreate && <PolicyModal onClose={() => setShowCreate(false)} onSaved={refresh} />}
      {editing && <PolicyModal initial={editing} onClose={() => setEditing(null)} onSaved={refresh} />}
      {deleting && (
        <ConfirmModal
          title={`Delete policy "${deleting.name}"?`}
          body="Linked jobs keep working â€” they fall back to their own inline settings."
          confirmLabel="Delete policy"
          busy={busy}
          onClose={() => setDeleting(null)}
          onConfirm={() => void confirmDelete()}
        />
      )}
    </div>
  );
}
