import React from "react";
import { View } from "react-native";

interface ServerRackIconProps {
  size?: number; // scales the overall component
}

export function ServerRackIcon({ size = 180 }: ServerRackIconProps) {
  // Base design is proportioned at 200px
  const scale = size / 200;

  return (
    <View
      style={{
        width: 200,
        height: 200,
        transform: [{ scale }],
      }}
      className="items-center justify-center"
    >
      {/* Top Amber Handle / Fin */}
      <View className="w-14 h-1.5 rounded-t-sm bg-[#e58e26] mb-1.5 shadow-sm shadow-[#e58e26]/30" />

      {/* Main Server Stack Enclosure */}
      <View className="gap-2.5">
        {/* Unit 1 (Top: Green, Green, Amber) */}
        <ServerUnit
          ledColors={["bg-[#10b981]", "bg-[#10b981]", "bg-[#f59e0b]"]}
        />

        {/* Unit 2 (Middle: Green, Amber, Green) */}
        <ServerUnit
          ledColors={["bg-[#10b981]", "bg-[#f59e0b]", "bg-[#10b981]"]}
        />

        {/* Unit 3 (Bottom: Green, Green, Red) */}
        <ServerUnit
          ledColors={["bg-[#10b981]", "bg-[#10b981]", "bg-[#ef4444]"]}
        />
      </View>
    </View>
  );
}

function ServerUnit({ ledColors }: { ledColors: [string, string, string] }) {
  return (
    <View className="w-44 h-12 rounded-xl bg-[#1c2127] border border-[#2b333c] flex-row items-center px-3.5 justify-between shadow-lg shadow-black/40">
      {/* Vertical Status LED Column */}
      <View className="gap-1.5 items-center justify-center">
        {ledColors.map((colorClass, idx) => (
          <View
            key={idx}
            className={`w-2.5 h-2.5 rounded-full ${colorClass}`}
          />
        ))}
      </View>

      {/* Server Vent Slots / Drive Trays */}
      <View className="flex-1 ml-4 gap-1.5 justify-center">
        <View className="h-1 rounded-full bg-[#8c887b]/60 w-full" />
        <View className="h-1 rounded-full bg-[#8c887b]/60 w-full" />
        <View className="h-1 rounded-full bg-[#8c887b]/60 w-full" />
      </View>
    </View>
  );
}
