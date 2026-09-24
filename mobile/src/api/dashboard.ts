import { client } from "./client";
import type { DashboardData, ServerInfo, AuditLog } from "../types";

export const dashboardApi = {
  dashboard: () =>
    client.get<DashboardData>("/server-hub/api/dashboard").then((r) => r.data),

  server: () =>
    client.get<ServerInfo>("/server-hub/api/server").then((r) => r.data),

  audit: (limit = 8) =>
    client
      .get<AuditLog[]>(`/server-hub/api/audit?limit=${limit}`)
      .then((r) => r.data),
};
