import React from "react";
import { View, Text } from "react-native";
import { colors } from "../theme/colors";

interface Props {
  label: string;
  pct: number;
  display: string;
  dark?: boolean;
}

export function Meter({ label, pct, display, dark = false }: Props) {
  const p = Math.max(0, Math.min(100, pct));
  const fillColor =
    p >= 90 ? colors.brick : p >= 70 ? colors.accent : dark ? colors.bone : colors.ink;

  return (
    <View className="my-1.5">
      <View className="flex-row justify-between items-baseline mb-1">
        <Text className={`text-xs ${dark ? "text-bone" : "text-ink"}`}>{label}</Text>
        <Text className={`font-mono text-xs ${dark ? "text-bone" : "text-ink"}`}>
          {display}
        </Text>
      </View>
      <View
        className={`h-1.5 rounded-full overflow-hidden border ${
          dark ? "bg-abyss border-edge" : "bg-paper border-line"
        }`}
      >
        <View
          style={{ width: `${p}%`, backgroundColor: fillColor }}
          className="h-full rounded-full"
        />
      </View>
    </View>
  );
}
