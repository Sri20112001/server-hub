import React from "react";
import { Redirect } from "expo-router";
import { useAuthStore } from "../src/stores/authStore";

export default function Index() {
  const user = useAuthStore((s) => s.user);
  return user ? <Redirect href="/(app)/(tabs)" /> : <Redirect href="/(auth)/login" />;
}
