import React from "react";
import { View, Text } from "react-native";
import { useIsOffline } from "../stores/connectivityStore";
import { colors } from "../theme/colors";

/** Slim offline indicator. Rendered once at the root so every screen shows it. */
export function OfflineBanner() {
  const offline = useIsOffline();
  if (!offline) return null;
  return (
    <View
      className="px-4 py-2 items-center"
      style={{ backgroundColor: "#3a2a10" }}
      accessibilityRole="alert"
      accessibilityLabel="You are offline"
    >
      <Text className="text-[12px] font-medium" style={{ color: colors.ember }}>
        You&apos;re offline — showing last synced data. Polling paused.
      </Text>
    </View>
  );
}
