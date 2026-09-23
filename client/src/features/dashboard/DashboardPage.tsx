import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useNavigate, useSearchParams } from "react-router";
import {
  Activity,
  ArrowRight,
  Clock,
  GitBranch,
  Hash,
  Plus,
  Radar,
  Radio,
  RefreshCw,
  Rocket,
  Ship,
} from "lucide-react";
import { api, ApiError } from "../../lib/api";
import {
  fleetLabel,
  toFleetStatus,
  type DashboardData,
  type Deployment,
  type DiscoveryResult,
  type FleetStatus,
  type Project,
  type Service,
} from "../../lib/types";
import {
  fmtBytes,
  fmtDuration,
  greeting,
  shortId,
  shortSha,
  timeAgo,
  todayLong,
} from "../../lib/format";
import { useAuth, useUi } from "../../stores/store";
import { useEvents } from "../../lib/useEvents";
import type { BusEvent } from "../../lib/types";
import { ConfirmModal, EmptyState, Field, Kicker, Meter, Modal, StatusPill, DashboardSkeleton } from "../../components/ui";
import { ProjectDrawer } from "./ProjectDrawer";
import { DiscoveryModal } from "./DiscoveryModal";
import { TelemetryModal } from "./TelemetryModal";

/* ---------- small pieces ---------- */

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
      pushToast(`“${name.trim()}” joined the fleet.`);
      onCreated();
      onClose();
    } catch (e2) {
      setErr(e2 instanceof Error ? e2.message : "Could not register ship");
      setBusy(false);
    }
  };

  return (
    <Modal onClose={onClose}>
      <h3 className="text-[18px] mb-1">New dispatch</h3>
      <p className="text-muted dark:text-fog text-[13px] mb-4">
        Register a vessel with the bridge. Runtime details can be charted afterwards.
      </p>
      <form onSubmit={submit} className="flex flex-col gap-4">
        <Field label="Ship name">
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
            {busy ? "Dispatching…" : "Dispatch ship"}
          </button>
        </div>
      </form>
    </Modal>
  );
}

/* ---------- page ---------- */

// Fleet preview stays compact; the full registry lives on the Fleet page.
export const FLEET_PREVIEW_LIMIT = 6;

export function DashboardPage({ setOnline }: { setOnline: (v: boolean) => void }) {
  const { user } = useAuth();
  const pushToast = useUi((s) => s.pushToast);
  const [params, setParams] = useSearchParams();
  const nav = useNavigate();
  const [data, setData] = useState<DashboardData | null>(null);
  const [servicesByProject, setServicesByProject] = useState<Record<number, Service[]>>({});
  const [feed, setFeed] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [showCreate, setShowCreate] = useState(false);
  const [showScan, setShowScan] = useState(false);
  const [showTelemetry, setShowTelemetry] = useState(false);
  const [confirmWipe, setConfirmWipe] = useState(false);
  const prevHealth = useRef<Map<number, string> | null>(null);
  const prevDepIds = useRef<Set<number> | null>(null);
  const prevHot = useRef(false);

  const load = useCallback(async () => {
    try {
      const [dash, audit] = await Promise.all([api.dashboard(), api.audit(9)]);
      setData(dash);
      setOnline(true);
      setFeed(
        audit.map(
          (a) => `[${(a.timestamp ?? "").slice(11, 19) || "--:--:--"}] ${a.actor} ${a.action} ${a.resource}${a.resourceId ? ` #${a.resourceId}` : ""} → ${a.result}`,
        ),
      );
      notifyTransitions(dash);
      // service chips per ship (best-effort, personal scale)
      const entries = await Promise.all(
        dash.projects.map(async (p) => {
          try {
            const s = await api.services(p.id);
            return [p.id, s] as const;
          } catch {
            return [p.id, []] as const;
          }
        }),
      );
      setServicesByProject(Object.fromEntries(entries));
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) {
        setOnline(true);
      } else {
        setOnline(false);
      }
    } finally {
      setLoading(false);
    }
  }, [setOnline]);

  // Auto-sync on app load: scan the shipyard once and import everything
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
          `${res.imported} ship${res.imported === 1 ? "" : "s"} auto-registered with ${res.servicesAdded} station${res.servicesAdded === 1 ? "" : "s"}.`,
        );
        await load();
      }
    } catch {
      // Offline, logged out, or Docker unreachable — stay silent;
      // manual Scan and New Ship keep working.
    }
  }, [load, pushToast]);

  useEffect(() => {
    void load();
    void autoSync();
    const t = window.setInterval(() => void load(), 30000);
    return () => window.clearInterval(t);
  }, [load, autoSync]);

  // Live signals: refresh the workbench and toast on deployments,
  // container/project lifecycle, gateway reloads, backups and threshold
  // crossings. Health transitions keep coming through the poll alerts
  // above to respect the Settings toggles without double-notifying.
  useEvents((ev: BusEvent) => {
    const d = (ev.data ?? {}) as Record<string, unknown>;
    const str = (v: unknown) => (typeof v === "string" ? v : "");
    switch (ev.type) {
      case "deployment.completed":
        pushToast(
          `Launch complete: ${str(d.project)} @ ${shortSha(str(d.commit))} (${str(d.durationSec)}s).`,
        );
        void load();
        break;
      case "deployment.failed":
        pushToast(`Launch failed: ${str(d.project)} @ ${shortSha(str(d.commit))}.`, true);
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
          pushToast(`Ship ${str(d.action)}: ${str(d.project)}.`);
          void load();
        } else if (str(d.detail)) {
          pushToast(`Ship ${str(d.action)} failed: ${str(d.detail)}.`, true);
        }
        break;
      case "gateway.reloaded":
        pushToast(d.ok ? "Gateway reloaded." : `Gateway reload failed: ${str(d.detail)}.`, !d.ok);
        break;
      case "discovery.completed":
        pushToast(`“${str(d.project)}” joined the fleet.`);
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
      setShowScan(true);
      setParams({}, { replace: true });
    }
  }, [params, setParams]);

  // Browser alerts for health transitions / fresh successful launches /
  // resource pressure, gated by Settings → Alerts toggles. First poll only
  // arms the baselines so it never spams on page load.
  const notifyTransitions = (dash: DashboardData) => {
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
      pushToast("Decks hot — CPU or RAM above 85%.", true);
    }
    prevHot.current = hot;
  };

  const projectById = useMemo(() => {
    const m = new Map<number, Project>();
    data?.projects.forEach((p) => m.set(p.id, p));
    return m;
  }, [data]);

  const selectedId = params.get("project");
  const selected = selectedId ? (projectById.get(Number(selectedId)) ?? null) : null;

  const recentByProject = useMemo(() => {
    const m = new Map<number, Deployment>();
    data?.recentDeployments.forEach((d) => {
      if (!m.has(d.projectId)) m.set(d.projectId, d);
    });
    return m;
  }, [data]);

  const deployStats = useMemo(() => {
    const list = data?.recentDeployments ?? [];
    const withDur = list.filter((d) => d.durationSec !== undefined && d.durationSec !== null);
    const avg = withDur.length
      ? Math.round(withDur.reduce((a, d) => a + (d.durationSec ?? 0), 0) / withDur.length)
      : undefined;
    const ok = list.filter((d) => d.status === "SUCCESS").length;
    const rate = list.length ? Math.round((ok / list.length) * 100) : 100;
    const today = new Date().toISOString().slice(0, 10);
    const todayCount = list.filter((d) => (d.startedAt ?? "").startsWith(today)).length;
    return { avg, rate, todayCount };
  }, [data]);

  const healthLine = useMemo(() => {
    if (!data) return "Reading the waters…";
    const { healthy, degraded, down } = data.counts;
    if (down > 0) return `${down} ship${down === 1 ? "" : "s"} flashing distress`;
    if (degraded > 0) return "Choppy waters on one or more ships";
    if (healthy > 0) return "All systems normal";
    return "No ships reporting yet";
  }, [data]);

  if (loading || !data) {
    return <DashboardSkeleton />;
  }

  const docked = data.projects.filter((p) => toFleetStatus(p.status) === "docked").length;

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
            {greeting()}, {user?.username ?? "Captain"}.
          </h1>
          <p className="text-muted dark:text-fog mt-1.5">
            {todayLong()} <span className="mx-1.5">•</span> {healthLine}
          </p>
        </div>
        <div className="flex items-center gap-2 flex-wrap">
          <button
            className="inline-flex items-center gap-2 rounded-input text-[13px] font-medium px-4 py-2 cursor-pointer border border-transparent whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-white dark:bg-panel border-line dark:border-edge text-ink dark:text-bone hover:bg-paper dark:hover:bg-emboss"
            onClick={() => void load()}
          >
            <RefreshCw size={14} /> Run Diagnostics
          </button>
          <button
            className="inline-flex items-center gap-2 rounded-input text-[13px] font-medium px-4 py-2 cursor-pointer border border-transparent whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-accent dark:bg-ember text-white dark:text-black hover:bg-accent-hover dark:hover:bg-ember-hover"
            onClick={() => setShowCreate(true)}
          >
            <Plus size={14} /> New Dispatch
          </button>
        </div>
      </div>

      {/* stat tiles */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4 mt-5">
        <StatShell
          kicker="Server load"
          icon={<Activity size={15} />}
          foot={<span>{data.dockerAvailable ? "Shipyard connected" : "Shipyard unreachable"}</span>}
          onClick={() => setShowTelemetry(true)}
        >
          <Meter label="CPU" pct={data.server.cpuPercent} display={`${Math.round(data.server.cpuPercent)}%`} />
          <Meter label="RAM" pct={data.server.memPercent} display={`${Math.round(data.server.memPercent)}%`} />
          <Meter label="Disk" pct={data.server.diskPercent} display={`${Math.round(data.server.diskPercent)}%`} />
        </StatShell>

        <StatShell
          kicker="Fleet"
          icon={<Ship size={15} />}
          foot={<span>{data.counts.services} stations crewed</span>}
        >
          <div className="flex items-center gap-2 flex-wrap items-baseline mt-2">
            <span className="font-head text-[34px] font-bold">{data.counts.projects}</span>
            <span className="text-muted dark:text-fog">ships active</span>
          </div>
          <div className="text-muted dark:text-fog text-xs mt-1.5">
            <span className="font-mono">{docked}</span> idle in shipyard
          </div>
        </StatShell>

        <StatShell
          kicker="Deploys"
          icon={<Rocket size={15} />}
          foot={
            <>
              <span>Avg duration: {deployStats.avg !== undefined ? fmtDuration(deployStats.avg) : "—"}</span>
              <Clock size={13} />
            </>
          }
        >
          <div className="flex items-center gap-2 flex-wrap items-baseline mt-2">
            <span className="font-head text-[34px] font-bold">{data.counts.deployments}</span>
            <span className="text-muted dark:text-fog">launches logged</span>
          </div>
          <div className="text-muted dark:text-fog text-xs mt-1.5">
            <span className="font-mono">{deployStats.todayCount}</span> today ·{" "}
            <span className="font-mono">{deployStats.rate}%</span> success rate
          </div>
        </StatShell>

        <StatShell
          kicker="Fleet health"
          icon={<Radio size={15} />}
          foot={<span>Heartbeat: 30s</span>}
        >
          <div className="flex flex-col gap-2 mt-2">
            <HealthRow label="Sailing" count={data.counts.healthy} tag="NOMINAL" pip="bg-moss" />
            <HealthRow label="Choppy" count={data.counts.degraded} tag={data.counts.degraded ? "WATCH" : "CALM"} pip="bg-status-amber" />
            <HealthRow label="Lost signal" count={data.counts.down} tag={data.counts.down ? "ALERT" : "NONE"} pip="bg-brick" />
          </div>
        </StatShell>
      </div>

      {/* fleet */}
      <div className="flex items-center justify-between gap-4 mt-8 mb-4">
        <div className="flex items-center gap-2 flex-wrap">
          <h2 className="font-head text-[22px] font-bold tracking-[-0.02em]">Fleet</h2>
          <span className="inline-flex items-center gap-1.5 font-mono text-[11px] bg-tint dark:bg-emboss border border-line dark:border-edge text-ink dark:text-bone rounded-md px-2 py-[3px] whitespace-nowrap">
            {data.projects.length} Active Units
          </span>
        </div>
        <div className="flex items-center gap-2 flex-wrap">
          <button
            className="inline-flex items-center gap-2 rounded-input font-medium cursor-pointer border border-transparent whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-white dark:bg-panel border-line dark:border-edge text-ink dark:text-bone hover:bg-paper dark:hover:bg-emboss px-3 py-1.5 text-xs"
            onClick={() => setShowScan(true)}
          >
            <Radar size={13} /> Scan
          </button>
          <button
            className="inline-flex items-center gap-2 rounded-input font-medium cursor-pointer border border-transparent whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-white dark:bg-panel border-line dark:border-edge text-ink dark:text-bone hover:bg-paper dark:hover:bg-emboss px-3 py-1.5 text-xs"
            onClick={() => setShowCreate(true)}
          >
            <Plus size={13} /> New Ship
          </button>
        </div>
      </div>
      {data.projects.length === 0 ? (
        <EmptyState
          title="No ships in the fleet yet"
          hint="Dispatch your first project to see it charted here."
        />
      ) : (
        <>
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
            {data.projects.slice(0, FLEET_PREVIEW_LIMIT).map((p) => (
              <ShipCard
                key={p.id}
                project={p}
                services={servicesByProject[p.id] ?? []}
                last={recentByProject.get(p.id)}
                onOpen={() => setParams({ project: String(p.id) })}
              />
            ))}
          </div>
          {data.projects.length > FLEET_PREVIEW_LIMIT && (
            <div className="flex justify-center mt-5">
              <button
                className="inline-flex items-center gap-2 rounded-input font-medium cursor-pointer border whitespace-nowrap transition-colors duration-150 bg-white dark:bg-panel border-line dark:border-edge text-ink dark:text-bone hover:bg-paper dark:hover:bg-emboss px-4 py-2 text-[13px]"
                onClick={() => nav("/fleet")}
              >
                View all {data.projects.length} ships <ArrowRight size={14} />
              </button>
            </div>
          )}
        </>
      )}

      {/* recent launches */}
      <div className="flex items-center justify-between gap-4 mt-8 mb-4">
        <div>
          <Kicker>Deployment activity</Kicker>
          <h2 className="font-head text-[22px] font-bold tracking-[-0.02em] mt-1">Recent launches</h2>
        </div>
      </div>
      <div className="bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-0 overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full border-collapse text-[13px]">
            <thead>
              <tr>
                <th className="text-left text-[11px] font-semibold tracking-[0.08em] uppercase text-muted dark:text-fog px-4 py-2.5 bg-paper dark:bg-abyss first:rounded-l-[10px] last:rounded-r-[10px] last:text-right">Commit SHA</th>
                <th className="text-left text-[11px] font-semibold tracking-[0.08em] uppercase text-muted dark:text-fog px-4 py-2.5 bg-paper dark:bg-abyss first:rounded-l-[10px] last:rounded-r-[10px] last:text-right">Target / Vessel</th>
                <th className="text-left text-[11px] font-semibold tracking-[0.08em] uppercase text-muted dark:text-fog px-4 py-2.5 bg-paper dark:bg-abyss first:rounded-l-[10px] last:rounded-r-[10px] last:text-right">Branch</th>
                <th className="text-left text-[11px] font-semibold tracking-[0.08em] uppercase text-muted dark:text-fog px-4 py-2.5 bg-paper dark:bg-abyss first:rounded-l-[10px] last:rounded-r-[10px] last:text-right">Duration</th>
                <th className="text-left text-[11px] font-semibold tracking-[0.08em] uppercase text-muted dark:text-fog px-4 py-2.5 bg-paper dark:bg-abyss first:rounded-l-[10px] last:rounded-r-[10px] last:text-right">Timestamp</th>
                <th className="text-left text-[11px] font-semibold tracking-[0.08em] uppercase text-muted dark:text-fog px-4 py-2.5 bg-paper dark:bg-abyss first:rounded-l-[10px] last:rounded-r-[10px] last:text-right">Status</th>
              </tr>
            </thead>
            <tbody>
              {data.recentDeployments.length === 0 && (
                <tr className="group">
                  <td colSpan={6} className="px-4 py-3 border-t border-line dark:border-edge align-middle text-center text-muted dark:text-fog">
                    No launches recorded yet.
                  </td>
                </tr>
              )}
              {data.recentDeployments.map((d) => (
                <tr key={d.id} className="group">
                  <td className="px-4 py-3 border-t border-line dark:border-edge align-middle last:text-right group-hover:bg-paper dark:group-hover:bg-emboss">
                    <span className="inline-flex items-center gap-1.5 font-mono text-[11px] bg-tint dark:bg-emboss border border-line dark:border-edge text-ink dark:text-bone rounded-md px-2 py-[3px] whitespace-nowrap">
                      <Hash size={11} /> {shortSha(d.commitSha)}
                    </span>
                  </td>
                  <td className="px-4 py-3 border-t border-line dark:border-edge align-middle last:text-right group-hover:bg-paper dark:group-hover:bg-emboss">
                    <div className="font-medium">{projectById.get(d.projectId)?.name ?? `#${d.projectId}`}</div>
                    <div className="font-mono text-muted dark:text-fog text-xs">{d.trigger ?? ""}</div>
                  </td>
                  <td className="px-4 py-3 border-t border-line dark:border-edge align-middle last:text-right group-hover:bg-paper dark:group-hover:bg-emboss">
                    <span className="inline-flex items-center gap-1.5 font-mono text-[11px] bg-tint dark:bg-emboss border border-line dark:border-edge text-ink dark:text-bone rounded-md px-2 py-[3px] whitespace-nowrap">
                      <GitBranch size={11} /> {d.branch || "—"}
                    </span>
                  </td>
                  <td className="px-4 py-3 border-t border-line dark:border-edge align-middle last:text-right group-hover:bg-paper dark:group-hover:bg-emboss font-mono">
                    {fmtDuration(d.durationSec)}
                  </td>
                  <td className="px-4 py-3 border-t border-line dark:border-edge align-middle last:text-right group-hover:bg-paper dark:group-hover:bg-emboss">
                    {timeAgo(d.startedAt)}
                  </td>
                  <td className="px-4 py-3 border-t border-line dark:border-edge align-middle last:text-right group-hover:bg-paper dark:group-hover:bg-emboss">
                    <StatusPill status={d.status} />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <div className="flex items-center gap-2 flex-wrap justify-between px-4 py-[0.7rem] border-t border-line dark:border-edge text-muted dark:text-fog text-xs">
          <span>
            Showing {data.recentDeployments.length} of {data.counts.deployments} launches logged
          </span>
          <button
            className="inline-flex items-center gap-2 rounded-input font-medium cursor-pointer border border-transparent whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-white dark:bg-panel border-line dark:border-edge text-ink dark:text-bone hover:bg-paper dark:hover:bg-emboss px-3 py-1.5 text-xs"
            onClick={() => setConfirmWipe(true)}
            title="Wipe deployment history"
          >
            Clear history
          </button>
        </div>
      </div>

      {/* live feed */}
      <div className="mt-8">
        <div className="flex items-center justify-between gap-4 mb-3">
          <div className="flex items-center gap-2 flex-wrap">
            <h2 className="font-head text-[18px] font-bold tracking-[-0.02em]">Sub-orbital Feed</h2>
            <span className="inline-flex items-center gap-1.5 font-mono text-[11px] bg-tint dark:bg-emboss border border-line dark:border-edge text-ink dark:text-bone rounded-md px-2 py-[3px] whitespace-nowrap">
              LIVE TTY
            </span>
          </div>
          <span className="inline-flex items-center gap-1.5 font-mono text-[11px] bg-tint dark:bg-emboss border border-line dark:border-edge text-ink dark:text-bone rounded-md px-2 py-[3px] whitespace-nowrap">
            AUDIT BUS
          </span>
        </div>
        <div className="font-mono text-xs leading-[1.7] bg-paper dark:bg-abyss border border-line dark:border-edge rounded-xl px-5 py-4 overflow-x-auto whitespace-pre-wrap break-words">
          {feed.length === 0 ? (
            <span className="text-muted dark:text-fog">— silence on the bus —</span>
          ) : (
            feed.map((line, i) => <div key={i}>{line}</div>)
          )}
        </div>
      </div>

      {showCreate && <CreateShipModal onClose={() => setShowCreate(false)} onCreated={() => void load()} />}

      {showScan && <DiscoveryModal onClose={() => setShowScan(false)} onImported={() => void load()} />}

      {showTelemetry && <TelemetryModal onClose={() => setShowTelemetry(false)} />}

      {selected && (
        <ProjectDrawer
          project={selected}
          onClose={() => setParams({})}
          onChanged={() => void load()}
        />
      )}

      {confirmWipe && (
        <ConfirmModal
          title="Wipe deployment history?"
          body="This deletes every recorded launch. Projects, services and secrets are untouched. This cannot be undone."
          confirmLabel="Wipe history"
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
}: {
  project: Project;
  services: Service[];
  last: Deployment | undefined;
  onOpen: () => void;
  selection?: { checked: boolean; onToggle: () => void };
}) {
  const f = toFleetStatus(project.status);
  return (
    <div
      className="bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-5 transition-colors duration-150 cursor-pointer hover:border-accent dark:hover:border-ember"
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
              className="w-4 h-4 accent-current cursor-pointer"
            />
          )}
          <span className={`w-2 h-2 rounded-full ${DOT_TONE[f]}`} />
          <span className="font-head font-bold text-[17px]">{project.name}</span>
        </div>
        {project.environment && (
          <span className="inline-flex items-center gap-1.5 font-mono text-[11px] bg-tint dark:bg-emboss border border-line dark:border-edge text-ink dark:text-bone rounded-md px-2 py-[3px] whitespace-nowrap">
            v·{project.environment}
          </span>
        )}
      </div>
      <div className="font-mono text-muted dark:text-fog text-[11px] mt-1">
        ~/{project.repository || project.name}
      </div>
      <div className="flex items-center gap-2 flex-wrap mt-2.5">
        {services.length === 0 && <span className="text-muted dark:text-fog text-xs">No stations charted</span>}
        {services.slice(0, 4).map((s) => (
          <span
            key={s.id}
            className="inline-flex items-center gap-1.5 font-mono text-[11px] bg-tint dark:bg-emboss border border-line dark:border-edge text-ink dark:text-bone rounded-md px-2 py-[3px] whitespace-nowrap"
          >
            <span className={`w-1.5 h-1.5 rounded-full ${DOT_TONE[toFleetStatus(s.status)]}`} />
            {s.name}
          </span>
        ))}
        {services.length > 4 && (
          <span className="inline-flex items-center gap-1.5 font-mono text-[11px] bg-tint dark:bg-emboss border border-line dark:border-edge text-ink dark:text-bone rounded-md px-2 py-[3px] whitespace-nowrap">
            +{services.length - 4}
          </span>
        )}
      </div>
      <div className="bg-paper dark:bg-abyss border border-line dark:border-edge rounded-xl px-4 py-3.5 flex items-center gap-2 flex-wrap justify-between mt-3">
        <div className="flex items-center gap-4 flex-wrap">
          <div>
            <Kicker>Stations</Kicker>
            <div className="font-mono text-[13px]">{services.length}</div>
          </div>
          <div>
            <Kicker>Last launch</Kicker>
            <div className="font-mono text-[13px]">
              {last ? `${timeAgo(last.startedAt)}` : "—"}
            </div>
          </div>
        </div>
        {last?.commitSha && (
          <span className="inline-flex items-center gap-1.5 font-mono text-[11px] bg-tint dark:bg-emboss border border-line dark:border-edge text-ink dark:text-bone rounded-md px-2 py-[3px] whitespace-nowrap">
            <Hash size={11} /> {shortSha(last.commitSha)}
          </span>
        )}
      </div>
      <div className="flex items-center justify-between gap-4 mt-3">
        <span className="text-muted dark:text-fog text-xs">
          {f === "sailing" ? "Ready to roll" : f === "choppy" ? "Throttle detected" : f === "lost" ? "Distress call" : "Standing by"}
        </span>
        <StatusPill status={project.status} label={fleetLabel[f]} />
      </div>
    </div>
  );
}
