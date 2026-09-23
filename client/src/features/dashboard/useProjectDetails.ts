import { useCallback, useEffect, useState } from "react";
import { api } from "../../lib/api";
import type { Backup, Deployment, Project, SecretMeta, Service } from "../../lib/types";
import { useUi } from "../../stores/store";

export type PendingAction = "stop" | "restart" | null;

// Shared data + actions for a single project. Used by the slim summary
// drawer and the full details page so both stay in sync without duplicating
// fetch/action logic.
export function useProjectDetails(project: Project, onChanged: () => void) {
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

  const refreshLists = useCallback(async () => {
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
  }, [project.id]);

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

  const shipIt = async () => {
    setBusy(true);
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
      pushToast(`Rolling back to ${(d.commitSha ?? "").slice(0, 7)} — dispatch running.`);
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

  return {
    services,
    deployments,
    secrets,
    revealed,
    pending, setPending,
    pendingRollback, setPendingRollback,
    pendingRestore, setPendingRestore,
    logService, setLogService,
    showTerminal, setShowTerminal,
    progress, setProgress,
    backups,
    busy,
    newName, setNewName,
    newValue, setNewValue,
    refreshLists,
    runLifecycle,
    shipIt,
    rollbackTo,
    createBackup,
    deleteBackup,
    restoreBackup,
    peek,
    addSecret,
  };
}

export type ProjectDetails = ReturnType<typeof useProjectDetails>;
