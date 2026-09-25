import React, { useState } from "react";
import {
  View,
  Text,
  ScrollView,
  TouchableOpacity,
  RefreshControl,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { router, useLocalSearchParams } from "expo-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import * as Haptics from "expo-haptics";
import { ArrowLeft } from "lucide-react-native";
import { projectsApi } from "../../../src/api/projects";
import { StatusBadge } from "../../../src/components/StatusBadge";
import { ConfirmSheet } from "../../../src/components/ConfirmSheet";
import { DeploymentRow } from "../../../src/components/DeploymentRow";
import { SkeletonCard } from "../../../src/components/SkeletonBox";
import { ErrorState } from "../../../src/components/ErrorState";
import { EmptyState } from "../../../src/components/EmptyState";
import { colors } from "../../../src/theme/colors";
import { toFleetStatus, fleetLabel } from "../../../src/types";
import { timeAgo } from "../../../src/utils/format";

type Action = "start" | "stop" | "restart";

const ACTION_LABELS: Record<Action, string> = {
  start: "Start",
  stop: "Stop",
  restart: "Restart",
};

const ACTION_BODY: Record<Action, string> = {
  start: "This will start all services for this project.",
  stop: "This will stop all services. The project will be unavailable.",
  restart: "Services will be temporarily unavailable during restart.",
};

export default function ProjectDetailScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const projectId = Number(id);
  const qc = useQueryClient();
  const [confirm, setConfirm] = useState<Action | null>(null);

  const { data: project, isLoading, isError, error, refetch, isRefetching } =
    useQuery({
      queryKey: ["project", projectId],
      queryFn: () => projectsApi.get(projectId),
    });

  const { data: services } = useQuery({
    queryKey: ["services", projectId],
    queryFn: () => projectsApi.services(projectId),
    enabled: !!projectId,
  });

  const { data: deployments } = useQuery({
    queryKey: ["deployments", projectId],
    queryFn: () => projectsApi.deployments(projectId),
    enabled: !!projectId,
  });

  const actionMutation = useMutation({
    mutationFn: (action: Action) =>
      projectsApi.action(projectId, action, action !== "start"),
    onSuccess: () => {
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
      void qc.invalidateQueries({ queryKey: ["project", projectId] });
      void qc.invalidateQueries({ queryKey: ["dashboard"] });
      setConfirm(null);
    },
    onError: () => {
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Error);
    },
  });

  if (isLoading) {
    return (
      <SafeAreaView style={{ flex: 1, backgroundColor: colors.abyss }}>
        <View className="px-4 pt-3 pb-2 border-b border-edge flex-row items-center gap-3">
          <TouchableOpacity onPress={() => router.back()} accessibilityLabel="Go back">
            <View className="flex-row items-center gap-1">
              <ArrowLeft size={15} color={colors.ember} />
              <Text className="text-ember text-[15px]">Back</Text>
            </View>
          </TouchableOpacity>
        </View>
        <View className="p-4">
          <SkeletonCard dark />
          <SkeletonCard dark />
        </View>
      </SafeAreaView>
    );
  }

  if (isError || !project) {
    return (
      <SafeAreaView style={{ flex: 1, backgroundColor: colors.abyss }}>
        <View className="px-4 pt-3 pb-2 border-b border-edge">
          <TouchableOpacity onPress={() => router.back()} accessibilityLabel="Go back">
            <View className="flex-row items-center gap-1">
              <ArrowLeft size={15} color={colors.ember} />
              <Text className="text-ember text-[15px]">Back</Text>
            </View>
          </TouchableOpacity>
        </View>
        <View className="p-4">
          <ErrorState
            message={error instanceof Error ? error.message : "Failed to load project"}
            onRetry={() => void refetch()}
            dark
          />
        </View>
      </SafeAreaView>
    );
  }

  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: colors.abyss }}>
      {/* Header */}
      <View className="px-4 pt-3 pb-3 border-b border-edge">
        <TouchableOpacity
          onPress={() => router.back()}
          className="mb-2"
          accessibilityLabel="Go back"
        >
          <View className="flex-row items-center gap-1">
            <ArrowLeft size={13} color={colors.ember} />
            <Text className="text-ember text-[13px] font-mono">Fleet</Text>
          </View>
        </TouchableOpacity>
        <View className="flex-row items-start justify-between">
          <View className="flex-1">
            <Text className="font-head font-bold text-[22px] text-bone" numberOfLines={1}>
              {project.name}
            </Text>
            {project.repository && (
              <Text className="font-mono text-[11px] text-fog mt-0.5" numberOfLines={1}>
                ~/{project.repository}
              </Text>
            )}
          </View>
          <StatusBadge status={project.status} dark />
        </View>
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
        {/* Info */}
        <View className="bg-panel border border-edge rounded-card p-4 mb-4">
          <Text className="text-[10px] font-semibold uppercase tracking-widest text-fog mb-3">
            Project info
          </Text>
          <View className="flex-row flex-wrap gap-x-6 gap-y-2">
            {project.environment && (
              <View>
                <Text className="text-[10px] uppercase tracking-widest text-fog">
                  Environment
                </Text>
                <Text className="font-mono text-[13px] text-bone">
                  {project.environment}
                </Text>
              </View>
            )}
            {project.branch && (
              <View>
                <Text className="text-[10px] uppercase tracking-widest text-fog">
                  Branch
                </Text>
                <Text className="font-mono text-[13px] text-bone">
                  ⎇ {project.branch}
                </Text>
              </View>
            )}
            <View>
              <Text className="text-[10px] uppercase tracking-widest text-fog">
                Updated
              </Text>
              <Text className="font-mono text-[13px] text-bone">
                {timeAgo(project.updatedAt)}
              </Text>
            </View>
            <View>
              <Text className="text-[10px] uppercase tracking-widest text-fog">
                Auto-deploy
              </Text>
              <Text className="font-mono text-[13px] text-bone">
                {project.autoDeploy ? "On" : "Off"}
              </Text>
            </View>
          </View>
        </View>

        {/* Services */}
        <View className="bg-panel border border-edge rounded-card p-4 mb-4">
          <Text className="text-[10px] font-semibold uppercase tracking-widest text-fog mb-3">
            Services ({services?.length ?? 0})
          </Text>
          {!services || services.length === 0 ? (
            <Text className="text-fog text-[13px]">No services charted.</Text>
          ) : (
            services.map((s) => {
              const sf = toFleetStatus(s.status);
              const dotColor =
                sf === "sailing"
                  ? colors.moss
                  : sf === "choppy"
                  ? colors.statusAmber
                  : sf === "lost"
                  ? colors.brick
                  : colors.stone;
              return (
                <View
                  key={s.id}
                  className="flex-row items-center justify-between py-2.5 border-b border-edge last:border-0"
                >
                  <View className="flex-row items-center gap-2">
                    <View
                      style={{ backgroundColor: dotColor }}
                      className="w-2 h-2 rounded-full"
                    />
                    <Text className="text-bone text-[13px]">{s.name}</Text>
                    {s.type && (
                      <Text className="font-mono text-[11px] text-fog">
                        {s.type}
                      </Text>
                    )}
                  </View>
                  <View className="items-end">
                    <Text className="font-mono text-[11px] text-fog">
                      {fleetLabel[sf]}
                    </Text>
                    {s.responseTimeMs !== undefined && (
                      <Text className="font-mono text-[10px] text-fog">
                        {s.responseTimeMs}ms
                      </Text>
                    )}
                  </View>
                </View>
              );
            })
          )}
        </View>

        {/* Actions */}
        <View className="bg-panel border border-edge rounded-card p-4 mb-4">
          <Text className="text-[10px] font-semibold uppercase tracking-widest text-fog mb-3">
            Actions
          </Text>
          <View className="flex-row gap-2 flex-wrap">
            {(["start", "stop", "restart"] as Action[]).map((action) => (
              <TouchableOpacity
                key={action}
                onPress={() => {
                  void Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Medium);
                  setConfirm(action);
                }}
                disabled={actionMutation.isPending}
                className={`rounded-input px-4 py-2 border ${
                  action === "stop" || action === "restart"
                    ? "border-brick"
                    : "border-edge"
                }`}
                accessibilityRole="button"
                accessibilityLabel={`${ACTION_LABELS[action]} project`}
              >
                <Text
                  className={`text-[13px] font-medium ${
                    action === "stop" || action === "restart"
                      ? "text-brick"
                      : "text-bone"
                  }`}
                >
                  {ACTION_LABELS[action]}
                </Text>
              </TouchableOpacity>
            ))}
            <TouchableOpacity
              onPress={() =>
                router.push({
                  pathname: "/(app)/logs",
                  params: { projectId: String(projectId) },
                })
              }
              className="rounded-input px-4 py-2 border border-edge"
              accessibilityRole="button"
              accessibilityLabel="View logs"
            >
              <Text className="text-bone text-[13px] font-medium">Logs</Text>
            </TouchableOpacity>
          </View>
          {actionMutation.isError && (
            <Text className="text-brick text-[12px] mt-2">
              {actionMutation.error instanceof Error
                ? actionMutation.error.message
                : "Action failed"}
            </Text>
          )}
        </View>

        {/* Recent deployments */}
        <Text className="font-head font-bold text-[16px] text-bone mb-3">
          Recent launches
        </Text>
        {!deployments || deployments.length === 0 ? (
          <EmptyState title="No launches recorded" dark />
        ) : (
          deployments.slice(0, 5).map((d) => (
            <DeploymentRow
              key={d.id}
              deployment={d}
              projectName={project.name}
              dark
            />
          ))
        )}
      </ScrollView>

      {/* Confirm sheet */}
      {confirm && (
        <ConfirmSheet
          visible
          title={`${ACTION_LABELS[confirm]} ${project.name}?`}
          body={ACTION_BODY[confirm]}
          confirmLabel={`${ACTION_LABELS[confirm]} project`}
          onCancel={() => setConfirm(null)}
          onConfirm={() => actionMutation.mutate(confirm)}
          busy={actionMutation.isPending}
          dark
        />
      )}
    </SafeAreaView>
  );
}
