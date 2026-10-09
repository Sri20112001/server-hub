import { client } from "./client";
import {
  alertmanagerStatus,
  monitoringAlerts,
  monitoringMetrics,
  monitoringOverview,
  monitoringSilence,
  monitoringSilences,
  prometheusRules,
  prometheusStatus,
  prometheusTargets,
} from "@serverhub/shared";
import type {
  AlertmanagerStatus,
  AmAlert,
  AmSilence,
  MonitoringMetricHistory,
  MonitoringOverview,
  PrometheusStatus,
  PromTargetsData,
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

  metricHistory: (metric: string, range = "1h", step = "60") =>
    client
      .get<MonitoringMetricHistory>(monitoringMetrics(metric, range, step))
      .then((r) => r.data),

  targets: () =>
    client.get<PromTargetsData>(prometheusTargets()).then((r) => r.data),

  rules: () =>
    client.get<unknown>(prometheusRules()).then((r) => r.data),

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
