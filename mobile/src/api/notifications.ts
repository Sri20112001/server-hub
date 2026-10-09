import { client } from "./client";
import {
  maintenanceWindow,
  maintenanceWindows,
  notificationDeliveries,
  notificationPolicy,
  notifySettings,
  notifyTest,
} from "@serverhub/shared";
import type {
  DeliveryRecord,
  DeliveryPolicy,
  MaintenanceWindow,
} from "@serverhub/shared";

export interface NotifySettingsView {
  enabled: boolean;
  events: {
    deployFailed: boolean;
    threshold: boolean;
    backupFailed: boolean;
    projectFailed: boolean;
  };
  telegram: { enabled: boolean; chatId: string; hasToken: boolean };
  email: {
    enabled: boolean;
    host: string;
    port: string;
    username: string;
    from: string;
    to: string;
    tls: boolean;
    hasPassword: boolean;
    emailGroupId: number | null;
  };
}

export const notificationsApi = {
  settings: () =>
    client.get<NotifySettingsView>(notifySettings()).then((r) => r.data),

  updateSettings: (body: object) =>
    client.put<{ ok: boolean }>(notifySettings(), body).then((r) => r.data),

  test: () =>
    client
      .post<{ telegram: string; email: string }>(notifyTest(), {})
      .then((r) => r.data),

  policy: () =>
    client.get<DeliveryPolicy>(notificationPolicy()).then((r) => r.data),

  updatePolicy: (body: object) =>
    client.put<DeliveryPolicy>(notificationPolicy(), body).then((r) => r.data),

  deliveries: (params?: { status?: string; channel?: string; limit?: number }) =>
    client
      .get<DeliveryRecord[]>(notificationDeliveries(params))
      .then((r) => r.data),

  windows: () =>
    client.get<MaintenanceWindow[]>(maintenanceWindows()).then((r) => r.data),

  createWindow: (body: object) =>
    client
      .post<{ id: number; name: string }>(maintenanceWindows(), body)
      .then((r) => r.data),

  deleteWindow: (id: number) =>
    client.delete<{ ok: boolean }>(maintenanceWindow(id)).then((r) => r.data),
};
