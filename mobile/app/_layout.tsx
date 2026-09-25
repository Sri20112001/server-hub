import "../global.css";
import React, { useCallback, useEffect, useState } from "react";
import { Stack } from "expo-router";
import { StatusBar } from "expo-status-bar";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  useFonts,
  Inter_400Regular,
  Inter_500Medium,
} from "@expo-google-fonts/inter";
import {
  SpaceGrotesk_700Bold,
} from "@expo-google-fonts/space-grotesk";
import {
  JetBrainsMono_400Regular,
} from "@expo-google-fonts/jetbrains-mono";
import * as SplashScreen from "expo-splash-screen";
import { useAuthStore } from "../src/stores/authStore";
import { useBiometricStore } from "../src/stores/biometricStore";
import { AnimatedSplash } from "../src/components/AnimatedSplash";

SplashScreen.preventAutoHideAsync();

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      staleTime: 30_000,
    },
  },
});

export default function RootLayout() {
  const hydrate = useAuthStore((s) => s.hydrate);
  const checked = useAuthStore((s) => s.checked);
  const loadBiometric = useBiometricStore((s) => s.load);

  const [fontsLoaded] = useFonts({
    Inter_400Regular,
    Inter_500Medium,
    SpaceGrotesk_700Bold,
    JetBrainsMono_400Regular,
  });
  const [introDone, setIntroDone] = useState(false);
  const handleIntroDone = useCallback(() => setIntroDone(true), []);

  useEffect(() => {
    void hydrate();
    void loadBiometric();
  }, [hydrate, loadBiometric]);

  useEffect(() => {
    // Swap the native splash for the animated intro once fonts are loaded
    // AND the auth check is done.
    if (fontsLoaded && checked) {
      void SplashScreen.hideAsync();
    }
  }, [fontsLoaded, checked]);

  if (!fontsLoaded || !checked) return null;

  if (!introDone) {
    return <AnimatedSplash onDone={handleIntroDone} />;
  }

  return (
    <QueryClientProvider client={queryClient}>
      <StatusBar style="light" />
      <Stack screenOptions={{ headerShown: false }} />
    </QueryClientProvider>
  );
}
