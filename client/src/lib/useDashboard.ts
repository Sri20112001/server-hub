import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError } from "./api";
import type { DashboardData, ManagedServer, Service } from "./types";

export interface FullDashboardState {
  dash: DashboardData;
  servers: ManagedServer[];
  servicesByProject: Record<number, Service[]>;
  feed: string[];
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
          api.servers().catch(() => []),
        ]);
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
        options?.setOnline?.(true);
        return {
          dash,
          servers: srvs,
          servicesByProject: Object.fromEntries(entries),
          feed: audit.map(
            (a) =>
              `[${(a.timestamp ?? "").slice(11, 19) || "--:--:--"}] ${a.actor} ${a.action} ${a.resource}${a.resourceId ? ` #${a.resourceId}` : ""} → ${a.result}`,
          ),
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
