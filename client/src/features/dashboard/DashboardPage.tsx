import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useNavigate, useSearchParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import {
  Activity,
  AlertTriangle,
  ArrowRight,
  Hash,
  Radio,
  RefreshCw,
  Server,
} from "lucide-react";
import { api } from "../../lib/api";
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
  STALE_AFTER_MS,
  timeAgo,
  todayLong,
} from "../../lib/format";
import { useAuth, useUi } from "../../stores/store";
import { useEvents } from "../../lib/useEvents";
import type { BusEvent } from "../../lib/types";
import { Kicker, Meter, StatusPill, DashboardSkeleton, PageError } from "../../components/ui";
import { ProjectDrawer } from "./ProjectDrawer";
import { DiscoveryModal } from "./DiscoveryModal";
import { useDashboardData } from "../../lib/useDashboard";

/* ---------- small pieces ---------- */

function useNow(intervalMs = 5000) {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), intervalMs);
    return () => clearInterval(id);
  }, [intervalMs]);
  return now;
}

function StatShell({
  kicker,
  icon,
  children,
  foot,
  onClick,
}: {
  kicker: string;
  icon: React.ReactNode;
  children: React.ReactNode;
  foot?: React.ReactNode;
  onClick?: () => void;
}) {
  return (
    <div
      className={`bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-5${
        onClick ? " transition-colors duration-150 cursor-pointer hover:border-accent dark:hover:border-ember" : ""
      }`}
      onClick={onClick}
      title={onClick ? "Open telemetry timeline" : undefined}
      role={onClick ? "button" : undefined}
      tabIndex={onClick ? 0 : undefined}
      onKeyDown={
        onClick
          ? (e) => {
              if (e.key === "Enter" || e.key === " ") {
                e.preventDefault();
                onClick();
              }
            }
          : undefined
      }
    >
      <div className="flex items-start justify-between gap-3 mb-4">
        <Kicker>{kicker}</Kicker>
        <span className="text-muted dark:text-fog flex">{icon}</span>
      </div>
      {children}
      {foot && (
        <div className="flex items-center gap-2 flex-wrap justify-between mt-3 text-muted dark:text-fog text-xs">
          {foot}
        </div>
      )}
    </div>
  );
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
    <main className="w-full max-w-7xl mx-auto px-10 max-md:px-4 pt-24 pb-36">
      {/* masthead */}
      <div className="flex items-end justify-between gap-4 flex-wrap">
        <div>
          {/* <div className="flex items-center gap-2 flex-wrap text-accent-deep dark:text-ember mb-1.5">
            <span className="w-[7px] h-[7px] rounded-full bg-accent-deep dark:bg-ember" />
            <Kicker>Bridge console · Station 04</Kicker>
          </div> */}
          <h1 className="font-head text-[32px] capitalize font-bold tracking-[-0.03em] leading-[1.2] max-md:text-[26px]">
            {greeting()}, {user?.username ?? "there"}.
          </h1>
          <p className="text-muted dark:text-fog mt-1.5">
            {todayLong()} <span className="mx-1.5">•</span> {healthLine} <span className="mx-1.5">•</span> Updated {formatAge(now - (fullData?.fetchedAt ?? now))}
            {now - (fullData?.fetchedAt ?? now) > STALE_AFTER_MS && (
              <span className="ml-2 inline-flex items-center gap-1 font-mono text-[11px] bg-status-amber/10 border border-status-amber/30 text-status-amber rounded-md px-2 py-[3px] whitespace-nowrap">
                Data may be outdated
              </span>
            )}
          </p>
        </div>
        <div className="flex items-center gap-2 flex-wrap w-full sm:w-auto">
          <button
            className="inline-flex flex-1 sm:flex-none items-center justify-center gap-2 rounded-input text-[13px] font-medium px-4 py-2 cursor-pointer border whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-white dark:bg-panel border-line dark:border-edge text-ink dark:text-bone hover:bg-paper dark:hover:bg-emboss"
            onClick={() => void load()}
          >
            <RefreshCw size={14} /> Refresh
          </button>
          {/* TEMP-HIDE (Projects-hiding review): New project button disabled
              while the Projects section is hidden.
          <button
            className="inline-flex flex-1 sm:flex-none items-center justify-center gap-2 rounded-input text-[13px] font-medium px-4 py-2 cursor-pointer border border-transparent whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-accent dark:bg-ember text-white dark:text-black hover:bg-accent-hover dark:hover:bg-ember-hover shadow-sm"
            onClick={() => setShowCreate(true)}
          >
            <Plus size={14} /> New project
          </button>
          */}
        </div>
      </div>

      {/* stat tiles */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3 mt-5">
        <StatShell
          kicker="Server load"
          icon={<Activity size={15} />}
          foot={<span>{data.dockerAvailable ? "Container service connected" : "Container service unreachable"}</span>}
          onClick={() => nav("/telemetry")}
        >
          <Meter label="CPU" pct={data.server.cpuPercent} display={`${Math.round(data.server.cpuPercent)}%`} />
          <Meter label="RAM" pct={data.server.memPercent} display={`${Math.round(data.server.memPercent)}%`} />
          <Meter label="Disk" pct={data.server.diskPercent} display={`${Math.round(data.server.diskPercent)}%`} />
        </StatShell>

        <StatShell
          kicker="Infrastructure"
          icon={<Server size={15} />}
          foot={<span>Remote agents</span>}
          onClick={serversOk ? () => nav("/servers") : undefined}
        >
          {serversOk ? (
            <>
              <div className="flex items-center gap-2 flex-wrap items-baseline mt-2">
                <span className="font-head text-[34px] font-bold text-ink dark:text-bone">{servers.length}</span>
                <span className="text-muted dark:text-fog">servers enrolled</span>
              </div>
              <div className="text-muted dark:text-fog text-xs mt-1.5 flex items-center gap-2">
                <span className="inline-flex items-center gap-1 font-mono text-moss font-medium">
                  <span className="w-1.5 h-1.5 rounded-full bg-moss" />
                  {servers.filter((s) => s.status === "ONLINE").length} online
                </span>
                {servers.some((s) => s.status === "OFFLINE" || s.status === "WARNING") && (
                  <span className="inline-flex items-center gap-1 font-mono text-brick font-medium">
                    <span className="w-1.5 h-1.5 rounded-full bg-brick" />
                    {servers.filter((s) => s.status === "OFFLINE").length} offline
                  </span>
                )}
              </div>
            </>
          ) : (
            <div className="mt-2">
              <p className="text-status-amber text-xs font-medium">Server list couldn't load.</p>
              <p className="text-muted dark:text-fog text-xs mt-1 font-mono break-words">
                {serversError ?? "The server did not answer."}
              </p>
              <button
                onClick={() => void load()}
                className="mt-2 inline-flex items-center gap-1.5 px-3 py-1.5 text-xs border border-line dark:border-edge rounded-lg cursor-pointer text-muted dark:text-fog hover:text-ink dark:hover:text-bone"
              >
                <RefreshCw size={13} /> Retry
              </button>
            </div>
          )}
        </StatShell>

        {/* TEMP-HIDE (Projects-hiding review): Deployments tile disabled —
            totals over the project-versioning update log. The deployStats
            memo below is hidden with it; uncomment both to restore.
        <StatShell
          kicker="Deployments"
          icon={<Rocket size={15} />}
          foot={
            <>
              <span>Avg duration: {deployStats.avg !== undefined ? fmtDuration(deployStats.avg) : "—"}</span>
              <Clock size={13} />
            </>
          }
        >
          <div className="flex items-center gap-2 flex-wrap items-baseline mt-2">
            <span className="font-head text-[34px] font-bold text-ink dark:text-bone">{data.counts.deployments}</span>
            <span className="text-muted dark:text-fog">updates logged</span>
          </div>
          <div className="text-muted dark:text-fog text-xs mt-1.5">
            <span className="font-mono text-ink dark:text-bone font-medium">{deployStats.todayCount}</span> today ·{" "}
            <span className="font-mono text-ink dark:text-bone font-medium">{deployStats.rate}%</span> successful
          </div>
        </StatShell>
        END TEMP-HIDE */}

        <StatShell
          kicker="Service Health"
          icon={<Radio size={15} />}
          foot={<span>Heartbeat: 30s</span>}
        >
          <div className="flex flex-col gap-2 mt-2">
            <HealthRow label="Working" count={data.counts.healthy} tag="GOOD" pip="bg-moss" />
            <HealthRow label="Needs attention" count={data.counts.degraded} tag={data.counts.degraded ? "WATCH" : "GOOD"} pip="bg-status-amber" />
            <HealthRow label="Down" count={data.counts.down} tag={data.counts.down ? "DOWN" : "NONE"} pip="bg-brick" />
          </div>
        </StatShell>
      </div>

      {/* needs attention */}
      {attention.length > 0 && (
        <div className="mt-4 rounded-card border border-status-amber/30 bg-white dark:bg-panel shadow-sm overflow-hidden">
          <div className="flex items-center gap-2 px-5 pt-4 pb-2">
            <AlertTriangle size={15} className="text-status-amber" />
            <h2 className="font-head text-[16px] font-bold tracking-[-0.02em] text-ink dark:text-bone">
              Needs attention
            </h2>
            <span className="inline-flex items-center font-mono text-[11px] bg-status-amber/10 border border-status-amber/30 text-status-amber rounded-md px-2 py-[3px] whitespace-nowrap">
              {attention.length}
            </span>
          </div>
          <div className="px-5 pb-4">
            {attention.map((item) => (
              <button
                key={item.key}
                onClick={() => nav(item.route)}
                className="w-full flex items-center gap-3 py-2.5 border-b border-line dark:border-edge last:border-0 text-left cursor-pointer group"
              >
                <span
                  className={`w-2 h-2 rounded-full shrink-0 ${
                    item.tone === "down" ? "bg-brick" : item.tone === "attention" ? "bg-status-amber" : "bg-stone"
                  }`}
                />
                <span className="flex-1 min-w-0">
                  <span className="block text-sm font-medium text-ink dark:text-bone truncate">{item.title}</span>
                  <span className="block text-xs text-muted dark:text-fog truncate">{item.detail}</span>
                </span>
                <ArrowRight size={14} className="shrink-0 text-muted dark:text-fog group-hover:text-ink dark:group-hover:text-bone" />
              </button>
            ))}
          </div>
        </div>
      )}

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
      END TEMP-HIDE */}
    </main>
  );
}

const DOT_TONE: Record<FleetStatus, string> = {
  sailing: "bg-moss",
  choppy: "bg-status-amber",
  lost: "bg-brick",
  docked: "bg-stone",
};

function HealthRow({ label, count, tag, pip }: { label: string; count: number; tag: string; pip: string }) {
  return (
    <div className="flex items-center gap-2 flex-wrap justify-between">
      <span className="inline-flex items-center gap-1.5 h-6 px-2.5 rounded-full font-mono text-[11px] bg-paper dark:bg-abyss text-ink dark:text-bone whitespace-nowrap border-0">
        <span className={`w-1.5 h-1.5 rounded-full ${pip}`} /> {count} {label}
      </span>
      <span className="font-mono text-[11px] text-muted dark:text-fog">{tag}</span>
    </div>
  );
}

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
