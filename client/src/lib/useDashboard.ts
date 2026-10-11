import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError } from "./api";
import type { DashboardData, ManagedServer, Service } from "./types";

export interface FullDashboardState {
  dash: DashboardData;
  servers: ManagedServer[];
  /** False when the servers fetch failed — `servers` is then stale/empty
      and must render as an error, never as "0 servers enrolled". */
  serversOk: boolean;
  serversError: string | null;
  servicesByProject: Record<number, Service[]>;
  /** Project ids whose services fetch failed — their cards must render a
      services-unavailable note, never "no services". */
  servicesFailedIds: number[];
  feed: string[];
  /** Wall-clock ms of the successful core fetch (freshness display). */
  fetchedAt: number;
}

function errMessage(e: unknown): string {
  if (e instanceof ApiError) return e.message;
  return e instanceof Error ? e.message : "Request failed";
}

export function useDashboardData(options?: { setOnline?: (v: boolean) => void }) {
  const qc = useQueryClient();
  const query = useQuery<FullDashboardState>({
    queryKey: ["dashboard-data"],
    queryFn: async () => {
      try {
        const [dash, audit, srvs] = await Promise.all([
          api.dashboard(),
          api.audit(9),
          api.servers().then(
            (s) => ({ ok: true as const, servers: s }),
            (e: unknown) => ({ ok: false as const, error: errMessage(e) }),
          ),
        ]);
        const entries = await Promise.all(
          dash.projects.map(async (p) => {
            try {
              const s = await api.services(p.id);
              return { id: p.id, services: s, failed: false as const };
            } catch {
              return { id: p.id, services: [] as Service[], failed: true as const };
            }
          }),
        );
        options?.setOnline?.(true);
        return {
          dash,
          servers: srvs.ok ? srvs.servers : [],
          serversOk: srvs.ok,
          serversError: srvs.ok ? null : srvs.error,
          servicesByProject: Object.fromEntries(entries.map((e) => [e.id, e.services])),
          servicesFailedIds: entries.filter((e) => e.failed).map((e) => e.id),
          feed: audit.map(
            (a) =>
              `[${(a.timestamp ?? "").slice(11, 19) || "--:--:--"}] ${a.actor} ${a.action} ${a.resource}${a.resourceId ? ` #${a.resourceId}` : ""} → ${a.result}`,
          ),
          fetchedAt: Date.now(),
        };
      } catch (e) {
        if (e instanceof ApiError && e.status === 401) {
          options?.setOnline?.(true);
        } else {
          options?.setOnline?.(false);
        }
        throw e;
      }
    },
    staleTime: 30_000,
    gcTime: 5 * 60_000,
    refetchInterval: 30_000,
  });

  const reload = () => qc.invalidateQueries({ queryKey: ["dashboard-data"] });

  return { ...query, reload };
}
