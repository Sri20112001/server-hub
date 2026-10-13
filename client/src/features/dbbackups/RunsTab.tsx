import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ChevronDown, ChevronUp, RefreshCw } from "lucide-react";
import { dbBackupsApi } from "../../lib/api";
import type { VgBackupArtifact, VgBackupRun } from "../../lib/types";
import { ConfirmModal, EmptyState } from "../../components/ui";
import { useUi } from "../../stores/store";
import { InlineError } from "./shared";
import { statusDot, timeStr, useDbBackupsRole } from "./helpers";

const STATUSES = ["", "PENDING", "RUNNING", "UPLOADING", "VERIFYING", "COMPLETED", "FAILED", "CANCELLED"];
const PAGE_SIZE = 20;

function fmtBytes(n: number): string {
  if (!n || n <= 0) return "0 B";
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}

function Artifacts({ runId }: { runId: string }) {
  const { data: arts = [], isLoading } = useQuery({
    queryKey: ["vg-artifacts", runId],
    queryFn: () => dbBackupsApi.runArtifacts(runId),
  });
  if (isLoading) return <p className="text-xs text-muted dark:text-fog">Loading filesâ€¦</p>;
  if (arts.length === 0) return <p className="text-xs text-muted dark:text-fog">No files recorded for this run.</p>;
  const badge = (a: VgBackupArtifact) =>
    a.verification_status === "VERIFIED" ? "text-moss border-moss/30 bg-moss/10"
    : a.verification_status === "FAILED" ? "text-brick border-brick/30 bg-brick/10"
    : "text-muted dark:text-fog border-line dark:border-edge";
  return (
    <div className="space-y-1.5">
      {arts.map((a) => (
        <div key={a.id} className="flex items-center justify-between gap-2 flex-wrap">
          <span className="font-mono text-xs text-ink dark:text-bone break-all">{a.name}</span>
          <span className="flex items-center gap-2">
            <span className="font-mono text-[11px] text-muted dark:text-fog">{fmtBytes(a.size)}</span>
            <span className={`font-mono text-[10px] px-1.5 py-0.5 rounded border ${badge(a)}`}>
              {a.verification_status === "PENDING" ? "not checked" : a.verification_status.toLowerCase()}
            </span>
          </span>
        </div>
      ))}
    </div>
  );
}

function RunCard({ run, onChanged }: { run: VgBackupRun; onChanged: () => void }) {
  const { pushToast } = useUi();
  const { canMutate } = useDbBackupsRole();
  const [expanded, setExpanded] = useState(false);
  const [busy, setBusy] = useState(false);
  const [confirmCancel, setConfirmCancel] = useState(false);
  const active = ["PENDING", "RUNNING", "UPLOADING", "VERIFYING"].includes(run.status);

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

  return (
    <div className="bg-surface-container-lowest border border-outline-variant/20 rounded-2xl p-4 shadow-sm">
      <div className="flex items-start justify-between gap-3">
        <div className="flex items-center gap-2.5 min-w-0">
          <span className={`w-2.5 h-2.5 rounded-full shrink-0 ${statusDot(run.status)}`} />
          <div className="min-w-0">
            <p className="font-mono text-sm font-semibold text-on-surface">{run.id.slice(0, 8)}</p>
            <p className="text-xs text-on-surface-variant font-mono mt-0.5">
              {run.status}
              {run.bytes_uploaded > 0 && ` · ${fmtBytes(run.bytes_uploaded)} uploaded`}
              {run.duration_seconds > 0 && ` · ${run.duration_seconds}s`}
            </p>
          </div>
        </div>
        <div className="flex items-center gap-1.5 shrink-0">
          {canMutate && active && (
            <button onClick={() => setConfirmCancel(true)}
              className="text-xs px-3 py-1 rounded-full border border-outline-variant/30 text-on-surface-variant hover:text-error bg-surface-container-low cursor-pointer transition-colors">
              Cancel
            </button>
          )}
          {canMutate && run.status === "COMPLETED" && (
            <button
              onClick={() => void act("Verification finished", () => dbBackupsApi.verifyRun(run.id))}
              disabled={busy}
              className="text-xs px-3 py-1 rounded-full border border-outline-variant/30 text-on-surface-variant hover:text-on-surface bg-surface-container-low hover:bg-surface-container cursor-pointer disabled:opacity-50 transition-colors">
              Verify
            </button>
          )}
          <button onClick={() => setExpanded(!expanded)} className="text-on-surface-variant hover:text-on-surface p-1.5 rounded-full cursor-pointer transition-colors" aria-label={expanded ? "Collapse" : "Expand"}>
            {expanded ? <ChevronUp size={15} /> : <ChevronDown size={15} />}
          </button>
        </div>
      </div>

      {expanded && (
        <div className="mt-3 border-t border-outline-variant/20 pt-3 space-y-2">
          <p className="text-xs text-on-surface-variant font-mono">
            Started {timeStr(run.started_at)} · Finished {timeStr(run.completed_at)}
            {run.failure_category ? ` · ${run.failure_category}` : ""}
          </p>
          {run.error_message && <p className="text-xs text-error font-medium">{run.error_message}</p>}
          {run.checksum && <p className="text-xs text-on-surface-variant font-mono break-all">Checksum: {run.checksum}</p>}
          <Artifacts runId={run.id} />
        </div>
      )}

      {confirmCancel && (
        <ConfirmModal
          title="Cancel this run?"
          body="The agent will stop at the next safe point. Partial uploads stay where they are."
          confirmLabel="Cancel run"
          busy={busy}
          onClose={() => setConfirmCancel(false)}
          onConfirm={() => void act("Run cancelled", async () => {
            await dbBackupsApi.cancelRun(run.id);
            setConfirmCancel(false);
          })}
        />
      )}
    </div>
  );
}

export function RunsTab({ jobId }: { jobId?: string }) {
  const qc = useQueryClient();
  const [status, setStatus] = useState("");
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(0);
  const params = {
    job_id: jobId || undefined,
    status: status || undefined,
    search: search.trim() || undefined,
    page: page + 1,
    limit: PAGE_SIZE,
  };
  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: ["vg-runs", jobId, status, search, page],
    queryFn: () => dbBackupsApi.runs(params),
    refetchInterval: 15_000,
  });

  const refresh = () => {
    void qc.invalidateQueries({ queryKey: ["vg-runs"] });
    void refetch();
  };
  const totalPages = data ? Math.max(1, Math.ceil(data.total / data.limit)) : 1;

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2.5 flex-wrap">
        <select
          value={status}
          onChange={(e) => { setStatus(e.target.value); setPage(0); }}
          className="bg-surface-container-lowest border border-outline-variant/30 rounded-xl px-3.5 py-2 text-sm text-on-surface outline-none cursor-pointer shadow-sm transition-colors"
          aria-label="Filter by status"
        >
          {STATUSES.map((s) => <option key={s} value={s}>{s === "" ? "All states" : s}</option>)}
        </select>
        <input
          value={search}
          onChange={(e) => { setSearch(e.target.value); setPage(0); }}
          placeholder="Search by job name…"
          className="flex-1 min-w-40 bg-surface-container-lowest border border-outline-variant/30 rounded-xl px-4 py-2 text-sm text-on-surface outline-none focus:border-primary placeholder:text-on-surface-variant/40 shadow-sm transition-colors"
        />
        <button onClick={refresh} aria-label="Refresh"
          className="w-10 h-10 rounded-full bg-surface-container-lowest border border-outline-variant/30 flex items-center justify-center text-on-surface-variant hover:text-on-surface cursor-pointer shadow-sm transition-colors">
          <RefreshCw size={15} />
        </button>
      </div>

      {isLoading && (
        <div className="space-y-3">
          {[1, 2, 3].map((i) => <div key={i} className="h-20 rounded-2xl bg-surface-container-low border border-outline-variant/20 animate-pulse" />)}
        </div>
      )}
      {!isLoading && isError && (
        <InlineError message={error instanceof Error ? error.message : "Could not load runs."} onRetry={refresh} />
      )}
      {!isLoading && !isError && (data?.data ?? []).length === 0 && (
        <EmptyState title="No runs found" hint="Runs appear here once a job executes." />
      )}
      {(data?.data ?? []).map((r) => <RunCard key={r.id} run={r} onChanged={refresh} />)}

      {totalPages > 1 && (
        <div className="flex items-center justify-between pt-2">
          <span className="text-xs text-on-surface-variant font-mono">
            Page {page + 1} of {totalPages} · {data?.total ?? 0} runs
          </span>
          <span className="flex gap-2">
            <button disabled={page === 0} onClick={() => setPage(page - 1)}
              className="px-4 py-1.5 text-xs border border-outline-variant/30 rounded-full cursor-pointer disabled:opacity-40 bg-surface-container-low text-on-surface hover:bg-surface-container transition-colors">Prev</button>
            <button disabled={page + 1 >= totalPages} onClick={() => setPage(page + 1)}
              className="px-4 py-1.5 text-xs border border-outline-variant/30 rounded-full cursor-pointer disabled:opacity-40 bg-surface-container-low text-on-surface hover:bg-surface-container transition-colors">Next</button>
          </span>
        </div>
      )}
    </div>
  );
}
