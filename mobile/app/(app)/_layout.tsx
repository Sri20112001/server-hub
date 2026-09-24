import React from "react";
import { Stack, Redirect } from "expo-router";
import { useAuthStore } from "../../src/stores/authStore";

export default function AppLayout() {
  const user = useAuthStore((s) => s.user);
  if (!user) return <Redirect href="/(auth)/login" />;

  return (
    <Stack screenOptions={{ headerShown: false }}>
      <Stack.Screen name="(tabs)" />
      <Stack.Screen name="projects/[id]" />
      <Stack.Screen name="logs" />
    </Stack>
  );
}
