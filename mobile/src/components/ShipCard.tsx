import React from "react";
import { View, Text, TouchableOpacity } from "react-native";
import { toFleetStatus, fleetLabel, type Project, type Service, type Deployment } from "../types";
import { StatusBadge } from "./StatusBadge";
import { timeAgo, shortSha } from "../utils/format";
import { colors } from "../theme/colors";

const DOT_COLOR = {
  sailing: colors.moss,
  choppy: colors.statusAmber,
  lost: colors.brick,
  docked: colors.stone,
};

interface Props {
  project: Project;
  services?: Service[];
  lastDeployment?: Deployment;
  onPress: () => void;
  dark?: boolean;
}

export function ShipCard({
  project,
  services = [],
  lastDeployment,
  onPress,
  dark = false,
}: Props) {
  const f = toFleetStatus(project.status);

  return (
    <TouchableOpacity
      onPress={onPress}
      className={`rounded-card border p-4 mb-3 ${
        dark ? "bg-panel border-edge" : "bg-white border-line"
      }`}
      accessibilityRole="button"
      accessibilityLabel={`${project.name}, status ${fleetLabel[f]}`}
      activeOpacity={0.7}
    >
      <View className="flex-row items-center justify-between mb-1">
        <View className="flex-row items-center gap-2 flex-1">
          <View
            style={{ backgroundColor: DOT_COLOR[f] }}
            className="w-2 h-2 rounded-full"
          />
          <Text
            className={`font-head font-bold text-[17px] flex-1 ${
              dark ? "text-bone" : "text-ink"
            }`}
            numberOfLines={1}
          >
            {project.name}
          </Text>
        </View>
        {project.environment && (
          <View
            className={`rounded-md px-2 py-0.5 border ${
              dark ? "bg-emboss border-edge" : "bg-tint border-line"
            }`}
          >
            <Text
              className={`font-mono text-[11px] ${dark ? "text-bone" : "text-ink"}`}
            >
              {project.environment}
            </Text>
          </View>
        )}
      </View>

      <Text
        className={`font-mono text-[11px] mb-2 ${dark ? "text-fog" : "text-muted"}`}
        numberOfLines={1}
      >
        ~/{project.repository || project.name}
      </Text>

      {services.length > 0 && (
        <View className="flex-row flex-wrap gap-1.5 mb-3">
          {services.slice(0, 4).map((s) => {
            const sf = toFleetStatus(s.status);
            return (
              <View
                key={s.id}
                className={`flex-row items-center gap-1 rounded-md px-2 py-0.5 border ${
                  dark ? "bg-emboss border-edge" : "bg-tint border-line"
                }`}
              >
                <View
                  style={{ backgroundColor: DOT_COLOR[sf] }}
                  className="w-1.5 h-1.5 rounded-full"
                />
                <Text
                  className={`font-mono text-[11px] ${dark ? "text-bone" : "text-ink"}`}
                >
                  {s.name}
                </Text>
              </View>
            );
          })}
          {services.length > 4 && (
            <View
              className={`rounded-md px-2 py-0.5 border ${
                dark ? "bg-emboss border-edge" : "bg-tint border-line"
              }`}
            >
              <Text
                className={`font-mono text-[11px] ${dark ? "text-bone" : "text-ink"}`}
              >
                +{services.length - 4}
              </Text>
            </View>
          )}
        </View>
      )}

      <View
        className={`rounded-xl border px-3 py-2.5 flex-row items-center justify-between ${
          dark ? "bg-abyss border-edge" : "bg-paper border-line"
        }`}
      >
        <View className="flex-row gap-4">
          <View>
            <Text
              className={`text-[10px] font-semibold uppercase tracking-widest ${
                dark ? "text-fog" : "text-muted"
              }`}
            >
              Stations
            </Text>
            <Text
              className={`font-mono text-[13px] ${dark ? "text-bone" : "text-ink"}`}
            >
              {services.length}
            </Text>
          </View>
          <View>
            <Text
              className={`text-[10px] font-semibold uppercase tracking-widest ${
                dark ? "text-fog" : "text-muted"
              }`}
            >
              Last launch
            </Text>
            <Text
              className={`font-mono text-[13px] ${dark ? "text-bone" : "text-ink"}`}
            >
              {lastDeployment ? timeAgo(lastDeployment.startedAt) : "—"}
            </Text>
          </View>
        </View>
        {lastDeployment?.commitSha && (
          <View
            className={`rounded-md px-2 py-0.5 border ${
              dark ? "bg-emboss border-edge" : "bg-tint border-line"
            }`}
          >
            <Text
              className={`font-mono text-[11px] ${dark ? "text-bone" : "text-ink"}`}
            >
              # {shortSha(lastDeployment.commitSha)}
            </Text>
          </View>
        )}
      </View>

      <View className="flex-row items-center justify-between mt-2.5">
        <Text className={`text-xs ${dark ? "text-fog" : "text-muted"}`}>
          {f === "sailing"
            ? "Ready to roll"
            : f === "choppy"
            ? "Throttle detected"
            : f === "lost"
            ? "Distress call"
            : "Standing by"}
        </Text>
        <StatusBadge status={project.status} dark={dark} />
      </View>
    </TouchableOpacity>
  );
}
