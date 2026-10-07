import React, { useState } from "react";
import {
  View, Text, ScrollView, TouchableOpacity, RefreshControl,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, CheckCircle } from "lucide-react-native";
import { alertsApi } from "../../src/api/servers";
import { colors } from "../../src/theme/colors";
import type { Alert } from "../../src/types";

const SEVERITY_BG: Record<string, string> = {
  CRITICAL: "rgba(239,68,68,0.12)",
  WARNING: "rgba(234,179,8,0.12)",
  INFO: "rgba(59,130,246,0.12)",
};
const SEVERITY_BORDER: Record<string, string> = {
  CRITICAL: "rgba(239,68,68,0.4)",
  WARNING: "rgba(234,179,8,0.4)",
  INFO: "rgba(59,130,246,0.4)",
};
const SEVERITY_TEXT: Record<string, string> = {
  CRITICAL: "#f87171",
  WARNING: "#facc15",
  INFO: "#60a5fa",
};

function AlertCard({ alert, onResolve }: { alert: Alert; onResolve: () => void }) {
  const [busy, setBusy] = useState(false);

  const resolve = async () => {
    setBusy(true);
    try {
      await alertsApi.resolve(alert.id);
      onResolve();
    } catch {
      // ignore
    } finally {
      setBusy(false);
    }
  };

  return (
    <View
      style={{
        backgroundColor: SEVERITY_BG[alert.severity] ?? SEVERITY_BG.WARNING,
        borderColor: SEVERITY_BORDER[alert.severity] ?? SEVERITY_BORDER.WARNING,
        borderWidth: 1,
        borderRadius: 12,
        padding: 14,
        marginBottom: 10,
      }}
    >
      <View className="flex-row items-center justify-between mb-1">
        <View className="flex-row items-center gap-2">
          <AlertTriangle size={14} color={SEVERITY_TEXT[alert.severity] ?? SEVERITY_TEXT.WARNING} />
          <Text
            className="text-xs font-mono font-bold uppercase"
            style={{ color: SEVERITY_TEXT[alert.severity] ?? SEVERITY_TEXT.WARNING }}
          >
            {alert.severity}
          </Text>
          <Text className="text-fog text-xs font-mono">{alert.condition}</Text>
        </View>
        {alert.status === "TRIGGERED" && (
          <TouchableOpacity
            onPress={resolve}
            disabled={busy}
            accessibilityRole="button"
            accessibilityLabel="Resolve alert"
          >
            <CheckCircle size={16} color={busy ? colors.fog : "#22c55e"} />
          </TouchableOpacity>
        )}
      </View>
      <Text className="text-bone text-sm mb-1">{alert.message}</Text>
      <Text className="text-fog text-[10px] font-mono">
        {new Date(alert.triggeredAt).toLocaleString()}
        {alert.resolvedAt ? ` → resolved ${new Date(alert.resolvedAt).toLocaleString()}` : ""}
      </Text>
    </View>
  );
}

export default function AlertsScreen() {
  const qc = useQueryClient();
  const [tab, setTab] = useState<"TRIGGERED" | "RESOLVED">("TRIGGERED");

  const { data: alerts = [], isLoading, isRefetching, refetch } = useQuery<Alert[]>({
    queryKey: ["alerts", tab],
    queryFn: () => alertsApi.list(tab),
    refetchInterval: 30_000,
  });

  const triggered = alerts.filter((a) => a.status === "TRIGGERED");

  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: colors.abyss }}>
      <View className="flex-row items-center justify-between px-4 py-3 border-b border-edge">
        <View className="flex-row items-center gap-2">
          <AlertTriangle size={18} color={colors.ember} />
          <Text className="font-head font-bold text-[18px] text-bone">Alerts</Text>
          {triggered.length > 0 && (
            <View className="bg-red-500 rounded-full px-2 py-0.5">
              <Text className="text-white text-xs font-bold">{triggered.length}</Text>
            </View>
          )}
        </View>
      </View>

      {/* Tabs */}
      <View className="flex-row px-4 py-2 gap-2">
        {(["TRIGGERED", "RESOLVED"] as const).map((t) => (
          <TouchableOpacity
            key={t}
            onPress={() => setTab(t)}
            className={`px-3 py-1 rounded-full border ${tab === t ? "border-ember" : "border-edge"}`}
          >
            <Text className={`text-xs font-mono ${tab === t ? "text-ember" : "text-fog"}`}>{t}</Text>
          </TouchableOpacity>
        ))}
      </View>

      <ScrollView
        contentContainerStyle={{ padding: 16, paddingBottom: 100 }}
        refreshControl={
          <RefreshControl refreshing={isRefetching} onRefresh={() => void refetch()} tintColor={colors.ember} />
        }
      >
        {isLoading && (
          <View className="items-center py-8">
            <Text className="text-fog text-sm">Loading…</Text>
          </View>
        )}

        {!isLoading && alerts.length === 0 && (
          <View className="items-center py-16">
            <CheckCircle size={36} color={colors.fog} />
            <Text className="text-fog text-sm mt-3">No {tab.toLowerCase()} alerts.</Text>
          </View>
        )}

        {alerts.map((a) => (
          <AlertCard
            key={a.id}
            alert={a}
            onResolve={() => void qc.invalidateQueries({ queryKey: ["alerts"] })}
          />
        ))}
      </ScrollView>
    </SafeAreaView>
  );
}
