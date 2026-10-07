import { client } from "./client";
import { audit, dashboard, server } from "@serverhub/shared";
import type { DashboardData, ServerInfo, AuditLog } from "@serverhub/shared";

export const dashboardApi = {
  dashboard: () =>
    client.get<DashboardData>(dashboard()).then((r) => r.data),

  server: () =>
    client.get<ServerInfo>(server()).then((r) => r.data),

  audit: (limit = 8) =>
    client
      .get<AuditLog[]>(audit(limit))
      .then((r) => r.data),
};
