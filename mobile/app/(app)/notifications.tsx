import React from "react";
import { View, Text, ScrollView, TouchableOpacity, RefreshControl } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Bell, CheckCheck } from "lucide-react-native";
import { notificationsApi } from "../../src/api/servers";
import { colors } from "../../src/theme/colors";
import type { InAppNotification } from "../../src/types";

const CATEGORY_COLOR: Record<string, string> = {
  alert: "#f87171",
  server: "#60a5fa",
  docker: "#a78bfa",
  security: "#fb923c",
  system: "#71717a",
};

function NotifRow({ notif, onRead }: { notif: InAppNotification; onRead: () => void }) {
  const markRead = async () => {
    if (notif.read) return;
    await notificationsApi.markRead(notif.id).catch(() => null);
    onRead();
  };

  return (
    <TouchableOpacity
      onPress={markRead}
      className={`flex-row items-start gap-3 px-4 py-3 border-b border-edge ${notif.read ? "opacity-50" : ""}`}
      accessibilityRole="button"
    >
      <View
        style={{
          width: 8, height: 8, borderRadius: 4, marginTop: 5,
          backgroundColor: notif.read ? colors.fog : (CATEGORY_COLOR[notif.category] ?? colors.fog),
        }}
      />
      <View className="flex-1">
        <Text className="text-bone text-sm font-semibold" numberOfLines={2}>{notif.title}</Text>
        {notif.body ? (
          <Text className="text-fog text-xs mt-0.5" numberOfLines={2}>{notif.body}</Text>
        ) : null}
        <Text className="text-fog text-[10px] font-mono mt-1">
          {new Date(notif.createdAt).toLocaleString()} · {notif.category}
        </Text>
      </View>
    </TouchableOpacity>
  );
}

export default function NotificationsScreen() {
  const qc = useQueryClient();

  const { data: notifications = [], isLoading, isRefetching, refetch } = useQuery<InAppNotification[]>({
    queryKey: ["notifications"],
    queryFn: notificationsApi.list,
    refetchInterval: 30_000,
  });

  const unread = notifications.filter((n) => !n.read).length;

  const markAll = async () => {
    await notificationsApi.markAllRead().catch(() => null);
    void qc.invalidateQueries({ queryKey: ["notifications"] });
  };

  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: colors.abyss }}>
      <View className="flex-row items-center justify-between px-4 py-3 border-b border-edge">
        <View className="flex-row items-center gap-2">
          <Bell size={18} color={colors.ember} />
          <Text className="font-head font-bold text-[18px] text-bone">Notifications</Text>
          {unread > 0 && (
            <View className="bg-ember rounded-full px-2 py-0.5">
              <Text className="text-black text-xs font-bold">{unread}</Text>
            </View>
          )}
        </View>
        {unread > 0 && (
          <TouchableOpacity onPress={markAll} className="flex-row items-center gap-1" accessibilityRole="button">
            <CheckCheck size={15} color={colors.fog} />
            <Text className="text-fog text-xs">Mark all read</Text>
          </TouchableOpacity>
        )}
      </View>

      <ScrollView
        refreshControl={
          <RefreshControl refreshing={isRefetching} onRefresh={() => void refetch()} tintColor={colors.ember} />
        }
        contentContainerStyle={{ paddingBottom: 100 }}
      >
        {isLoading && (
          <View className="items-center py-8">
            <Text className="text-fog text-sm">Loading…</Text>
          </View>
        )}

        {!isLoading && notifications.length === 0 && (
          <View className="items-center py-16">
            <Bell size={36} color={colors.fog} />
            <Text className="text-fog text-sm mt-3">No notifications yet.</Text>
          </View>
        )}

        {notifications.map((n) => (
          <NotifRow
            key={n.id}
            notif={n}
            onRead={() => void qc.invalidateQueries({ queryKey: ["notifications"] })}
          />
        ))}
      </ScrollView>
    </SafeAreaView>
  );
}
