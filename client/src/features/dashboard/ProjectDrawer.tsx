import { useEffect, useState } from "react";
import {
  Archive,
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
  X,
} from "lucide-react";
import { api } from "../../lib/api";
import type { Backup, Deployment, Project, SecretMeta, Service } from "../../lib/types";
import { fmtDuration, shortSha, timeAgo } from "../../lib/format";
import { useUi } from "../../stores/store";
import { ConfirmModal, EmptyState, Field, Kicker, StatusPill } from "../../components/ui";
import { LogsViewer } from "./LogsViewer";
import { TerminalModal } from "./TerminalModal";
import { DeployProgress } from "./DeployProgress";

type PendingAction = "stop" | "restart" | null;

export function ProjectDrawer({
  project,
  onClose,
  onChanged,
}: {
  project: Project;
  onClose: () => void;
  onChanged: () => void;
}) {
  const pushToast = useUi((s) => s.pushToast);
  const [services, setServices] = useState<Service[]>([]);
  const [deployments, setDeployments] = useState<Deployment[]>([]);
  const [secrets, setSecrets] = useState<SecretMeta[]>([]);
  const [revealed, setRevealed] = useState<Record<number, string>>({});
  const [pending, setPending] = useState<PendingAction>(null);
  const [pendingRollback, setPendingRollback] = useState<Deployment | null>(null);
  const [pendingRestore, setPendingRestore] = useState<Backup | null>(null);
  const [logService, setLogService] = useState<string | null>(null);
  const [showTerminal, setShowTerminal] = useState(false);
  const [progress, setProgress] = useState<{ id: string; title: string } | null>(null);
  const [backups, setBackups] = useState<Backup[]>([]);
  const [busy, setBusy] = useState(false);
  const [newName, setNewName] = useState("");
  const [newValue, setNewValue] = useState("");

  useEffect(() => {
    let alive = true;
    (async () => {
      try {
        const [s, d, sec, b] = await Promise.all([
          api.services(project.id),
          api.deployments(project.id),
          api.secrets(project.id),
          api.backups(project.id).catch(() => [] as Backup[]),
        ]);
        if (alive) {
          setServices(s);
          setDeployments(d);
          setSecrets(sec);
          setBackups(b);
        }
      } catch {
        if (alive) pushToast("Could not load ship details", true);
      }
    })();
    return () => {
      alive = false;
    };
  }, [project.id, pushToast]);

  useEffect(() => {
    const fn = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", fn);
    return () => window.removeEventListener("keydown", fn);
  }, [onClose]);

  const refreshLists = async () => {
    try {
      const [s, d, sec, b] = await Promise.all([
        api.services(project.id),
        api.deployments(project.id),
        api.secrets(project.id),
        api.backups(project.id).catch(() => [] as Backup[]),
      ]);
      setServices(s);
      setDeployments(d);
      setSecrets(sec);
      setBackups(b);
    } catch {
      /* keep stale */
    }
  };

  const runLifecycle = async (action: "start" | "stop" | "restart") => {
    setBusy(true);
    try {
      await api.projectAction(project.id, action, action !== "start");
      pushToast(
        action === "start" ? `“${project.name}” waking up.` :
        action === "stop" ? `“${project.name}” taking a nap.` :
        `“${project.name}” fresh-started.`,
      );
      setPending(null);
      onChanged();
    } catch (e) {
      pushToast(e instanceof Error ? e.message : `${action} failed`, true);
    } finally {
      setBusy(false);
    }
  };

  const shipIt = async () => {    setBusy(true);
    try {
      const res = await api.deploy(project.id);
      pushToast(`Dispatch launched for “${project.name}”.`);
      setProgress({ id: res.operationId, title: `Dispatching ${project.name}` });
      onChanged();
      void refreshLists();
    } catch (e) {
      pushToast(e instanceof Error ? e.message : "Deploy failed", true);
    } finally {
      setBusy(false);
    }
  };

  const rollbackTo = async (d: Deployment) => {
    setBusy(true);
    try {
      const res = await api.rollback(project.id, d.id);
      pushToast(`Rolling back to ${shortSha(d.commitSha)} — dispatch running.`);
      setPendingRollback(null);
      setProgress({ id: res.operationId, title: `Rolling back ${project.name}` });
      onChanged();
      void refreshLists();
    } catch (e) {
      pushToast(e instanceof Error ? e.message : "Rollback failed", true);
    } finally {
      setBusy(false);
    }
  };

  const createBackup = async () => {
    setBusy(true);
    try {
      const res = await api.createBackup(project.id);
      pushToast(`Snapshot started for “${project.name}”.`);
      setProgress({ id: res.operationId, title: `Snapshotting ${project.name}` });
      void refreshLists();
    } catch (e) {
      pushToast(e instanceof Error ? e.message : "Backup failed", true);
    } finally {
      setBusy(false);
    }
  };

  const deleteBackup = async (id: number) => {
    try {
      await api.deleteBackup(id);
      pushToast("Snapshot deleted.");
      void refreshLists();
    } catch (e) {
      pushToast(e instanceof Error ? e.message : "Delete failed", true);
    }
  };

  const restoreBackup = async (b: Backup) => {
    setBusy(true);
    try {
      const res = await api.restoreBackup(b.id);
      pushToast("Restore running — live files are being replaced.");
      setPendingRestore(null);
      setProgress({ id: res.operationId, title: `Restoring ${project.name}` });
      void refreshLists();
    } catch (e) {
      pushToast(e instanceof Error ? e.message : "Restore failed", true);
    } finally {
      setBusy(false);
    }
  };

  const peek = async (id: number) => {
    if (revealed[id]) {
      setRevealed((r) => {
        const next = { ...r };
        delete next[id];
        return next;
      });
      return;
    }
    try {
      const res = await api.revealSecret(id);
      setRevealed((r) => ({ ...r, [id]: res.value }));
      pushToast("Secret revealed — eyes only.");
    } catch (e) {
      pushToast(e instanceof Error ? e.message : "Reveal failed", true);
    }
  };

  const addSecret = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newName.trim() || !newValue) return;
    try {
      await api.upsertSecret(project.id, newName.trim(), newValue);
      pushToast(`Sealed “${newName.trim()}” into the vault.`);
      setNewName("");
      setNewValue("");
      void refreshLists();
    } catch (err) {
      pushToast(err instanceof Error ? err.message : "Could not seal secret", true);
    }
  };

  const facts: Array<[string, string]> = [
    ["Repository", project.repository || "—"],
    ["Branch", project.branch || "—"],
    ["Environment", project.environment || "—"],
    ["Compose file", project.composeFile || "—"],
    ["Deployment path", project.deploymentPath || "—"],
    ["Gateway prefix", project.gatewayPrefix || "—"],
  ];

  return (
    <>
      <div
        className="fixed inset-0 z-[60] flex justify-end bg-[rgba(28,25,23,0.28)] dark:bg-[rgba(0,0,0,0.55)]"
        onClick={onClose}
      >
        <aside
          className="bg-paper dark:bg-abyss border-l border-line dark:border-edge w-full max-w-[560px] h-full overflow-y-auto px-6 pt-6 pb-12"
          onClick={(e) => e.stopPropagation()}
        >
          <div className="flex items-center justify-between gap-4">
            <div>
              <Kicker>Ship manifest</Kicker>
              <h2 className="text-[24px] mt-1">{project.name}</h2>
            </div>
            <div className="flex items-center gap-2 flex-wrap">
              <StatusPill status={project.status} />
              <button className="inline-flex items-center gap-2 rounded-input text-[13px] font-medium px-4 py-2 cursor-pointer border border-transparent whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-white dark:bg-panel border-line dark:border-edge text-ink dark:text-bone hover:bg-paper dark:hover:bg-emboss px-3 py-1.5 text-xs" onClick={onClose} aria-label="Close">
                <X size={14} />
              </button>
            </div>
          </div>

          {/* helm */}
          <div className="bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-5 mt-4">
            <Kicker>Helm</Kicker>
            <div className="flex items-center gap-2 flex-wrap mt-2.5">
              <button
                className="inline-flex items-center gap-2 rounded-input text-[13px] font-medium px-4 py-2 cursor-pointer border border-transparent whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-accent dark:bg-ember text-white dark:text-black hover:bg-accent-hover dark:hover:bg-ember-hover px-3 py-1.5 text-xs"
                disabled={busy}
                onClick={() => void shipIt()}
              >
                <Rocket size={13} /> Ship it
              </button>
              <button className="inline-flex items-center gap-2 rounded-input text-[13px] font-medium px-4 py-2 cursor-pointer border border-transparent whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-white dark:bg-panel border-line dark:border-edge text-ink dark:text-bone hover:bg-paper dark:hover:bg-emboss px-3 py-1.5 text-xs" disabled={busy} onClick={() => void runLifecycle("start")}>
                <Play size={13} /> Wake up
              </button>
              <button className="inline-flex items-center gap-2 rounded-input text-[13px] font-medium px-4 py-2 cursor-pointer border border-transparent whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-white dark:bg-panel border-line dark:border-edge text-ink dark:text-bone hover:bg-paper dark:hover:bg-emboss px-3 py-1.5 text-xs" disabled={busy} onClick={() => setPending("stop")}>
                <Power size={13} /> Nap
              </button>
              <button className="inline-flex items-center gap-2 rounded-input text-[13px] font-medium px-4 py-2 cursor-pointer border border-transparent whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-white dark:bg-panel border-line dark:border-edge text-ink dark:text-bone hover:bg-paper dark:hover:bg-emboss px-3 py-1.5 text-xs" disabled={busy} onClick={() => setPending("restart")}>
                <RotateCcw size={13} /> Fresh start
              </button>
              <button className="inline-flex items-center gap-2 rounded-input text-[13px] font-medium px-4 py-2 cursor-pointer border border-transparent whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-white dark:bg-panel border-line dark:border-edge text-ink dark:text-bone hover:bg-paper dark:hover:bg-emboss px-3 py-1.5 text-xs" disabled={busy || services.filter((s) => s.containerName).length === 0} onClick={() => setShowTerminal(true)} title="Open an interactive shell in a station container">
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
            <Kicker>Stations · {services.length}</Kicker>
            <div className="mt-2.5">
              {services.length === 0 ? (
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
                    {services.map((s) => (
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
            {services.filter((s) => s.containerName).length === 0 ? (
              <EmptyState
                title="No loggable stations"
                hint="Stations need a container name before their logs can stream."
              />
            ) : (
              <>
                <div className="flex items-center gap-2 flex-wrap mb-2.5">
                  {services
                    .filter((s) => s.containerName)
                    .map((s) => {
                      const active =
                        (logService ?? services.find((x) => x.containerName)?.containerName) ===
                        s.containerName;
                      return (
                        <button
                          key={s.id}
                          onClick={() => setLogService(s.containerName ?? null)}
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
                    logService ??
                    services.find((x) => x.containerName)?.containerName ??
                    ""
                  }
                />
              </>
            )}
          </div>

          {/* launches */}
          <div className="bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-5 mt-4">
            <Kicker>Launch log · {deployments.length}</Kicker>
            <div className="mt-2.5 flex flex-col gap-4">
              {deployments.length === 0 && <EmptyState title="No launches yet" />}
              {deployments.slice(0, 6).map((d) => (
                <div key={d.id} className="flex items-center justify-between gap-4 text-[13px]">
                  <span className="inline-flex items-center gap-1.5 font-mono text-[11px] bg-tint dark:bg-emboss border border-line dark:border-edge text-ink dark:text-bone rounded-md px-2 py-[3px] whitespace-nowrap">
                    <Hash size={11} /> {shortSha(d.commitSha)}
                  </span>
                  <span className="font-mono text-muted dark:text-fog">{fmtDuration(d.durationSec)}</span>
                  <span className="text-muted dark:text-fog">{timeAgo(d.startedAt)}</span>
                  <StatusPill status={d.status} />
                  <button
                    className="inline-flex items-center gap-2 rounded-input text-[13px] font-medium px-4 py-2 cursor-pointer border border-transparent whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-white dark:bg-panel border-line dark:border-edge text-ink dark:text-bone hover:bg-paper dark:hover:bg-emboss px-3 py-1.5 text-xs"
                    title={`Roll back to ${shortSha(d.commitSha)}`}
                    disabled={busy}
                    onClick={() => setPendingRollback(d)}
                  >
                    <History size={12} />
                  </button>
                </div>
              ))}
            </div>
          </div>

          {/* vault */}
          <div className="bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-5 mt-4">
            <Kicker>Vault · {secrets.length} sealed</Kicker>
            <div className="mt-2.5 flex flex-col gap-4">
              {secrets.map((s) => (
                <div key={s.id} className="flex items-center justify-between gap-4 text-[13px]">
                  <span className="font-mono">{s.name}</span>
                  <span className="flex items-center gap-2 flex-wrap">
                    <span className="inline-flex items-center gap-1.5 font-mono text-[11px] bg-tint dark:bg-emboss border border-line dark:border-edge text-ink dark:text-bone rounded-md px-2 py-[3px] whitespace-nowrap">
                      {s.environment}
                    </span>
                    {revealed[s.id] ? (
                      <span className="flex items-center gap-2 flex-wrap">
                        <code className="font-mono text-[11px] bg-paper dark:bg-abyss px-1.5 py-[2px] rounded-md">
                          {revealed[s.id].slice(0, 24)}{revealed[s.id].length > 24 ? "…" : ""}
                        </code>
                        <button
                          className="inline-flex items-center gap-2 rounded-input text-[13px] font-medium px-4 py-2 cursor-pointer border border-transparent whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-white dark:bg-panel border-line dark:border-edge text-ink dark:text-bone hover:bg-paper dark:hover:bg-emboss px-3 py-1.5 text-xs"
                          title="Copy"
                          onClick={() => {
                            void navigator.clipboard.writeText(revealed[s.id]);
                            pushToast("Copied to clipboard.");
                          }}
                        >
                          <Copy size={12} />
                        </button>
                        <button className="inline-flex items-center gap-2 rounded-input text-[13px] font-medium px-4 py-2 cursor-pointer border border-transparent whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-white dark:bg-panel border-line dark:border-edge text-ink dark:text-bone hover:bg-paper dark:hover:bg-emboss px-3 py-1.5 text-xs" onClick={() => void peek(s.id)} aria-label="Hide">
                          <EyeOff size={12} />
                        </button>
                      </span>
                    ) : (
                      <button className="inline-flex items-center gap-2 rounded-input text-[13px] font-medium px-4 py-2 cursor-pointer border border-transparent whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-white dark:bg-panel border-line dark:border-edge text-ink dark:text-bone hover:bg-paper dark:hover:bg-emboss px-3 py-1.5 text-xs" onClick={() => void peek(s.id)}>
                        <Eye size={12} /> Peek
                      </button>
                    )}
                  </span>
                </div>
              ))}
              {secrets.length === 0 && <EmptyState title="Vault is empty" hint="Seal your first secret below." />}
              <form onSubmit={(e) => void addSecret(e)}>
                <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                  <Field label="Name">
                    <input
                      className="w-full bg-white dark:bg-panel border border-line dark:border-edge rounded-input font-body text-[13px] text-ink dark:text-bone px-3 py-2.5 outline-none focus:border-accent dark:focus:border-ember placeholder:text-stone dark:placeholder:text-fog font-mono"
                      value={newName}
                      onChange={(e) => setNewName(e.target.value)}
                      placeholder="JWT_SECRET"
                    />
                  </Field>
                  <Field label="Value">
                    <input
                      className="w-full bg-white dark:bg-panel border border-line dark:border-edge rounded-input font-body text-[13px] text-ink dark:text-bone px-3 py-2.5 outline-none focus:border-accent dark:focus:border-ember placeholder:text-stone dark:placeholder:text-fog font-mono"
                      type="password"
                      value={newValue}
                      onChange={(e) => setNewValue(e.target.value)}
                      placeholder="••••••"
                    />
                  </Field>
                </div>
                <button
                  type="submit"
                  className="inline-flex items-center gap-2 rounded-input text-[13px] font-medium px-4 py-2 cursor-pointer border border-transparent whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-white dark:bg-panel border-line dark:border-edge text-ink dark:text-bone hover:bg-paper dark:hover:bg-emboss px-3 py-1.5 text-xs mt-4"
                  disabled={!newName.trim() || !newValue}
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
                <Kicker>Snapshots · {backups.length}</Kicker>
              </div>
              <button
                className="inline-flex items-center gap-2 rounded-input text-[13px] font-medium px-4 py-2 cursor-pointer border border-transparent whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-white dark:bg-panel border-line dark:border-edge text-ink dark:text-bone hover:bg-paper dark:hover:bg-emboss px-3 py-1.5 text-xs"
                disabled={busy}
                onClick={() => void createBackup()}
              >
                <Plus size={12} /> Snapshot now
              </button>
            </div>
            {backups.length === 0 ? (
              <EmptyState
                title="No snapshots yet"
                hint="Snapshots tar the deployment directory plus a metadata manifest."
              />
            ) : (
              <div className="flex flex-col gap-2.5">
                {backups.map((b) => (
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
                        disabled={busy}
                        onClick={() => setPendingRestore(b)}
                      >
                        Restore
                      </button>
                      <button
                        className="inline-flex items-center gap-2 rounded-input text-[13px] font-medium px-4 py-2 cursor-pointer border border-transparent whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-white dark:bg-panel border-line dark:border-edge text-ink dark:text-bone hover:bg-paper dark:hover:bg-emboss px-3 py-1.5 text-xs"
                        disabled={busy}
                        onClick={() => void deleteBackup(b.id)}
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
        </aside>
      </div>

      {showTerminal && (
        <TerminalModal
          projectName={project.name}
          services={services}
          onClose={() => setShowTerminal(false)}
        />
      )}

      {progress && (
        <DeployProgress
          projectName={project.name}
          operationId={progress.id}
          onClose={() => setProgress(null)}
          onDone={() => {
            onChanged();
            void refreshLists();
          }}
        />
      )}

      {pending && (
        <ConfirmModal
          title={pending === "stop" ? `Nap “${project.name}”?` : `Fresh-start “${project.name}”?`}
          body={
            pending === "stop"
              ? "This stops every container in the ship. Traffic goes dark until you wake it up."
              : "This restarts every container in the ship. Expect a brief blackout."
          }
          confirmLabel={pending === "stop" ? "Nap it" : "Fresh-start it"}
          busy={busy}
          onClose={() => setPending(null)}
          onConfirm={() => void runLifecycle(pending)}
        />
      )}
      {pendingRollback && (
        <ConfirmModal
          title={`Roll back to ${shortSha(pendingRollback.commitSha)}?`}
          body="This re-runs docker compose up for that launch's commit and records a new rollback deployment. Current containers will be replaced."
          confirmLabel="Roll back"
          busy={busy}
          onClose={() => setPendingRollback(null)}
          onConfirm={() => void rollbackTo(pendingRollback)}
        />
      )}
      {pendingRestore && (
        <ConfirmModal
          title="Restore this snapshot?"
          body="Live files in the deployment directory will be overwritten with the snapshot contents. Current state is not saved unless you snapshot first."
          confirmLabel="Restore snapshot"
          requireText="RESTORE"
          busy={busy}
          onClose={() => setPendingRestore(null)}
          onConfirm={() => void restoreBackup(pendingRestore)}
        />
      )}
    </>
  );
}
