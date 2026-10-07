import { client } from "./client";
import {
  alertmanagerStatus,
  monitoringAlerts,
  monitoringOverview,
  monitoringSilence,
  monitoringSilences,
  prometheusStatus,
} from "@serverhub/shared";
import type {
  AlertmanagerStatus,
  AmAlert,
  AmSilence,
  MonitoringOverview,
  PrometheusStatus,
} from "@serverhub/shared";

export const monitoringApi = {
  overview: () =>
    client.get<MonitoringOverview>(monitoringOverview()).then((r) => r.data),

  prometheusStatus: () =>
    client.get<PrometheusStatus>(prometheusStatus()).then((r) => r.data),

  alertmanagerStatus: () =>
    client.get<AlertmanagerStatus>(alertmanagerStatus()).then((r) => r.data),

  alerts: () =>
    client.get<AmAlert[]>(monitoringAlerts()).then((r) => r.data),

  silences: () =>
    client.get<AmSilence[]>(monitoringSilences()).then((r) => r.data),

  createSilence: (body: {
    matchers: { name: string; value: string; isRegex: boolean; isEqual: boolean }[];
    startsAt: string;
    endsAt: string;
    createdBy: string;
    comment: string;
  }) =>
    client.post<{ silenceID: string }>(monitoringSilences(), body).then((r) => r.data),

  deleteSilence: (id: string) =>
    client.delete<{ ok: boolean }>(monitoringSilence(id)).then((r) => r.data),
};
