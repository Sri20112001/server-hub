// Mirrored from client/src/lib/types.ts

export interface User {
  username: string;
  role: string;
}

export interface Project {
  id: number;
  name: string;
  description?: string;
  repository?: string;
  branch?: string;
  environment?: string;
  deploymentPath?: string;
  composeFile?: string;
  gatewayPrefix?: string;
  healthUrl?: string;
  status: string;
  autoDeploy: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface Service {
  id: number;
  projectId: number;
  name: string;
  type: string;
  containerName?: string;
  internalPort?: number;
  hostPort?: number;
  healthUrl?: string;
  dockerServiceName?: string;
  status: string;
  lastHealth?: string;
  lastHealthAt?: string;
  responseTimeMs?: number;
}

export interface Deployment {
  id: number;
  projectId: number;
  commitSha?: string;
  branch?: string;
  trigger?: string;
  status: string;
  startedAt: string;
  completedAt?: string;
  durationSec?: number;
  logs?: string;
}

export interface DashboardData {
  server: {
    cpuPercent: number;
    memTotalMB: number;
    memUsedMB: number;
    memPercent: number;
    diskTotalGB: number;
    diskUsedGB: number;
    diskPercent: number;
    uptimeSec: number;
  };
  counts: {
    projects: number;
    services: number;
    deployments: number;
    healthy: number;
    degraded: number;
    down: number;
  };
  projects: Project[];
  recentDeployments: Deployment[];
  dockerAvailable: boolean;
}

export interface ServerInfo {
  cpu: number;
  memTotalMB: number;
  memUsedMB: number;
  memPercent: number;
  diskTotalGB: number;
  diskUsedGB: number;
  diskPercent: number;
  uptimeSec: number;
  cpuCores: number;
  netRxRate: number;
  netTxRate: number;
  projects: number;
  services: number;
  deployments: number;
  containers: number;
  runningContainers: number;
  dockerAvailable: boolean;
}

export interface AuditLog {
  id: number;
  actor: string;
  action: string;
  resource: string;
  resourceId?: string;
  timestamp: string;
  result: string;
  metadata?: string;
}

export interface Operation {
  id: string;
  type: string;
  targetType: string;
  targetId: string;
  status: string;
  stage: string;
  stages: { name: string; state: string }[];
  initiatedBy: string;
  error?: string;
  createdAt: string;
  startedAt?: string;
  completedAt?: string;
}

export interface AppLog {
  id: number;
  timestamp: string;
  level: string;
  source: string;
  actor?: string;
  action?: string;
  resource?: string;
  resourceId?: string;
  projectId?: number;
  message: string;
  metadata?: string;
  requestId?: string;
  method?: string;
  path?: string;
  statusCode?: number;
  latencyMs?: number;
}

export interface NotifySettings {
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
  };
}

// Fleet status — mirrors web toFleetStatus()
export type FleetStatus = "sailing" | "choppy" | "lost" | "docked";

// ─── Managed Servers ─────────────────────────────────────────────────────────

export interface ManagedServer {
  id: number;
  name: string;
  hostname: string;
  ipAddress: string;
  os: string;
  osVersion: string;
  arch: string;
  cpuInfo: string;
  cpuCores: number;
  ramTotal: number;
  diskTotal: number;
  status: "ONLINE" | "OFFLINE" | "WARNING" | "UNKNOWN";
  agentStatus: "CONNECTED" | "DISCONNECTED" | "UNKNOWN";
  lastHeartbeat: string | null;
  groupId: number | null;
  createdAt: string;
  updatedAt: string;
}

export interface ServerGroup {
  id: number;
  name: string;
  description: string;
  createdAt: string;
}

export interface Alert {
  id: number;
  serverId: number | null;
  condition: string;
  threshold: number;
  severity: "INFO" | "WARNING" | "CRITICAL";
  status: "TRIGGERED" | "RESOLVED";
  message: string;
  triggeredAt: string;
  resolvedAt: string | null;
}

export interface InAppNotification {
  id: number;
  username: string;
  title: string;
  body: string;
  category: string;
  read: boolean;
  serverId: number | null;
  alertId: number | null;
  createdAt: string;
}

export function toFleetStatus(raw: string | undefined): FleetStatus {
  const s = (raw ?? "").toLowerCase();
  if (s === "healthy" || s === "running" || s === "success" || s === "ok") return "sailing";
  if (s === "degraded" || s === "choppy" || s === "running_partial") return "choppy";
  if (s === "down" || s === "failed" || s === "error") return "lost";
  return "docked";
}

export const fleetLabel: Record<FleetStatus, string> = {
  sailing: "Sailing",
  choppy: "Choppy",
  lost: "Lost signal",
  docked: "Docked",
};
