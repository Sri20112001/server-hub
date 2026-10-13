import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ChevronDown, ChevronUp, Pencil, Play, Plus, RefreshCw, ShieldCheck, Trash2 } from "lucide-react";
import { dbBackupsApi } from "../../lib/api";
import type { VgBackupJob, VgJobInput, VgPreflight } from "../../lib/types";
import { ConfirmModal, EmptyState, Modal } from "../../components/ui";
import { useUi } from "../../stores/store";
import { InlineError } from "./shared";
import { ghostBtn, inputCls, labelCls, primaryBtn, useDbBackupsRole } from "./helpers";

const SOURCES = ["FILESYSTEM", "POSTGRES", "MONGODB", "SQL_SERVER", "MSSQL_SERVER", "DBF"];
const DB_SOURCES = new Set(["POSTGRES", "MONGODB", "SQL_SERVER", "MSSQL_SERVER"]);

function emptyJob(): VgJobInput {
  return {
    name: "",
    agent_id: "",
    storage_target_id: "",
    source_type: "FILESYSTEM",
    mode: "NORMAL",
    retention_days: 30,
    timezone: "UTC",
  };
}

function JobModal({ initial, onClose, onSaved }: {
  initial?: VgBackupJob;
  onClose: () => void;
  onSaved: () => void;
}) {
  const { pushToast } = useUi();
  const [busy, setBusy] = useState(false);
  const [form, setForm] = useState<VgJobInput>(() => initial ? {
    name: initial.name,
    agent_id: initial.agent_id,
    storage_target_id: initial.storage_target_id,
    source_type: initial.source_type,
    connection_id: initial.connection_id ?? "",
    source_path: initial.source_path,
    source_database: initial.source_database,
    include_patterns: initial.include_patterns,
    exclude_patterns: initial.exclude_patterns,
    mode: initial.mode,
    encrypted: initial.encrypted,
    retention_days: initial.retention_days,
    export_format: initial.export_format,
    cron_expr: initial.schedule?.cron_expr ?? "",
    timezone: initial.schedule?.timezone ?? "UTC",
    policy_id: initial.policy_id ?? "",
    sla_target_minutes: initial.sla_target_minutes,
    rpo_target_minutes: initial.rpo_target_minutes,
    rto_target_minutes: initial.rto_target_minutes,
  } : emptyJob());

  const { data: agents = [] } = useQuery({ queryKey: ["vg-agents"], queryFn: dbBackupsApi.agents });
  const { data: targets = [] } = useQuery({ queryKey: ["vg-targets"], queryFn: dbBackupsApi.targets });
  const { data: connections = [] } = useQuery({ queryKey: ["vg-connections"], queryFn: dbBackupsApi.connections });
  const { data: policies = [] } = useQuery({ queryKey: ["vg-policies"], queryFn: dbBackupsApi.policies });

  const set = (k: keyof VgJobInput, v: string | number | boolean) =>
    setForm((f) => ({ ...f, [k]: v }));

  const submit = async () => {
    if (!form.name.trim() || !form.agent_id || !form.storage_target_id) return;
    setBusy(true);
    try {
      if (initial) await dbBackupsApi.updateJob(initial.id, form);
      else await dbBackupsApi.createJob(form);
      pushToast(initial ? `Job "${form.name}" updated` : `Job "${form.name}" created`);
      onSaved();
      onClose();
    } catch (e) {
      pushToast(e instanceof Error ? e.message : "Failed", true);
    } finally {
      setBusy(false);
    }
  };

  const dbSource = DB_SOURCES.has(form.source_type);
  return (
    <Modal onClose={onClose}>
      <h3 className="font-head font-bold text-[18px] text-ink dark:text-bone mb-4">
        {initial ? "Edit backup job" : "New backup job"}
      </h3>
      <div className="space-y-3">
        <div>
          <label className={labelCls}>Name *</label>
          <input className={inputCls} value={form.name} onChange={(e) => set("name", e.target.value)} placeholder="Nightly website files" />
        </div>
        <div className="grid grid-cols-2 gap-2">
          <div>
            <label className={labelCls}>Agent *</label>
            <select className={inputCls} value={form.agent_id} onChange={(e) => set("agent_id", e.target.value)}
              disabled={!!initial} title={initial ? "Agent cannot be changed after creation — recreate the job to move it" : undefined}>
              <option value="">Select...</option>
              {agents.map((a) => <option key={a.id} value={a.id}>{a.name}</option>)}
            </select>
          </div>
          <div>
            <label className={labelCls}>Storage *</label>
            <select className={inputCls} value={form.storage_target_id} onChange={(e) => set("storage_target_id", e.target.value)}
              disabled={!!initial} title={initial ? "Storage cannot be changed after creation — recreate the job to move it" : undefined}>
              <option value="">Select...</option>
              {targets.map((t) => <option key={t.id} value={t.id}>{t.name} ({t.type})</option>)}
            </select>
          </div>
        </div>
        <div className="grid grid-cols-2 gap-2">
          <div>
            <label className={labelCls}>Source type *</label>
            <select className={inputCls} value={form.source_type} onChange={(e) => set("source_type", e.target.value)}
              disabled={!!initial} title={initial ? "Source type cannot be changed after creation — recreate the job" : undefined}>
              {SOURCES.map((s) => <option key={s} value={s}>{s}</option>)}
            </select>
          </div>
          <div>
            <label className={labelCls}>Policy (optional)</label>
            <select className={inputCls} value={form.policy_id ?? ""} onChange={(e) => set("policy_id", e.target.value)}>
              <option value="">Inline settings</option>
              {policies.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
            </select>
          </div>
        </div>
        {dbSource ? (
          <div className="grid grid-cols-2 gap-2">
            <div>
              <label className={labelCls}>Connection</label>
              <select className={inputCls} value={form.connection_id ?? ""} onChange={(e) => set("connection_id", e.target.value)}>
                <option value="">None (agent environment)</option>
                {connections.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
              </select>
            </div>
            <div>
              <label className={labelCls}>Database *</label>
              <input className={inputCls} value={form.source_database ?? ""} onChange={(e) => set("source_database", e.target.value)} placeholder="app_production" />
            </div>
          </div>
        ) : (
          <div>
            <label className={labelCls}>Source path</label>
            <input className={inputCls} value={form.source_path ?? ""} onChange={(e) => set("source_path", e.target.value)} placeholder="/var/www" />
          </div>
        )}
        <div className="grid grid-cols-3 gap-2">
          <div>
            <label className={labelCls}>Mode</label>
            <select className={inputCls} value={form.mode ?? "NORMAL"} onChange={(e) => set("mode", e.target.value)}>
              <option value="NORMAL">Normal</option>
              <option value="COMPRESSED">Compressed</option>
            </select>
          </div>
          <div>
            <label className={labelCls}>Retention (days)</label>
            <input type="number" min={0} className={inputCls} value={form.retention_days ?? 30} onChange={(e) => set("retention_days", Number(e.target.value))} />
          </div>
          <div className="flex items-end pb-2">
            <label className="inline-flex items-center gap-2 text-sm text-ink dark:text-bone cursor-pointer">
              <input type="checkbox" checked={!!form.encrypted} onChange={(e) => set("encrypted", e.target.checked)} className="w-4 h-4" />
              Encrypted
            </label>
          </div>
        </div>
        <div className="grid grid-cols-2 gap-2">
          <div>
            <label className={labelCls}>Schedule (cron, optional)</label>
            <input className={`${inputCls} font-mono`} value={form.cron_expr ?? ""} onChange={(e) => set("cron_expr", e.target.value)} placeholder="0 2 * * *" />
          </div>
          <div>
            <label className={labelCls}>Timezone</label>
            <input className={inputCls} value={form.timezone ?? "UTC"} onChange={(e) => set("timezone", e.target.value)} placeholder="UTC" />
          </div>
        </div>
      </div>
      <div className="flex gap-2 mt-5 justify-end">
        <button onClick={onClose} className={ghostBtn}>Cancel</button>
        <button
          onClick={submit}
          disabled={busy || !form.name.trim() || !form.agent_id || !form.storage_target_id}
          className={primaryBtn}
        >
          {busy ? "Savingâ€¦" : initial ? "Save" : "Create"}
        </button>
      </div>
    </Modal>
  );
}

function JobCard({ job, onChanged }: { job: VgBackupJob; onChanged: () => void }) {
  const { pushToast } = useUi();
  const { canMutate, isAdmin } = useDbBackupsRole();
  const [expanded, setExpanded] = useState(false);
  const [editing, setEditing] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [busy, setBusy] = useState(false);
  const [preflight, setPreflight] = useState<VgPreflight | null>(null);
  const [preflightBusy, setPreflightBusy] = useState(false);

  const { data: runsPage } = useQuery({
    queryKey: ["vg-runs", job.id],
    queryFn: () => dbBackupsApi.runs({ job_id: job.id, limit: 5 }),
    enabled: expanded,
    refetchInterval: 15_000,
  });

  const act = async (label: string, fn: () => Promise<unknown>) => {
    setBusy(true);
    try {
      await fn();
      pushToast(label);
      onChanged();
    } catch (e) {
      pushToast(e instanceof Error ? e.message : "Failed", true);
    } finally {
      setBusy(false);
    }
  };

  const runPreflight = async () => {
    setPreflightBusy(true);
    try {
      setPreflight(await dbBackupsApi.preflight(job.id));
    } catch (e) {
      pushToast(e instanceof Error ? e.message : "Preflight failed", true);
    } finally {
      setPreflightBusy(false);
    }
  };

  return (
    <div className="bg-surface-container-lowest border border-outline-variant/20 rounded-2xl p-5 shadow-sm">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2 flex-wrap">
            <span className="font-head font-bold text-[16px] text-on-surface">{job.name}</span>
            {!job.enabled && (
              <span className="text-[11px] font-mono px-2.5 py-0.5 rounded-full border border-outline-variant/30 text-on-surface-variant bg-surface-container-low">paused</span>
            )}
          </div>
          <p className="text-on-surface-variant text-xs font-mono mt-1">
            {job.source_type}{job.source_database ? ` · ${job.source_database}` : job.source_path ? ` · ${job.source_path}` : ""}
            {job.schedule?.cron_expr ? ` · ${job.schedule.cron_expr}` : " · manual"}
          </p>
        </div>
        <div className="flex items-center gap-1 shrink-0">
          {canMutate && (
            <>
              <button
                title={job.enabled ? "Pause schedule" : "Resume schedule"}
                disabled={busy}
                onClick={() => void act(job.enabled ? "Schedule paused" : "Schedule resumed",
                  () => job.enabled ? dbBackupsApi.disableJob(job.id) : dbBackupsApi.enableJob(job.id))}
                className={`text-xs px-3 py-1 rounded-full border font-mono font-semibold cursor-pointer disabled:opacity-50 transition-colors ${job.enabled ? "border-primary/40 text-primary bg-primary/10" : "border-outline-variant/30 text-on-surface-variant bg-surface-container-low"}`}
              >
                {job.enabled ? "ON" : "OFF"}
              </button>
              <button title="Run now" disabled={busy} onClick={() => void act("Backup started", () => dbBackupsApi.runJob(job.id))}
                className="text-on-surface-variant hover:text-primary p-2 rounded-full cursor-pointer disabled:opacity-50 transition-colors">
                <Play size={15} />
              </button>
              <button title="Edit" onClick={() => setEditing(true)} className="text-on-surface-variant hover:text-on-surface p-2 rounded-full cursor-pointer transition-colors">
                <Pencil size={14} />
              </button>
            </>
          )}
          {isAdmin && (
            <button title="Delete" onClick={() => setConfirmDelete(true)} className="text-on-surface-variant hover:text-error p-2 rounded-full cursor-pointer transition-colors">
              <Trash2 size={14} />
            </button>
          )}
          <button onClick={() => setExpanded(!expanded)} className="text-on-surface-variant hover:text-on-surface p-2 rounded-full cursor-pointer transition-colors" aria-label={expanded ? "Collapse" : "Expand"}>
            {expanded ? <ChevronUp size={15} /> : <ChevronDown size={15} />}
          </button>
        </div>
      </div>

      {expanded && (
        <div className="mt-4 border-t border-outline-variant/20 pt-4 space-y-3">
          {canMutate && (
            <button onClick={runPreflight} disabled={preflightBusy}
              className="inline-flex items-center gap-1.5 text-xs px-3.5 py-1.5 border border-outline-variant/30 rounded-full cursor-pointer text-on-surface-variant hover:text-on-surface bg-surface-container-low hover:bg-surface-container disabled:opacity-50 transition-colors">
              <ShieldCheck size={13} /> {preflightBusy ? "Checking…" : "Run preflight checks"}
            </button>
          )}
          {preflight && (
            <div className="rounded-xl border border-outline-variant/20 bg-surface-container-low p-3.5">
              <p className="text-xs font-semibold text-on-surface mb-1.5">
                Preflight: <span className={preflight.overall === "OK" ? "text-primary" : preflight.overall === "WARN" ? "text-tertiary" : "text-error"}>{preflight.overall}</span>
              </p>
              {preflight.checks.map((ch) => (
                <p key={ch.name} className="text-xs text-on-surface-variant">
                  <span className={ch.status === "OK" ? "text-primary" : ch.status === "WARN" ? "text-tertiary" : "text-error"}>● </span>{" "}
                  {ch.name}{ch.detail ? ` — ${ch.detail}` : ""}
                </p>
              ))}
            </div>
          )}
          <div>
            <p className="text-xs font-semibold text-on-surface-variant mb-1.5">Recent runs</p>
            {(runsPage?.data ?? []).length === 0 ? (
              <p className="text-xs text-on-surface-variant">No runs yet.</p>
            ) : (
              (runsPage?.data ?? []).map((r) => (
                <p key={r.id} className="text-xs font-mono text-on-surface-variant py-0.5">
                  {r.status} · {r.started_at ? new Date(r.started_at).toLocaleString() : "not started"}
                  {r.error_message ? <span className="text-error"> · {r.error_message}</span> : ""}
                </p>
              ))
            )}
          </div>
        </div>
      )}

      {editing && (
        <JobModal initial={job} onClose={() => setEditing(false)} onSaved={onChanged} />
      )}
      {confirmDelete && (
        <ConfirmModal
          title={`Delete job "${job.name}"?`}
          body="Scheduled runs stop. Past run records stay in history. This cannot be undone."
          confirmLabel="Delete job"
          busy={busy}
          onClose={() => setConfirmDelete(false)}
          onConfirm={() => void act("Job deleted", async () => {
            await dbBackupsApi.deleteJob(job.id);
            setConfirmDelete(false);
          })}
        />
      )}
    </div>
  );
}

export function JobsTab() {
  const qc = useQueryClient();
  const { canMutate } = useDbBackupsRole();
  const [q, setQ] = useState("");
  const [showCreate, setShowCreate] = useState(false);
  const { data: jobs = [], isLoading, isError, error, refetch } = useQuery({
    queryKey: ["vg-jobs"],
    queryFn: dbBackupsApi.jobs,
    refetchInterval: 30_000,
  });

  const refresh = () => {
    void qc.invalidateQueries({ queryKey: ["vg-jobs"] });
    void refetch();
  };

  const needle = q.trim().toLowerCase();
  const shown = needle ? jobs.filter((j) => j.name.toLowerCase().includes(needle)) : jobs;

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2.5 flex-wrap">
        <input
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder="Filter jobs…"
          className="flex-1 min-w-40 bg-surface-container-lowest border border-outline-variant/30 rounded-xl px-4 py-2 text-sm text-on-surface outline-none focus:border-primary placeholder:text-on-surface-variant/40 transition-colors shadow-sm"
        />
        <button onClick={refresh} aria-label="Refresh"
          className="w-10 h-10 rounded-full bg-surface-container-lowest border border-outline-variant/30 flex items-center justify-center text-on-surface-variant hover:text-on-surface cursor-pointer shadow-sm transition-colors">
          <RefreshCw size={15} />
        </button>
        {canMutate && (
          <button onClick={() => setShowCreate(true)}
            className="flex items-center gap-1.5 px-5 py-2 bg-primary text-on-primary text-sm font-semibold rounded-full cursor-pointer shadow-sm hover:bg-primary/90 transition-all">
            <Plus size={15} /> New job
          </button>
        )}
      </div>

      {isLoading && (
        <div className="space-y-3">
          {[1, 2, 3].map((i) => <div key={i} className="h-24 rounded-2xl bg-surface-container-low border border-outline-variant/20 animate-pulse" />)}
        </div>
      )}
      {!isLoading && isError && (
        <InlineError message={error instanceof Error ? error.message : "Could not load jobs."} onRetry={refresh} />
      )}
      {!isLoading && !isError && shown.length === 0 && (
        <EmptyState title="No backup jobs" hint="Create a job to start protecting a database or folder." />
      )}
      {shown.map((j) => <JobCard key={j.id} job={j} onChanged={refresh} />)}

      {showCreate && (
        <JobModal onClose={() => setShowCreate(false)} onSaved={refresh} />
      )}
    </div>
  );
}
