export const API_BASE = (import.meta.env.VITE_API_URL as string | undefined) ?? "http://localhost:4000";

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, {
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    ...init,
  });
  if (res.status === 204) return undefined as T;
  let body: unknown = null;
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
  createProject: (p: Partial<import("./types").Project>) =>
    req<import("./types").Project>("/server-hub/api/projects", { method: "POST", body: JSON.stringify(p) }),
  projectAction: (id: number, action: "start" | "stop" | "restart", confirm = false) =>
    req<{ ok: boolean; logs?: string }>(
      `/server-hub/api/projects/${id}/${action}${confirm ? "?confirm=true" : ""}`,
      { method: "POST" },
    ),

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
};
