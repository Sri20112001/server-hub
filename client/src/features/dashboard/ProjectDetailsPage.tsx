import { useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router";
import {
  Archive,
  ArrowLeft,
  Copy,
  Eye,
  EyeOff,
  Hash,
  History,
  Play,
  Plus,
  Power,
  RotateCcw,
  Rocket,
  ScrollText,
  Terminal,
  Trash2,
} from "lucide-react";
import { api, ApiError } from "../../lib/api";
import type { Project } from "../../lib/types";
import { fmtDuration, shortSha, timeAgo } from "../../lib/format";
import { useUi } from "../../stores/store";
import { ConfirmModal, DashboardSkeleton, EmptyState, Field, Kicker, StatusPill } from "../../components/ui";
import { LogsViewer } from "./LogsViewer";
import { TerminalModal } from "./TerminalModal";
import { DeployProgress } from "./DeployProgress";
import { useProjectDetails } from "./useProjectDetails";

// Full ship manifest: helm, rigging, stations, logs, launches, vault and
// snapshots. Opened from the summary drawer's "View details".
export function ProjectDetailsPage({ setOnline }: { setOnline: (v: boolean) => void }) {
  const { id } = useParams();
  const nav = useNavigate();
  const pushToast = useUi((s) => s.pushToast);
  const [project, setProject] = useState<Project | null>(null);
  const [missing, setMissing] = useState(false);

  const reload = async () => {
    try {
      const p = await api.project(Number(id));
      setProject(p);
      setOnline(true);
    } catch (e) {
      if (e instanceof ApiError && e.status === 404) setMissing(true);
      else pushToast(e instanceof Error ? e.message : "Could not load ship", true);
    }
  };

  useEffect(() => {
    // Intentional: refetch when the route id changes.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    void reload();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id]);

  const goBack = () => {
    if (window.history.length > 1) nav(-1);
    else nav("/");
  };

  if (missing) {
    return (
      <main className="w-full max-w-7xl mx-auto px-10 max-md:px-4 pt-24 pb-36">
        <EmptyState title="Ship not found" hint="It may have been deleted." />
        <div className="flex justify-center mt-4">
          <button
            className="inline-flex items-center gap-2 rounded-input text-[13px] font-medium px-4 py-2 cursor-pointer border border-transparent whitespace-nowrap bg-white dark:bg-panel border-line dark:border-edge text-ink dark:text-bone hover:bg-paper dark:hover:bg-emboss"
            onClick={() => nav("/")}
          >
            <ArrowLeft size={14} /> Dashboard
          </button>
        </div>
      </main>
    );
  }

  if (!project) {
    return <DashboardSkeleton />;
  }

  return <DetailsBody key={project.id} project={project} onBack={goBack} onChanged={() => void reload()} />;
}

function DetailsBody({
  project,
  onBack,
  onChanged,
}: {
  project: Project;
  onBack: () => void;
  onChanged: () => void;
}) {
  const pushToast = useUi((s) => s.pushToast);
  const d = useProjectDetails(project, onChanged);

  const facts: Array<[string, string]> = [
    ["Repository", project.repository || "—"],
    ["Branch", project.branch || "—"],
    ["Environment", project.environment || "—"],
    ["Compose file", project.composeFile || "—"],
    ["Deployment path", project.deploymentPath || "—"],
    ["Gateway prefix", project.gatewayPrefix || "—"],
  ];

  return (
    <main className="w-full max-w-5xl mx-auto px-10 max-md:px-4 pt-24 pb-36">
      <button
        className="inline-flex items-center gap-1.5 bg-transparent border-0 cursor-pointer text-muted dark:text-fog hover:text-ink dark:hover:text-bone text-[13px] mb-2 px-0"
        onClick={onBack}
      >
        <ArrowLeft size={14} /> Back
      </button>
      <div className="flex items-center justify-between gap-4 flex-wrap">
        <div>
          <Kicker>Ship manifest</Kicker>
          <h1 className="font-head text-[32px] font-bold tracking-[-0.03em] leading-[1.2] max-md:text-[26px] mt-1">
            {project.name}
          </h1>
        </div>
        <StatusPill status={project.status} />
      </div>

      {/* helm */}
      <div className="bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-5 mt-5">
        <Kicker>Helm</Kicker>
        <div className="flex items-center gap-2 flex-wrap mt-2.5">
          <button
            className="inline-flex items-center gap-2 rounded-input text-[13px] font-medium px-4 py-2 cursor-pointer border border-transparent whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-accent dark:bg-ember text-white dark:text-black hover:bg-accent-hover dark:hover:bg-ember-hover px-3 py-1.5 text-xs"
            disabled={d.busy}
            onClick={() => void d.shipIt()}
          >
            <Rocket size={13} /> Ship it
          </button>
          <button className="inline-flex items-center gap-2 rounded-input text-[13px] font-medium px-4 py-2 cursor-pointer border border-transparent whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-white dark:bg-panel border-line dark:border-edge text-ink dark:text-bone hover:bg-paper dark:hover:bg-emboss px-3 py-1.5 text-xs" disabled={d.busy} onClick={() => void d.runLifecycle("start")}>
            <Play size={13} /> Wake up
          </button>
          <button className="inline-flex items-center gap-2 rounded-input text-[13px] font-medium px-4 py-2 cursor-pointer border border-transparent whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-white dark:bg-panel border-line dark:border-edge text-ink dark:text-bone hover:bg-paper dark:hover:bg-emboss px-3 py-1.5 text-xs" disabled={d.busy} onClick={() => d.setPending("stop")}>
            <Power size={13} /> Nap
          </button>
          <button className="inline-flex items-center gap-2 rounded-input text-[13px] font-medium px-4 py-2 cursor-pointer border border-transparent whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-white dark:bg-panel border-line dark:border-edge text-ink dark:text-bone hover:bg-paper dark:hover:bg-emboss px-3 py-1.5 text-xs" disabled={d.busy} onClick={() => d.setPending("restart")}>
            <RotateCcw size={13} /> Fresh start
          </button>
          <button className="inline-flex items-center gap-2 rounded-input text-[13px] font-medium px-4 py-2 cursor-pointer border border-transparent whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-white dark:bg-panel border-line dark:border-edge text-ink dark:text-bone hover:bg-paper dark:hover:bg-emboss px-3 py-1.5 text-xs" disabled={d.busy || d.services.filter((s) => s.containerName).length === 0} onClick={() => d.setShowTerminal(true)} title="Open an interactive shell in a station container">
            <Terminal size={13} /> Terminal
          </button>
        </div>
      </div>

      {/* facts */}
      <div className="bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-5 mt-4">
        <Kicker>Rigging</Kicker>
        <div className="mt-2.5 flex flex-col gap-4">
          {facts.map(([k, v]) => (
            <div key={k} className="flex items-center justify-between gap-4 text-[13px]">
              <span className="text-muted dark:text-fog">{k}</span>
              <span className="font-mono text-right max-w-[60%] overflow-hidden text-ellipsis" title={v}>
                {v}
              </span>
            </div>
          ))}
          <div className="flex items-center justify-between gap-4 text-[13px]">
            <span className="text-muted dark:text-fog">Auto-deploy</span>
            <span className="inline-flex items-center gap-1.5 h-6 px-2.5 rounded-full font-mono text-[11px] bg-paper dark:bg-abyss text-ink dark:text-bone whitespace-nowrap border-0">
              {project.autoDeploy ? "ON — webhook steers" : "OFF — manual helm"}
            </span>
          </div>
        </div>
      </div>

      {/* stations */}
      <div className="bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-5 mt-4">
        <Kicker>Stations · {d.services.length}</Kicker>
        <div className="mt-2.5">
          {d.services.length === 0 ? (
            <EmptyState title="No stations charted" hint="Add services via the API to track them here." />
          ) : (
            <table className="w-full border-collapse text-[13px]">
              <thead>
                <tr>
                  <th className="text-left text-[11px] font-semibold tracking-[0.08em] uppercase text-muted dark:text-fog px-4 py-2.5 bg-paper dark:bg-abyss first:rounded-l-[10px] last:rounded-r-[10px] last:text-right">Station</th>
                  <th className="text-left text-[11px] font-semibold tracking-[0.08em] uppercase text-muted dark:text-fog px-4 py-2.5 bg-paper dark:bg-abyss first:rounded-l-[10px] last:rounded-r-[10px] last:text-right">Type</th>
                  <th className="text-left text-[11px] font-semibold tracking-[0.08em] uppercase text-muted dark:text-fog px-4 py-2.5 bg-paper dark:bg-abyss first:rounded-l-[10px] last:rounded-r-[10px] last:text-right">Status</th>
                </tr>
              </thead>
              <tbody>
                {d.services.map((s) => (
                  <tr key={s.id} className="group">
                    <td className="px-4 py-3 border-t border-line dark:border-edge align-middle last:text-right group-hover:bg-paper dark:group-hover:bg-emboss">
                      <div className="font-mono">{s.name}</div>
                      {s.containerName && <div className="font-mono text-muted dark:text-fog text-xs">{s.containerName}</div>}
                    </td>
                    <td className="px-4 py-3 border-t border-line dark:border-edge align-middle last:text-right group-hover:bg-paper dark:group-hover:bg-emboss">
                      <span className="inline-flex items-center gap-1.5 font-mono text-[11px] bg-tint dark:bg-emboss border border-line dark:border-edge text-ink dark:text-bone rounded-md px-2 py-[3px] whitespace-nowrap">
                        {s.type}
                      </span>
                    </td>
                    <td className="px-4 py-3 border-t border-line dark:border-edge align-middle last:text-right group-hover:bg-paper dark:group-hover:bg-emboss">
                      <StatusPill status={s.status} />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>

      {/* live logs */}
      <div className="bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-5 mt-4">
        <div className="flex items-center gap-2 flex-wrap mb-2.5">
          <ScrollText size={15} />
          <Kicker>Live logs</Kicker>
        </div>
        {d.services.filter((s) => s.containerName).length === 0 ? (
          <EmptyState
            title="No loggable stations"
            hint="Stations need a container name before their logs can stream."
          />
        ) : (
          <>
            <div className="flex items-center gap-2 flex-wrap mb-2.5">
              {d.services
                .filter((s) => s.containerName)
                .map((s) => {
                  const active =
                    (d.logService ?? d.services.find((x) => x.containerName)?.containerName) ===
                    s.containerName;
                  return (
                    <button
                      key={s.id}
                      onClick={() => d.setLogService(s.containerName ?? null)}
                      aria-pressed={active}
                      className={`font-mono text-[11px] px-2.5 py-1.5 rounded-md border cursor-pointer transition-colors duration-150 ${
                        active
                          ? "bg-tint dark:bg-emboss border-line dark:border-edge text-ink dark:text-bone"
                          : "bg-transparent border-transparent text-muted dark:text-fog hover:text-ink dark:hover:text-bone"
                      }`}
                    >
                      {s.name}
                    </button>
                  );
                })}
            </div>
            <LogsViewer
              container={
                d.logService ??
                d.services.find((x) => x.containerName)?.containerName ??
                ""
              }
            />
          </>
        )}
      </div>

      {/* launches */}
      <div className="bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-5 mt-4">
        <Kicker>Launch log · {d.deployments.length}</Kicker>
        <div className="mt-2.5 flex flex-col gap-4">
          {d.deployments.length === 0 && <EmptyState title="No launches yet" />}
          {d.deployments.map((dep) => (
            <div key={dep.id} className="flex items-center justify-between gap-4 text-[13px]">
              <span className="inline-flex items-center gap-1.5 font-mono text-[11px] bg-tint dark:bg-emboss border border-line dark:border-edge text-ink dark:text-bone rounded-md px-2 py-[3px] whitespace-nowrap">
                <Hash size={11} /> {shortSha(dep.commitSha)}
              </span>
              <span className="font-mono text-muted dark:text-fog">{fmtDuration(dep.durationSec)}</span>
              <span className="text-muted dark:text-fog">{timeAgo(dep.startedAt)}</span>
              <StatusPill status={dep.status} />
              <button
                className="inline-flex items-center gap-2 rounded-input text-[13px] font-medium px-4 py-2 cursor-pointer border border-transparent whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-white dark:bg-panel border-line dark:border-edge text-ink dark:text-bone hover:bg-paper dark:hover:bg-emboss px-3 py-1.5 text-xs"
                title={`Roll back to ${shortSha(dep.commitSha)}`}
                disabled={d.busy}
                onClick={() => d.setPendingRollback(dep)}
              >
                <History size={12} />
              </button>
            </div>
          ))}
        </div>
      </div>

      {/* vault */}
      <div className="bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-5 mt-4">
        <Kicker>Vault · {d.secrets.length} sealed</Kicker>
        <div className="mt-2.5 flex flex-col gap-4">
          {d.secrets.map((s) => (
            <div key={s.id} className="flex items-center justify-between gap-4 text-[13px]">
              <span className="font-mono">{s.name}</span>
              <span className="flex items-center gap-2 flex-wrap">
                <span className="inline-flex items-center gap-1.5 font-mono text-[11px] bg-tint dark:bg-emboss border border-line dark:border-edge text-ink dark:text-bone rounded-md px-2 py-[3px] whitespace-nowrap">
                  {s.environment}
                </span>
                {d.revealed[s.id] ? (
                  <span className="flex items-center gap-2 flex-wrap">
                    <code className="font-mono text-[11px] bg-paper dark:bg-abyss px-1.5 py-[2px] rounded-md">
                      {d.revealed[s.id].slice(0, 24)}{d.revealed[s.id].length > 24 ? "…" : ""}
                    </code>
                    <button
                      className="inline-flex items-center gap-2 rounded-input text-[13px] font-medium px-4 py-2 cursor-pointer border border-transparent whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-white dark:bg-panel border-line dark:border-edge text-ink dark:text-bone hover:bg-paper dark:hover:bg-emboss px-3 py-1.5 text-xs"
                      title="Copy"
                      onClick={() => {
                        void navigator.clipboard.writeText(d.revealed[s.id]);
                        pushToast("Copied to clipboard.");
                      }}
                    >
                      <Copy size={12} />
                    </button>
                    <button className="inline-flex items-center gap-2 rounded-input text-[13px] font-medium px-4 py-2 cursor-pointer border border-transparent whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-white dark:bg-panel border-line dark:border-edge text-ink dark:text-bone hover:bg-paper dark:hover:bg-emboss px-3 py-1.5 text-xs" onClick={() => void d.peek(s.id)} aria-label="Hide">
                      <EyeOff size={12} />
                    </button>
                  </span>
                ) : (
                  <button className="inline-flex items-center gap-2 rounded-input text-[13px] font-medium px-4 py-2 cursor-pointer border border-transparent whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-white dark:bg-panel border-line dark:border-edge text-ink dark:text-bone hover:bg-paper dark:hover:bg-emboss px-3 py-1.5 text-xs" onClick={() => void d.peek(s.id)}>
                    <Eye size={12} /> Peek
                  </button>
                )}
              </span>
            </div>
          ))}
          {d.secrets.length === 0 && <EmptyState title="Vault is empty" hint="Seal your first secret below." />}
          <form onSubmit={(e) => void d.addSecret(e)}>
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <Field label="Name">
                <input
                  className="w-full bg-white dark:bg-panel border border-line dark:border-edge rounded-input font-body text-[13px] text-ink dark:text-bone px-3 py-2.5 outline-none focus:border-accent dark:focus:border-ember placeholder:text-stone dark:placeholder:text-fog font-mono"
                  value={d.newName}
                  onChange={(e) => d.setNewName(e.target.value)}
                  placeholder="JWT_SECRET"
                />
              </Field>
              <Field label="Value">
                <input
                  className="w-full bg-white dark:bg-panel border border-line dark:border-edge rounded-input font-body text-[13px] text-ink dark:text-bone px-3 py-2.5 outline-none focus:border-accent dark:focus:border-ember placeholder:text-stone dark:placeholder:text-fog font-mono"
                  type="password"
                  value={d.newValue}
                  onChange={(e) => d.setNewValue(e.target.value)}
                  placeholder="••••••"
                />
              </Field>
            </div>
            <button
              type="submit"
              className="inline-flex items-center gap-2 rounded-input text-[13px] font-medium px-4 py-2 cursor-pointer border border-transparent whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-white dark:bg-panel border-line dark:border-edge text-ink dark:text-bone hover:bg-paper dark:hover:bg-emboss px-3 py-1.5 text-xs mt-4"
              disabled={!d.newName.trim() || !d.newValue}
            >
              <Plus size={12} /> Seal secret
            </button>
          </form>
        </div>
      </div>

      {/* snapshots */}
      <div className="bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-5 mt-4">
        <div className="flex items-center justify-between gap-4 mb-2.5">
          <div className="flex items-center gap-2 flex-wrap">
            <Archive size={15} />
            <Kicker>Snapshots · {d.backups.length}</Kicker>
          </div>
          <button
            className="inline-flex items-center gap-2 rounded-input text-[13px] font-medium px-4 py-2 cursor-pointer border border-transparent whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-white dark:bg-panel border-line dark:border-edge text-ink dark:text-bone hover:bg-paper dark:hover:bg-emboss px-3 py-1.5 text-xs"
            disabled={d.busy}
            onClick={() => void d.createBackup()}
          >
            <Plus size={12} /> Snapshot now
          </button>
        </div>
        {d.backups.length === 0 ? (
          <EmptyState
            title="No snapshots yet"
            hint="Snapshots tar the deployment directory plus a metadata manifest."
          />
        ) : (
          <div className="flex flex-col gap-2.5">
            {d.backups.map((b) => (
              <div key={b.id} className="flex items-center justify-between gap-4 text-[13px]">
                <div>
                  <div className="font-mono">
                    {new Date(b.createdAt.replace(" ", "T") + "Z").toLocaleString("en-GB", {
                      day: "2-digit",
                      month: "short",
                      hour: "2-digit",
                      minute: "2-digit",
                    })}
                  </div>
                  <div className="font-mono text-muted dark:text-fog text-[11px]">
                    {(b.sizeBytes / 1024 / 1024).toFixed(1)} MB · {b.status}
                  </div>
                </div>
                <span className="flex items-center gap-2 flex-wrap">
                  <button
                    className="inline-flex items-center gap-2 rounded-input text-[13px] font-medium px-4 py-2 cursor-pointer border border-transparent whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-white dark:bg-panel border-line dark:border-edge text-ink dark:text-bone hover:bg-paper dark:hover:bg-emboss px-3 py-1.5 text-xs"
                    disabled={d.busy}
                    onClick={() => d.setPendingRestore(b)}
                  >
                    Restore
                  </button>
                  <button
                    className="inline-flex items-center gap-2 rounded-input text-[13px] font-medium px-4 py-2 cursor-pointer border border-transparent whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-white dark:bg-panel border-line dark:border-edge text-ink dark:text-bone hover:bg-paper dark:hover:bg-emboss px-3 py-1.5 text-xs"
                    disabled={d.busy}
                    onClick={() => void d.deleteBackup(b.id)}
                    aria-label={`Delete snapshot ${b.id}`}
                  >
                    <Trash2 size={12} />
                  </button>
                </span>
              </div>
            ))}
          </div>
        )}
      </div>

      {d.showTerminal && (
        <TerminalModal
          projectName={project.name}
          services={d.services}
          onClose={() => d.setShowTerminal(false)}
        />
      )}

      {d.progress && (
        <DeployProgress
          projectName={project.name}
          operationId={d.progress.id}
          onClose={() => d.setProgress(null)}
          onDone={() => {
            onChanged();
            void d.refreshLists();
          }}
        />
      )}

      {d.pending && (
        <ConfirmModal
          title={d.pending === "stop" ? `Nap “${project.name}”?` : `Fresh-start “${project.name}”?`}
          body={
            d.pending === "stop"
              ? "This stops every container in the ship. Traffic goes dark until you wake it up."
              : "This restarts every container in the ship. Expect a brief blackout."
          }
          confirmLabel={d.pending === "stop" ? "Nap it" : "Fresh-start it"}
          busy={d.busy}
          onClose={() => d.setPending(null)}
          onConfirm={() => void d.runLifecycle(d.pending ?? "stop")}
        />
      )}
      {d.pendingRollback && (
        <ConfirmModal
          title={`Roll back to ${shortSha(d.pendingRollback.commitSha)}?`}
          body="This re-runs docker compose up for that launch's commit and records a new rollback deployment. Current containers will be replaced."
          confirmLabel="Roll back"
          busy={d.busy}
          onClose={() => d.setPendingRollback(null)}
          onConfirm={() => {
            const dep = d.pendingRollback;
            if (dep) void d.rollbackTo(dep);
          }}
        />
      )}
      {d.pendingRestore && (
        <ConfirmModal
          title="Restore this snapshot?"
          body="Live files in the deployment directory will be overwritten with the snapshot contents. Current state is not saved unless you snapshot first."
          confirmLabel="Restore snapshot"
          requireText="RESTORE"
          busy={d.busy}
          onClose={() => d.setPendingRestore(null)}
          onConfirm={() => {
            const b = d.pendingRestore;
            if (b) void d.restoreBackup(b);
          }}
        />
      )}
    </main>
  );
}
