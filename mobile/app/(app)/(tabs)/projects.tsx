import React, { useState, useMemo } from "react";
import {
  View,
  Text,
  FlatList,
  TextInput,
  RefreshControl,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { router } from "expo-router";
import { useQuery } from "@tanstack/react-query";
import { projectsApi } from "../../../src/api/projects";
import { dashboardApi } from "../../../src/api/dashboard";
import { ShipCard } from "../../../src/components/ShipCard";
import { SkeletonCard } from "../../../src/components/SkeletonBox";
import { ErrorState } from "../../../src/components/ErrorState";
import { EmptyState } from "../../../src/components/EmptyState";
import { colors } from "../../../src/theme/colors";

export default function ProjectsScreen() {
  const [search, setSearch] = useState("");

  const { data: projects, isLoading, isError, error, refetch, isRefetching } =
    useQuery({
      queryKey: ["projects"],
      queryFn: projectsApi.list,
      refetchInterval: 30_000,
    });

  const { data: dash } = useQuery({
    queryKey: ["dashboard"],
    queryFn: dashboardApi.dashboard,
  });

  const recentByProject = new Map(
    dash?.recentDeployments.map((d) => [d.projectId, d]) ?? [],
  );

  const filtered = useMemo(() => {
    if (!projects) return [];
    if (!search.trim()) return projects;
    const q = search.toLowerCase();
    return projects.filter(
      (p) =>
        p.name.toLowerCase().includes(q) ||
        p.repository?.toLowerCase().includes(q),
    );
  }, [projects, search]);

  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: colors.abyss }}>
      <View className="px-4 pt-3 pb-2 border-b border-edge">
        <Text className="font-head font-bold text-[20px] text-bone mb-3">
          Projects
        </Text>
        <TextInput
          value={search}
          onChangeText={setSearch}
          placeholder="Search projects…"
          placeholderTextColor={colors.fog}
          className="bg-emboss border border-edge rounded-input px-3 py-2.5 text-bone text-[13px]"
          style={{ fontFamily: "Inter_400Regular" }}
          accessibilityLabel="Search projects"
        />
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
            message={error instanceof Error ? error.message : "Failed to load projects"}
            onRetry={() => void refetch()}
            dark
          />
        </View>
      )}

      {!isLoading && !isError && (
        <FlatList
          data={filtered}
          keyExtractor={(p) => String(p.id)}
          contentContainerStyle={{ padding: 16, paddingBottom: 32 }}
          refreshControl={
            <RefreshControl
              refreshing={isRefetching}
              onRefresh={() => void refetch()}
              tintColor={colors.ember}
            />
          }
          renderItem={({ item }) => (
            <ShipCard
              project={item}
              lastDeployment={recentByProject.get(item.id)}
              onPress={() => router.push(`/(app)/projects/${item.id}`)}
              dark
            />
          )}
          ListEmptyComponent={
            <EmptyState
              title="No projects found"
              hint={search ? "Try a different search term." : "No projects registered yet."}
              dark
            />
          }
        />
      )}
    </SafeAreaView>
  );
}
