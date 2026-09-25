import React, { useEffect, useState } from "react";
import { Animated, View, type DimensionValue } from "react-native";
import { colors } from "../theme/colors";

interface Props {
  width?: DimensionValue;
  height?: number;
  rounded?: boolean;
  dark?: boolean;
}

export function SkeletonBox({
  width = "100%",
  height = 16,
  rounded = false,
  dark = false,
}: Props) {
  const [opacity] = useState(() => new Animated.Value(0.4));

  useEffect(() => {
    const anim = Animated.loop(
      Animated.sequence([
        Animated.timing(opacity, {
          toValue: 1,
          duration: 900,
          useNativeDriver: true,
        }),
        Animated.timing(opacity, {
          toValue: 0.4,
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
      style={{
        width,
        height,
        backgroundColor: dark ? colors.edge : colors.skel,
        borderRadius: rounded ? height / 2 : 6,
        opacity,
      }}
      accessible={false}
    />
  );
}

export function SkeletonCard({ dark = false }: { dark?: boolean }) {
  return (
    <View
      className={`rounded-card border p-4 mb-3 ${
        dark ? "bg-panel border-edge" : "bg-white border-line"
      }`}
    >
      <View className="flex-row items-center gap-2 mb-3">
        <SkeletonBox width={8} height={8} rounded dark={dark} />
        <SkeletonBox width={140} height={18} dark={dark} />
      </View>
      <SkeletonBox width="70%" height={12} dark={dark} />
      <View className="flex-row gap-2 mt-3">
        <SkeletonBox width={60} height={20} rounded dark={dark} />
        <SkeletonBox width={60} height={20} rounded dark={dark} />
      </View>
    </View>
  );
}
