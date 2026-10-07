import React, { useState, useCallback } from "react";
import {
  View,
  Text,
  ScrollView,
  TouchableOpacity,
  RefreshControl,
  Modal,
  TextInput,
  ActivityIndicator,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Activity,
  AlertTriangle,
  CheckCircle,
  Wifi,
  WifiOff,
  XCircle,
  RefreshCw,
  VolumeX,
} from "lucide-react-native";
import { monitoringApi } from "../../src/api/monitoring";
import type { AmAlert, AmSilence } from "../../src/types";
import { colors } from "../../src/theme/colors";

type Tab = "dashboard" | "alerts" | "silences";

// ── helpers ───────────────────────────────────────────────────────────────────

function fmtBytes(b: number | null): string {
  if (b === null || b < 0) return "—";
  if (b < 1024) return `${b.toFixed(0)} B/s`;
  if (b < 1024 * 1024) return `${(b / 1024).toFixed(1)} KB/s`;
  return `${(b / 1024 / 1024).toFixed(1)} MB/s`;
}

function fmtDuration(start: string): string {
  const ms = Date.now() - new Date(start).getTime();
  const s = Math.floor(ms / 1000);
  if (s < 60) return `${s}s`;
  if (s < 3600) return `${Math.floor(s / 60)}m`;
  return `${Math.floor(s / 3600)}h`;
}

function severityColor(sev: string | undefined): string {
  switch ((sev ?? "").toLowerCase()) {
    case "critical": return "#f87171";
    case "warning": return "#facc15";
    default: return colors.fog;
  }
}

// ── MetricBar ─────────────────────────────────────────────────────────────────

function MetricBar({ label, value }: { label: string; value: number | null }) {
  if (value === null || value < 0) {
    return (
      <View className="mb-3">
        <View className="flex-row justify-between mb-1">
          <Text className="text-fog text-xs">{label}</Text>
          <Text className="text-fog text-xs font-mono">—</Text>
        </View>
        <View className="h-1.5 bg-emboss rounded-full" />
      </View>
    );
  }
  const pct = Math.max(0, Math.min(100, value));
  const barColor = pct >= 90 ? "#ef4444" : pct >= 70 ? "#eab308" : "#22c55e";
  const widthPct = `${Math.round(pct)}%` as `${number}%`;
  return (
    <View className="mb-3">
      <View className="flex-row justify-between mb-1">
        <Text className="text-fog text-xs">{label}</Text>
        <Text className="text-bone text-xs font-mono">{pct.toFixed(1)}%</Text>
      </View>
      <View className="h-1.5 bg-emboss rounded-full overflow-hidden">
        <View style={{ width: widthPct, height: "100%", backgroundColor: barColor, borderRadius: 999 }} />
      </View>
    </View>
  );
}

// ── StatusRow ─────────────────────────────────────────────────────────────────

function StatusRow({ label, ok, detail }: { label: string; ok: boolean | undefined; detail?: string }) {
  return (
    <View className="flex-row items-center justify-between py-2.5 border-b border-edge">
      <Text className="text-fog text-sm">{label}</Text>
      <View className="flex-row items-center gap-2">
        {detail ? <Text className="text-fog text-xs font-mono">{detail}</Text> : null}
        {ok === undefined
          ? <ActivityIndicator size="small" color={colors.fog} />
          : ok
            ? <Wifi size={14} color={colors.moss} />
            : <WifiOff size={14} color={colors.brick} />}
        <Text className="font-mono text-xs" style={{ color: ok === undefined ? colors.fog : ok ? colors.moss : colors.brick }}>
          {ok === undefined ? "…" : ok ? "UP" : "DOWN"}
        </Text>
      </View>
    </View>
  );
}

// ── Dashboard tab ─────────────────────────────────────────────────────────────

function DashboardTab() {
  const { data: overview, isLoading: ovLoading, refetch: refetchOv, isRefetching: ovRefetching } =
    useQuery({ queryKey: ["monitoring-overview"], queryFn: monitoringApi.overview, refetchInterval: 60_000 });

  const { data: promStatus } =
    useQuery({ queryKey: ["prom-status"], queryFn: monitoringApi.prometheusStatus, refetchInterval: 60_000 });

  const { data: amStatus } =
    useQuery({ queryKey: ["am-status"], queryFn: monitoringApi.alertmanagerStatus, refetchInterval: 60_000 });

  const { data: amAlerts = [] } =
    useQuery<AmAlert[]>({ queryKey: ["am-alerts"], queryFn: monitoringApi.alerts, refetchInterval: 30_000 });

  const firing = amAlerts.filter((a) => a.status.state === "active");

  return (
    <ScrollView
      contentContainerStyle={{ padding: 16, paddingBottom: 100 }}
      refreshControl={
        <RefreshControl refreshing={ovRefetching} onRefresh={() => void refetchOv()} tintColor={colors.ember} />
      }
    >
      {/* Status */}
      <View className="bg-panel border border-edge rounded-xl p-4 mb-4">
        <Text className="text-fog text-[10px] uppercase tracking-wider mb-2">Integration Status</Text>
        <StatusRow label="Prometheus" ok={promStatus?.healthy ?? (promStatus?.available === false ? false : undefined)} />
        <StatusRow
          label="Alertmanager"
          ok={amStatus?.healthy ?? (amStatus?.available === false ? false : undefined)}
          detail={firing.length > 0 ? `${firing.length} firing` : undefined}
        />
      </View>

      {/* Resources */}
      {ovLoading ? (
        <View className="items-center py-6">
          <ActivityIndicator color={colors.ember} />
        </View>
      ) : overview?.available ? (
        <View className="bg-panel border border-edge rounded-xl p-4 mb-4">
          <Text className="text-fog text-[10px] uppercase tracking-wider mb-3">Resources</Text>
          <MetricBar label="CPU" value={overview.cpu} />
          <MetricBar label="Memory" value={overview.memory} />
          <MetricBar label="Disk" value={overview.disk} />
          <View className="flex-row gap-3 mt-1">
            <View className="flex-1 bg-emboss rounded-lg p-2">
              <Text className="text-fog text-[10px]">Net ↓</Text>
              <Text className="text-bone font-mono text-xs">{fmtBytes(overview.networkRx)}</Text>
            </View>
            <View className="flex-1 bg-emboss rounded-lg p-2">
              <Text className="text-fog text-[10px]">Net ↑</Text>
              <Text className="text-bone font-mono text-xs">{fmtBytes(overview.networkTx)}</Text>
            </View>
          </View>
        </View>
      ) : (
        <View className="bg-panel border border-edge rounded-xl p-4 mb-4 items-center">
          <WifiOff size={24} color={colors.fog} />
          <Text className="text-fog text-sm mt-2 text-center">
            Prometheus not configured.{"\n"}Set PROMETHEUS_URL on the backend.
          </Text>
        </View>
      )}

      {/* Active alerts summary */}
      {firing.length > 0 && (
        <View className="bg-panel border border-edge rounded-xl p-4 mb-4">
          <Text className="text-fog text-[10px] uppercase tracking-wider mb-2">
            Active Alerts ({firing.length})
          </Text>
          {firing.slice(0, 5).map((a) => (
            <View key={a.fingerprint} className="flex-row items-start gap-2 py-2 border-t border-edge first:border-0">
              <AlertTriangle size={13} color={severityColor(a.labels.severity)} style={{ marginTop: 2 }} />
              <View className="flex-1">
                <Text className="text-bone text-sm font-medium" numberOfLines={1}>
                  {a.labels.alertname ?? "Alert"}
                </Text>
                <Text className="text-fog text-[10px] font-mono">
                  {a.labels.instance ?? a.labels.job ?? ""} · {fmtDuration(a.startsAt)} ago
                </Text>
              </View>
            </View>
          ))}
        </View>
      )}
    </ScrollView>
  );
}

// ── Alerts tab ────────────────────────────────────────────────────────────────

const SILENCE_DURATIONS = [
  { label: "30m", minutes: 30 },
  { label: "1h", minutes: 60 },
  { label: "4h", minutes: 240 },
];

function AlertsTab() {
  const qc = useQueryClient();
  const [silencing, setSilencing] = useState<AmAlert | null>(null);
  const [silenceDur, setSilenceDur] = useState(60);
  const [silenceComment, setSilenceComment] = useState("");
  const [busy, setBusy] = useState(false);

  const { data: alerts = [], isLoading, isRefetching, refetch, error } =
    useQuery<AmAlert[]>({
      queryKey: ["am-alerts"],
      queryFn: monitoringApi.alerts,
      refetchInterval: 30_000,
    });

  const doSilence = useCallback(async () => {
    if (!silencing) return;
    setBusy(true);
    const now = new Date();
    const end = new Date(now.getTime() + silenceDur * 60000);
    const matchers = Object.entries(silencing.labels).map(([name, value]) => ({
      name, value, isRegex: false, isEqual: true,
    }));
    try {
      await monitoringApi.createSilence({
        matchers,
        startsAt: now.toISOString(),
        endsAt: end.toISOString(),
        createdBy: "serverhub-mobile",
        comment: silenceComment || `Silenced via ServerHub mobile for ${silenceDur}m`,
      });
      setSilencing(null);
      setSilenceComment("");
      void qc.invalidateQueries({ queryKey: ["am-alerts"] });
      void qc.invalidateQueries({ queryKey: ["am-silences"] });
    } catch {
      // error handled by busy state reset
    } finally {
      setBusy(false);
    }
  }, [silencing, silenceDur, silenceComment, qc]);

  if (isLoading) return (
    <View className="flex-1 items-center justify-center">
      <ActivityIndicator color={colors.ember} />
    </View>
  );

  if (error) return (
    <View className="flex-1 items-center justify-center p-6">
      <WifiOff size={32} color={colors.fog} />
      <Text className="text-fog text-sm mt-3 text-center">Alertmanager unavailable</Text>
      <Text className="text-fog text-xs mt-1 text-center font-mono">
        {error instanceof Error ? error.message : "Unknown error"}
      </Text>
    </View>
  );

  const firing = alerts.filter((a) => a.status.state === "active");
  const suppressed = alerts.filter((a) => a.status.state === "suppressed");

  return (
    <>
      <ScrollView
        contentContainerStyle={{ padding: 16, paddingBottom: 100 }}
        refreshControl={
          <RefreshControl refreshing={isRefetching} onRefresh={() => void refetch()} tintColor={colors.ember} />
        }
      >
        <View className="flex-row items-center justify-between mb-3">
          <Text className="text-fog text-xs font-mono">
            {firing.length} firing · {suppressed.length} suppressed
          </Text>
          <TouchableOpacity onPress={() => void refetch()} className="flex-row items-center gap-1">
            <RefreshCw size={12} color={colors.fog} />
            <Text className="text-fog text-xs">Refresh</Text>
          </TouchableOpacity>
        </View>

        {alerts.length === 0 ? (
          <View className="items-center py-16">
            <CheckCircle size={36} color={colors.moss} />
            <Text className="text-fog text-sm mt-3">No active alerts.</Text>
          </View>
        ) : (
          alerts.map((a) => (
            <View
              key={a.fingerprint}
              className="bg-panel border border-edge rounded-xl p-4 mb-3"
            >
              <View className="flex-row items-start justify-between gap-2">
                <View className="flex-row items-center gap-2 flex-1 min-w-0">
                  {a.status.state === "active"
                    ? <XCircle size={14} color={severityColor(a.labels.severity)} />
                    : <CheckCircle size={14} color={colors.fog} />}
                  <Text className="text-bone font-semibold text-sm flex-1" numberOfLines={1}>
                    {a.labels.alertname ?? "Alert"}
                  </Text>
                </View>
                {a.status.state === "active" && (
                  <TouchableOpacity
                    onPress={() => setSilencing(a)}
                    className="bg-emboss border border-edge rounded-lg px-2 py-1"
                    accessibilityRole="button"
                    accessibilityLabel="Silence alert"
                  >
                    <VolumeX size={12} color={colors.fog} />
                  </TouchableOpacity>
                )}
              </View>

              <View className="flex-row items-center gap-2 mt-1 flex-wrap">
                <Text className="font-mono text-[10px] uppercase" style={{ color: severityColor(a.labels.severity) }}>
                  {a.labels.severity ?? ""}
                </Text>
                <Text className="text-fog text-[10px] font-mono">{a.status.state}</Text>
              </View>

              {a.labels.instance ? (
                <Text className="text-fog text-[10px] font-mono mt-0.5">{a.labels.instance}</Text>
              ) : null}

              {a.annotations.summary ? (
                <Text className="text-fog text-xs mt-1" numberOfLines={2}>{a.annotations.summary}</Text>
              ) : null}

              <Text className="text-fog text-[10px] font-mono mt-1">
                Started {fmtDuration(a.startsAt)} ago
              </Text>
            </View>
          ))
        )}
      </ScrollView>

      {/* Silence modal */}
      <Modal visible={!!silencing} transparent animationType="slide" onRequestClose={() => setSilencing(null)}>
        <View style={{ flex: 1, justifyContent: "flex-end", backgroundColor: "rgba(0,0,0,0.5)" }}>
          <View className="bg-panel border-t border-edge rounded-t-2xl p-5">
            <Text className="text-bone font-bold text-[17px] mb-1">Silence alert</Text>
            <Text className="text-fog text-sm mb-4" numberOfLines={1}>
              {silencing?.labels.alertname} · {silencing?.labels.instance ?? ""}
            </Text>

            <Text className="text-fog text-[10px] uppercase tracking-wider mb-2">Duration</Text>
            <View className="flex-row gap-2 mb-4">
              {SILENCE_DURATIONS.map((d) => (
                <TouchableOpacity
                  key={d.label}
                  onPress={() => setSilenceDur(d.minutes)}
                  className={`flex-1 py-2 rounded-lg border items-center ${silenceDur === d.minutes ? "border-ember" : "border-edge"}`}
                >
                  <Text className={`font-mono text-sm ${silenceDur === d.minutes ? "text-ember" : "text-fog"}`}>
                    {d.label}
                  </Text>
                </TouchableOpacity>
              ))}
            </View>

            <Text className="text-fog text-[10px] uppercase tracking-wider mb-2">Comment (optional)</Text>
            <TextInput
              className="bg-emboss border border-edge rounded-xl px-3 py-2.5 text-bone text-sm mb-4"
              placeholder="Reason for silence…"
              placeholderTextColor={colors.fog}
              value={silenceComment}
              onChangeText={setSilenceComment}
            />

            <View className="flex-row gap-3">
              <TouchableOpacity
                onPress={() => setSilencing(null)}
                className="flex-1 py-3 rounded-xl border border-edge items-center"
              >
                <Text className="text-fog text-sm">Cancel</Text>
              </TouchableOpacity>
              <TouchableOpacity
                onPress={() => void doSilence()}
                disabled={busy}
                className="flex-1 py-3 rounded-xl items-center"
                style={{ backgroundColor: busy ? colors.emboss : colors.ember }}
              >
                <Text className="text-black font-semibold text-sm">
                  {busy ? "Creating…" : "Create silence"}
                </Text>
              </TouchableOpacity>
            </View>
          </View>
        </View>
      </Modal>
    </>
  );
}

// ── Silences tab ──────────────────────────────────────────────────────────────

function SilencesTab() {
  const qc = useQueryClient();
  const [deleting, setDeleting] = useState<string | null>(null);

  const { data: silences = [], isLoading, isRefetching, refetch, error } =
    useQuery<AmSilence[]>({
      queryKey: ["am-silences"],
      queryFn: monitoringApi.silences,
      refetchInterval: 60_000,
    });

  const doDelete = useCallback(async (id: string) => {
    setDeleting(id);
    try {
      await monitoringApi.deleteSilence(id);
      void qc.invalidateQueries({ queryKey: ["am-silences"] });
    } catch {
      // ignore
    } finally {
      setDeleting(null);
    }
  }, [qc]);

  if (isLoading) return (
    <View className="flex-1 items-center justify-center">
      <ActivityIndicator color={colors.ember} />
    </View>
  );

  if (error) return (
    <View className="flex-1 items-center justify-center p-6">
      <WifiOff size={32} color={colors.fog} />
      <Text className="text-fog text-sm mt-3 text-center">Alertmanager unavailable</Text>
    </View>
  );

  const active = silences.filter((s) => s.status.state === "active");
  const expired = silences.filter((s) => s.status.state !== "active");

  return (
    <ScrollView
      contentContainerStyle={{ padding: 16, paddingBottom: 100 }}
      refreshControl={
        <RefreshControl refreshing={isRefetching} onRefresh={() => void refetch()} tintColor={colors.ember} />
      }
    >
      <Text className="text-fog text-xs font-mono mb-3">
        {active.length} active · {expired.length} expired
      </Text>

      {silences.length === 0 ? (
        <View className="items-center py-16">
          <VolumeX size={36} color={colors.fog} />
          <Text className="text-fog text-sm mt-3">No silences configured.</Text>
        </View>
      ) : (
        silences.map((s) => (
          <View key={s.id} className="bg-panel border border-edge rounded-xl p-4 mb-3">
            <View className="flex-row items-start justify-between gap-2">
              <View className="flex-1">
                <View className="flex-row items-center gap-2 mb-1">
                  <Text
                    className="font-mono text-[10px] uppercase"
                    style={{ color: s.status.state === "active" ? colors.moss : colors.fog }}
                  >
                    {s.status.state}
                  </Text>
                  <Text className="text-fog text-[10px] font-mono">by {s.createdBy}</Text>
                </View>
                <Text className="text-bone text-sm" numberOfLines={2}>{s.comment || "—"}</Text>
                <View className="flex-row flex-wrap gap-1 mt-1.5">
                  {s.matchers.slice(0, 3).map((m, i) => (
                    <View key={i} className="bg-emboss border border-edge rounded px-1.5 py-0.5">
                      <Text className="font-mono text-[10px] text-fog">
                        {m.name}{m.isRegex ? "=~" : "="}{m.value}
                      </Text>
                    </View>
                  ))}
                  {s.matchers.length > 3 && (
                    <Text className="text-fog text-[10px] font-mono">+{s.matchers.length - 3}</Text>
                  )}
                </View>
                <Text className="text-fog text-[10px] font-mono mt-1">
                  {new Date(s.endsAt).toLocaleString()}
                </Text>
              </View>
              {s.status.state === "active" && (
                <TouchableOpacity
                  onPress={() => void doDelete(s.id)}
                  disabled={deleting === s.id}
                  className="bg-emboss border border-edge rounded-lg px-2 py-1.5"
                  accessibilityRole="button"
                  accessibilityLabel="Expire silence"
                >
                  <Text className="text-brick text-xs font-mono">
                    {deleting === s.id ? "…" : "Expire"}
                  </Text>
                </TouchableOpacity>
              )}
            </View>
          </View>
        ))
      )}
    </ScrollView>
  );
}

// ── Main screen ───────────────────────────────────────────────────────────────

const TABS: { id: Tab; label: string }[] = [
  { id: "dashboard", label: "Dashboard" },
  { id: "alerts", label: "Alerts" },
  { id: "silences", label: "Silences" },
];

export default function MonitoringScreen() {
  const [tab, setTab] = useState<Tab>("dashboard");

  const { data: amAlerts = [] } = useQuery<AmAlert[]>({
    queryKey: ["am-alerts"],
    queryFn: monitoringApi.alerts,
    refetchInterval: 30_000,
  });
  const firingCount = amAlerts.filter((a) => a.status.state === "active").length;

  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: colors.abyss }}>
      {/* Header */}
      <View className="flex-row items-center justify-between px-4 py-3 border-b border-edge">
        <View className="flex-row items-center gap-2">
          <Activity size={18} color={colors.ember} />
          <Text className="font-head font-bold text-[18px] text-bone">Monitoring</Text>
          {firingCount > 0 && (
            <View className="bg-red-500 rounded-full px-2 py-0.5">
              <Text className="text-white text-xs font-bold">{firingCount}</Text>
            </View>
          )}
        </View>
      </View>

      {/* Tab bar */}
      <View className="flex-row px-4 py-2 gap-2 border-b border-edge">
        {TABS.map((t) => (
          <TouchableOpacity
            key={t.id}
            onPress={() => setTab(t.id)}
            className={`px-3 py-1 rounded-full border ${tab === t.id ? "border-ember" : "border-edge"}`}
            accessibilityRole="tab"
            accessibilityState={{ selected: tab === t.id }}
          >
            <Text className={`text-xs font-mono ${tab === t.id ? "text-ember" : "text-fog"}`}>
              {t.label}
            </Text>
          </TouchableOpacity>
        ))}
      </View>

      <View style={{ flex: 1 }}>
        {tab === "dashboard" && <DashboardTab />}
        {tab === "alerts" && <AlertsTab />}
        {tab === "silences" && <SilencesTab />}
      </View>
    </SafeAreaView>
  );
}
