import { client } from "./client";
import {
  alertResolve,
  alerts,
  notificationRead,
  notifications,
  notificationsReadAll,
  serverById,
  serverGroups,
  serverMetrics,
  serverMetricsLatest,
  servers,
} from "@serverhub/shared";
import type {
  Alert,
  InAppNotification,
  ManagedServer,
  ServerGroup,
  ServerMetricPoint,
} from "@serverhub/shared";

export const serversApi = {
  list: () =>
    client.get<ManagedServer[]>(servers()).then((r) => r.data),

  get: (id: number) =>
    client.get<ManagedServer>(serverById(id)).then((r) => r.data),

  metricsLatest: (id: number) =>
    client.get<ServerMetricPoint>(serverMetricsLatest(id)).then((r) => r.data),

  metrics: (id: number, range = "1h") =>
    client
      .get<{ range: string; points: ServerMetricPoint[] }>(serverMetrics(id, range))
      .then((r) => r.data),

  groups: () =>
    client.get<ServerGroup[]>(serverGroups()).then((r) => r.data),
};

export const alertsApi = {
  list: (status?: string) =>
    client
      .get<Alert[]>(alerts(status))
      .then((r) => r.data),

  resolve: (id: number) =>
    client.patch<{ ok: boolean }>(alertResolve(id)).then((r) => r.data),
};

export const notificationsApi = {
  list: () =>
    client.get<InAppNotification[]>(notifications()).then((r) => r.data),

  markRead: (id: number) =>
    client.patch<{ ok: boolean }>(notificationRead(id)).then((r) => r.data),

  markAllRead: () =>
    client.post<{ ok: boolean }>(notificationsReadAll()).then((r) => r.data),
};
