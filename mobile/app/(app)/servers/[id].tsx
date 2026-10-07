import React, { useState } from "react";
import {
  View, Text, ScrollView, TouchableOpacity, RefreshControl,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { router, useLocalSearchParams } from "expo-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, AlertTriangle } from "lucide-react-native";
import { serversApi, alertsApi } from "../../../src/api/servers";
import { colors } from "../../../src/theme/colors";

const RANGES = ["1h", "6h", "24h", "7d"] as const;

function MetricBar({ label, pct, value }: { label: string; pct: number; value: string }) {
  const barColor = pct > 85 ? "#ef4444" : pct > 70 ? "#eab308" : "#22c55e";
  const widthPct = `${Math.min(Math.max(pct, 0), 100)}%` as `${number}%`;
  return (
    <View className="mb-3">
      <View className="flex-row justify-between mb-1">
        <Text className="text-fog text-xs">{label}</Text>
        <Text className="text-bone text-xs font-mono">{value}</Text>
      </View>
      <View className="h-1.5 bg-emboss rounded-full overflow-hidden">
        <View style={{ width: widthPct, height: "100%", backgroundColor: barColor, borderRadius: 999 }} />
      </View>
    </View>
  );
}

function fmtUptime(sec: number): string {
  if (sec < 60) return `${sec}s`;
  if (sec < 3600) return `${Math.floor(sec / 60)}m`;
  if (sec < 86400) return `${Math.floor(sec / 3600)}h ${Math.floor((sec % 3600) / 60)}m`;
  return `${Math.floor(sec / 86400)}d ${Math.floor((sec % 86400) / 3600)}h`;
}

export default function ServerDetailScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const serverId = Number(id);
  const qc = useQueryClient();
  const [range, setRange] = useState<string>("1h");

  const { data: server, isLoading, isRefetching, refetch } = useQuery({
    queryKey: ["server", serverId],
    queryFn: () => serversApi.get(serverId),
    refetchInterval: 30_000,
  });

  const { data: latest } = useQuery({
    queryKey: ["server-metrics-latest", serverId],
    queryFn: () => serversApi.metricsLatest(serverId),
    refetchInterval: 30_000,
  });

  const { data: alerts = [] } = useQuery({
    queryKey: ["alerts", "TRIGGERED"],
    queryFn: () => alertsApi.list("TRIGGERED"),
    refetchInterval: 30_000,
  });

  const serverAlerts = alerts.filter((a) => a.serverId === serverId);

  const STATUS_COLOR: Record<string, string> = {
    ONLINE: "#22c55e", OFFLINE: "#ef4444", WARNING: "#eab308", UNKNOWN: "#71717a",
  };

  if (isLoading) {
    return (
      <SafeAreaView style={{ flex: 1, backgroundColor: colors.abyss }}>
        <View className="px-4 pt-3 pb-2 border-b border-edge">
          <TouchableOpacity onPress={() => router.back()} accessibilityLabel="Go back">
            <View className="flex-row items-center gap-1">
              <ArrowLeft size={15} color={colors.ember} />
              <Text className="text-ember text-sm">Servers</Text>
            </View>
          </TouchableOpacity>
        </View>
        <View className="p-4 space-y-3">
          {[1, 2, 3].map((i) => <View key={i} className="h-20 bg-panel rounded-xl" />)}
        </View>
      </SafeAreaView>
    );
  }

  if (!server) {
    return (
      <SafeAreaView style={{ flex: 1, backgroundColor: colors.abyss }}>
        <View className="p-4">
          <Text className="text-fog">Server not found.</Text>
        </View>
      </SafeAreaView>
    );
  }

  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: colors.abyss }}>
      {/* Header */}
      <View className="px-4 pt-3 pb-3 border-b border-edge">
        <TouchableOpacity onPress={() => router.back()} className="mb-2" accessibilityLabel="Go back">
          <View className="flex-row items-center gap-1">
            <ArrowLeft size={13} color={colors.ember} />
            <Text className="text-ember text-xs font-mono">Servers</Text>
          </View>
        </TouchableOpacity>
        <View className="flex-row items-center gap-2">
          <View style={{ width: 8, height: 8, borderRadius: 4, backgroundColor: STATUS_COLOR[server.status] ?? "#71717a" }} />
          <Text className="font-head font-bold text-[20px] text-bone flex-1" numberOfLines={1}>{server.name}</Text>
          <Text className="font-mono text-xs" style={{ color: STATUS_COLOR[server.status] ?? "#71717a" }}>{server.status}</Text>
        </View>
        {(server.hostname || server.ipAddress) && (
          <Text className="text-fog text-xs font-mono mt-0.5">{server.hostname || server.ipAddress}</Text>
        )}
      </View>

      <ScrollView
        contentContainerStyle={{ padding: 16, paddingBottom: 100 }}
        refreshControl={<RefreshControl refreshing={isRefetching} onRefresh={() => void refetch()} tintColor={colors.ember} />}
      >
        {/* Active alerts */}
        {serverAlerts.map((a) => (
          <View key={a.id} style={{
            backgroundColor: a.severity === "CRITICAL" ? "rgba(239,68,68,0.12)" : "rgba(234,179,8,0.12)",
            borderColor: a.severity === "CRITICAL" ? "rgba(239,68,68,0.4)" : "rgba(234,179,8,0.4)",
            borderWidth: 1, borderRadius: 10, padding: 12, marginBottom: 10,
          }}>
            <View className="flex-row items-center gap-2">
              <AlertTriangle size={14} color={a.severity === "CRITICAL" ? "#f87171" : "#facc15"} />
              <Text className="text-sm flex-1" style={{ color: a.severity === "CRITICAL" ? "#f87171" : "#facc15" }}>
                {a.message}
              </Text>
            </View>
            <TouchableOpacity onPress={async () => {
              await alertsApi.resolve(a.id).catch(() => null);
              void qc.invalidateQueries({ queryKey: ["alerts"] });
            }} className="mt-1">
              <Text className="text-xs underline opacity-70" style={{ color: a.severity === "CRITICAL" ? "#f87171" : "#facc15" }}>
                Resolve
              </Text>
            </TouchableOpacity>
          </View>
        ))}

        {/* System info */}
        <View className="grid grid-cols-2 gap-3 mb-4">
          <View className="flex-row flex-wrap gap-2 mb-4">
            {[
              { label: "OS", value: server.os ? `${server.os} ${server.osVersion}` : "—" },
              { label: "Arch", value: server.arch || "—" },
              { label: "CPU", value: server.cpuCores > 0 ? `${server.cpuCores} cores` : "—" },
              { label: "RAM", value: server.ramTotal > 0 ? `${Math.round(server.ramTotal / 1024 / 1024 / 1024)}GB` : "—" },
            ].map((item) => (
              <View key={item.label} className="bg-panel border border-edge rounded-xl p-3 flex-1 min-w-[45%]">
                <Text className="text-fog text-[10px] uppercase tracking-wider">{item.label}</Text>
                <Text className="text-bone font-mono text-xs mt-0.5" numberOfLines={1}>{item.value}</Text>
              </View>
            ))}
          </View>
        </View>

        {/* Live metrics */}
        {latest ? (
          <View className="bg-panel border border-edge rounded-xl p-4 mb-4">
            <View className="flex-row items-center justify-between mb-3">
              <Text className="text-bone font-semibold text-sm">Live Metrics</Text>
              <Text className="text-fog text-[10px] font-mono">{new Date(latest.timestamp).toLocaleTimeString()}</Text>
            </View>
            <MetricBar label="CPU" pct={latest.cpuUsage} value={`${latest.cpuUsage.toFixed(1)}%`} />
            <MetricBar label="Memory" pct={latest.memoryUsage} value={`${latest.memoryUsage.toFixed(1)}%`} />
            <MetricBar label="Disk" pct={latest.diskUsage} value={`${latest.diskUsage.toFixed(1)}%`} />
            <View className="flex-row gap-2 mt-2">
              <View className="flex-1 bg-emboss rounded-lg p-2">
                <Text className="text-fog text-[10px]">Load Avg</Text>
                <Text className="text-bone font-mono text-xs">{latest.loadAvg1.toFixed(2)}</Text>
              </View>
              <View className="flex-1 bg-emboss rounded-lg p-2">
                <Text className="text-fog text-[10px]">Uptime</Text>
                <Text className="text-bone font-mono text-xs">{fmtUptime(latest.uptimeSec)}</Text>
              </View>
              <View className="flex-1 bg-emboss rounded-lg p-2">
                <Text className="text-fog text-[10px]">Agent</Text>
                <Text className="font-mono text-xs" style={{ color: server.agentStatus === "CONNECTED" ? "#22c55e" : "#71717a" }}>
                  {server.agentStatus}
                </Text>
              </View>
            </View>
          </View>
        ) : (
          <View className="bg-panel border border-edge rounded-xl p-6 mb-4 items-center">
            <Text className="text-fog text-sm">No metrics yet. Install the agent to begin monitoring.</Text>
          </View>
        )}

        {/* Range selector */}
        <View className="flex-row gap-2 mb-4">
          {RANGES.map((r) => (
            <TouchableOpacity key={r} onPress={() => setRange(r)}
              className={`px-3 py-1 rounded-full border ${range === r ? "border-ember" : "border-edge"}`}>
              <Text className={`text-xs font-mono ${range === r ? "text-ember" : "text-fog"}`}>{r}</Text>
            </TouchableOpacity>
          ))}
        </View>

        {/* Last heartbeat */}
        <View className="bg-panel border border-edge rounded-xl p-4">
          <Text className="text-fog text-xs uppercase tracking-wider mb-2">Connection</Text>
          <View className="flex-row justify-between py-1.5 border-b border-edge">
            <Text className="text-fog text-sm">Last heartbeat</Text>
            <Text className="text-bone text-sm font-mono">
              {server.lastHeartbeat ? new Date(server.lastHeartbeat).toLocaleString() : "Never"}
            </Text>
          </View>
          <View className="flex-row justify-between py-1.5">
            <Text className="text-fog text-sm">Registered</Text>
            <Text className="text-bone text-sm font-mono">{new Date(server.createdAt).toLocaleDateString()}</Text>
          </View>
        </View>
      </ScrollView>
    </SafeAreaView>
  );
}
