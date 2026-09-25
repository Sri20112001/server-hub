import React from "react";
import { Tabs } from "expo-router";
import { View, Text } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import {
  House,
  Sailboat,
  LayoutGrid,
  Rocket,
  Settings as SettingsIcon,
  type LucideIcon,
} from "lucide-react-native";

function TabIcon({
  icon: Icon,
  label,
  focused,
}: {
  icon: LucideIcon;
  label: string;
  focused: boolean;
}) {
  return (
    <View
      className={`w-full h-full items-center justify-center relative py-1.5 transition-all ${
        focused ? "bg-white/[0.07] rounded-2xl border border-white/10" : ""
      }`}
    >
      {/* Top Laser Accent Dot for Active Tab */}
      {focused && (
        <View className="absolute top-1.5 w-4 h-1 rounded-full bg-orange-500 shadow-sm shadow-orange-500" />
      )}

      {/* Tab Icon */}
      <Icon
        size={20}
        color={focused ? "#f97316" : "#71717a"}
        strokeWidth={focused ? 2.4 : 1.8}
      />

      {/* Label */}
      <Text
        numberOfLines={1}
        className={`text-[10px] tracking-tight font-mono mt-0.5 ${
          focused ? "text-zinc-100 font-semibold" : "text-zinc-500 font-medium"
        }`}
      >
        {label}
      </Text>
    </View>
  );
}

export default function TabsLayout() {
  const insets = useSafeAreaInsets();
  const bottomOffset = Math.max(insets.bottom, 16);

  return (
    <Tabs
      screenOptions={{
        headerShown: false,
        tabBarHideOnKeyboard: true,
        tabBarShowLabel: false,
        tabBarItemStyle: {
          height: 62,
          padding: 4,
          alignItems: "center",
          justifyContent: "center",
        },
        tabBarIconStyle: {
          width: "100%",
          height: "100%",
          alignItems: "center",
          justifyContent: "center",
        },
        tabBarStyle: {
          position: "absolute",
          bottom: bottomOffset,
          left: 14,
          right: 14,
          height: 62,
          paddingTop: 0,
          paddingBottom: 0,
          paddingHorizontal: 6,
          backgroundColor: "rgba(10, 11, 16, 0.94)",
          borderRadius: 28,
          borderWidth: 1,
          borderColor: "rgba(255, 255, 255, 0.12)",
          borderTopWidth: 1,
          borderTopColor: "rgba(255, 255, 255, 0.2)",
          elevation: 20,
          shadowColor: "#000",
          shadowOffset: { width: 0, height: 12 },
          shadowOpacity: 0.5,
          shadowRadius: 24,
        },
      }}
    >
      <Tabs.Screen
        name="index"
        options={{
          tabBarIcon: ({ focused }) => (
            <TabIcon icon={House} label="Home" focused={focused} />
          ),
          tabBarAccessibilityLabel: "Dashboard",
        }}
      />
      <Tabs.Screen
        name="fleet"
        options={{
          tabBarIcon: ({ focused }) => (
            <TabIcon icon={Sailboat} label="Fleet" focused={focused} />
          ),
          tabBarAccessibilityLabel: "Fleet",
        }}
      />
      <Tabs.Screen
        name="projects"
        options={{
          tabBarIcon: ({ focused }) => (
            <TabIcon icon={LayoutGrid} label="Projects" focused={focused} />
          ),
          tabBarAccessibilityLabel: "Projects",
        }}
      />
      <Tabs.Screen
        name="deployments"
        options={{
          tabBarIcon: ({ focused }) => (
            <TabIcon icon={Rocket} label="Deploys" focused={focused} />
          ),
          tabBarAccessibilityLabel: "Deployments",
        }}
      />
      <Tabs.Screen
        name="settings"
        options={{
          tabBarIcon: ({ focused }) => (
            <TabIcon icon={SettingsIcon} label="Settings" focused={focused} />
          ),
          tabBarAccessibilityLabel: "Settings",
        }}
      />
    </Tabs>
  );
}