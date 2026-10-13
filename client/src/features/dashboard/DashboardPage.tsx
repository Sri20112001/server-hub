import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useNavigate, useSearchParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import {
  Activity,
  AlertTriangle,
  ArrowRight,
  DatabaseBackup,
  Hash,
  RefreshCw,
  Server,
} from "lucide-react";
import { api, dbBackupsApi } from "../../lib/api";
import {
  fleetLabel,
  queryState,
  toBusinessStatus,
  toFleetStatus,
  toneRank,
  type BusinessTone,
  type DashboardData,
  type Deployment,
  type DiscoveryResult,
  type FleetStatus,
  type Project,
  type Service,
} from "../../lib/types";
import {
  fmtBytes,
  formatAge,
  greeting,
  shortId,
  shortSha,
  timeAgo,
} from "../../lib/format";
import { useAuth, useUi } from "../../stores/store";
import { useEvents } from "../../lib/useEvents";
import type { BusEvent } from "../../lib/types";
import { Kicker, StatusPill, DashboardSkeleton, PageError } from "../../components/ui";
import { ProjectDrawer } from "./ProjectDrawer";
import { DiscoveryModal } from "./DiscoveryModal";
import { useDashboardData } from "../../lib/useDashboard";

/* ---------- small pieces ---------- */

function useNow(intervalMs = 5000) {  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), intervalMs);
    return () => clearInterval(id);
  }, [intervalMs]);
  return now;
}



/* TEMP-HIDE (Projects-hiding review): manual project creation is disabled
   while the Projects section is hidden. The code below is preserved as-is;
   uncomment the component, the showCreate state, the masthead button, and
   the modal invocation (plus the Plus/Field/Modal imports) to restore it.
function CreateShipModal({ onClose, onCreated }: { onClose: () => void; onCreated: () => void }) {
  const pushToast = useUi((s) => s.pushToast);
  const [name, setName] = useState("");
  const [repository, setRepository] = useState("");
  const [branch, setBranch] = useState("main");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setErr(null);
    try {
      await api.createProject({ name: name.trim(), repository: repository.trim(), branch: branch.trim() || "main" });
      pushToast(`“${name.trim()}” was added.`);
      onCreated();
      onClose();
    } catch (e2) {
      setErr(e2 instanceof Error ? e2.message : "Could not add project");
      setBusy(false);
    }
  };

  return (
    <Modal onClose={onClose}>
      <h3 className="text-[18px] mb-1">New project</h3>
      <p className="text-muted dark:text-fog text-[13px] mb-4">
        Add a project to monitor. Runtime details can be filled in afterwards.
      </p>
      <form onSubmit={submit} className="flex flex-col gap-4">
        <Field label="Project name">
          <input
            className="w-full bg-white dark:bg-panel border border-line dark:border-edge rounded-input font-body text-[13px] text-ink dark:text-bone px-3 py-2.5 outline-none focus:border-accent dark:focus:border-ember placeholder:text-stone dark:placeholder:text-fog"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="nodevault"
            autoFocus
          />
        </Field>
        <Field label="Repository">
          <input
            className="w-full bg-white dark:bg-panel border border-line dark:border-edge rounded-input font-body text-[13px] text-ink dark:text-bone px-3 py-2.5 outline-none focus:border-accent dark:focus:border-ember placeholder:text-stone dark:placeholder:text-fog font-mono"
            value={repository}
            onChange={(e) => setRepository(e.target.value)}
            placeholder="github.com/acme/nodevault"
          />
        </Field>
        <Field label="Branch">
          <input
            className="w-full bg-white dark:bg-panel border border-line dark:border-edge rounded-input font-body text-[13px] text-ink dark:text-bone px-3 py-2.5 outline-none focus:border-accent dark:focus:border-ember placeholder:text-stone dark:placeholder:text-fog font-mono"
            value={branch}
            onChange={(e) => setBranch(e.target.value)}
          />
        </Field>
        {err && (
          <div className="bg-red-50 dark:bg-red-950 border border-red-200 dark:border-red-900 text-red-800 dark:text-red-200 rounded-input px-3 py-2.5 text-[13px]">
            {err}
          </div>
        )}
        <div className="flex items-center gap-2 flex-wrap justify-end">
          <button
            type="button"
            className="inline-flex items-center gap-2 rounded-input text-[13px] font-medium px-4 py-2 cursor-pointer border border-transparent whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-white dark:bg-panel border-line dark:border-edge text-ink dark:text-bone hover:bg-paper dark:hover:bg-emboss"
            onClick={onClose}
            disabled={busy}
          >
            Cancel
          </button>
          <button
            type="submit"
            className="inline-flex items-center gap-2 rounded-input text-[13px] font-medium px-4 py-2 cursor-pointer border border-transparent whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-accent dark:bg-ember text-white dark:text-black hover:bg-accent-hover dark:hover:bg-ember-hover"
            disabled={busy || !name.trim()}
          >
            {busy ? "Adding…" : "Add project"}
          </button>
        </div>
      </form>
    </Modal>
  );
}
END TEMP-HIDE */

/* ---------- page ---------- */

// Compact Database Backups strip for the main dashboard. Reads the real
// backup summary endpoint on the standard 30s cadence (no extra loops);
// the full section lives at /db-backups. Silent while loading, honest
// warning row on failure — never fabricated numbers.
function DbBackupStrip() {
  const nav = useNavigate();
  const { data, isLoading, isError } = useQuery({
    queryKey: ["dbbackup-summary"],
    queryFn: dbBackupsApi.dashboard,
    refetchInterval: 30_000,
    retry: 1,
  });
  // No data yet: an error with nothing to show becomes the honest
  // warning row (checked before the loading branch — data is undefined
  // on error, so testing !data first would hide failures forever).
  if (!data) {
    if (isError && !isLoading) {
      return (
        <button
          onClick={() => nav("/db-backups")}
          className="mt-4 w-full text-left rounded-card border border-line dark:border-edge bg-white dark:bg-panel px-5 py-3.5 cursor-pointer"
        >
          <span className="text-sm text-muted dark:text-fog">Database backups status unavailable — open the section to retry.</span>
        </button>
      );
    }
    return null;
  }
  const failed = (data.recent_runs ?? []).filter((r) => r.status === "FAILED").slice(0, 3);
  return (
    <button
      onClick={() => nav("/db-backups")}
      className="mt-4 w-full text-left rounded-card border border-line dark:border-edge bg-white dark:bg-panel px-5 py-4 shadow-sm cursor-pointer hover:border-accent dark:hover:border-ember transition-colors"
    >
      <span className="flex items-center gap-2">
        <DatabaseBackup size={15} className="text-muted dark:text-fog" />
        <span className="text-sm font-semibold text-ink dark:text-bone">Database backups</span>
        <span className="font-mono text-[11px] text-muted dark:text-fog">
          {data.total_jobs} job{data.total_jobs === 1 ? "" : "s"} · {data.agents_online} agent{data.agents_online === 1 ? "" : "s"} online
        </span>
        {data.failed_runs > 0 && (
          <span className="font-mono text-[11px] px-2 py-0.5 rounded border border-brick/30 bg-brick/10 text-brick">
            {data.failed_runs} failed · 24h
          </span>
        )}
      </span>
      {failed.length > 0 && (
        <span className="block text-xs text-muted dark:text-fog mt-1.5 truncate">
          Latest failure: {failed[0].error_message || failed[0].id.slice(0, 8)}
        </span>
      )}
    </button>
  );
}

export function DashboardPage({ setOnline }: { setOnline: (v: boolean) => void }) {
  const { user } = useAuth();
  const pushToast = useUi((s) => s.pushToast);
  const [params, setParams] = useSearchParams();
  const nav = useNavigate();
  const { data: fullData, reload, isLoading, isError, error } = useDashboardData({ setOnline });
  const data = fullData?.dash ?? null;
  const servers = fullData?.servers ?? [];
  const serversOk = fullData?.serversOk ?? false;
  const serversError = fullData?.serversError ?? null;
  const now = useNow(5000);

  // Needs-attention inputs reuse existing endpoints only: active alerts and
  // website/service checks join the projects + servers already on the page.
  const alertsQ = useQuery({
    queryKey: ["alerts-triggered"],
    queryFn: () => api.alerts("TRIGGERED"),
    refetchInterval: 30_000,
    retry: 1,
  });
  const checksQ = useQuery({
    queryKey: ["health-checks"],
    queryFn: api.healthChecks,
    refetchInterval: 60_000,
    retry: 1,
  });
  const feed = fullData?.feed ?? [];
  // TEMP-HIDE (Projects-hiding review): see CreateShipModal below.
  // const [showCreate, setShowCreate] = useState(false);
  const [showScan, setShowScan] = useState(false);
  // TEMP-HIDE (Projects-hiding review): wipe confirm for the hidden
  // Recent updates section.
  // const [confirmWipe, setConfirmWipe] = useState(false);
  const prevHealth = useRef<Map<number, string> | null>(null);
  const prevDepIds = useRef<Set<number> | null>(null);
  const prevHot = useRef(false);

  // Browser alerts for health transitions / fresh successful launches /
  // resource pressure, gated by Settings → Alerts toggles. First poll only
  // arms the baselines so it never spams on page load.
  // Declared (and memoized) before `load` so the dependency is stable.
  const notifyTransitions = useCallback((dash: DashboardData) => {
    let prefs = { deployFinished: true, healthChanged: true, highUsage: false };
    try {
      prefs = {
        deployFinished: true,
        healthChanged: true,
        highUsage: false,
        ...JSON.parse(localStorage.getItem("serverhub.alerts") ?? "{}"),
      };
    } catch {
      /* keep defaults */
    }
    const cur = new Map(dash.projects.map((p) => [p.id, toFleetStatus(p.status)]));
    if (prevHealth.current !== null && prefs.healthChanged) {
      for (const [id, st] of cur) {
        const before = prevHealth.current.get(id);
        if (before !== undefined && before !== st && st !== "docked") {
          const name = dash.projects.find((p) => p.id === id)?.name ?? `#${id}`;
          pushToast(
            `${name} is ${fleetLabel[st]}.`,
            st === "lost" || st === "choppy",
          );
        }
      }
    }
    prevHealth.current = cur;

    const ids = new Set(dash.recentDeployments.map((d) => d.id));
    if (prevDepIds.current !== null && prefs.deployFinished) {
      for (const d of dash.recentDeployments) {
        if (!prevDepIds.current.has(d.id) && d.status === "SUCCESS") {
          const name =
            dash.projects.find((p) => p.id === d.projectId)?.name ?? `#${d.projectId}`;
          pushToast(`Launch succeeded: ${name} @ ${d.commitSha || "unknown"}.`);
          break;
        }
      }
    }
    prevDepIds.current = ids;

    const hot = dash.server.cpuPercent > 85 || dash.server.memPercent > 85;
    if (hot && !prevHot.current && prefs.highUsage) {
      pushToast("High load — processor or memory above 85%.", true);
    }
    prevHot.current = hot;
  }, [pushToast]);

  useEffect(() => {
    if (data) {
      notifyTransitions(data);
    }
  }, [data, notifyTransitions]);

  const load = useCallback(() => reload(), [reload]);

  // Auto-sync on app load: scan once and import everything
  // unregistered, so no Docker find is ever missed and nobody has to click
  // Scan → Import one by one. Silent unless something actually joined.
  // Import-all is idempotent, so re-running on every mount is safe.
  const autoSync = useCallback(async () => {
    try {
      const r: DiscoveryResult = await api.discovery();
      const pending =
        r.projects.filter((p) => !p.registered).length +
        (r.filesystem ?? []).filter((f) => !f.registered).length;
      if (pending === 0) return;
      const res = await api.importAllDiscovered();
      if (res.imported > 0) {
        pushToast(
          `${res.imported} project${res.imported === 1 ? "" : "s"} auto-registered with ${res.servicesAdded} service${res.servicesAdded === 1 ? "" : "s"}.`,
        );
        await load();
      }
    } catch {
      // Offline, logged out, or Docker unreachable — stay silent;
      // manual Scan and New project keep working.
    }
  }, [load, pushToast]);

  useEffect(() => {
    void autoSync();
  }, [autoSync]);

  // Live signals: refresh the workbench and toast on deployments,
  // container/project lifecycle, gateway reloads, backups, threshold
  // crossings, and managed-server / alert transitions. Health transitions
  // keep coming through the poll alerts above to respect the Settings
  // toggles without double-notifying.
  useEvents((ev: BusEvent) => {
    const d = (ev.data ?? {}) as Record<string, unknown>;
    const str = (v: unknown) => (typeof v === "string" ? v : "");
    switch (ev.type) {
      case "server.heartbeat":
      case "server.metrics":
      case "server.status":
      case "alert.triggered":
      case "alert.resolved":
      case "monitoring.alert.firing":
      case "monitoring.alert.resolved":
        void load();
        break;
      case "deployment.completed":
        pushToast(
          `Update complete: ${str(d.project)} @ ${shortSha(str(d.commit))} (${str(d.durationSec)}s).`,
        );
        void load();
        break;
      case "deployment.failed":
        pushToast(`Update failed: ${str(d.project)} @ ${shortSha(str(d.commit))}.`, true);
        void load();
        break;
      case "deployment.started":
        void load();
        break;
      case "container.start":
      case "container.stop":
      case "container.restart":
        if (d.ok) {
          pushToast(`Container ${str(d.action)}: ${shortId(str(d.id))}.`);
          void load();
        } else if (str(d.detail)) {
          pushToast(`Container ${str(d.action)} failed: ${str(d.detail)}.`, true);
        }
        break;
      case "project.start":
      case "project.stop":
      case "project.restart":
        if (d.ok) {
          pushToast(`Project ${str(d.action)}: ${str(d.project)}.`);
          void load();
        } else if (str(d.detail)) {
          pushToast(`Project ${str(d.action)} failed: ${str(d.detail)}.`, true);
        }
        break;
      case "gateway.reloaded":
        pushToast(d.ok ? "Gateway reloaded." : `Gateway reload failed: ${str(d.detail)}.`, !d.ok);
        break;
      case "discovery.completed":
        pushToast(`“${str(d.project)}” was added.`);
        void load();
        break;
      case "backup.created":
        pushToast(`Snapshot ready: ${str(d.project)} (${fmtBytes(Number(d.sizeBytes ?? 0))}).`);
        break;
      case "backup.failed":
        pushToast(`Snapshot failed: ${str(d.error)}.`, true);
        break;
      case "backup.restored":
        pushToast(`Snapshot restored: ${str(d.project)}.`);
        break;
      case "backup.restoreFailed":
        pushToast(`Restore failed: ${str(d.error)}.`, true);
        break;
      case "backup.deleted":
        break;
      case "telemetry.threshold": {
        let prefs = { highUsage: false };
        try {
          prefs = {
            highUsage: false,
            ...JSON.parse(localStorage.getItem("serverhub.alerts") ?? "{}"),
          };
        } catch {
          /* keep defaults */
        }
        if (prefs.highUsage) {
          pushToast(
            `Resource signal: host ${str(d.resource)} at ${Number(d.value ?? 0).toFixed(1)}% (limit ${str(d.threshold)}%).`,
            true,
          );
        }
        break;
      }
      default:
        break;
    }
  });

  // Opened from the command palette ("Scan shipyard" action).
  useEffect(() => {
    if (params.get("scan") !== null) {
      // Intentional: consume the one-shot ?scan URL param on mount.
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setShowScan(true);
      setParams({}, { replace: true });
    }
  }, [params, setParams]);

  const projectById = useMemo(() => {
    const m = new Map<number, Project>();
    data?.projects.forEach((p) => m.set(p.id, p));
    return m;
  }, [data]);

  const selectedId = params.get("project");
  const selected = selectedId ? (projectById.get(Number(selectedId)) ?? null) : null;

  // TEMP-HIDE (Projects-hiding review): update-history summary behind the
  // hidden Deployments tile. Uncomment with the tile to restore.
  // const deployStats = useMemo(() => {
  //   const list = data?.recentDeployments ?? [];
  //   const withDur = list.filter((d) => d.durationSec !== undefined && d.durationSec !== null);
  //   const avg = withDur.length
  //     ? Math.round(withDur.reduce((a, d) => a + (d.durationSec ?? 0), 0) / withDur.length)
  //     : undefined;
  //   const ok = list.filter((d) => d.status === "SUCCESS").length;
  //   const rate = list.length ? Math.round((ok / list.length) * 100) : 100;
  //   const today = new Date().toISOString().slice(0, 10);
  //   const todayCount = list.filter((d) => (d.startedAt ?? "").startsWith(today)).length;
  //   return { avg, rate, todayCount };
  // }, [data]);

  const healthLine = useMemo(() => {
    if (!data) return "Checking…";
    const { healthy, degraded, down } = data.counts;
    if (down > 0) return `${down} thing${down === 1 ? " is" : "s are"} down`;
    if (degraded > 0) return `${degraded} need${degraded === 1 ? "s" : ""} attention`;
    if (healthy > 0) return "Everything is working";
    return "Nothing reporting yet";
  }, [data]);

  interface AttentionItem {
    key: string;
    tone: BusinessTone;
    title: string;
    detail: string;
    route: string;
  }

  // One outage list from data already on the page: down/unknown projects,
  // offline servers, failing website checks, active alerts. Most severe
  // first; stopped projects are a legitimate state, not a problem.
  const attention = useMemo(() => {
    const items: AttentionItem[] = [];
    for (const p of data?.projects ?? []) {
      const b = toBusinessStatus(toFleetStatus(p.status));
      if (b.tone === "down" || b.tone === "attention" || b.tone === "unknown") {
        items.push({
          key: `project-${p.id}`,
          tone: b.tone,
          title: p.name,
          detail: b.tone === "unknown"
            ? "Status unknown — open the project to check."
            : b.tone === "attention"
              ? "Project needs attention."
              : `Project is ${b.label.toLowerCase()}.`,
          route: `/?project=${p.id}`,
        });
      }
    }
    for (const s of fullData?.servers ?? []) {
      if (s.status === "OFFLINE") {
        items.push({
          key: `server-${s.id}`,
          tone: "down",
          title: s.name,
          detail: "Server is offline — check power, network, and its monitoring agent.",
          route: `/servers/${s.id}`,
        });
      } else if (s.status === "WARNING") {
        items.push({
          key: `server-${s.id}`,
          tone: "attention",
          title: s.name,
          detail: "Server needs attention — open it for details.",
          route: `/servers/${s.id}`,
        });
      } else if (s.status === "UNKNOWN") {
        items.push({
          key: `server-${s.id}`,
          tone: "unknown",
          title: s.name,
          detail: "Server hasn't reported yet.",
          route: `/servers/${s.id}`,
        });
      }
    }
    for (const c of checksQ.data ?? []) {
      if (c.status === "DOWN") {
        items.push({
          key: `check-${c.id}`,
          tone: "down",
          title: c.name,
          detail: `Website/service check failed${c.lastCheckedAt ? ` · last checked ${timeAgo(c.lastCheckedAt)}` : ""}.`,
          route: "/health-checks",
        });
      }
    }
    for (const a of alertsQ.data ?? []) {
      if (a.status !== "TRIGGERED") continue;
      items.push({
        key: `alert-${a.id}`,
        tone: a.severity === "CRITICAL" ? "down" : "attention",
        title: a.message || a.condition,
        detail: `Alert raised${a.triggeredAt ? ` ${timeAgo(a.triggeredAt)}` : ""} — see Alerts to act on it.`,
        route: "/alerts",
      });
    }
    return items.sort((x, y) => toneRank(x.tone) - toneRank(y.tone)).slice(0, 6);
  }, [data, fullData, alertsQ.data, checksQ.data]);

  // Loading / error / ready are mutually exclusive and explicit: a failed
  // fetch renders as an error with retry, never as an endless skeleton and
  // never as a healthy-looking empty page.
  const state = queryState({ hasData: fullData != null, isLoading, isError });
  if (state === "loading") {
    return <DashboardSkeleton />;
  }
  if (state === "error" || !data) {
    return (
      <PageError
        title="Couldn't load the dashboard"
        detail={error instanceof Error ? error.message : undefined}
        onRetry={() => void load()}
      />
    );
  }

  return (
    <main className="w-full max-w-5xl mx-auto px-6 pt-12 pb-28">
      <div className="flex flex-col space-y-8">
        {/* 1. Workspace Header */}
        <div className="flex flex-col md:flex-row md:items-end justify-between gap-4">
          <div className="flex flex-col space-y-3">
            <div className="flex items-center gap-2">
              <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full bg-surface-container-low text-on-surface-variant text-[12px] font-medium">
                <span className="font-semibold text-on-surface">ServiceHub</span>
                <span>v2.4</span>
                <span className="w-1.5 h-1.5 rounded-full bg-secondary inline-block ml-0.5" />
                <span className="text-secondary font-medium">Operational</span>
              </span>
            </div>
            <h1 className="text-[32px] sm:text-[36px] font-bold text-on-surface tracking-tight leading-tight">
              {greeting()}, {user?.username ?? "Operations"}
            </h1>
            <p className="text-[16px] text-on-surface-variant max-w-xl leading-relaxed">
              All core services and backup workers are running smoothly across your fleet.
            </p>
          </div>
          <div className="flex items-center gap-2">
            <button
              onClick={() => void reload()}
              className="inline-flex items-center gap-1.5 text-on-surface-variant hover:text-on-surface text-[13px] font-medium transition-colors py-2 px-3.5 rounded-full bg-surface-container-low hover:bg-surface-container cursor-pointer border-0"
              type="button"
            >
              <RefreshCw size={14} />
              <span>Refresh</span>
            </button>
            <button
              onClick={() => nav("/settings")}
              className="inline-flex items-center gap-1.5 text-on-surface-variant hover:text-on-surface text-[13px] font-medium transition-colors py-2 px-3.5 rounded-full hover:bg-surface-container-low cursor-pointer border-0"
              type="button"
            >
              <span>Workspace Settings</span>
            </button>
          </div>
        </div>

        {/* 2. Overall System Health Banner */}
        <div className="bg-surface-container-low rounded-2xl p-5 flex flex-col sm:flex-row sm:items-center justify-between gap-4 shadow-sm">
          <div className="flex items-center gap-3.5">
            <div className="w-10 h-10 rounded-xl bg-secondary-fixed flex items-center justify-center shrink-0 text-on-secondary-fixed">
              <Activity size={22} className="text-secondary" />
            </div>
            <div>
              <p className="text-[16px] font-semibold text-on-surface">
                {attention.length === 0 ? "All systems normal" : "Attention needed on some components"}
              </p>
              <p className="text-[14px] text-on-surface-variant">
                {servers.length} active servers responding · {healthLine} · {formatAge(now - (fullData?.fetchedAt ?? now))} ago
              </p>
            </div>
          </div>
          <button
            onClick={() => void reload()}
            className="inline-flex items-center justify-center gap-2 px-4 py-2 rounded-xl bg-surface-container-lowest text-primary hover:bg-primary hover:text-on-primary text-[14px] font-medium shadow-sm transition-all shrink-0 cursor-pointer border-0"
            type="button"
          >
            <RefreshCw size={15} />
            <span>Run Health Audit</span>
          </button>
        </div>

        {/* 3. Attention Items (if any) */}
        {attention.length > 0 && (
          <div className="bg-tertiary-fixed/30 rounded-2xl p-4 flex flex-col gap-3 shadow-sm transition-all duration-300">
            <div className="flex items-center justify-between gap-2">
              <div className="flex items-center gap-2">
                <AlertTriangle size={18} className="text-tertiary shrink-0" />
                <span className="text-[14px] font-semibold text-on-surface">
                  {attention.length} item{attention.length === 1 ? "" : "s"} need attention:
                </span>
              </div>
            </div>
            <div className="flex flex-col gap-1.5">
              {attention.map((item) => (
                <div
                  key={item.key}
                  onClick={() => nav(item.route)}
                  className="flex items-center justify-between p-2.5 rounded-xl bg-surface-container-lowest hover:bg-surface-container transition-colors cursor-pointer"
                >
                  <div className="flex items-center gap-2.5 min-w-0">
                    <span
                      className={`w-2 h-2 rounded-full shrink-0 ${
                        item.tone === "down" ? "bg-error" : item.tone === "attention" ? "bg-tertiary" : "bg-outline"
                      }`}
                    />
                    <span className="text-[13px] font-medium text-on-surface truncate">{item.title}</span>
                    <span className="text-[13px] text-on-surface-variant truncate hidden sm:inline">{item.detail}</span>
                  </div>
                  <ArrowRight size={14} className="text-on-surface-variant shrink-0 ml-2" />
                </div>
              ))}
            </div>
          </div>
        )}

        {/* 4. Three Compact Summaries (Servers, Backups, System Load) */}
        <div className="grid grid-cols-1 md:grid-cols-3 gap-5">
          {/* Servers Card */}
          <div
            onClick={serversOk ? () => nav("/servers") : undefined}
            className="bg-surface-container-lowest rounded-2xl p-6 flex flex-col justify-between shadow-sm hover:shadow-md transition-all group cursor-pointer"
          >
            <div>
              <div className="flex items-center justify-between mb-4">
                <span className="text-[11px] font-semibold uppercase text-on-surface-variant tracking-wider">Servers</span>
                <div className="w-8 h-8 rounded-lg bg-surface-container-low flex items-center justify-center text-primary">
                  <Server size={18} />
                </div>
              </div>
              <div className="space-y-1">
                <div className="text-[28px] font-bold text-on-surface tracking-tight">
                  {serversOk ? `${servers.length} Active` : "Offline"}
                </div>
                <p className="text-[13px] text-on-surface-variant flex items-center gap-1.5">
                  <span className="w-2 h-2 rounded-full bg-secondary" />
                  {serversOk ? `${servers.filter((s) => s.status === "ONLINE").length} reporting online` : (serversError ?? "Unable to load servers")}
                </p>
              </div>
            </div>
            <div className="pt-6 mt-2">
              <span className="inline-flex items-center gap-1 text-[13px] font-medium text-primary group-hover:translate-x-0.5 transition-transform">
                <span>Inspect fleet</span>
                <ArrowRight size={14} />
              </span>
            </div>
          </div>

          {/* Backups Card */}
          <div
            onClick={() => nav("/db-backups")}
            className="bg-surface-container-lowest rounded-2xl p-6 flex flex-col justify-between shadow-sm hover:shadow-md transition-all group cursor-pointer"
          >
            <div>
              <div className="flex items-center justify-between mb-4">
                <span className="text-[11px] font-semibold uppercase text-on-surface-variant tracking-wider">Backups</span>
                <div className="w-8 h-8 rounded-lg bg-surface-container-low flex items-center justify-center text-primary">
                  <DatabaseBackup size={18} />
                </div>
              </div>
              <div className="space-y-1">
                <div className="text-[28px] font-bold text-on-surface tracking-tight">
                  Database Backups
                </div>
                <p className="text-[13px] text-on-surface-variant flex items-center gap-1.5">
                  <span className="w-2 h-2 rounded-full bg-secondary" />
                  Point-in-time recovery & snapshots
                </p>
              </div>
            </div>
            <div className="pt-6 mt-2">
              <span className="inline-flex items-center gap-1 text-[13px] font-medium text-primary group-hover:translate-x-0.5 transition-transform">
                <span>View backup runs</span>
                <ArrowRight size={14} />
              </span>
            </div>
          </div>

          {/* System Load Card */}
          <div
            onClick={() => nav("/telemetry")}
            className="bg-surface-container-lowest rounded-2xl p-6 flex flex-col justify-between shadow-sm hover:shadow-md transition-all group cursor-pointer"
          >
            <div>
              <div className="flex items-center justify-between mb-4">
                <span className="text-[11px] font-semibold uppercase text-on-surface-variant tracking-wider">System Load</span>
                <div className="w-8 h-8 rounded-lg bg-surface-container-low flex items-center justify-center text-primary">
                  <Activity size={18} />
                </div>
              </div>
              <div className="space-y-2">
                <div className="flex justify-between items-center text-[13px]">
                  <span className="text-on-surface-variant">CPU</span>
                  <span className="font-mono font-medium text-on-surface">{Math.round(data.server.cpuPercent)}%</span>
                </div>
                <div className="h-1.5 rounded-full bg-surface-container overflow-hidden">
                  <div
                    className="h-full rounded-full bg-primary"
                    style={{ width: `${Math.min(100, Math.max(0, data.server.cpuPercent))}%` }}
                  />
                </div>
                <div className="flex justify-between items-center text-[13px] pt-1">
                  <span className="text-on-surface-variant">RAM</span>
                  <span className="font-mono font-medium text-on-surface">{Math.round(data.server.memPercent)}%</span>
                </div>
                <div className="h-1.5 rounded-full bg-surface-container overflow-hidden">
                  <div
                    className="h-full rounded-full bg-secondary"
                    style={{ width: `${Math.min(100, Math.max(0, data.server.memPercent))}%` }}
                  />
                </div>
              </div>
            </div>
            <div className="pt-6 mt-2">
              <span className="inline-flex items-center gap-1 text-[13px] font-medium text-primary group-hover:translate-x-0.5 transition-transform">
                <span>Host Telemetry</span>
                <ArrowRight size={14} />
              </span>
            </div>
          </div>
        </div>

        {/* Database Backups Summary Strip */}
        <DbBackupStrip />

      {/* TEMP-HIDE (Projects-hiding review): Recent updates / Update
          history section disabled — it is the visible end of the
          project-versioning deployments log. Preserved as-is (including
          the Clear-history footer, which the backend refuses by design
          with 410); uncomment this block plus the wipe modal/state below
          to restore it.
      <div className="flex items-center justify-between gap-4 mt-8 mb-4">
        <div>
          <Kicker>Update history</Kicker>
          <h2 className="font-head text-[22px] font-bold tracking-[-0.02em] mt-1 text-ink dark:text-bone">Recent updates</h2>
        </div>
      </div>
      <div className="bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-0 overflow-hidden shadow-sm">
        <div className="overflow-x-auto -mx-px">
          <table className="w-full min-w-[560px] md:min-w-0 border-collapse text-[13px]">
            <thead>
              <tr className="border-b border-line dark:border-edge">
                <th className="text-left text-[11px] font-semibold tracking-[0.08em] uppercase text-muted dark:text-fog px-4 py-2.5 bg-paper dark:bg-abyss">Version</th>
                <th className="text-left text-[11px] font-semibold tracking-[0.08em] uppercase text-muted dark:text-fog px-4 py-2.5 bg-paper dark:bg-abyss">Project</th>
                <th className="hidden md:table-cell text-left text-[11px] font-semibold tracking-[0.08em] uppercase text-muted dark:text-fog px-4 py-2.5 bg-paper dark:bg-abyss">Branch</th>
                <th className="hidden md:table-cell text-left text-[11px] font-semibold tracking-[0.08em] uppercase text-muted dark:text-fog px-4 py-2.5 bg-paper dark:bg-abyss">Duration</th>
                <th className="text-left text-[11px] font-semibold tracking-[0.08em] uppercase text-muted dark:text-fog px-4 py-2.5 bg-paper dark:bg-abyss">Time</th>
                <th className="text-left text-[11px] font-semibold tracking-[0.08em] uppercase text-muted dark:text-fog px-4 py-2.5 bg-paper dark:bg-abyss">Status</th>
              </tr>
            </thead>
            <tbody>
              {data.recentDeployments.length === 0 && (
                <tr className="group">
                  <td colSpan={6} className="px-4 py-6 border-t border-line dark:border-edge align-middle text-center text-muted dark:text-fog">
                    No updates yet.
                  </td>
                </tr>
              )}
              {data.recentDeployments.map((d) => (
                <tr key={d.id} className="group hover:bg-paper/60 dark:hover:bg-emboss/40 transition-colors">
                  <td className="px-4 py-3 border-t border-line dark:border-edge align-middle">
                    <span className="inline-flex items-center gap-1.5 font-mono text-[11px] bg-paper dark:bg-emboss border border-line dark:border-edge text-ink dark:text-bone rounded-md px-2 py-[3px] whitespace-nowrap">
                      <Hash size={11} /> {shortSha(d.commitSha)}
                    </span>
                  </td>
                  <td className="px-4 py-3 border-t border-line dark:border-edge align-middle">
                    <div className="font-medium text-ink dark:text-bone">{projectById.get(d.projectId)?.name ?? `#${d.projectId}`}</div>
                    <div className="font-mono text-muted dark:text-fog text-xs">{d.trigger ?? ""}</div>
                  </td>
                  <td className="hidden md:table-cell px-4 py-3 border-t border-line dark:border-edge align-middle">
                    <span className="inline-flex items-center gap-1.5 font-mono text-[11px] bg-paper dark:bg-emboss border border-line dark:border-edge text-ink dark:text-bone rounded-md px-2 py-[3px] whitespace-nowrap">
                      <GitBranch size={11} /> {d.branch || "—"}
                    </span>
                  </td>
                  <td className="hidden md:table-cell px-4 py-3 border-t border-line dark:border-edge align-middle font-mono text-muted dark:text-fog">
                    {fmtDuration(d.durationSec)}
                  </td>
                  <td className="px-4 py-3 border-t border-line dark:border-edge align-middle text-muted dark:text-fog font-mono text-xs">
                    {timeAgo(d.startedAt)}
                  </td>
                  <td className="px-4 py-3 border-t border-line dark:border-edge align-middle">
                    <StatusPill status={d.status} />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <div className="flex flex-col sm:flex-row sm:items-center gap-2 flex-wrap justify-between px-4 py-3 border-t border-line dark:border-edge text-muted dark:text-fog text-xs bg-paper/30 dark:bg-abyss/30">
          <span>
            Showing {data.recentDeployments.length} of {data.counts.deployments} updates recorded
          </span>
          <button
            className="inline-flex items-center gap-2 rounded-input font-medium cursor-pointer border whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-white dark:bg-panel border-line dark:border-edge text-ink dark:text-bone hover:bg-paper dark:hover:bg-emboss px-3 py-1.5 text-xs"
            onClick={() => setConfirmWipe(true)}
            title="Wipe deployment history"
          >
            Clear history
          </button>
        </div>
      </div>
      END TEMP-HIDE */}

      {/* activity */}
      <div className="mt-8">
        <div className="flex items-center justify-between gap-4 mb-3">
          <div className="flex items-center gap-2 flex-wrap">
            <h2 className="font-head text-[18px] font-bold tracking-[-0.02em] text-ink dark:text-bone">Recent activity</h2>
            <span className="inline-flex items-center gap-1.5 font-mono text-[11px] bg-paper dark:bg-emboss border border-line dark:border-edge text-ink dark:text-bone rounded-md px-2 py-[3px] whitespace-nowrap">
              LIVE
            </span>
          </div>
        </div>
        <div className="font-mono text-xs leading-[1.7] bg-paper dark:bg-abyss border border-line dark:border-edge rounded-xl px-5 py-4 overflow-x-auto whitespace-pre-wrap break-words text-ink dark:text-bone shadow-inner">
          {feed.length === 0 ? (
            <span className="text-muted dark:text-fog">No recent activity.</span>
          ) : (
            feed.map((line, i) => <div key={i}>{line}</div>)
          )}
        </div>
      </div>

      {/* TEMP-HIDE (Projects-hiding review): creation modal disabled.
      {showCreate && <CreateShipModal onClose={() => setShowCreate(false)} onCreated={() => void load()} />}
      */}

      {showScan && <DiscoveryModal onClose={() => setShowScan(false)} onImported={() => void load()} />}

      {selected && (
        <ProjectDrawer
          project={selected}
          onClose={() => setParams({})}
          onChanged={() => void load()}
        />
      )}

      {/* TEMP-HIDE (Projects-hiding review): wipe confirmation for the
          hidden Recent updates section (backend refuses with 410 anyway).
      {confirmWipe && (
        <ConfirmModal
          title="Clear update history?"
          body="This deletes every recorded update. Projects, services and secrets are untouched. This cannot be undone."
          confirmLabel="Clear history"
          requireText="WIPE"
          onClose={() => setConfirmWipe(false)}
          onConfirm={async () => {
            try {
              const r = await api.wipeDeployments();
              pushToast(`History wiped. ${r.deleted} entries cleared.`);
              setConfirmWipe(false);
              void load();
            } catch (e) {
              pushToast(e instanceof Error ? e.message : "Wipe failed", true);
            }
          }}
        />
      )}
      */}
      </div>
    </main>
  );
}

const DOT_TONE: Record<FleetStatus, string> = {
  sailing: "bg-moss",
  choppy: "bg-status-amber",
  lost: "bg-brick",
  docked: "bg-stone",
};



export function ShipCard({
  project,
  services,
  last,
  onOpen,
  selection,
  servicesUnknown,
}: {
  project: Project;
  services: Service[];
  last: Deployment | undefined;
  onOpen: () => void;
  selection?: { checked: boolean; onToggle: () => void };
  /** True when the services fetch for this project failed: the card must
      say so instead of reporting "no services". */
  servicesUnknown?: boolean;
}) {
  const f = toFleetStatus(project.status);
  return (
    <div
      className="bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-5 transition-all duration-150 cursor-pointer hover:border-accent dark:hover:border-ember shadow-sm hover:shadow-chrome"
      onClick={onOpen}
    >
      <div className="flex items-center justify-between gap-4">
        <div className="flex items-center gap-2 flex-wrap">
          {selection && (
            <input
              type="checkbox"
              checked={selection.checked}
              onChange={selection.onToggle}
              onClick={(e) => e.stopPropagation()}
              aria-label={`Select ${project.name}`}
              className="w-4 h-4 accent-accent dark:accent-ember cursor-pointer rounded"
            />
          )}
          <span className={`w-2 h-2 rounded-full ${DOT_TONE[f]}`} />
          <span className="font-head font-bold text-[17px] text-ink dark:text-bone">{project.name}</span>
        </div>
        {project.environment && (
          <span className="inline-flex items-center gap-1.5 font-mono text-[11px] bg-paper dark:bg-emboss border border-line dark:border-edge text-ink dark:text-bone rounded-md px-2 py-[3px] whitespace-nowrap">
            v·{project.environment}
          </span>
        )}
      </div>
      <div className="font-mono text-muted dark:text-fog text-[11px] mt-1 truncate">
        ~/{project.repository || project.name}
      </div>
      <div className="flex items-center gap-2 flex-wrap mt-2.5">
        {servicesUnknown ? (
          <span className="text-status-amber text-xs font-medium">Services couldn't load — open the project to retry.</span>
        ) : (
          <>
            {services.length === 0 && <span className="text-muted dark:text-fog text-xs">No services yet</span>}
        {services.slice(0, 4).map((s) => (
          <span
            key={s.id}
            className="inline-flex items-center gap-1.5 font-mono text-[11px] bg-paper dark:bg-emboss border border-line dark:border-edge text-ink dark:text-bone rounded-md px-2 py-[3px] whitespace-nowrap"
          >
            <span className={`w-1.5 h-1.5 rounded-full ${DOT_TONE[toFleetStatus(s.status)]}`} />
            {s.name}
          </span>
        ))}
        {services.length > 4 && !servicesUnknown && (
          <span className="inline-flex items-center gap-1.5 font-mono text-[11px] bg-paper dark:bg-emboss border border-line dark:border-edge text-ink dark:text-bone rounded-md px-2 py-[3px] whitespace-nowrap">
            +{services.length - 4}
          </span>
        )}
          </>
        )}
      </div>
      <div className="bg-paper dark:bg-abyss border border-line dark:border-edge rounded-xl px-4 py-3.5 flex items-center gap-2 flex-wrap justify-between mt-3">
        <div className="flex items-center gap-4 flex-wrap">
          <div>
            <Kicker>Stations</Kicker>
            <div className="font-mono text-[13px] text-ink dark:text-bone font-medium">{servicesUnknown ? "—" : services.length}</div>
          </div>
          <div>
            <Kicker>Last launch</Kicker>
            <div className="font-mono text-[13px] text-ink dark:text-bone font-medium">
              {last ? `${timeAgo(last.startedAt)}` : "—"}
            </div>
          </div>
        </div>
        {last?.commitSha && (
          <span className="inline-flex items-center gap-1.5 font-mono text-[11px] bg-white dark:bg-panel border border-line dark:border-edge text-ink dark:text-bone rounded-md px-2 py-[3px] whitespace-nowrap shadow-sm">
            <Hash size={11} /> {shortSha(last.commitSha)}
          </span>
        )}
      </div>
      <div className="flex items-center justify-between gap-4 mt-3">
        <span className="text-muted dark:text-fog text-xs">
          {toBusinessStatus(f).label}
        </span>
        <StatusPill status={project.status} label={fleetLabel[f]} />
      </div>
    </div>
  );
}
