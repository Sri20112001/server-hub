// Central API path builders for the ServerHub Go backend.
//
// These produce the path portion of each endpoint (no host, no base URL —
// the base URL stays platform-specific: VITE_API_URL on web,
// EXPO_PUBLIC_API_URL + SecureStore override on mobile).
// Behaviour (encoding, defaults, query shapes) matches what both clients
// already send, so swapping call sites is behaviour-preserving.

const API = "/server-hub/api";

// ─── Auth ────────────────────────────────────────────────────────────────────
/** Web login (cookie session). Mobile uses token() instead. */
export function authLogin(): string {
  return `${API}/auth/login`;
}
/** Mobile token auth. Web uses login() instead. */
export function authToken(): string {
  return `${API}/auth/token`;
}
export function authRefresh(): string {
  return `${API}/auth/refresh`;
}
export function authLogout(): string {
  return `${API}/auth/logout`;
}
export function authLogoutAll(): string {
  return `${API}/auth/logout-all`;
}
export function authMe(): string {
  return `${API}/auth/me`;
}
export function authPassword(): string {
  return `${API}/auth/password`;
}

// ─── Dashboard / host ────────────────────────────────────────────────────────
export function dashboard(): string {
  return `${API}/dashboard`;
}
export function server(): string {
  return `${API}/server`;
}
export function serverDetail(): string {
  return `${API}/server/detail`;
}
export function storage(): string {
  return `${API}/server/storage`;
}
export function audit(limit = 8): string {
  return `${API}/audit?limit=${limit}`;
}

// ─── Projects / services / deployments ───────────────────────────────────────
export function projects(): string {
  return `${API}/projects`;
}
export function project(id: number): string {
  return `${API}/projects/${id}`;
}
export function projectServices(projectId: number): string {
  return `${API}/projects/${projectId}/services`;
}
export function projectDeployments(projectId: number): string {
  return `${API}/projects/${projectId}/deployments`;
}
export function projectAction(id: number, action: "start" | "stop" | "restart", confirm = false): string {
  return `${API}/projects/${id}/${action}${confirm ? "?confirm=true" : ""}`;
}
export function projectDeploy(projectId: number): string {
  return `${API}/projects/${projectId}/deploy`;
}
export function deploymentRollback(projectId: number, depId: number): string {
  return `${API}/projects/${projectId}/deployments/${depId}/rollback`;
}
export function deploymentsAll(): string {
  return `${API}/deployments`;
}
export function bulkLifecycle(): string {
  return `${API}/projects/bulk-lifecycle`;
}
export function service(id: number): string {
  return `${API}/services/${id}`;
}

// ─── Discovery ───────────────────────────────────────────────────────────────
export function discovery(): string {
  return `${API}/discovery`;
}
export function discoveryImport(): string {
  return `${API}/discovery/import`;
}
export function discoveryImportAll(): string {
  return `${API}/discovery/import-all`;
}

// ─── Secrets ─────────────────────────────────────────────────────────────────
export function secrets(projectId: number): string {
  return `${API}/projects/${projectId}/secrets`;
}
export function secret(id: number): string {
  return `${API}/secrets/${id}`;
}
export function secretReveal(id: number): string {
  return `${API}/secrets/${id}/reveal`;
}

// ─── Telemetry / containers ──────────────────────────────────────────────────
export function telemetry(range = "1h"): string {
  return `${API}/telemetry?range=${range}`;
}
export function telemetryLatest(): string {
  return `${API}/telemetry/latest`;
}
export function containersStats(): string {
  return `${API}/containers/stats`;
}
export function container(id: string): string {
  return `${API}/containers/${encodeURIComponent(id)}`;
}
export function containerLogs(id: string): string {
  return `${API}/containers/${encodeURIComponent(id)}/logs`;
}
/** Streaming (SSE) container logs with tail/follow — used by the log viewer. */
export function containerLogsStream(id: string, tail: string, follow = true): string {
  return `${containerLogs(id)}?tail=${tail}${follow ? "&follow=1" : ""}`;
}
export function containerStats(id: string): string {
  return `${API}/containers/${encodeURIComponent(id)}/stats`;
}
export function containerExec(id: string, confirm = true): string {
  return `${API}/containers/${encodeURIComponent(id)}/exec${confirm ? "?confirm=true" : ""}`;
}
export function images(): string {
  return `${API}/images`;
}
export function volumes(): string {
  return `${API}/volumes`;
}

// ─── Operations ──────────────────────────────────────────────────────────────
export function operations(limit = 20): string {
  return `${API}/operations?limit=${limit}`;
}
export function operation(id: string): string {
  return `${API}/operations/${id}`;
}

// ─── Backups ─────────────────────────────────────────────────────────────────
export function backups(projectId: number): string {
  return `${API}/projects/${projectId}/backups`;
}
export function backup(id: number): string {
  return `${API}/backups/${id}`;
}
export function backupDownload(id: number): string {
  return `${API}/backups/${id}/download`;
}
export function backupRestore(id: number): string {
  return `${API}/backups/${id}/restore?confirm=true`;
}

// ─── Databases ───────────────────────────────────────────────────────────────
export function dbServers(): string {
  return `${API}/databases/servers`;
}
export function dbBrowse(): string {
  return `${API}/databases/browse`;
}
export function dbConnect(): string {
  return `${API}/databases/connect`;
}
export function dbRegister(): string {
  return `${API}/databases/register`;
}

// ─── Logs ────────────────────────────────────────────────────────────────────
export function logs(params?: {
  level?: string;
  source?: string;
  search?: string;
  projectId?: number;
  limit?: number;
  offset?: number;
}): string {
  const q = new URLSearchParams();
  if (params?.level) q.set("level", params.level);
  if (params?.source) q.set("source", params.source);
  if (params?.search) q.set("search", params.search);
  if (params?.projectId !== undefined) q.set("projectId", String(params.projectId));
  if (params?.limit !== undefined) q.set("limit", String(params.limit));
  if (params?.offset !== undefined) q.set("offset", String(params.offset));
  const suffix = q.toString() ? `?${q.toString()}` : "";
  return `${API}/logs${suffix}`;
}

// ─── Users ───────────────────────────────────────────────────────────────────
export function users(): string {
  return `${API}/users`;
}
export function userRole(username: string): string {
  return `${API}/users/${encodeURIComponent(username)}/role`;
}
export function user(username: string): string {
  return `${API}/users/${encodeURIComponent(username)}`;
}

// ─── Notification channels ───────────────────────────────────────────────────
export function notifySettings(): string {
  return `${API}/settings/notifications`;
}
export function notifyTest(): string {
  return `${API}/settings/notifications/test`;
}

// ─── Notification groups (Phase 1: multi-recipient email) ───────────────────
export function notificationGroups(): string {
  return `${API}/notification-groups`;
}
export function notificationGroup(id: number): string {
  return `${API}/notification-groups/${id}`;
}
export function notificationGroupMembers(id: number): string {
  return `${API}/notification-groups/${id}/members`;
}
export function notificationGroupMember(id: number, memberId: number): string {
  return `${API}/notification-groups/${id}/members/${memberId}`;
}

// ─── Managed servers ─────────────────────────────────────────────────────────
export function servers(): string {
  return `${API}/servers`;
}
export function serverById(id: number): string {
  return `${API}/servers/${id}`;
}
export function serverMetrics(id: number, range = "1h"): string {
  return `${API}/servers/${id}/metrics?range=${range}`;
}
export function serverMetricsLatest(id: number): string {
  return `${API}/servers/${id}/metrics/latest`;
}
export function serverTokens(serverId: number): string {
  return `${API}/servers/${serverId}/tokens`;
}
export function serverToken(serverId: number, tokenId: number): string {
  return `${API}/servers/${serverId}/tokens/${tokenId}`;
}
export function serverGroups(): string {
  return `${API}/server-groups`;
}
export function serverGroup(id: number): string {
  return `${API}/server-groups/${id}`;
}

// ─── Agent ───────────────────────────────────────────────────────────────────
export function agentHeartbeat(): string {
  return `${API}/agent/heartbeat`;
}
export function agentMetrics(): string {
  return `${API}/agent/metrics`;
}

// ─── Alerts ──────────────────────────────────────────────────────────────────
export function alerts(status?: string): string {
  return `${API}/alerts${status ? `?status=${status}` : ""}`;
}
export function alertResolve(id: number): string {
  return `${API}/alerts/${id}/resolve`;
}

// ─── In-app notifications ────────────────────────────────────────────────────
export function notifications(): string {
  return `${API}/notifications`;
}
export function notificationRead(id: number): string {
  return `${API}/notifications/${id}/read`;
}
export function notificationsReadAll(): string {
  return `${API}/notifications/read-all`;
}

// ─── Health checks ───────────────────────────────────────────────────────────
export function healthChecks(): string {
  return `${API}/health-checks`;
}
export function healthCheck(id: number): string {
  return `${API}/health-checks/${id}`;
}
export function healthCheckResults(id: number): string {
  return `${API}/health-checks/${id}/results`;
}

// ─── Monitoring (Prometheus / Alertmanager proxy) ────────────────────────────
export function monitoringOverview(): string {
  return `${API}/monitoring/overview`;
}
export function prometheusStatus(): string {
  return `${API}/monitoring/prometheus/status`;
}
export function prometheusTargets(): string {
  return `${API}/monitoring/prometheus/targets`;
}
export function prometheusRules(): string {
  return `${API}/monitoring/prometheus/rules`;
}
export function prometheusQuery(query: string): string {
  return `${API}/monitoring/prometheus/query?query=${encodeURIComponent(query)}`;
}
export function prometheusQueryRange(query: string, start: string, end: string, step = "60"): string {
  return `${API}/monitoring/prometheus/query-range?query=${encodeURIComponent(query)}&start=${start}&end=${end}&step=${step}`;
}
export function monitoringMetrics(metric: string, range = "1h", step = "60"): string {
  return `${API}/monitoring/metrics?metric=${metric}&range=${range}&step=${step}`;
}
export function alertmanagerStatus(): string {
  return `${API}/monitoring/alertmanager/status`;
}
export function monitoringAlerts(): string {
  return `${API}/monitoring/alerts`;
}
export function monitoringSilences(): string {
  return `${API}/monitoring/silences`;
}
export function monitoringSilence(id: string): string {
  return `${API}/monitoring/silences/${encodeURIComponent(id)}`;
}

// ─── Webhooks / exec ─────────────────────────────────────────────────────────
export function webhookGithub(): string {
  return `${API}/webhooks/github`;
}
export function webhookAlertmanager(): string {
  return `${API}/webhooks/alertmanager`;
}
export function execSession(token: string): string {
  return `${API}/exec/${token}`;
}
export function events(): string {
  return `${API}/events`;
}
