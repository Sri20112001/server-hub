import React from "react";
import { View, Text, FlatList, RefreshControl } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { useQuery } from "@tanstack/react-query";
import { dashboardApi } from "../../../../src/api/dashboard";
import { DeploymentRow } from "../../../../src/components/DeploymentRow";
import { SkeletonCard } from "../../../../src/components/SkeletonBox";
import { ErrorState } from "../../../../src/components/ErrorState";
import { EmptyState } from "../../../../src/components/EmptyState";
import { colors } from "../../../../src/theme/colors";

export default function DeploymentsScreen() {
  const { data: dash, isLoading, isError, error, refetch, isRefetching } =
    useQuery({
      queryKey: ["dashboard"],
      queryFn: dashboardApi.dashboard,
      refetchInterval: 30_000,
    });

  const projectMap = new Map(dash?.projects.map((p) => [p.id, p]) ?? []);
  const deployments = dash?.recentDeployments ?? [];

  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: colors.abyss }}>
      <View className="px-4 pt-3 pb-3 border-b border-edge">
        <Text className="font-head font-bold text-[20px] text-bone">
          Deployments
        </Text>
        <Text className="font-mono text-[11px] text-fog mt-0.5">
          {dash?.counts.deployments ?? "—"} launches logged
        </Text>
      </View>

      {isLoading && (
        <View className="p-4">
          {[0, 1, 2, 3].map((i) => (
            <SkeletonCard key={i} dark />
          ))}
        </View>
      )}

      {isError && (
        <View className="p-4">
          <ErrorState
            message={error instanceof Error ? error.message : "Failed to load deployments"}
            onRetry={() => void refetch()}
            dark
          />
        </View>
      )}

      {!isLoading && !isError && (
        <FlatList
          data={deployments}
          keyExtractor={(d) => String(d.id)}
          contentContainerStyle={{ padding: 16, paddingBottom: 32 }}
          refreshControl={
            <RefreshControl
              refreshing={isRefetching}
              onRefresh={() => void refetch()}
              tintColor={colors.ember}
            />
          }
          renderItem={({ item }) => (
            <DeploymentRow
              deployment={item}
              projectName={projectMap.get(item.projectId)?.name}
              dark
            />
          )}
          ListEmptyComponent={
            <EmptyState
              title="No launches recorded"
              hint="Deployments will appear here once projects are deployed."
              dark
            />
          }
        />
      )}
    </SafeAreaView>
  );
}
