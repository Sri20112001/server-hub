import { useQuery } from "@tanstack/react-query";
import { ArrowRight, DatabaseBackup } from "lucide-react";
import { dbBackupsApi } from "../../lib/api";
import { InlineError } from "./shared";
import { statusDot, timeStr } from "./helpers";
import type { DbBackupsTab } from "./DatabaseBackupsPage";

function fmtBytes(n: number): string {
  if (!n || n <= 0) return "0 B";
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}

export function OverviewTab({ go }: { go: (t: DbBackupsTab) => void }) {
  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: ["dbbackups-dashboard"],
    queryFn: dbBackupsApi.dashboard,
    refetchInterval: 30_000,
  });

  if (isLoading) {
    return (
      <div className="space-y-3">
        {[1, 2, 3].map((i) => (
          <div key={i} className="h-24 bg-white dark:bg-panel border border-line dark:border-edge rounded-card animate-pulse shadow-sm" />
        ))}
      </div>
    );
  }

  if (isError || !data) {
    return (
      <InlineError
        message={error instanceof Error ? error.message : "Could not load backup overview."}
        onRetry={() => void refetch()}
      />
    );
  }

  const failed = (data.recent_runs ?? []).filter((r) => r.status === "FAILED");
  const rpoBreaches = data.rpo_breaches ?? [];
  const slaBreaches = data.sla_breaches ?? [];
  const attentionCount = failed.length + rpoBreaches.length + slaBreaches.length;

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4">
        <div className="bg-surface-container-lowest rounded-2xl p-6 shadow-sm">
          <span className="text-[11px] font-semibold uppercase text-on-surface-variant tracking-wider">Backup jobs</span>
          <div className="text-[28px] font-bold text-on-surface tracking-tight mt-2">{data.total_jobs}</div>
          <div className="text-on-surface-variant text-[13px] mt-1">{data.enabled_jobs} scheduled</div>
        </div>
        <div className="bg-surface-container-lowest rounded-2xl p-6 shadow-sm">
          <span className="text-[11px] font-semibold uppercase text-on-surface-variant tracking-wider">Agents</span>
          <div className="flex items-center gap-4 mt-2">
            <span className="inline-flex items-center gap-1.5 font-mono text-[13px] text-secondary font-medium">
              <span className="w-1.5 h-1.5 rounded-full bg-secondary" />{data.agents_online} online
            </span>
            <span className="inline-flex items-center gap-1.5 font-mono text-[13px] text-on-surface-variant font-medium">
              <span className="w-1.5 h-1.5 rounded-full bg-outline" />{data.agents_offline} offline
            </span>
          </div>
          <button onClick={() => go("agents")} className="text-primary text-[13px] font-medium mt-3 cursor-pointer inline-flex items-center gap-1 border-0 bg-transparent">
            Manage agents <ArrowRight size={13} />
          </button>
        </div>
        <div className="bg-surface-container-lowest rounded-2xl p-6 shadow-sm">
          <span className="text-[11px] font-semibold uppercase text-on-surface-variant tracking-wider">Runs · last 24h</span>
          <div className="flex items-center gap-4 mt-2">
            <span className="font-mono text-[13px] text-secondary font-medium">{data.successful_runs} ok</span>
            <span className={`font-mono text-[13px] font-medium ${data.failed_runs > 0 ? "text-error" : "text-on-surface-variant"}`}>
              {data.failed_runs} failed
            </span>
          </div>
          <button onClick={() => go("runs")} className="text-primary text-[13px] font-medium mt-3 cursor-pointer inline-flex items-center gap-1 border-0 bg-transparent">
            View run history <ArrowRight size={13} />
          </button>
        </div>
        <div className="bg-surface-container-lowest rounded-2xl p-6 shadow-sm">
          <span className="text-[11px] font-semibold uppercase text-on-surface-variant tracking-wider">Protected data</span>
          <div className="text-[28px] font-bold text-on-surface tracking-tight mt-2">{fmtBytes(data.total_bytes)}</div>
          <div className="text-on-surface-variant text-[13px] mt-1">uploaded by completed runs</div>
        </div>
      </div>

      {attentionCount > 0 && (
        <div className="rounded-2xl border border-error/30 bg-error-container/20 p-5">
          <div className="flex items-center gap-2 mb-2">
            <DatabaseBackup size={15} className="text-brick" />
            <h2 className="text-sm font-semibold text-ink dark:text-bone">Needs attention — {attentionCount} item{attentionCount === 1 ? "" : "s"}</h2>
          </div>
          {rpoBreaches.map((b) => (
            <button
              key={`rpo-${b.job_id}`}
              onClick={() => go("jobs")}
              className="w-full text-left py-2 border-b border-brick/20 last:border-0 cursor-pointer group"
            >
              <span className="text-sm font-medium text-ink dark:text-bone group-hover:underline">
                {b.job_name}: no successful backup within RPO
              </span>
              <span className="block text-xs text-muted dark:text-fog font-mono mt-0.5">
                RPO {b.rpo_target_minutes} min{b.minutes_since_success != null ? ` · last success ${b.minutes_since_success} min ago` : " · never succeeded"}
              </span>
            </button>
          ))}
          {slaBreaches.map((b) => (
            <button
              key={`sla-${b.job_id}`}
              onClick={() => go("jobs")}
              className="w-full text-left py-2 border-b border-brick/20 last:border-0 cursor-pointer group"
            >
              <span className="text-sm font-medium text-ink dark:text-bone group-hover:underline">
                {b.job_name}: last backup exceeded SLA
              </span>
              <span className="block text-xs text-muted dark:text-fog font-mono mt-0.5">
                took {b.duration_seconds}s · SLA {b.sla_target_minutes} min
              </span>
            </button>
          ))}
          {failed.slice(0, 5).map((r) => (
            <button
              key={r.id}
              onClick={() => go("runs")}
              className="w-full text-left py-2 border-b border-brick/20 last:border-0 cursor-pointer group"
            >
              <span className="text-sm font-medium text-ink dark:text-bone group-hover:underline">
                {r.error_message || "Backup failed"}
              </span>
              <span className="block text-xs text-muted dark:text-fog font-mono mt-0.5">
                {timeStr(r.completed_at ?? r.created_at)}{r.failure_category ? ` Â· ${r.failure_category}` : ""}
              </span>
            </button>
          ))}
        </div>
      )}

      <div className="rounded-card border border-line dark:border-edge p-5 shadow-sm bg-white dark:bg-panel">
        <h2 className="text-sm font-semibold text-ink dark:text-bone mb-2">Recent runs</h2>
        {(data.recent_runs ?? []).length === 0 ? (
          <p className="text-sm text-muted dark:text-fog">No runs yet. Create a job to schedule your first backup.</p>
        ) : (
          <div className="divide-y divide-line dark:divide-edge">
            {(data.recent_runs ?? []).slice(0, 8).map((r) => (
              <div key={r.id} className="py-2 flex items-center justify-between gap-3">
                <span className="flex items-center gap-2 min-w-0">
                  <span className={`w-2 h-2 rounded-full shrink-0 ${statusDot(r.status)}`} />
                  <span className="font-mono text-xs text-ink dark:text-bone truncate">{r.id.slice(0, 8)}</span>
                </span>
                <span className="text-xs text-muted dark:text-fog font-mono whitespace-nowrap">
                  {r.status} Â· {timeStr(r.completed_at ?? r.created_at)}
                </span>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
