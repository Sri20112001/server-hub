import { client } from "./client";

export interface MonitoringOverview {
  available: boolean;
  cpu: number | null;
  memory: number | null;
  disk: number | null;
  networkRx: number | null;
  networkTx: number | null;
}

export interface PrometheusStatus {
  available: boolean;
  healthy?: boolean;
  status: string;
}

export interface AlertmanagerStatus {
  available: boolean;
  healthy?: boolean;
  status: string;
}

export interface AmAlert {
  fingerprint: string;
  status: {
    state: "active" | "suppressed" | "unprocessed";
    silencedBy: string[];
    inhibitedBy: string[];
  };
  labels: Record<string, string>;
  annotations: Record<string, string>;
  startsAt: string;
  endsAt: string;
  generatorURL: string;
  receivers: { name: string }[];
}

export interface AmSilence {
  id: string;
  status: { state: "active" | "expired" | "pending" };
  updatedAt: string;
  comment: string;
  createdBy: string;
  startsAt: string;
  endsAt: string;
  matchers: { name: string; value: string; isRegex: boolean; isEqual: boolean }[];
}

export const monitoringApi = {
  overview: () =>
    client.get<MonitoringOverview>("/server-hub/api/monitoring/overview").then((r) => r.data),

  prometheusStatus: () =>
    client.get<PrometheusStatus>("/server-hub/api/monitoring/prometheus/status").then((r) => r.data),

  alertmanagerStatus: () =>
    client.get<AlertmanagerStatus>("/server-hub/api/monitoring/alertmanager/status").then((r) => r.data),

  alerts: () =>
    client.get<AmAlert[]>("/server-hub/api/monitoring/alerts").then((r) => r.data),

  silences: () =>
    client.get<AmSilence[]>("/server-hub/api/monitoring/silences").then((r) => r.data),

  createSilence: (body: {
    matchers: { name: string; value: string; isRegex: boolean; isEqual: boolean }[];
    startsAt: string;
    endsAt: string;
    createdBy: string;
    comment: string;
  }) =>
    client.post<{ silenceID: string }>("/server-hub/api/monitoring/silences", body).then((r) => r.data),

  deleteSilence: (id: string) =>
    client.delete<{ ok: boolean }>(`/server-hub/api/monitoring/silences/${encodeURIComponent(id)}`).then((r) => r.data),
};
