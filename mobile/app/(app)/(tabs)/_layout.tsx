import React from "react";
import { Tabs } from "expo-router";
import { View, Text } from "react-native";
import { colors } from "../../../src/theme/colors";

function TabIcon({
  icon,
  label,
  focused,
}: {
  icon: string;
  label: string;
  focused: boolean;
}) {
  return (
    <View style={{ alignItems: "center", paddingTop: 4 }}>
      <Text style={{ fontSize: 20, color: focused ? colors.ember : colors.fog }}>
        {icon}
      </Text>
      <Text
        style={{
          fontSize: 10,
          color: focused ? colors.ember : colors.fog,
          fontFamily: "Inter_400Regular",
          marginTop: 2,
        }}
      >
        {label}
      </Text>
    </View>
  );
}

export default function TabsLayout() {
  return (
    <Tabs
      screenOptions={{
        headerShown: false,
        tabBarStyle: {
          backgroundColor: colors.panel,
          borderTopColor: colors.edge,
          borderTopWidth: 1,
          height: 60,
          paddingBottom: 4,
        },
        tabBarActiveTintColor: colors.ember,
        tabBarInactiveTintColor: colors.fog,
        tabBarShowLabel: false,
      }}
    >
      <Tabs.Screen
        name="index"
        options={{
          tabBarIcon: ({ focused }) => (
            <TabIcon icon="⌂" label="Home" focused={focused} />
          ),
          tabBarAccessibilityLabel: "Dashboard",
        }}
      />
      <Tabs.Screen
        name="fleet"
        options={{
          tabBarIcon: ({ focused }) => (
            <TabIcon icon="⛵" label="Fleet" focused={focused} />
          ),
          tabBarAccessibilityLabel: "Fleet",
        }}
      />
      <Tabs.Screen
        name="projects"
        options={{
          tabBarIcon: ({ focused }) => (
            <TabIcon icon="◫" label="Projects" focused={focused} />
          ),
          tabBarAccessibilityLabel: "Projects",
        }}
      />
      <Tabs.Screen
        name="deployments"
        options={{
          tabBarIcon: ({ focused }) => (
            <TabIcon icon="⚡" label="Deploys" focused={focused} />
          ),
          tabBarAccessibilityLabel: "Deployments",
        }}
      />
      <Tabs.Screen
        name="settings"
        options={{
          tabBarIcon: ({ focused }) => (
            <TabIcon icon="⚙" label="Settings" focused={focused} />
          ),
          tabBarAccessibilityLabel: "Settings",
        }}
      />
    </Tabs>
  );
}
