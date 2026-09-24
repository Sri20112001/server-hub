import React, { useState, useMemo } from "react";
import {
  View,
  Text,
  FlatList,
  TextInput,
  TouchableOpacity,
  RefreshControl,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { router } from "expo-router";
import { useQuery } from "@tanstack/react-query";
import { dashboardApi } from "../../../../src/api/dashboard";
import { ShipCard } from "../../../../src/components/ShipCard";
import { SkeletonCard } from "../../../../src/components/SkeletonBox";
import { ErrorState } from "../../../../src/components/ErrorState";
import { EmptyState } from "../../../../src/components/EmptyState";
import { colors } from "../../../../src/theme/colors";
import { toFleetStatus } from "../../../../src/types";

type Filter = "all" | "sailing" | "choppy" | "lost" | "docked";

export default function FleetScreen() {
  const [search, setSearch] = useState("");
  const [filter, setFilter] = useState<Filter>("all");

  const { data: dash, isLoading, isError, error, refetch, isRefetching } =
    useQuery({
      queryKey: ["dashboard"],
      queryFn: dashboardApi.dashboard,
      refetchInterval: 30_000,
    });

  const projects = dash?.projects ?? [];

  const filtered = useMemo(() => {
    let list = projects;
    if (filter !== "all") {
      list = list.filter((p) => toFleetStatus(p.status) === filter);
    }
    if (search.trim()) {
      const q = search.toLowerCase();
      list = list.filter(
        (p) =>
          p.name.toLowerCase().includes(q) ||
          p.repository?.toLowerCase().includes(q),
      );
    }
    return list;
  }, [projects, filter, search]);

  const FILTERS: { key: Filter; label: string }[] = [
    { key: "all", label: "All" },
    { key: "sailing", label: "● Sailing" },
    { key: "choppy", label: "◐ Choppy" },
    { key: "lost", label: "○ Lost" },
    { key: "docked", label: "◌ Docked" },
  ];

  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: colors.abyss }}>
      <View className="px-4 pt-3 pb-2 border-b border-edge">
        <Text className="font-head font-bold text-[20px] text-bone mb-3">
          Fleet
        </Text>
        <TextInput
          value={search}
          onChangeText={setSearch}
          placeholder="Search ships…"
          placeholderTextColor={colors.fog}
          className="bg-emboss border border-edge rounded-input px-3 py-2.5 text-bone text-[13px] mb-3"
          style={{ fontFamily: "Inter_400Regular" }}
          accessibilityLabel="Search ships"
        />
        <View className="flex-row gap-2 flex-wrap">
          {FILTERS.map((f) => (
            <TouchableOpacity
              key={f.key}
              onPress={() => setFilter(f.key)}
              className={`rounded-full px-3 py-1 border ${
                filter === f.key
                  ? "bg-ember border-ember"
                  : "bg-emboss border-edge"
              }`}
              accessibilityRole="button"
              accessibilityLabel={`Filter: ${f.label}`}
              accessibilityState={{ selected: filter === f.key }}
            >
              <Text
                className={`font-mono text-[11px] ${
                  filter === f.key ? "text-black" : "text-fog"
                }`}
              >
                {f.label}
              </Text>
            </TouchableOpacity>
          ))}
        </View>
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
            message={error instanceof Error ? error.message : "Failed to load fleet"}
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
              onPress={() => router.push(`/(app)/projects/${item.id}`)}
              dark
            />
          )}
          ListEmptyComponent={
            <EmptyState
              title="No ships found"
              hint={
                search
                  ? "Try a different search term."
                  : "No ships match the selected filter."
              }
              dark
            />
          }
        />
      )}
    </SafeAreaView>
  );
}
