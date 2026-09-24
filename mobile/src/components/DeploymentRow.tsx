import React from "react";
import { View, Text } from "react-native";
import { toFleetStatus, type Deployment } from "../types";
import { timeAgo, fmtDuration, shortSha } from "../utils/format";
import { colors } from "../theme/colors";

const STATUS_COLOR = {
  sailing: colors.moss,
  choppy: colors.statusAmber,
  lost: colors.brick,
  docked: colors.stone,
};

interface Props {
  deployment: Deployment;
  projectName?: string;
  dark?: boolean;
}

export function DeploymentRow({ deployment, projectName, dark = false }: Props) {
  const f = toFleetStatus(deployment.status);
  const dotColor = STATUS_COLOR[f];

  return (
    <View
      className={`rounded-card border p-4 mb-3 ${
        dark ? "bg-panel border-edge" : "bg-white border-line"
      }`}
    >
      <View className="flex-row items-center justify-between mb-1">
        <View className="flex-row items-center gap-2 flex-1">
          <View
            style={{ backgroundColor: dotColor }}
            className="w-2 h-2 rounded-full"
          />
          <Text
            className={`font-head font-bold text-[15px] flex-1 ${
              dark ? "text-bone" : "text-ink"
            }`}
            numberOfLines={1}
          >
            {projectName ?? `#${deployment.projectId}`}
          </Text>
        </View>
        <View
          className={`rounded-md px-2 py-0.5 border ${
            dark ? "bg-emboss border-edge" : "bg-tint border-line"
          }`}
        >
          <Text
            className={`font-mono text-[11px] ${dark ? "text-bone" : "text-ink"}`}
          >
            # {shortSha(deployment.commitSha)}
          </Text>
        </View>
      </View>

      <View className="flex-row items-center gap-3 mt-1">
        <Text className={`font-mono text-xs ${dark ? "text-fog" : "text-muted"}`}>
          {timeAgo(deployment.startedAt)}
        </Text>
        <Text className={`font-mono text-xs ${dark ? "text-fog" : "text-muted"}`}>
          {fmtDuration(deployment.durationSec)}
        </Text>
        {deployment.branch && (
          <Text className={`font-mono text-xs ${dark ? "text-fog" : "text-muted"}`}>
            ⎇ {deployment.branch}
          </Text>
        )}
      </View>
    </View>
  );
}
