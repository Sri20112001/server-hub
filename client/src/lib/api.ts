import * as ep from "@serverhub/shared";
import type {
  AgentToken,
  Alert,
  AlertmanagerStatus,
  AmAlert,
  AmSilence,
  AppLog,
  AuditLog,
  Backup,
  BulkLifecycleResult,
  ContainerStat,
  DbItem,
  DbServerInfo,
  DbTarget,
  Deployment,
  DiscoveryResult,
  HealthCheck,
  HealthCheckResult,
  ImportAllResult,
  InAppNotification,
  LogsParams,
  ManagedServer,
  MonitoringOverview,
  NotificationGroup,
  NotificationGroupDetail,
  NotificationRule,
  NotificationRuleCreate,
  NotificationRuleUpdate,
  NotifySettings,
  Operation,
  Project,
  PromRangeResult,
  PromTargetsData,
  PrometheusStatus,
  SecretMeta,
  ServerDetail,
  ServerGroup,
  ServerInfo,
  ServerMetricPoint,
  Service,
  StorageInfo,
  TelemetryHistory,
  DashboardData,
} from "@serverhub/shared";

export const API_BASE = (import.meta.env.VITE_API_URL as string | undefined) ?? "http://localhost:4000";

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

// Phase 3C per-server Prometheus response shapes (web-only; backend builds
// the PromQL, the browser only supplies metric/range/step).
export interface ServerPromPoint {
  timestamp: number;
  value: number | null;
}

export interface ServerPromSeries {
  name: string;
  values: ServerPromPoint[];
}

export interface ServerPromMetrics {
  serverId: string;
  metric: string;
  range: string;
  step: string;
  series: ServerPromSeries[];
}

// Single-flight session refresh: concurrent 401s share one rotation call.
let refreshPromise: Promise<boolean> | null = null;

function refreshSession(): Promise<boolean> {
  if (!refreshPromise) {
    refreshPromise = fetch(`${API_BASE}${ep.authRefresh()}`, {
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
    req<{ username: string; role: string; token: string }>(ep.authLogin(), {
      method: "POST",
      body: JSON.stringify({ username, password }),
    }),
  logout: () => req<{ ok: boolean }>(ep.authLogout(), { method: "POST" }),
  me: () => req<{ username: string; role: string }>(ep.authMe()),
  changePassword: (currentPassword: string, newPassword: string) =>
    req<{ ok: boolean }>(ep.authPassword(), {
      method: "PUT",
      body: JSON.stringify({ currentPassword, newPassword }),
    }),

  // dashboard / server
  dashboard: () => req<DashboardData>(ep.dashboard()),
  server: () => req<ServerInfo>(ep.server()),
  audit: (limit = 8) => req<AuditLog[]>(ep.audit(limit)),

  // projects
  projects: () => req<Project[]>(ep.projects()),
  project: (id: number) => req<Project>(ep.project(id)),
  createProject: (p: Partial<Project>) =>
    req<Project>(ep.projects(), { method: "POST", body: JSON.stringify(p) }),
  projectAction: (id: number, action: "start" | "stop" | "restart", confirm = false) =>
    req<{ ok: boolean; logs?: string }>(
      ep.projectAction(id, action, confirm),
      { method: "POST" },
    ),
  // Bulk lifecycle: one action across many ships (stop/restart need confirm).
  bulkLifecycle: (ids: number[], action: "start" | "stop" | "restart", confirm = false) =>
    req<BulkLifecycleResult>(ep.bulkLifecycle(), {
      method: "POST",
      body: JSON.stringify({ ids, action, confirm }),
    }),
  // Notification channels (Telegram / email) for failure + pressure signals.
  notifySettings: () =>
    req<NotifySettings>(ep.notifySettings()),
  saveNotifySettings: (body: object) =>
    req<{ ok: boolean }>(ep.notifySettings(), {
      method: "PUT",
      body: JSON.stringify(body),
    }),
  testNotify: () =>
    req<{ telegram: string; email: string }>(ep.notifyTest(), {
      method: "POST",
    }),

  // notification groups (Phase 1: multi-recipient email)
  notificationGroups: () =>
    req<NotificationGroup[]>(ep.notificationGroups()),
  notificationGroup: (id: number) =>
    req<NotificationGroupDetail>(ep.notificationGroup(id)),
  createNotificationGroup: (name: string, description?: string) =>
    req<{ id: number; name: string }>(ep.notificationGroups(), {
      method: "POST",
      body: JSON.stringify({ name, description: description ?? "" }),
    }),
  updateNotificationGroup: (id: number, body: { name?: string; description?: string }) =>
    req<{ ok: boolean }>(ep.notificationGroup(id), {
      method: "PATCH",
      body: JSON.stringify(body),
    }),
  deleteNotificationGroup: (id: number) =>
    req<{ ok: boolean }>(ep.notificationGroup(id), { method: "DELETE" }),
  addGroupMember: (id: number, email: string) =>
    req<{ id: number; email: string }>(ep.notificationGroupMembers(id), {
      method: "POST",
      body: JSON.stringify({ email }),
    }),
  removeGroupMember: (id: number, memberId: number) =>
    req<{ ok: boolean }>(ep.notificationGroupMember(id, memberId), { method: "DELETE" }),

  // notification rules (Phase 2 engine)
  notificationRules: () =>
    req<NotificationRule[]>(ep.notificationRules()),
  notificationRule: (id: number) =>
    req<NotificationRule>(ep.notificationRule(id)),
  createNotificationRule: (body: NotificationRuleCreate) =>
    req<{ id: number; name: string }>(ep.notificationRules(), {
      method: "POST",
      body: JSON.stringify(body),
    }),
  updateNotificationRule: (id: number, body: NotificationRuleUpdate) =>
    req<{ ok: boolean }>(ep.notificationRule(id), {
      method: "PATCH",
      body: JSON.stringify(body),
    }),
  deleteNotificationRule: (id: number) =>
    req<{ ok: boolean }>(ep.notificationRule(id), { method: "DELETE" }),

  // services & deployments
  services: (projectId: number) =>
    req<Service[]>(ep.projectServices(projectId)),
  deployments: (projectId: number) =>
    req<Deployment[]>(ep.projectDeployments(projectId)),
  deploy: (projectId: number, commitSha?: string) =>
    req<{ deployment: Deployment; operationId: string }>(
      ep.projectDeploy(projectId),
      {
        method: "POST",
        body: JSON.stringify({ commitSha: commitSha ?? "" }),
      },
    ),
  wipeDeployments: () =>
    req<{ ok: boolean; deleted: number }>(ep.deploymentsAll(), { method: "DELETE" }),
  rollback: (projectId: number, depId: number) =>
    req<{ deployment: Deployment; operationId: string }>(
      ep.deploymentRollback(projectId, depId),
      { method: "POST" },
    ),

  // shipyard discovery (auto-detect compose projects from Docker labels)
  discovery: () => req<DiscoveryResult>(ep.discovery()),
  importDiscovered: (name: string) =>
    req<Project & { servicesAdded: number }>(ep.discoveryImport(), {
      method: "POST",
      body: JSON.stringify({ name }),
    }),
  // Bulk import: registers every unregistered Docker + directory find at
  // once. Idempotent — already-registered ships are skipped server-side.
  importAllDiscovered: () =>
    req<ImportAllResult>(ep.discoveryImportAll(), {
      method: "POST",
    }),

  // secrets (metadata only; values only via reveal)
  secrets: (projectId: number) =>
    req<SecretMeta[]>(ep.secrets(projectId)),
  upsertSecret: (projectId: number, name: string, value: string, environment = "production") =>
    req<{ name: string; configured: boolean }>(ep.secrets(projectId), {
      method: "POST",
      body: JSON.stringify({ name, value, environment }),
    }),
  revealSecret: (id: number) =>
    req<{ name: string; value: string }>(ep.secretReveal(id), { method: "POST" }),

  // telemetry history for the resource timeline
  telemetry: (range: string = "1h") =>
    req<TelemetryHistory>(ep.telemetry(range)),

  // host inventory (detail, storage, per-container stats)
  serverDetail: () => req<ServerDetail>(ep.serverDetail()),
  storage: () => req<StorageInfo>(ep.storage()),
  containerStats: () =>
    req<ContainerStat[]>(ep.containersStats()),

  // operations (unified progress for deploys, restarts, backups …)
  operation: (id: string) => req<Operation>(ep.operation(id)),
  operations: (limit = 20) =>
    req<Operation[]>(ep.operations(limit)),

  // container exec terminal (single-use token, then WebSocket)
  execToken: (container: string, shell = "/bin/sh", confirm = true) =>
    req<{ token: string; operationId: string; id: string }>(
      ep.containerExec(container, confirm),
      { method: "POST", body: JSON.stringify({ shell }) },
    ),

  // backups (directory snapshots + metadata)
  backups: (projectId: number) =>
    req<Backup[]>(ep.backups(projectId)),
  createBackup: (projectId: number) =>
    req<{ ok: boolean; operationId: string; projectId: number }>(
      ep.backups(projectId),
      { method: "POST" },
    ),
  deleteBackup: (id: number) =>
    req<{ ok: boolean }>(ep.backup(id), { method: "DELETE" }),
  downloadBackupUrl: (id: number) =>
    `${API_BASE}${ep.backupDownload(id)}`,
  restoreBackup: (id: number) =>
    req<{ ok: boolean; operationId: string; backupId: number }>(
      ep.backupRestore(id),
      { method: "POST" },
    ),

  // databases: auto-detected servers (Docker, host ports, fleet services).
  // Browse lists what's inside; connect saves the credential; register
  // turns ticked databases into services under a project.
  dbServers: () =>
    req<{ servers: DbServerInfo[]; dockerAvailable: boolean }>(
      ep.dbServers(),
    ),
  browseDatabases: (t: DbTarget) =>
    req<{ databases: DbItem[]; authRequired: boolean }>(
      ep.dbBrowse(),
      { method: "POST", body: JSON.stringify(t) },
    ),
  connectDatabase: (t: DbTarget) =>
    req<{ ok: boolean; key: string; hasCreds: boolean; databases: number }>(
      ep.dbConnect(),
      { method: "POST", body: JSON.stringify(t) },
    ),
  registerDatabases: (
    projectId: number,
    server: DbTarget,
    databases: string[],
    username?: string,
    password?: string,
  ) =>
    req<{ ok: boolean; registered: { database: string; serviceId: number }[]; skipped: string[] }>(
      ep.dbRegister(),
      { method: "POST", body: JSON.stringify({ projectId, server, databases, username, password }) },
    ),

  // central activity log (single place for all logs; feeds future aggregator UI)
  logs: (params?: LogsParams) => req<AppLog[]>(ep.logs(params)),

  // user + role management (admin only)
  users: () =>
    req<{ username: string; role: string; createdAt: string }[]>(ep.users()),
  createUser: (username: string, password: string, role: string) =>
    req<{ username: string; role: string }>(ep.users(), {
      method: "POST",
      body: JSON.stringify({ username, password, role }),
    }),
  setUserRole: (username: string, role: string) =>
    req<{ username: string; role: string }>(ep.userRole(username), {
      method: "PUT",
      body: JSON.stringify({ role }),
    }),
  deleteUser: (username: string) =>
    req<{ ok: boolean }>(ep.user(username), {
      method: "DELETE",
    }),
  logoutAll: () =>
    req<{ ok: boolean }>(ep.authLogoutAll(), { method: "POST" }),

  // managed servers
  servers: () => req<ManagedServer[]>(ep.servers()),
  managedServer: (id: number) => req<ManagedServer>(ep.serverById(id)),
  createServer: (body: { name: string; hostname?: string; ipAddress?: string; groupId?: number }) =>
    req<{ id: number; name: string }>(ep.servers(), { method: "POST", body: JSON.stringify(body) }),
  updateServer: (id: number, body: Partial<{ name: string; hostname: string; ipAddress: string; groupId: number }>) =>
    req<{ ok: boolean }>(ep.serverById(id), { method: "PATCH", body: JSON.stringify(body) }),
  deleteServer: (id: number) =>
    req<{ ok: boolean }>(ep.serverById(id), { method: "DELETE" }),
  serverMetrics: (id: number, range = "1h") =>
    req<{ range: string; points: ServerMetricPoint[] }>(ep.serverMetrics(id, range)),
  serverMetricsLatest: (id: number) =>
    req<ServerMetricPoint>(ep.serverMetricsLatest(id)),

  // per-server Prometheus history (Phase 3C; web-only types, see below)
  serverPrometheusMetrics: (id: number, metric: string, range: string, step: string) =>
    req<ServerPromMetrics>(
      `/server-hub/api/servers/${id}/prometheus/metrics?metric=${encodeURIComponent(metric)}&range=${encodeURIComponent(range)}&step=${encodeURIComponent(step)}`,
    ),

  // agent tokens
  agentTokens: (serverId: number) =>
    req<AgentToken[]>(ep.serverTokens(serverId)),
  createAgentToken: (serverId: number, label?: string) =>
    req<{ id: number; token: string; label: string }>(ep.serverTokens(serverId), {
      method: "POST", body: JSON.stringify({ label: label ?? "" }),
    }),
  revokeAgentToken: (serverId: number, tokenId: number) =>
    req<{ ok: boolean }>(ep.serverToken(serverId, tokenId), { method: "DELETE" }),

  // server groups
  serverGroups: () => req<ServerGroup[]>(ep.serverGroups()),
  createServerGroup: (name: string, description?: string) =>
    req<{ id: number; name: string }>(ep.serverGroups(), {
      method: "POST", body: JSON.stringify({ name, description: description ?? "" }),
    }),
  deleteServerGroup: (id: number) =>
    req<{ ok: boolean }>(ep.serverGroup(id), { method: "DELETE" }),

  // alerts
  alerts: (status?: string) =>
    req<Alert[]>(ep.alerts(status)),
  resolveAlert: (id: number) =>
    req<{ ok: boolean }>(ep.alertResolve(id), { method: "PATCH" }),

  // in-app notifications
  notifications: () => req<InAppNotification[]>(ep.notifications()),
  markNotificationRead: (id: number) =>
    req<{ ok: boolean }>(ep.notificationRead(id), { method: "PATCH" }),
  markAllNotificationsRead: () =>
    req<{ ok: boolean }>(ep.notificationsReadAll(), { method: "POST" }),

  // health checks
  healthChecks: () => req<HealthCheck[]>(ep.healthChecks()),
  createHealthCheck: (body: Partial<HealthCheck>) =>
    req<{ id: number; name: string }>(ep.healthChecks(), { method: "POST", body: JSON.stringify(body) }),
  updateHealthCheck: (id: number, body: Partial<HealthCheck>) =>
    req<{ ok: boolean }>(ep.healthCheck(id), { method: "PATCH", body: JSON.stringify(body) }),
  deleteHealthCheck: (id: number) =>
    req<{ ok: boolean }>(ep.healthCheck(id), { method: "DELETE" }),
  healthCheckResults: (id: number) =>
    req<HealthCheckResult[]>(ep.healthCheckResults(id)),

  // monitoring — Prometheus + Alertmanager
  monitoringOverview: () =>
    req<MonitoringOverview>(ep.monitoringOverview()),
  prometheusStatus: () =>
    req<PrometheusStatus>(ep.prometheusStatus()),
  prometheusTargets: () =>
    req<{ data: PromTargetsData }>(ep.prometheusTargets()),
  prometheusRules: () =>
    req<{ data: unknown }>(ep.prometheusRules()),
  prometheusQuery: (query: string) =>
    req<{ data: unknown }>(ep.prometheusQuery(query)),
  prometheusQueryRange: (query: string, rangeStr: string, step = "60") => {
    const now = Math.floor(Date.now() / 1000);
    const durations: Record<string, number> = { "1h": 3600, "6h": 21600, "24h": 86400, "7d": 604800 };
    const dur = durations[rangeStr] ?? 3600;
    return req<{ data: PromRangeResult }>(
      ep.prometheusQueryRange(query, String(now - dur), String(now), step)
    );
  },
  monitoringMetrics: (metric: string, range = "1h", step = "60") =>
    req<{ metric: string; range: string; data: PromRangeResult }>(
      ep.monitoringMetrics(metric, range, step)
    ),
  alertmanagerStatus: () =>
    req<AlertmanagerStatus>(ep.alertmanagerStatus()),
  monitoringAlerts: () =>
    req<AmAlert[]>(ep.monitoringAlerts()),
  monitoringSilences: () =>
    req<AmSilence[]>(ep.monitoringSilences()),
  createSilence: (body: object) =>
    req<{ silenceID: string }>(ep.monitoringSilences(), {
      method: "POST",
      body: JSON.stringify(body),
    }),
  deleteSilence: (id: string) =>
    req<{ ok: boolean }>(ep.monitoringSilence(id), { method: "DELETE" }),
};
