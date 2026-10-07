import { client } from "./client";
import type { ManagedServer, ServerGroup, Alert, InAppNotification } from "../types";

export interface ServerMetricPoint {
  id: number;
  serverId: number;
  timestamp: string;
  cpuUsage: number;
  memoryUsage: number;
  memoryUsedMB: number;
  diskUsage: number;
  diskUsedGB: number;
  netRx: number;
  netTx: number;
  loadAvg1: number;
  uptimeSec: number;
}

export const serversApi = {
  list: () =>
    client.get<ManagedServer[]>("/server-hub/api/servers").then((r) => r.data),

  get: (id: number) =>
    client.get<ManagedServer>(`/server-hub/api/servers/${id}`).then((r) => r.data),

  metricsLatest: (id: number) =>
    client.get<ServerMetricPoint>(`/server-hub/api/servers/${id}/metrics/latest`).then((r) => r.data),

  metrics: (id: number, range = "1h") =>
    client
      .get<{ range: string; points: ServerMetricPoint[] }>(`/server-hub/api/servers/${id}/metrics?range=${range}`)
      .then((r) => r.data),

  groups: () =>
    client.get<ServerGroup[]>("/server-hub/api/server-groups").then((r) => r.data),
};

export const alertsApi = {
  list: (status?: string) =>
    client
      .get<Alert[]>(`/server-hub/api/alerts${status ? `?status=${status}` : ""}`)
      .then((r) => r.data),

  resolve: (id: number) =>
    client.patch<{ ok: boolean }>(`/server-hub/api/alerts/${id}/resolve`).then((r) => r.data),
};

export const notificationsApi = {
  list: () =>
    client.get<InAppNotification[]>("/server-hub/api/notifications").then((r) => r.data),

  markRead: (id: number) =>
    client.patch<{ ok: boolean }>(`/server-hub/api/notifications/${id}/read`).then((r) => r.data),

  markAllRead: () =>
    client.post<{ ok: boolean }>("/server-hub/api/notifications/read-all").then((r) => r.data),
};
