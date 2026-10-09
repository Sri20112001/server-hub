// Shared ServerHub API contracts (JSON DTOs + pure helpers).
//
// Every interface here mirrors a Go/Gin backend response shape exactly —
// field names are backend-owned and must not be renamed for convenience.
// Pure helpers (toFleetStatus/fleetLabel) are platform-independent and safe
// for both Vite and Metro.
//
// This file must stay dependency-free and free of platform APIs
// (no React, Expo, DOM, SecureStore, or Node imports).
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

export interface SecretMeta {
  id: number;
  projectId: number;
  name: string;
  environment: string;
  serviceId?: number;
  configured: boolean;
  updatedAt: string;
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

export interface DiscoveredService {
  name: string;
  container: string;
  image: string;
  state: string;
  type: string;
}

export interface DiscoveredProject {
  name: string;
  deploymentPath: string;
  composeFile: string;
  registered: boolean;
  services: DiscoveredService[];
}

export interface DiscoveryResult {
  dockerAvailable: boolean;
  projects: DiscoveredProject[];
  filesystem: FoundProject[];
}

export interface FoundProject {
  name: string;
  path: string;
  stack: string[];
  composeFile: string;
  registered: boolean;
}

export interface DbServerInfo {
  key: string;
  engine: string;
  source: string;
  name: string;
  host: string;
  port: number;
  container: string;
  version: string;
  state: string;
  verified: boolean;
  hasCreds: boolean;
  registered: string[];
}

export interface DbItem {
  name: string;
  sizeBytes: number;
  registered: boolean;
  system: boolean;
}

export interface DbTarget {
  engine: string;
  source: string;
  host: string;
  port: number;
  container: string;
  username?: string;
  password?: string;
}

export interface BulkLifecycleResult {
  ok: boolean;
  action: string;
  succeeded: number;
  failed: number;
  results: { id: number; name: string; ok: boolean; error?: string; operationId?: string }[];
}

export interface NotifySettings {
  enabled: boolean;
  events: { deployFailed: boolean; threshold: boolean; backupFailed: boolean; projectFailed: boolean };
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
    /** Default notification group id; null = legacy To fallback. */
    emailGroupId: number | null;
  };
}

export interface ImportAllResult {
  ok: boolean;
  imported: number;
  servicesAdded: number;
  projects: { id: number; name: string; servicesAdded: number }[];
  failed: { name: string; error: string }[];
}

export interface TelemetryPoint {
  ts: number;
  cpu: number;
  memPct: number;
  memMB: number;
  diskPct: number;
  netRx: number;
  netTx: number;
  diskRead: number;
  diskWrite: number;
}

export interface TelemetryHistory {
  range: string;
  points: TelemetryPoint[];
}

export interface OpStage {
  name: string;
  state: string;
}

export interface Operation {
  id: string;
  type: string;
  targetType: string;
  targetId: string;
  status: string;
  stage: string;
  stages: OpStage[];
  initiatedBy: string;
  error?: string;
  createdAt: string;
  startedAt?: string;
  completedAt?: string;
}

export interface Backup {
  id: number;
  projectId: number;
  kind: string;
  sizeBytes: number;
  status: string;
  createdAt: string;
}

export interface BusEvent {
  type: string;
  data?: Record<string, unknown>;
  timestamp: string;
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

export interface MemDetail {
  totalMB: number;
  usedMB: number;
  availableMB: number;
  freeMB: number;
  cachedMB: number;
  buffersMB: number;
  usedPct: number;
  swapTotalMB: number;
  swapUsedMB: number;
  swapFreeMB: number;
  swapPct: number;
}

export interface Filesystem {
  device: string;
  mount: string;
  fstype: string;
  totalGB: number;
  usedGB: number;
  freeGB: number;
  usedPct: number;
}

export interface CPUDetail {
  logicalCores: number;
  physicalCores: number;
  model: string;
  mhz: number;
  load1: number;
  load5: number;
  load15: number;
  timesUserPct: number;
  timesSysPct: number;
  timesIdlePct: number;
  timesIowaitPct: number;
}

export interface NetIface {
  name: string;
  rxBytes: number;
  txBytes: number;
  rxErrors: number;
  txErrors: number;
}

export interface DiskIO {
  device: string;
  readMB: number;
  writeMB: number;
  readOps: number;
  writeOps: number;
}

export interface ProcInfo {
  pid: number;
  name: string;
  status: string;
  memMB: number;
  memPct: number;
}

export interface ServerDetail {
  host: {
    hostname: string;
    os: string;
    platform: string;
    kernel: string;
    arch: string;
    uptimeSec: number;
    goVersion: string;
  };
  cpu: CPUDetail;
  memory: MemDetail;
  filesystems: Filesystem[];
  network: NetIface[];
  diskIO: DiskIO[];
  processes: {
    total: number;
    running: number;
    sleeping: number;
    other: number;
    zombie: number;
    top: ProcInfo[];
  };
  pressure: { cpu: string; memory: string; disk: string };
}

export interface AppUsage {
  name: string;
  path: string;
  bytesMB: number;
}

export interface StorageInfo {
  filesystems: Filesystem[];
  appsRoot: string;
  apps: AppUsage[];
  appsPartial: boolean;
  docker: {
    imagesMB: number;
    containersMB: number;
    volumesMB: number;
    buildCacheMB: number;
    imagesTop: { name: string; sizeMB: number }[];
  } | null;
}

export interface ContainerStat {
  id: string;
  name: string;
  image: string;
  state: string;
  cpuPercent: number;
  memMB: number;
  memLimitMB: number;
  memPct: number;
  netRxMB: number;
  netTxMB: number;
  blockReadMB: number;
  blockWriteMB: number;
  pids: number;
}

/** Normalised fleet status used across the UI. */
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

export interface ServerMetricPoint {
  id: number;
  serverId: number;
  timestamp: string;
  cpuUsage: number;
  memoryUsage: number;
  memoryUsedMB: number;
  diskUsage: number;
  diskUsedGB: number;
  netRx: number;
  netTx: number;
  loadAvg1: number;
  uptimeSec: number;
}

export interface AgentToken {
  id: number;
  serverId: number;
  label: string;
  revoked: boolean;
  createdAt: string;
  lastUsedAt: string | null;
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
  /** Alertmanager fingerprint for externally-sourced alerts ("" for internal). */
  fingerprint: string;
  /** Alert origin, e.g. "alertmanager" ("" for internal threshold/offline alerts). */
  source: string;
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

export interface HealthCheck {
  id: number;
  name: string;
  type: "http" | "tcp" | "ping";
  target: string;
  interval: number;
  timeout: number;
  expectedStatus: number;
  enabled: boolean;
  status: "UP" | "DOWN" | "UNKNOWN";
  responseTimeMs: number | null;
  lastCheckedAt: string | null;
  serverId: number | null;
  createdAt: string;
  updatedAt: string;
}

export interface HealthCheckResult {
  id: number;
  healthCheckId: number;
  timestamp: string;
  status: "UP" | "DOWN";
  responseTimeMs: number;
  error: string;
}

// ─── Prometheus / Alertmanager monitoring ────────────────────────────────────

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

/** Raw Prometheus target as returned by /api/v1/targets */
export interface PromTarget {
  discoveredLabels: Record<string, string>;
  labels: Record<string, string>;
  scrapePool: string;
  scrapeUrl: string;
  globalUrl: string;
  lastError: string;
  lastScrape: string;
  lastScrapeDuration: number;
  health: "up" | "down" | "unknown";
}

export interface PromTargetsData {
  activeTargets: PromTarget[];
  droppedTargets: PromTarget[];
}

/** Alertmanager alert as returned by /api/v2/alerts */
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

/** Alertmanager silence as returned by /api/v2/silences */
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

/** Prometheus range query result (matrix) */
export interface PromRangeResult {
  resultType: string;
  result: {
    metric: Record<string, string>;
    values: [number, string][];
  }[];
}

/** GET /server-hub/api/monitoring/metrics envelope */
export interface MonitoringMetricHistory {
  metric: string;
  range: string;
  data: PromRangeResult;
}

/** Query filters for GET /server-hub/api/logs (shared by web + mobile). */
export interface LogsParams {
  level?: string;
  source?: string;
  search?: string;
  projectId?: number;
  limit?: number;
  offset?: number;
}

// ─── Notification groups (Phase 1: multi-recipient email) ───────────────────

export interface NotificationGroup {
  id: number;
  name: string;
  description: string;
  memberCount: number;
  createdAt: string;
  updatedAt: string;
}

export interface NotificationGroupMember {
  id: number;
  email: string;
  createdAt: string;
}

export interface NotificationGroupDetail {
  id: number;
  name: string;
  description: string;
  members: NotificationGroupMember[];
  createdAt: string;
  updatedAt: string;
}

// ─── Notification rules (Phase 2 engine) ────────────────────────────────────

/** Supported rule event types (see server/internal/rules/event.go). */
export type NotificationEventType =
  | "AGENT_OFFLINE"
  | "AGENT_ONLINE"
  | "SERVER_ALERT"
  | "SERVER_ALERT_RESOLVED";

/** Supported rule channels. */
export type NotificationChannel = "EMAIL" | "TELEGRAM" | "IN_APP";

export type DigestMode = "immediate" | "digest";

export interface NotificationRule {
  id: number;
  name: string;
  description: string;
  enabled: boolean;
  eventType: NotificationEventType;
  severity: string;
  conditionJson: string;
  notificationGroupId: number;
  groupName: string;
  channels: string;
  cooldownSeconds: number;
  notifyOnRecovery: boolean;
  repeatIntervalSec: number;
  maxRepeats: number;
  digestMode: DigestMode;
  digestIntervalMin: number;
  groupBy: string;
  createdBy: string;
  updatedBy: string;
  createdAt: string;
  updatedAt: string;
}

export interface NotificationRuleCondition {
  field: "severity" | "condition" | "value" | "serverId";
  operator: "eq" | "neq" | "gt" | "gte" | "lt" | "lte" | "contains" | "in";
  value: unknown;
}

export interface NotificationRuleCreate {
  name: string;
  description?: string;
  enabled?: boolean;
  eventType: NotificationEventType;
  severity?: string;
  conditionJson?: string;
  notificationGroupId: number;
  channels?: NotificationChannel[];
  cooldownSeconds?: number;
  notifyOnRecovery?: boolean;
  repeatIntervalSec?: number;
  maxRepeats?: number;
  digestMode?: DigestMode;
  digestIntervalMin?: number;
  groupBy?: string[];
}

export interface NotificationRuleUpdate {
  name?: string;
  description?: string;
  enabled?: boolean;
  eventType?: NotificationEventType;
  severity?: string;
  conditionJson?: string;
  notificationGroupId?: number;
  channels?: NotificationChannel[];
  cooldownSeconds?: number;
  notifyOnRecovery?: boolean;
  repeatIntervalSec?: number;
  maxRepeats?: number;
  digestMode?: DigestMode;
  digestIntervalMin?: number;
  groupBy?: string[];
}

export interface NotificationGroupQuiet {
  quietStart: string;
  quietEnd: string;
  quietTZ: string;
  quietAllowCritical: boolean;
}

// ─── Central delivery policy ─────────────────────────────────────────────

export interface DeliveryPolicy {
  emergencyPause: boolean;
  pauseUntil: string | null;
  pauseReason: string;
  cooldownCriticalSec: number;
  cooldownWarningSec: number;
  cooldownInfoSec: number;
  repeatIntervalSec: number;
  maxRepeats: number;
  notifyOnRecovery: boolean;
  emailPerHour: number;
  tgPerHour: number;
  digestIntervalMin: number;
}

export type DeliveryStatus = "pending" | "sending" | "sent" | "failed" | "deferred";

export interface DeliveryRecord {
  id: number;
  dedupeKey: string;
  severity: string;
  channel: string;
  groupId: number;
  title: string;
  body: string;
  status: DeliveryStatus;
  attempts: number;
  nextRetryAt: string;
  lastError: string;
  createdAt: string;
  sentAt: string | null;
}

export interface MaintenanceWindow {
  id: number;
  name: string;
  scope: string;
  startsAt: string;
  endsAt: string;
  reason: string;
  enabled: boolean;
  active: boolean;
  createdBy: string;
  createdAt: string;
  updatedAt: string;
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
