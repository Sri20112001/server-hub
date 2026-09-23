import { useCallback, useEffect, useMemo, useState } from "react";
import { useNavigate, useSearchParams } from "react-router";
import { ArrowLeft, Search } from "lucide-react";
import { api, ApiError } from "../../lib/api";
import type { DashboardData, Deployment, Project, Service } from "../../lib/types";
import { DashboardSkeleton, EmptyState, Kicker } from "../../components/ui";
import { ProjectDrawer } from "./ProjectDrawer";
import { ShipCard } from "./DashboardPage";

// Full fleet registry: every registered ship in one place with search.
// Cards open the same details drawer as the dashboard preview.
export function FleetPage({ setOnline }: { setOnline: (v: boolean) => void }) {
  const [params, setParams] = useSearchParams();
  const nav = useNavigate();
  const [data, setData] = useState<DashboardData | null>(null);
  const [servicesByProject, setServicesByProject] = useState<Record<number, Service[]>>({});
  const [loading, setLoading] = useState(true);
  const [q, setQ] = useState("");

  const load = useCallback(async () => {
    try {
      const dash = await api.dashboard();
      setData(dash);
      setOnline(true);
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

  useEffect(() => {
    void load();
  }, [load]);

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

  const filtered = useMemo(() => {
    const needle = q.trim().toLowerCase();
    if (!needle || !data) return data?.projects ?? [];
    return (data?.projects ?? []).filter(
      (p) =>
        p.name.toLowerCase().includes(needle) ||
        (p.repository ?? "").toLowerCase().includes(needle),
    );
  }, [data, q]);

  if (loading || !data) {
    return <DashboardSkeleton />;
  }

  return (
    <main className="w-full max-w-7xl mx-auto px-10 max-md:px-4 pt-24 pb-36">
      <div className="flex items-end justify-between gap-4 flex-wrap">
        <div>
          <button
            className="inline-flex items-center gap-1.5 bg-transparent border-0 cursor-pointer text-muted dark:text-fog hover:text-ink dark:hover:text-bone text-[13px] mb-2 px-0"
            onClick={() => nav("/")}
          >
            <ArrowLeft size={14} /> Dashboard
          </button>
          <h1 className="font-head text-[32px] font-bold tracking-[-0.03em] leading-[1.2] max-md:text-[26px]">
            Fleet registry
          </h1>
          <p className="text-muted dark:text-fog mt-1.5">
            <Kicker>
              {filtered.length} of {data.projects.length} ships
            </Kicker>
          </p>
        </div>
        <label className="flex items-center gap-2 bg-white dark:bg-panel border border-line dark:border-edge rounded-input px-3 py-2 text-[13px] text-muted dark:text-fog focus-within:border-accent dark:focus-within:border-ember">
          <Search size={14} />
          <input
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder="Filter ships…"
            className="bg-transparent outline-none text-ink dark:text-bone placeholder:text-muted dark:placeholder:text-fog w-44"
          />
        </label>
      </div>

      <div className="mt-6">
        {filtered.length === 0 ? (
          <EmptyState
            title={data.projects.length === 0 ? "No ships in the fleet yet" : "No ships match"}
            hint={
              data.projects.length === 0
                ? "New Docker finds auto-register on dashboard load."
                : "Try a different filter."
            }
          />
        ) : (
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
            {filtered.map((p) => (
              <ShipCard
                key={p.id}
                project={p}
                services={servicesByProject[p.id] ?? []}
                last={recentByProject.get(p.id)}
                onOpen={() => setParams({ project: String(p.id) })}
              />
            ))}
          </div>
        )}
      </div>

      {selected && (
        <ProjectDrawer
          project={selected}
          onClose={() => setParams({})}
          onChanged={() => void load()}
        />
      )}
    </main>
  );
}
