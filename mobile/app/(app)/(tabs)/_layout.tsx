import React from "react";
import { Tabs } from "expo-router";
import { View, Text, Platform, StyleSheet } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import {
  House,
  Sailboat,
  LayoutGrid,
  Rocket,
  Settings as SettingsIcon,
  type LucideIcon,
} from "lucide-react-native";
import { colors } from "../../../src/theme/colors";

function TabIcon({
  icon: Icon,
  label,
  focused,
}: {
  icon: LucideIcon;
  label: string;
  focused: boolean;
}) {
  const activeColor = colors.ember ?? "#ff5c35";
  const inactiveColor = colors.fog ?? "#71717a";

  return (
    <View
      style={[
        styles.iconContainer,
        focused && styles.iconContainerFocused,
      ]}
    >
      {focused && (
        <View
          style={[
            styles.activeGlow,
            { backgroundColor: activeColor, shadowColor: activeColor },
          ]}
        />
      )}

      <Icon
        size={20}
        color={focused ? activeColor : inactiveColor}
        strokeWidth={focused ? 2.5 : 1.75}
      />

      <Text
        numberOfLines={1}
        style={[
          styles.label,
          {
            color: focused ? "#ffffff" : inactiveColor,
            fontWeight: focused ? "600" : "400",
          },
        ]}
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
          height: 64,
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
          left: 16,
          right: 16,
          height: 64,
          paddingTop: 0,
          paddingBottom: 0,
          paddingHorizontal: 6,
          backgroundColor: "rgba(18, 18, 22, 0.92)",
          borderRadius: 32,
          borderWidth: 1,
          borderColor: "rgba(255, 255, 255, 0.12)",
          borderTopWidth: 1,
          borderTopColor: "rgba(255, 255, 255, 0.16)",
          elevation: 12,
          shadowColor: "#000",
          shadowOffset: { width: 0, height: 10 },
          shadowOpacity: 0.35,
          shadowRadius: 20,
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

const styles = StyleSheet.create({
  iconContainer: {
    width: "100%",
    height: "100%",
    alignItems: "center",
    justifyContent: "center",
    borderRadius: 22,
    gap: 3,
    position: "relative",
  },
  iconContainerFocused: {
    backgroundColor: "rgba(255, 255, 255, 0.06)",
    borderWidth: 1,
    borderColor: "rgba(255, 255, 255, 0.08)",
  },
  activeGlow: {
    position: "absolute",
    top: 4,
    width: 20,
    height: 3,
    borderRadius: 2,
    shadowOffset: { width: 0, height: 2 },
    shadowOpacity: 0.8,
    shadowRadius: 6,
    elevation: 3,
  },
  label: {
    fontSize: 10,
    lineHeight: 12,
    letterSpacing: 0.2,
    fontFamily: Platform.select({
      ios: "System",
      default: "Inter_400Regular",
    }),
  },
});