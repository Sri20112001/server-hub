import React, { useEffect } from "react";
import { View, Text } from "react-native";
import Animated, {
  useSharedValue,
  useAnimatedStyle,
  withTiming,
  withDelay,
  withRepeat,
  withSequence,
  runOnJS,
  Easing,
} from "react-native-reanimated";
import { ServerRackIcon } from "./ServerRackIcon";

const EXIT_DELAY_MS = 1700;
const EXIT_MS = 400;

export function AnimatedSplash({ onDone }: { onDone: () => void }) {
  const mark = useSharedValue(0);
  const text = useSharedValue(0);
  const pulse = useSharedValue(1);
  const progress = useSharedValue(0);
  const out = useSharedValue(1);

  useEffect(() => {
    // Reveal logo & glow ring
    mark.value = withTiming(1, {
      duration: 800,
      easing: Easing.out(Easing.cubic),
    });

    // Ambient breathing pulse for glow ring
    pulse.value = withRepeat(
      withSequence(
        withTiming(1.08, { duration: 900, easing: Easing.inOut(Easing.ease) }),
        withTiming(1, { duration: 900, easing: Easing.inOut(Easing.ease) }),
      ),
      -1,
      true,
    );

    // Text slide up
    text.value = withDelay(
      350,
      withTiming(1, { duration: 650, easing: Easing.out(Easing.cubic) }),
    );

    // Simulated cyber load progress bar
    progress.value = withDelay(
      200,
      withTiming(1, { duration: 1200, easing: Easing.inOut(Easing.quad) }),
    );

    // Exit transition
    out.value = withDelay(
      EXIT_DELAY_MS,
      withTiming(0, { duration: EXIT_MS, easing: Easing.in(Easing.cubic) }, (finished) => {
        if (finished) runOnJS(onDone)();
      }),
    );
  }, [mark, text, pulse, progress, out, onDone]);

  const markStyle = useAnimatedStyle(() => ({
    opacity: mark.value,
    transform: [{ scale: 0.8 + 0.2 * mark.value }],
  }));

  const pulseRingStyle = useAnimatedStyle(() => ({
    transform: [{ scale: pulse.value }],
    opacity: mark.value * 0.75,
  }));

  const textStyle = useAnimatedStyle(() => ({
    opacity: text.value,
    transform: [{ translateY: (1 - text.value) * 16 }],
  }));

  const progressBarStyle = useAnimatedStyle(() => ({
    width: `${progress.value * 100}%`,
  }));

  const containerStyle = useAnimatedStyle(() => ({
    opacity: out.value,
    transform: [{ scale: 1 + (1 - out.value) * 0.05 }],
  }));

  return (
    <Animated.View
      style={containerStyle}
      className="flex-1 bg-[#07080c] items-center justify-center relative overflow-hidden"
    >
      {/* Background Ambient Glow Orbs */}
      <View className="absolute -top-24 -left-24 w-80 h-80 rounded-full bg-orange-600/10 blur-3xl pointer-events-none" />
      <View className="absolute -bottom-24 -right-24 w-80 h-80 rounded-full bg-amber-500/10 blur-3xl pointer-events-none" />

      {/* Center Console Reactor HUD */}
      <View className="items-center justify-center gap-7">
        <Animated.View style={markStyle} className="relative items-center justify-center">
          {/* Animated Neon Backdrop Pulse Ring */}
          <Animated.View
            style={pulseRingStyle}
            className="absolute w-52 h-52 rounded-full border border-orange-500/25 bg-orange-500/5 shadow-2xl shadow-orange-500/50"
          />

          {/* Glass Portal Enclosure */}
          <View className="w-44 h-44 rounded-3xl bg-zinc-950/80 border border-white/10 items-center justify-center shadow-2xl shadow-black relative overflow-hidden backdrop-blur-2xl">
            {/* Corner Bracket Decals */}
            <View className="absolute top-2 left-2 w-2 h-2 border-t-2 border-l-2 border-orange-500/60" />
            <View className="absolute top-2 right-2 w-2 h-2 border-t-2 border-r-2 border-orange-500/60" />
            <View className="absolute bottom-2 left-2 w-2 h-2 border-b-2 border-l-2 border-orange-500/60" />
            <View className="absolute bottom-2 right-2 w-2 h-2 border-b-2 border-r-2 border-orange-500/60" />

            {/* Specular Inner Glare Line */}
            <View className="absolute top-0 left-0 right-0 h-[1px] bg-white/20" />

            {/* Logo Mark */}
            <ServerRackIcon size={112} />
          </View>
        </Animated.View>

        {/* Title, Badge & Progress Module */}
        <Animated.View style={textStyle} className="items-center gap-3.5">
          {/* System Tag Capsule */}
          <View className="flex-row items-center gap-2 px-3 py-1 rounded-full bg-orange-500/10 border border-orange-500/30">
            <View className="w-1.5 h-1.5 rounded-full bg-orange-400 shadow-sm shadow-orange-400" />
            <Text className="font-mono text-[10px] tracking-[3px] text-orange-400 font-bold uppercase">
              Bridge Console
            </Text>
          </View>

          {/* Main Title */}
          <Text className="font-mono font-black text-2xl tracking-[6px] text-zinc-100 uppercase">
            SERVER HUB
          </Text>

          {/* Telemetry Loader Bar */}
          <View className="w-44 mt-1">
            <View className="h-1 w-full bg-white/[0.06] rounded-full overflow-hidden border border-white/[0.04]">
              <Animated.View
                style={progressBarStyle}
                className="h-full bg-gradient-to-r from-orange-600 to-amber-400 rounded-full shadow-md shadow-orange-500"
              />
            </View>
            <View className="flex-row justify-between items-center mt-1.5 px-0.5">
              <Text className="font-mono text-[9px] text-zinc-600 tracking-widest uppercase">
                Init Core
              </Text>
              <Text className="font-mono text-[9px] text-orange-500/80 tracking-widest">
                SYS_OK
              </Text>
            </View>
          </View>
        </Animated.View>
      </View>
    </Animated.View>
  );
}