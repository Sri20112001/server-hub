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
