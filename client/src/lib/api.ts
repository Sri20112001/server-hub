export const API_BASE = (import.meta.env.VITE_API_URL as string | undefined) ?? "http://localhost:4000";

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

// Single-flight session refresh: concurrent 401s share one rotation call.
let refreshPromise: Promise<boolean> | null = null;

function refreshSession(): Promise<boolean> {
  if (!refreshPromise) {
    refreshPromise = fetch(`${API_BASE}/server-hub/api/auth/refresh`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
    })
      .then((r) => r.ok)
      .catch(() => false)
      .finally(() => {
        refreshPromise = null;
      });
  }
  return refreshPromise;
}

async function req<T>(path: string, init?: RequestInit, retry = true): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, {
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    ...init,
  });
  // Access tokens live 30 minutes. On expiry, transparently rotate via the
  // refresh cookie and retry once (single-flight across concurrent calls).
  // Auth endpoints themselves never retry (a 401 there is a real failure).
  if (res.status === 401 && retry && !path.startsWith("/server-hub/api/auth/")) {
    const ok = await refreshSession();
    if (ok) return req<T>(path, init, false);
  }
  if (res.status === 204) return undefined as T;
  let body: unknown;
  try {
    body = await res.json();
  } catch {
    body = null;
  }
  if (!res.ok) {
    const msg =
      body && typeof body === "object" && "error" in body
        ? String((body as { error: unknown }).error)
        : `Request failed (${res.status})`;
    throw new ApiError(res.status, msg);
  }
  return body as T;
}

export const api = {
  // auth
  login: (username: string, password: string) =>
    req<{ username: string; role: string; token: string }>("/server-hub/api/auth/login", {
      method: "POST",
      body: JSON.stringify({ username, password }),
    }),
  logout: () => req<{ ok: boolean }>("/server-hub/api/auth/logout", { method: "POST" }),
  me: () => req<{ username: string; role: string }>("/server-hub/api/auth/me"),
  changePassword: (currentPassword: string, newPassword: string) =>
    req<{ ok: boolean }>("/server-hub/api/auth/password", {
      method: "PUT",
      body: JSON.stringify({ currentPassword, newPassword }),
    }),

  // dashboard / server
  dashboard: () => req<import("./types").DashboardData>("/server-hub/api/dashboard"),
  server: () => req<import("./types").ServerInfo>("/server-hub/api/server"),
  audit: (limit = 8) => req<import("./types").AuditLog[]>(`/server-hub/api/audit?limit=${limit}`),

  // projects
  projects: () => req<import("./types").Project[]>("/server-hub/api/projects"),
  project: (id: number) => req<import("./types").Project>(`/server-hub/api/projects/${id}`),
  createProject: (p: Partial<import("./types").Project>) =>
    req<import("./types").Project>("/server-hub/api/projects", { method: "POST", body: JSON.stringify(p) }),
  projectAction: (id: number, action: "start" | "stop" | "restart", confirm = false) =>
    req<{ ok: boolean; logs?: string }>(
      `/server-hub/api/projects/${id}/${action}${confirm ? "?confirm=true" : ""}`,
      { method: "POST" },
    ),
  // Bulk lifecycle: one action across many ships (stop/restart need confirm).
  bulkLifecycle: (ids: number[], action: "start" | "stop" | "restart", confirm = false) =>
    req<import("./types").BulkLifecycleResult>("/server-hub/api/projects/bulk-lifecycle", {
      method: "POST",
      body: JSON.stringify({ ids, action, confirm }),
    }),
  // Notification channels (Telegram / email) for failure + pressure signals.
  notifySettings: () =>
    req<import("./types").NotifySettings>("/server-hub/api/settings/notifications"),
  saveNotifySettings: (body: object) =>
    req<{ ok: boolean }>("/server-hub/api/settings/notifications", {
      method: "PUT",
      body: JSON.stringify(body),
    }),
  testNotify: () =>
    req<{ telegram: string; email: string }>("/server-hub/api/settings/notifications/test", {
      method: "POST",
    }),

  // services & deployments
  services: (projectId: number) =>
    req<import("./types").Service[]>(`/server-hub/api/projects/${projectId}/services`),
  deployments: (projectId: number) =>
    req<import("./types").Deployment[]>(`/server-hub/api/projects/${projectId}/deployments`),
  deploy: (projectId: number, commitSha?: string) =>
    req<{ deployment: import("./types").Deployment; operationId: string }>(
      `/server-hub/api/projects/${projectId}/deploy`,
      {
        method: "POST",
        body: JSON.stringify({ commitSha: commitSha ?? "" }),
      },
    ),
  wipeDeployments: () =>
    req<{ ok: boolean; deleted: number }>("/server-hub/api/deployments", { method: "DELETE" }),
  rollback: (projectId: number, depId: number) =>
    req<{ deployment: import("./types").Deployment; operationId: string }>(
      `/server-hub/api/projects/${projectId}/deployments/${depId}/rollback`,
      { method: "POST" },
    ),

  // shipyard discovery (auto-detect compose projects from Docker labels)
  discovery: () => req<import("./types").DiscoveryResult>("/server-hub/api/discovery"),
  importDiscovered: (name: string) =>
    req<import("./types").Project & { servicesAdded: number }>("/server-hub/api/discovery/import", {
      method: "POST",
      body: JSON.stringify({ name }),
    }),
  // Bulk import: registers every unregistered Docker + directory find at
  // once. Idempotent — already-registered ships are skipped server-side.
  importAllDiscovered: () =>
    req<import("./types").ImportAllResult>("/server-hub/api/discovery/import-all", {
      method: "POST",
    }),

  // secrets (metadata only; values only via reveal)
  secrets: (projectId: number) =>
    req<import("./types").SecretMeta[]>(`/server-hub/api/projects/${projectId}/secrets`),
  upsertSecret: (projectId: number, name: string, value: string, environment = "production") =>
    req<{ name: string; configured: boolean }>(`/server-hub/api/projects/${projectId}/secrets`, {
      method: "POST",
      body: JSON.stringify({ name, value, environment }),
    }),
  revealSecret: (id: number) =>
    req<{ name: string; value: string }>(`/server-hub/api/secrets/${id}/reveal`, { method: "POST" }),

  // telemetry history for the resource timeline
  telemetry: (range: string = "1h") =>
    req<import("./types").TelemetryHistory>(`/server-hub/api/telemetry?range=${range}`),

  // host inventory (detail, storage, per-container stats)
  serverDetail: () => req<import("./types").ServerDetail>("/server-hub/api/server/detail"),
  storage: () => req<import("./types").StorageInfo>("/server-hub/api/server/storage"),
  containerStats: () =>
    req<import("./types").ContainerStat[]>("/server-hub/api/containers/stats"),

  // operations (unified progress for deploys, restarts, backups …)
  operation: (id: string) => req<import("./types").Operation>(`/server-hub/api/operations/${id}`),
  operations: (limit = 20) =>
    req<import("./types").Operation[]>(`/server-hub/api/operations?limit=${limit}`),

  // container exec terminal (single-use token, then WebSocket)
  execToken: (container: string, shell = "/bin/sh", confirm = true) =>
    req<{ token: string; operationId: string; id: string }>(
      `/server-hub/api/containers/${encodeURIComponent(container)}/exec${
        confirm ? "?confirm=true" : ""
      }`,
      { method: "POST", body: JSON.stringify({ shell }) },
    ),

  // backups (directory snapshots + metadata)
  backups: (projectId: number) =>
    req<import("./types").Backup[]>(`/server-hub/api/projects/${projectId}/backups`),
  createBackup: (projectId: number) =>
    req<{ ok: boolean; operationId: string; projectId: number }>(
      `/server-hub/api/projects/${projectId}/backups`,
      { method: "POST" },
    ),
  deleteBackup: (id: number) =>
    req<{ ok: boolean }>(`/server-hub/api/backups/${id}`, { method: "DELETE" }),
  downloadBackupUrl: (id: number) =>
    `${API_BASE}/server-hub/api/backups/${id}/download`,
  restoreBackup: (id: number) =>
    req<{ ok: boolean; operationId: string; backupId: number }>(
      `/server-hub/api/backups/${id}/restore?confirm=true`,
      { method: "POST" },
    ),

  // databases: auto-detected servers (Docker, host ports, fleet services).
  // Browse lists what's inside; connect saves the credential; register
  // turns ticked databases into services under a project.
  dbServers: () =>
    req<{ servers: import("./types").DbServerInfo[]; dockerAvailable: boolean }>(
      "/server-hub/api/databases/servers",
    ),
  browseDatabases: (t: import("./types").DbTarget) =>
    req<{ databases: import("./types").DbItem[]; authRequired: boolean }>(
      "/server-hub/api/databases/browse",
      { method: "POST", body: JSON.stringify(t) },
    ),
  connectDatabase: (t: import("./types").DbTarget) =>
    req<{ ok: boolean; key: string; hasCreds: boolean; databases: number }>(
      "/server-hub/api/databases/connect",
      { method: "POST", body: JSON.stringify(t) },
    ),
  registerDatabases: (
    projectId: number,
    server: import("./types").DbTarget,
    databases: string[],
    username?: string,
    password?: string,
  ) =>
    req<{ ok: boolean; registered: { database: string; serviceId: number }[]; skipped: string[] }>(
      "/server-hub/api/databases/register",
      { method: "POST", body: JSON.stringify({ projectId, server, databases, username, password }) },
    ),

  // central activity log (single place for all logs; feeds future aggregator UI)
  logs: (params?: { level?: string; source?: string; search?: string; projectId?: number; limit?: number; offset?: number }) => {
    const q = new URLSearchParams();
    if (params?.level) q.set("level", params.level);
    if (params?.source) q.set("source", params.source);
    if (params?.search) q.set("search", params.search);
    if (params?.projectId !== undefined) q.set("projectId", String(params.projectId));
    if (params?.limit !== undefined) q.set("limit", String(params.limit));
    if (params?.offset !== undefined) q.set("offset", String(params.offset));
    const suffix = q.toString() ? `?${q.toString()}` : "";
    return req<import("./types").AppLog[]>(`/server-hub/api/logs${suffix}`);
  },

  // user + role management (admin only)
  users: () =>
    req<{ username: string; role: string; createdAt: string }[]>("/server-hub/api/users"),
  createUser: (username: string, password: string, role: string) =>
    req<{ username: string; role: string }>("/server-hub/api/users", {
      method: "POST",
      body: JSON.stringify({ username, password, role }),
    }),
  setUserRole: (username: string, role: string) =>
    req<{ username: string; role: string }>(`/server-hub/api/users/${encodeURIComponent(username)}/role`, {
      method: "PUT",
      body: JSON.stringify({ role }),
    }),
  deleteUser: (username: string) =>
    req<{ ok: boolean }>(`/server-hub/api/users/${encodeURIComponent(username)}`, {
      method: "DELETE",
    }),
  logoutAll: () =>
    req<{ ok: boolean }>("/server-hub/api/auth/logout-all", { method: "POST" }),

  // managed servers
  servers: () => req<import("./types").ManagedServer[]>("/server-hub/api/servers"),
  managedServer: (id: number) => req<import("./types").ManagedServer>(`/server-hub/api/servers/${id}`),
  createServer: (body: { name: string; hostname?: string; ipAddress?: string; groupId?: number }) =>
    req<{ id: number; name: string }>("/server-hub/api/servers", { method: "POST", body: JSON.stringify(body) }),
  updateServer: (id: number, body: Partial<{ name: string; hostname: string; ipAddress: string; groupId: number }>) =>
    req<{ ok: boolean }>(`/server-hub/api/servers/${id}`, { method: "PATCH", body: JSON.stringify(body) }),
  deleteServer: (id: number) =>
    req<{ ok: boolean }>(`/server-hub/api/servers/${id}`, { method: "DELETE" }),
  serverMetrics: (id: number, range = "1h") =>
    req<{ range: string; points: import("./types").ServerMetricPoint[] }>(`/server-hub/api/servers/${id}/metrics?range=${range}`),
  serverMetricsLatest: (id: number) =>
    req<import("./types").ServerMetricPoint>(`/server-hub/api/servers/${id}/metrics/latest`),

  // agent tokens
  agentTokens: (serverId: number) =>
    req<import("./types").AgentToken[]>(`/server-hub/api/servers/${serverId}/tokens`),
  createAgentToken: (serverId: number, label?: string) =>
    req<{ id: number; token: string; label: string }>(`/server-hub/api/servers/${serverId}/tokens`, {
      method: "POST", body: JSON.stringify({ label: label ?? "" }),
    }),
  revokeAgentToken: (serverId: number, tokenId: number) =>
    req<{ ok: boolean }>(`/server-hub/api/servers/${serverId}/tokens/${tokenId}`, { method: "DELETE" }),

  // server groups
  serverGroups: () => req<import("./types").ServerGroup[]>("/server-hub/api/server-groups"),
  createServerGroup: (name: string, description?: string) =>
    req<{ id: number; name: string }>("/server-hub/api/server-groups", {
      method: "POST", body: JSON.stringify({ name, description: description ?? "" }),
    }),
  deleteServerGroup: (id: number) =>
    req<{ ok: boolean }>(`/server-hub/api/server-groups/${id}`, { method: "DELETE" }),

  // alerts
  alerts: (status?: string) =>
    req<import("./types").Alert[]>(`/server-hub/api/alerts${status ? `?status=${status}` : ""}`),
  resolveAlert: (id: number) =>
    req<{ ok: boolean }>(`/server-hub/api/alerts/${id}/resolve`, { method: "PATCH" }),

  // in-app notifications
  notifications: () => req<import("./types").InAppNotification[]>("/server-hub/api/notifications"),
  markNotificationRead: (id: number) =>
    req<{ ok: boolean }>(`/server-hub/api/notifications/${id}/read`, { method: "PATCH" }),
  markAllNotificationsRead: () =>
    req<{ ok: boolean }>("/server-hub/api/notifications/read-all", { method: "POST" }),

  // health checks
  healthChecks: () => req<import("./types").HealthCheck[]>("/server-hub/api/health-checks"),
  createHealthCheck: (body: Partial<import("./types").HealthCheck>) =>
    req<{ id: number; name: string }>("/server-hub/api/health-checks", { method: "POST", body: JSON.stringify(body) }),
  updateHealthCheck: (id: number, body: Partial<import("./types").HealthCheck>) =>
    req<{ ok: boolean }>(`/server-hub/api/health-checks/${id}`, { method: "PATCH", body: JSON.stringify(body) }),
  deleteHealthCheck: (id: number) =>
    req<{ ok: boolean }>(`/server-hub/api/health-checks/${id}`, { method: "DELETE" }),
  healthCheckResults: (id: number) =>
    req<import("./types").HealthCheckResult[]>(`/server-hub/api/health-checks/${id}/results`),

  // monitoring — Prometheus + Alertmanager
  monitoringOverview: () =>
    req<import("./types").MonitoringOverview>("/server-hub/api/monitoring/overview"),
  prometheusStatus: () =>
    req<import("./types").PrometheusStatus>("/server-hub/api/monitoring/prometheus/status"),
  prometheusTargets: () =>
    req<{ data: import("./types").PromTargetsData }>("/server-hub/api/monitoring/prometheus/targets"),
  prometheusRules: () =>
    req<{ data: unknown }>("/server-hub/api/monitoring/prometheus/rules"),
  prometheusQuery: (query: string) =>
    req<{ data: unknown }>(`/server-hub/api/monitoring/prometheus/query?query=${encodeURIComponent(query)}`),
  prometheusQueryRange: (query: string, rangeStr: string, step = "60") => {
    const now = Math.floor(Date.now() / 1000);
    const durations: Record<string, number> = { "1h": 3600, "6h": 21600, "24h": 86400, "7d": 604800 };
    const dur = durations[rangeStr] ?? 3600;
    return req<{ data: import("./types").PromRangeResult }>(
      `/server-hub/api/monitoring/prometheus/query-range?query=${encodeURIComponent(query)}&start=${now - dur}&end=${now}&step=${step}`
    );
  },
  monitoringMetrics: (metric: string, range = "1h", step = "60") =>
    req<{ metric: string; range: string; data: import("./types").PromRangeResult }>(
      `/server-hub/api/monitoring/metrics?metric=${metric}&range=${range}&step=${step}`
    ),
  alertmanagerStatus: () =>
    req<import("./types").AlertmanagerStatus>("/server-hub/api/monitoring/alertmanager/status"),
  monitoringAlerts: () =>
    req<import("./types").AmAlert[]>("/server-hub/api/monitoring/alerts"),
  monitoringSilences: () =>
    req<import("./types").AmSilence[]>("/server-hub/api/monitoring/silences"),
  createSilence: (body: object) =>
    req<{ silenceID: string }>("/server-hub/api/monitoring/silences", {
      method: "POST",
      body: JSON.stringify(body),
    }),
  deleteSilence: (id: string) =>
    req<{ ok: boolean }>(`/server-hub/api/monitoring/silences/${encodeURIComponent(id)}`, { method: "DELETE" }),
};
