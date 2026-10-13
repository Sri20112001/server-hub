import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus, RefreshCw } from "lucide-react";
import { dbBackupsApi } from "../../lib/api";
import { ConfirmModal, EmptyState, Modal } from "../../components/ui";
import { useUi } from "../../stores/store";
import { InlineError } from "./shared";
import { ghostBtn, inputCls, labelCls, primaryBtn, statusDot, timeStr, useDbBackupsRole } from "./helpers";

function RestoreModal({ onClose, onSubmit }: {
  onClose: () => void;
  onSubmit: (details: { runId: string; agentId: string; dest: string; db: string }) => void;
}) {
  const [runId, setRunId] = useState("");
  const [agentId, setAgentId] = useState("");
  const [destination, setDestination] = useState("");
  const [targetDb, setTargetDb] = useState("");

  const { data: runsPage } = useQuery({
    queryKey: ["vg-runs-completed"],
    queryFn: () => dbBackupsApi.runs({ status: "COMPLETED", limit: 50 }),
  });
  const { data: agents = [] } = useQuery({ queryKey: ["vg-agents"], queryFn: dbBackupsApi.agents });

  return (
    <Modal onClose={onClose}>
      <h3 className="font-head font-bold text-[18px] text-ink dark:text-bone mb-1">Start a restore</h3>
      <p className="text-muted dark:text-fog text-[13px] mb-4">
        Only finished backups can be restored. The agent writes the data to the destination you choose.
      </p>
      <div className="space-y-3">
        <div>
          <label className={labelCls}>Finished backup *</label>
          <select className={inputCls} value={runId} onChange={(e) => setRunId(e.target.value)}>
            <option value="">Select...</option>
            {(runsPage?.data ?? []).map((r) => (
              <option key={r.id} value={r.id}>
                {r.id.slice(0, 8)} · {timeStr(r.completed_at)}
              </option>
            ))}
          </select>
        </div>
        <div>
          <label className={labelCls}>Agent *</label>
          <select className={inputCls} value={agentId} onChange={(e) => setAgentId(e.target.value)}>
            <option value="">Select...</option>
            {agents.map((a) => <option key={a.id} value={a.id}>{a.name}</option>)}
          </select>
        </div>
        <div>
          <label className={labelCls}>Destination folder (file backups)</label>
          <input className={`${inputCls} font-mono`} value={destination} onChange={(e) => setDestination(e.target.value)}
            placeholder="/var/restore" />
        </div>
        <div>
          <label className={labelCls}>Target database (database restores)</label>
          <input className={`${inputCls} font-mono`} value={targetDb} onChange={(e) => setTargetDb(e.target.value)}
            placeholder="app_restored" />
        </div>
      </div>
      <div className="flex gap-2 mt-5 justify-end">
        <button onClick={onClose} className={ghostBtn}>Cancel</button>
        <button
          onClick={() => onSubmit({ runId, agentId, dest: destination.trim(), db: targetDb.trim() })}
          disabled={!runId || !agentId}
          className={primaryBtn}
        >
          Review & start
        </button>
      </div>
    </Modal>
  );
}

export function RestoresTab() {
  const qc = useQueryClient();
  const { pushToast } = useUi();
  const { canMutate } = useDbBackupsRole();
  const [showCreate, setShowCreate] = useState(false);
  const [pending, setPending] = useState<{ runId: string; agentId: string; dest: string; db: string } | null>(null);
  const [busy, setBusy] = useState(false);

  const { data: restores = [], isLoading, isError, error, refetch } = useQuery({
    queryKey: ["vg-restores"],
    queryFn: dbBackupsApi.restores,
    refetchInterval: 15_000,
  });

  const refresh = () => {
    void qc.invalidateQueries({ queryKey: ["vg-restores"] });
    void refetch();
  };

  // The modal collects details; this confirmation is the explicit
  // overwrite acknowledgement the backend requires (confirmed: true).
  const confirmStart = async () => {
    if (!pending) return;
    setBusy(true);
    try {
      await dbBackupsApi.createRestore({
        backup_run_id: pending.runId,
        agent_id: pending.agentId,
        destination_path: pending.dest || undefined,
        target_database: pending.db || undefined,
        confirmed: true,
      });
      pushToast("Restore started");
      setPending(null);
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
          Restores replay a finished backup onto an agent. The destination is overwritten.
        </p>
        <span className="flex items-center gap-2">
          <button onClick={refresh} aria-label="Refresh"
            className="w-10 h-10 rounded-full bg-surface-container-lowest border border-outline-variant/30 flex items-center justify-center text-on-surface-variant hover:text-on-surface cursor-pointer shadow-sm transition-colors">
            <RefreshCw size={15} />
          </button>
          {canMutate && (
            <button onClick={() => setShowCreate(true)}
              className="flex items-center gap-1.5 px-5 py-2 bg-primary text-on-primary text-sm font-semibold rounded-full cursor-pointer shadow-sm hover:bg-primary/90 transition-all">
              <Plus size={15} /> New restore
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
        <InlineError message={error instanceof Error ? error.message : "Could not load restores."} onRetry={refresh} />
      )}
      {!isLoading && !isError && restores.length === 0 && (
        <EmptyState title="No restores" hint="Restores appear here once you replay a finished backup." />
      )}
      {restores.map((r) => (
        <div key={r.id} className="bg-surface-container-lowest border border-outline-variant/20 rounded-2xl p-4 shadow-sm flex items-start justify-between gap-3">
          <div className="flex items-center gap-2.5 min-w-0">
            <span className={`w-2.5 h-2.5 rounded-full shrink-0 ${statusDot(r.status)}`} />
            <div className="min-w-0">
              <p className="font-mono text-sm font-semibold text-on-surface">{r.id.slice(0, 8)}</p>
              <p className="text-xs text-on-surface-variant font-mono mt-0.5 truncate">
                {r.status}
                {r.destination_path ? ` → ${r.destination_path}` : ""}
                {r.target_database ? ` · db ${r.target_database}` : ""}
                {r.duration_seconds > 0 ? ` · ${r.duration_seconds}s` : ""}
              </p>
              {r.error_message && <p className="text-xs text-error mt-0.5 font-medium">{r.error_message}</p>}
            </div>
          </div>
          <span className="text-xs text-on-surface-variant font-mono whitespace-nowrap">{timeStr(r.completed_at ?? r.created_at)}</span>
        </div>
      ))}

      {showCreate && (
        <RestoreModal
          onClose={() => setShowCreate(false)}
          onSubmit={(details) => {
            setShowCreate(false);
            setPending(details);
          }}
        />
      )}
      {pending && (
        <ConfirmModal
          title="Start this restore?"
          body={`The agent will write the backup to ${pending.dest || pending.db || "the chosen destination"}, replacing whatever is there. This cannot be undone.`}
          confirmLabel="Start restore"
          busy={busy}
          onClose={() => setPending(null)}
          onConfirm={() => void confirmStart()}
        />
      )}
    </div>
  );
}
