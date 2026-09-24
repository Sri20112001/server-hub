import React, { useState } from "react";
import {
  View,
  Text,
  FlatList,
  TouchableOpacity,
  RefreshControl,
  ScrollView,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { router, useLocalSearchParams } from "expo-router";
import { useQuery } from "@tanstack/react-query";
import { logsApi } from "../../../src/api/logs";
import { SkeletonCard } from "../../../src/components/SkeletonBox";
import { ErrorState } from "../../../src/components/ErrorState";
import { EmptyState } from "../../../src/components/EmptyState";
import { colors } from "../../../src/theme/colors";
import type { AppLog } from "../../../src/types";

type Level = "all" | "info" | "warn" | "error";

const LEVEL_COLOR: Record<string, string> = {
  info: colors.moss,
  warn: colors.statusAmber,
  error: colors.brick,
  debug: colors.fog,
};

function LogLine({ log }: { log: AppLog }) {
  const levelColor = LEVEL_COLOR[log.level?.toLowerCase()] ?? colors.fog;
  const ts = log.timestamp?.slice(11, 19) ?? "--:--:--";

  return (
    <View className="py-1 border-b border-edge/50">
      <ScrollView horizontal showsHorizontalScrollIndicator={false}>
        <Text
          style={{
            fontFamily: "JetBrainsMono_400Regular",
            fontSize: 11,
            color: colors.bone,
          }}
        >
          <Text style={{ color: colors.fog }}>{ts} </Text>
          <Text style={{ color: levelColor }}>
            {(log.level ?? "INFO").toUpperCase().padEnd(5)}{" "}
          </Text>
          <Text style={{ color: colors.fog }}>{log.source} </Text>
          {log.message}
        </Text>
      </ScrollView>
    </View>
  );
}

export default function LogsScreen() {
  const { projectId } = useLocalSearchParams<{ projectId?: string }>();
  const [level, setLevel] = useState<Level>("all");

  const { data: logs, isLoading, isError, error, refetch, isRefetching } =
    useQuery({
      queryKey: ["logs", level, projectId],
      queryFn: () =>
        logsApi.list({
          level: level === "all" ? undefined : level,
          projectId: projectId ? Number(projectId) : undefined,
          limit: 100,
        }),
      refetchInterval: 15_000,
    });

  const LEVELS: { key: Level; label: string }[] = [
    { key: "all", label: "All" },
    { key: "info", label: "Info" },
    { key: "warn", label: "Warn" },
    { key: "error", label: "Error" },
  ];

  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: colors.abyss }}>
      <View className="px-4 pt-3 pb-2 border-b border-edge">
        <View className="flex-row items-center gap-3 mb-2">
          <TouchableOpacity onPress={() => router.back()} accessibilityLabel="Go back">
            <Text className="text-ember text-[13px] font-mono">← Back</Text>
          </TouchableOpacity>
          <Text className="font-head font-bold text-[18px] text-bone">
            Logs
          </Text>
          {projectId && (
            <View className="bg-emboss border border-edge rounded-md px-2 py-0.5">
              <Text className="font-mono text-[11px] text-fog">
                project #{projectId}
              </Text>
            </View>
          )}
        </View>
        <View className="flex-row gap-2">
          {LEVELS.map((l) => (
            <TouchableOpacity
              key={l.key}
              onPress={() => setLevel(l.key)}
              className={`rounded-full px-3 py-1 border ${
                level === l.key ? "bg-ember border-ember" : "bg-emboss border-edge"
              }`}
              accessibilityRole="button"
              accessibilityLabel={`Filter: ${l.label}`}
              accessibilityState={{ selected: level === l.key }}
            >
              <Text
                className={`font-mono text-[11px] ${
                  level === l.key ? "text-black" : "text-fog"
                }`}
              >
                {l.label}
              </Text>
            </TouchableOpacity>
          ))}
          <TouchableOpacity
            onPress={() => void refetch()}
            className="ml-auto rounded-full px-3 py-1 border border-edge bg-emboss"
            accessibilityRole="button"
            accessibilityLabel="Refresh logs"
          >
            <Text className="font-mono text-[11px] text-fog">↻</Text>
          </TouchableOpacity>
        </View>
      </View>

      {isLoading && (
        <View className="p-4">
          {[0, 1, 2].map((i) => (
            <SkeletonCard key={i} dark />
          ))}
        </View>
      )}

      {isError && (
        <View className="p-4">
          <ErrorState
            message={error instanceof Error ? error.message : "Failed to load logs"}
            onRetry={() => void refetch()}
            dark
          />
        </View>
      )}

      {!isLoading && !isError && (
        <FlatList
          data={logs ?? []}
          keyExtractor={(l) => String(l.id)}
          contentContainerStyle={{ padding: 12, paddingBottom: 32 }}
          refreshControl={
            <RefreshControl
              refreshing={isRefetching}
              onRefresh={() => void refetch()}
              tintColor={colors.ember}
            />
          }
          renderItem={({ item }) => <LogLine log={item} />}
          ListEmptyComponent={
            <EmptyState
              title="No log entries"
              hint="Logs will appear here as activity occurs."
              dark
            />
          }
        />
      )}
    </SafeAreaView>
  );
}
