import React, { useState, useMemo } from "react";
import {
  View, Text, ScrollView, TextInput, TouchableOpacity,
  RefreshControl, FlatList,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { router } from "expo-router";
import { useQuery } from "@tanstack/react-query";
import { Search, Server } from "lucide-react-native";
import { serversApi } from "../../src/api/servers";
import { colors } from "../../src/theme/colors";
import type { ManagedServer } from "../../src/types";

const STATUS_COLOR: Record<string, string> = {
  ONLINE: "#22c55e",
  OFFLINE: "#ef4444",
  WARNING: "#eab308",
  UNKNOWN: "#71717a",
};

function ServerCard({ server }: { server: ManagedServer }) {
  return (
    <TouchableOpacity
      onPress={() => router.push(`/(app)/servers/${server.id}`)}
      className="bg-panel border border-edge rounded-xl p-4 mb-3"
      accessibilityRole="button"
      accessibilityLabel={`Server ${server.name}, status ${server.status}`}
    >
      <View className="flex-row items-center justify-between mb-2">
        <View className="flex-row items-center gap-2 flex-1 min-w-0">
          <View
            style={{ width: 8, height: 8, borderRadius: 4, backgroundColor: STATUS_COLOR[server.status] ?? "#71717a" }}
          />
          <Text className="text-bone font-semibold text-sm flex-1" numberOfLines={1}>
            {server.name}
          </Text>
        </View>
        <Text className="font-mono text-xs" style={{ color: STATUS_COLOR[server.status] ?? "#71717a" }}>
          {server.status}
        </Text>
      </View>

      {(server.hostname || server.ipAddress) && (
        <Text className="text-fog text-xs font-mono mb-2" numberOfLines={1}>
          {server.hostname || server.ipAddress}
        </Text>
      )}

      {server.os && (
        <Text className="text-fog text-xs mb-2" numberOfLines={1}>
          {server.os} {server.osVersion}
        </Text>
      )}

      <View className="flex-row gap-2">
        {server.cpuCores > 0 && (
          <View className="bg-emboss rounded-lg px-2 py-1">
            <Text className="text-fog text-[10px]">CPU</Text>
            <Text className="text-bone font-mono text-xs">{server.cpuCores}c</Text>
          </View>
        )}
        {server.ramTotal > 0 && (
          <View className="bg-emboss rounded-lg px-2 py-1">
            <Text className="text-fog text-[10px]">RAM</Text>
            <Text className="text-bone font-mono text-xs">
              {Math.round(server.ramTotal / 1024 / 1024 / 1024)}GB
            </Text>
          </View>
        )}
        <View className="bg-emboss rounded-lg px-2 py-1">
          <Text className="text-fog text-[10px]">Agent</Text>
          <Text className="text-bone font-mono text-xs">{server.agentStatus}</Text>
        </View>
      </View>

      {server.lastHeartbeat && (
        <Text className="text-fog text-[10px] font-mono mt-2">
          Last seen: {new Date(server.lastHeartbeat).toLocaleString()}
        </Text>
      )}
    </TouchableOpacity>
  );
}

export default function ServersScreen() {
  const [search, setSearch] = useState("");
  const [groupFilter, setGroupFilter] = useState<number | null>(null);

  const { data: servers = [], isLoading, isRefetching, refetch } = useQuery<ManagedServer[]>({
    queryKey: ["servers"],
    queryFn: serversApi.list,
    refetchInterval: 30_000,
  });

  const { data: groups = [] } = useQuery<import("../../src/types").ServerGroup[]>({
    queryKey: ["server-groups"],
    queryFn: serversApi.groups,
  });

  const filtered = useMemo(() => {
    let list = servers;
    if (groupFilter !== null) list = list.filter((s) => s.groupId === groupFilter);
    if (search.trim()) {
      const q = search.toLowerCase();
      list = list.filter(
        (s) =>
          s.name.toLowerCase().includes(q) ||
          s.hostname.toLowerCase().includes(q) ||
          s.ipAddress.toLowerCase().includes(q),
      );
    }
    return list;
  }, [servers, search, groupFilter]);

  const counts = {
    online: servers.filter((s) => s.status === "ONLINE").length,
    offline: servers.filter((s) => s.status === "OFFLINE").length,
  };

  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: colors.abyss }}>
      {/* Header */}
      <View className="flex-row items-center justify-between px-4 py-3 border-b border-edge">
        <View className="flex-row items-center gap-2">
          <Server size={18} color={colors.ember} />
          <Text className="font-head font-bold text-[18px] text-bone">Servers</Text>
          <Text className="text-fog text-xs font-mono">
            {counts.online} online · {counts.offline} offline
          </Text>
        </View>
      </View>

      {/* Search */}
      <View className="px-4 py-2">
        <View className="flex-row items-center bg-panel border border-edge rounded-xl px-3 gap-2">
          <Search size={15} color={colors.fog} />
          <TextInput
            className="flex-1 py-2.5 text-bone text-sm font-mono"
            placeholder="Search servers…"
            placeholderTextColor={colors.fog}
            value={search}
            onChangeText={setSearch}
          />
        </View>
      </View>

      {/* Group filter */}
      {groups.length > 0 && (
        <ScrollView horizontal showsHorizontalScrollIndicator={false} className="px-4 mb-2">
          <TouchableOpacity
            onPress={() => setGroupFilter(null)}
            className={`mr-2 px-3 py-1 rounded-full border ${groupFilter === null ? "border-ember" : "border-edge"}`}
          >
            <Text className={`text-xs font-mono ${groupFilter === null ? "text-ember" : "text-fog"}`}>All</Text>
          </TouchableOpacity>
          {groups.map((g) => (
            <TouchableOpacity
              key={g.id}
              onPress={() => setGroupFilter(g.id)}
              className={`mr-2 px-3 py-1 rounded-full border ${groupFilter === g.id ? "border-ember" : "border-edge"}`}
            >
              <Text className={`text-xs font-mono ${groupFilter === g.id ? "text-ember" : "text-fog"}`}>{g.name}</Text>
            </TouchableOpacity>
          ))}
        </ScrollView>
      )}

      <FlatList
        data={filtered}
        keyExtractor={(s) => String(s.id)}
        renderItem={({ item }) => <ServerCard server={item} />}
        contentContainerStyle={{ padding: 16, paddingBottom: 100 }}
        refreshControl={
          <RefreshControl refreshing={isRefetching} onRefresh={() => void refetch()} tintColor={colors.ember} />
        }
        ListEmptyComponent={
          isLoading ? null : (
            <View className="items-center py-16">
              <Server size={36} color={colors.fog} />
              <Text className="text-fog text-sm mt-3">
                {search ? "No servers match your search." : "No servers registered yet."}
              </Text>
            </View>
          )
        }
      />
    </SafeAreaView>
  );
}
