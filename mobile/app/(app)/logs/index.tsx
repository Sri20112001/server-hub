import React, { useState, useEffect } from "react";
import {
  View,
  Text,
  FlatList,
  TouchableOpacity,
  RefreshControl,
  ScrollView,
  Animated,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { router, useLocalSearchParams } from "expo-router";
import { useQuery } from "@tanstack/react-query";
import {
  ChevronLeft,
  RefreshCw,
  Terminal,
  Activity,
  Filter,
} from "lucide-react-native";
import { logsApi } from "../../../src/api/logs";
import { SkeletonCard } from "../../../src/components/SkeletonBox";
import { ErrorState } from "../../../src/components/ErrorState";
import { EmptyState } from "../../../src/components/EmptyState";
import type { AppLog } from "../../../src/types";

type Level = "all" | "info" | "warn" | "error";

interface LevelStyleConfig {
  badge: string;
  text: string;
  dot: string;
}

const LEVEL_CONFIG: Record<string, LevelStyleConfig> = {
  info: {
    badge: "bg-emerald-500/10 border-emerald-500/30",
    text: "text-emerald-400",
    dot: "bg-emerald-400 shadow-emerald-400/50",
  },
  warn: {
    badge: "bg-amber-500/10 border-amber-500/30",
    text: "text-amber-400",
    dot: "bg-amber-400 shadow-amber-400/50",
  },
  error: {
    badge: "bg-rose-500/15 border-rose-500/40",
    text: "text-rose-400",
    dot: "bg-rose-500 shadow-rose-500/60",
  },
  debug: {
    badge: "bg-slate-500/10 border-slate-500/30",
    text: "text-slate-400",
    dot: "bg-slate-400",
  },
};

function LogLine({ log, index }: { log: AppLog; index: number }) {
  const normLevel = log.level?.toLowerCase() ?? "info";
  const theme = LEVEL_CONFIG[normLevel] ?? LEVEL_CONFIG.info;
  const ts = log.timestamp?.slice(11, 19) ?? "--:--:--";
  const isOdd = index % 2 === 1;

  return (
    <View
      className={`py-2 px-3 border-b border-white/[0.04] ${
        isOdd ? "bg-white/[0.015]" : "bg-transparent"
      }`}
    >
      <ScrollView horizontal showsHorizontalScrollIndicator={false}>
        <View className="flex-row items-center gap-2.5">
          {/* Status Dot with Glow Effect */}
          <View className={`w-1.5 h-1.5 rounded-full ${theme.dot}`} />

          {/* Timestamp */}
          <Text className="text-[11px] font-mono text-zinc-500 tracking-tight">
            {ts}
          </Text>

          {/* Level Micro-Badge */}
          <View
            className={`px-1.5 py-0.5 rounded border ${theme.badge} items-center justify-center`}
          >
            <Text
              className={`text-[9px] font-mono font-bold tracking-wider ${theme.text}`}
            >
              {(log.level ?? "INFO").toUpperCase()}
            </Text>
          </View>

          {/* Source Tag */}
          {log.source && (
            <View className="px-1.5 py-0.5 rounded bg-zinc-800/80 border border-zinc-700/40">
              <Text className="text-[10px] font-mono text-zinc-400">
                {log.source}
              </Text>
            </View>
          )}

          {/* Main Log Line Message */}
          <Text className="text-[12px] font-mono text-zinc-200 tracking-wide">
            {log.message}
          </Text>
        </View>
      </ScrollView>
    </View>
  );
}

function LiveDot() {
  // animate-pulse equivalent: RN has no CSS animations, so pulse via Animated.
  const [opacity] = useState(() => new Animated.Value(1));
  useEffect(() => {
    const anim = Animated.loop(
      Animated.sequence([
        Animated.timing(opacity, {
          toValue: 0.25,
          duration: 900,
          useNativeDriver: true,
        }),
        Animated.timing(opacity, {
          toValue: 1,
          duration: 900,
          useNativeDriver: true,
        }),
      ]),
    );
    anim.start();
    return () => anim.stop();
  }, [opacity]);
  return (
    <Animated.View
      style={{ opacity }}
      className="w-1.5 h-1.5 rounded-full bg-emerald-400"
    />
  );
}

export default function LogsScreen() {
  const { projectId } = useLocalSearchParams<{ projectId?: string }>();
  const [level, setLevel] = useState<Level>("all");

  const {
    data: logs,
    isLoading,
    isError,
    error,
    refetch,
    isRefetching,
  } = useQuery({
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
    <SafeAreaView className="flex-1 bg-[#090a0f]">
      {/* HUD Floating Header Box */}
      <View className="mx-4 mt-2 mb-3 p-3 rounded-2xl bg-zinc-900/60 border border-white/10 shadow-2xl shadow-black/80">
        <View className="flex-row items-center justify-between">
          {/* Back Action Pill */}
          <TouchableOpacity
            onPress={() => router.back()}
            activeOpacity={0.75}
            className="flex-row items-center gap-1 px-2.5 py-1.5 rounded-xl bg-white/[0.04] border border-white/10"
            accessibilityLabel="Go back"
          >
            <ChevronLeft size={16} color="#fb923c" />
            <Text className="text-zinc-300 font-mono text-xs font-semibold">
              Back
            </Text>
          </TouchableOpacity>

          {/* Title & Metadata Badge */}
          <View className="flex-row items-center gap-2">
            <View className="p-1 rounded-lg bg-orange-500/10 border border-orange-500/20">
              <Terminal size={14} color="#f97316" />
            </View>
            <Text className="text-zinc-100 font-bold text-sm tracking-tight">
              Console Output
            </Text>
            {projectId && (
              <View className="flex-row items-center gap-1 px-2 py-0.5 rounded-full bg-zinc-800/80 border border-zinc-700/50">
                <Activity size={10} color="#a1a1aa" />
                <Text className="text-[10px] font-mono text-zinc-400">
                  #{projectId}
                </Text>
              </View>
            )}
          </View>

          {/* Refresh Action Button */}
          <TouchableOpacity
            onPress={() => void refetch()}
            activeOpacity={0.75}
            className="p-2 rounded-xl bg-white/[0.04] border border-white/10"
            accessibilityLabel="Refresh logs"
          >
            <RefreshCw
              size={14}
              color={isRefetching ? "#f97316" : "#a1a1aa"}
            />
          </TouchableOpacity>
        </View>

        {/* Level Filter Segmented Dock */}
        <View className="flex-row items-center justify-between mt-3 pt-2.5 border-t border-white/[0.06]">
          <View className="flex-row items-center gap-1.5">
            <Filter size={12} color="#71717a" />
            <Text className="text-[10px] font-mono uppercase tracking-wider text-zinc-500">
              Filter
            </Text>
          </View>

          <View className="flex-row gap-1.5 p-1 rounded-xl bg-black/40 border border-white/[0.05]">
            {LEVELS.map((l) => {
              const isSelected = level === l.key;
              return (
                <TouchableOpacity
                  key={l.key}
                  onPress={() => setLevel(l.key)}
                  activeOpacity={0.8}
                  className={`px-3 py-1 rounded-lg ${
                    isSelected
                      ? "bg-orange-500/20 border border-orange-500/40 shadow-sm"
                      : "bg-transparent border border-transparent"
                  }`}
                  accessibilityRole="button"
                  accessibilityLabel={`Filter: ${l.label}`}
                  accessibilityState={{ selected: isSelected }}
                >
                  <Text
                    className={`font-mono text-[11px] ${
                      isSelected
                        ? "text-orange-400 font-bold"
                        : "text-zinc-400 font-normal"
                    }`}
                  >
                    {l.label}
                  </Text>
                </TouchableOpacity>
              );
            })}
          </View>
        </View>
      </View>

      {/* Terminal Display Screen Frame */}
      <View className="flex-1 mx-4 mb-3 rounded-2xl bg-[#0d0e15] border border-white/[0.08] overflow-hidden shadow-2xl">
        {/* Terminal Title Bar */}
        <View className="h-8 flex-row items-center justify-between px-3 bg-zinc-900/40 border-b border-white/[0.06]">
          {/* Glass window buttons */}
          <View className="flex-row items-center gap-1.5">
            <View className="w-2.5 h-2.5 rounded-full bg-rose-500/80" />
            <View className="w-2.5 h-2.5 rounded-full bg-amber-500/80" />
            <View className="w-2.5 h-2.5 rounded-full bg-emerald-500/80" />
          </View>

          <Text className="text-[10px] font-mono tracking-widest text-zinc-500 uppercase">
            bash ~ stream:active
          </Text>

          {/* Pulsing indicator */}
          <View className="flex-row items-center gap-1.5">
            <LiveDot />
            <Text className="text-[9px] font-mono text-zinc-500 uppercase">
              Live
            </Text>
          </View>
        </View>

        {/* State Renderers */}
        {isLoading && (
          <View className="p-4 gap-3">
            {[0, 1, 2].map((i) => (
              <SkeletonCard key={i} dark />
            ))}
          </View>
        )}

        {isError && (
          <View className="p-4">
            <ErrorState
              message={
                error instanceof Error ? error.message : "Failed to load logs"
              }
              onRetry={() => void refetch()}
              dark
            />
          </View>
        )}

        {!isLoading && !isError && (
          <FlatList
            data={logs ?? []}
            keyExtractor={(l) => String(l.id)}
            contentContainerStyle={{ flexGrow: 1, paddingVertical: 4, paddingBottom: 24 }}
            refreshControl={
              <RefreshControl
                refreshing={isRefetching}
                onRefresh={() => void refetch()}
                tintColor="#f97316"
              />
            }
            renderItem={({ item, index }) => (
              <LogLine log={item} index={index} />
            )}
            ListEmptyComponent={
              <View className="flex-1 items-center justify-center py-12">
                <EmptyState
                  title="No log entries"
                  hint="Waiting for stdout/stderr stream packets."
                  dark
                />
              </View>
            }
          />
        )}
      </View>
    </SafeAreaView>
  );
}