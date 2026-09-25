import React from "react";
import { View, Text } from "react-native";
import { toFleetStatus, type FleetStatus } from "../types";
import { colors } from "../theme/colors";

const DOT_COLOR: Record<FleetStatus, string> = {
  sailing: colors.moss,
  choppy: colors.statusAmber,
  lost: colors.brick,
  docked: colors.stone,
};

const LABEL: Record<FleetStatus, string> = {
  sailing: "Sailing",
  choppy: "Choppy",
  lost: "Lost signal",
  docked: "Docked",
};

interface Props {
  status: string;
  label?: string;
  dark?: boolean;
}

export function StatusBadge({ status, label, dark = false }: Props) {
  const f = toFleetStatus(status);
  const dotColor = DOT_COLOR[f];
  const text = label ?? LABEL[f];

  return (
    <View
      className={`flex-row items-center gap-1.5 px-2.5 py-1 rounded-full border ${
        dark
          ? "bg-panel border-edge"
          : "bg-white border-line"
      }`}
      accessibilityLabel={`Status: ${text}`}
    >
      <View
        style={{ backgroundColor: dotColor }}
        className="w-1.5 h-1.5 rounded-full"
        accessible={false}
      />
      <Text
        className={`font-mono text-[11px] ${dark ? "text-bone" : "text-ink"}`}
        accessible={false}
      >
        {text}
      </Text>
    </View>
  );
}
