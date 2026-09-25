import React from "react";
import {
  View,
  Text,
  ScrollView,
  RefreshControl,
  TouchableOpacity,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { router } from "expo-router";
import { useQuery } from "@tanstack/react-query";
import { RefreshCw, ArrowRight, ChevronRight, Check, X } from "lucide-react-native";
import { dashboardApi } from "../../../src/api/dashboard";
import { useAuthStore } from "../../../src/stores/authStore";
import { Meter } from "../../../src/components/Meter";
import { ShipCard } from "../../../src/components/ShipCard";
import { DeploymentRow } from "../../../src/components/DeploymentRow";
import { SkeletonCard } from "../../../src/components/SkeletonBox";
import { ErrorState } from "../../../src/components/ErrorState";
import { EmptyState } from "../../../src/components/EmptyState";
import { colors } from "../../../src/theme/colors";
import { greeting, fmtUptime } from "../../../src/utils/format";

export default function DashboardScreen() {
  const user = useAuthStore((s) => s.user);

  const { data, isLoading, isError, error, refetch, isRefetching } = useQuery({
    queryKey: ["dashboard"],
    queryFn: dashboardApi.dashboard,
    refetchInterval: 30_000,
  });

  const projectMap = new Map(data?.projects.map((p) => [p.id, p]) ?? []);
  const recentByProject = new Map(
    data?.recentDeployments.map((d) => [d.projectId, d]) ?? [],
  );

  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: colors.abyss }}>
      <View className="flex-row items-center justify-between px-4 py-3 border-b border-edge">
        <View>
          <Text className="font-head font-bold text-[20px] text-bone">
            Server Hub
          </Text>
          <Text className="text-fog text-[11px] font-mono">
            {greeting()}, {user?.username ?? "Captain"}
          </Text>
        </View>
        <TouchableOpacity
          onPress={() => void refetch()}
          className="w-9 h-9 rounded-full bg-emboss items-center justify-center"
          accessibilityLabel="Refresh dashboard"
          accessibilityRole="button"
        >
          <RefreshCw size={18} color={colors.fog} />
        </TouchableOpacity>
      </View>

      <ScrollView
        className="flex-1"
        contentContainerStyle={{ padding: 16, paddingBottom: 32 }}
        refreshControl={
          <RefreshControl
            refreshing={isRefetching}
            onRefresh={() => void refetch()}
            tintColor={colors.ember}
          />
        }
      >
        {isLoading && (
          <>
            <SkeletonCard dark />
            <SkeletonCard dark />
            <SkeletonCard dark />
          </>
        )}

        {isError && (
          <ErrorState
            message={
              error instanceof Error
                ? error.message
                : "Unable to connect to Server Hub"
            }
            onRetry={() => void refetch()}
            dark
          />
        )}

        {data && (
          <>
            {/* Infrastructure summary */}
            <View className="flex-row gap-3 mb-4">
              <View className="flex-1 bg-panel border border-edge rounded-card p-4">
                <Text className="text-[10px] font-semibold uppercase tracking-widest text-fog mb-1">
                  Fleet
                </Text>
                <Text className="font-head font-bold text-[28px] text-bone">
                  {data.counts.projects}
                </Text>
                <Text className="font-mono text-[11px] text-fog">
                  {data.counts.services} stations
                </Text>
              </View>
              <View className="flex-1 bg-panel border border-edge rounded-card p-4">
                <Text className="text-[10px] font-semibold uppercase tracking-widest text-fog mb-1">
                  Health
                </Text>
                <View className="flex-row items-center gap-1.5 mb-0.5">
                  <View className="w-1.5 h-1.5 rounded-full bg-moss" />
                  <Text className="font-mono text-[12px] text-bone">
                    {data.counts.healthy} sailing
                  </Text>
                </View>
                <View className="flex-row items-center gap-1.5 mb-0.5">
                  <View className="w-1.5 h-1.5 rounded-full bg-status-amber" />
                  <Text className="font-mono text-[12px] text-bone">
                    {data.counts.degraded} choppy
                  </Text>
                </View>
                <View className="flex-row items-center gap-1.5">
                  <View className="w-1.5 h-1.5 rounded-full bg-brick" />
                  <Text className="font-mono text-[12px] text-bone">
                    {data.counts.down} lost
                  </Text>
                </View>
              </View>
            </View>

            {/* Server load */}
            <View className="bg-panel border border-edge rounded-card p-4 mb-4">
              <View className="flex-row items-center justify-between mb-3">
                <Text className="text-[10px] font-semibold uppercase tracking-widest text-fog">
                  Server load
                </Text>
                <TouchableOpacity
                  onPress={() => router.push("/(app)/logs")}
                  accessibilityRole="button"
                  accessibilityLabel="View logs"
                >
                  <View className="flex-row items-center gap-1">
                    <Text className="text-ember text-[11px] font-mono">
                      Logs
                    </Text>
                    <ArrowRight size={12} color={colors.ember} />
                  </View>
                </TouchableOpacity>
              </View>
              <Meter
                label="CPU"
                pct={data.server.cpuPercent}
                display={`${Math.round(data.server.cpuPercent)}%`}
                dark
              />
              <Meter
                label="RAM"
                pct={data.server.memPercent}
                display={`${Math.round(data.server.memPercent)}%`}
                dark
              />
              <Meter
                label="Disk"
                pct={data.server.diskPercent}
                display={`${Math.round(data.server.diskPercent)}%`}
                dark
              />
              <View className="flex-row items-center gap-1.5 mt-2">
                <Text className="font-mono text-[11px] text-fog">
                  Uptime: {fmtUptime(data.server.uptimeSec)}
                </Text>
                <Text className="font-mono text-[11px] text-fog">·</Text>
                <Text className="font-mono text-[11px] text-fog">
                  Shipyard
                </Text>
                {data.dockerAvailable ? (
                  <Check size={12} color={colors.moss} />
                ) : (
                  <X size={12} color={colors.brick} />
                )}
              </View>
            </View>

            {/* Fleet preview */}
            <View className="flex-row items-center justify-between mb-3">
              <Text className="font-head font-bold text-[18px] text-bone">
                Fleet
              </Text>
              <TouchableOpacity
                onPress={() => router.navigate("/(app)/(tabs)/fleet")}
                accessibilityRole="button"
                accessibilityLabel="View all ships"
              >
                <View className="flex-row items-center gap-0.5">
                  <Text className="text-ember text-[13px] font-mono">
                    All {data.projects.length}
                  </Text>
                  <ChevronRight size={14} color={colors.ember} />
                </View>
              </TouchableOpacity>
            </View>

            {data.projects.length === 0 ? (
              <EmptyState
                title="No ships in the fleet"
                hint="Register a project from the web console."
                dark
              />
            ) : (
              data.projects.slice(0, 4).map((p) => (
                <ShipCard
                  key={p.id}
                  project={p}
                  lastDeployment={recentByProject.get(p.id)}
                  onPress={() => router.push(`/(app)/projects/${p.id}`)}
                  dark
                />
              ))
            )}

            {/* Recent deployments */}
            <View className="flex-row items-center justify-between mt-4 mb-3">
              <Text className="font-head font-bold text-[18px] text-bone">
                Recent launches
              </Text>
              <TouchableOpacity
                onPress={() => router.navigate("/(app)/(tabs)/deployments")}
                accessibilityRole="button"
                accessibilityLabel="View all deployments"
              >
                <View className="flex-row items-center gap-0.5">
                  <Text className="text-ember text-[13px] font-mono">All</Text>
                  <ChevronRight size={14} color={colors.ember} />
                </View>
              </TouchableOpacity>
            </View>

            {data.recentDeployments.length === 0 ? (
              <EmptyState
                title="No launches recorded"
                hint="Deployments will appear here."
                dark
              />
            ) : (
              data.recentDeployments.slice(0, 5).map((d) => (
                <DeploymentRow
                  key={d.id}
                  deployment={d}
                  projectName={projectMap.get(d.projectId)?.name}
                  dark
                />
              ))
            )}
          </>
        )}
      </ScrollView>
    </SafeAreaView>
  );
}
