import React, { useState } from "react";
import {
  View,
  Text,
  TextInput,
  TouchableOpacity,
  KeyboardAvoidingView,
  Platform,
  ScrollView,
  ActivityIndicator,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { router } from "expo-router";
import { useAuthStore } from "../../src/stores/authStore";
import { colors } from "../../src/theme/colors";

export default function LoginScreen() {
  const { login, busy, error, clearError } = useAuthStore();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");

  const handleLogin = async () => {
    if (!username.trim() || !password) return;
    clearError();
    try {
      await login(username.trim(), password);
      router.replace("/(app)/(tabs)");
    } catch {
      // error shown via store
    }
  };

  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: colors.abyss }}>
      <KeyboardAvoidingView
        behavior={Platform.OS === "ios" ? "padding" : "height"}
        style={{ flex: 1 }}
      >
        <ScrollView
          contentContainerStyle={{ flexGrow: 1, justifyContent: "center", padding: 24 }}
          keyboardShouldPersistTaps="handled"
        >
          {/* Logo / header */}
          <View className="items-center mb-8">
            <View className="w-14 h-14 rounded-full bg-ember items-center justify-center mb-4">
              <Text className="text-black text-2xl font-bold">⚙</Text>
            </View>
            <Text className="font-head font-bold text-[28px] text-bone">
              ServerHub
            </Text>
            <Text className="font-mono text-[11px] uppercase tracking-widest text-fog mt-1">
              Bridge console · sign in
            </Text>
          </View>

          {/* Card */}
          <View className="bg-panel border border-edge rounded-card p-5">
            <Text className="text-fog text-[13px] mb-5">
              One server. One captain. Identify yourself to take the helm.
            </Text>

            {/* Username */}
            <View className="mb-4">
              <Text className="text-[11px] font-semibold uppercase tracking-widest text-fog mb-1.5">
                Username
              </Text>
              <TextInput
                value={username}
                onChangeText={setUsername}
                autoCapitalize="none"
                autoCorrect={false}
                autoComplete="username"
                returnKeyType="next"
                className="bg-panel border border-edge rounded-input px-3 py-3 text-bone text-[13px]"
                placeholderTextColor={colors.fog}
                style={{ fontFamily: "Inter_400Regular" }}
              />
            </View>

            {/* Password */}
            <View className="mb-4">
              <Text className="text-[11px] font-semibold uppercase tracking-widest text-fog mb-1.5">
                Password
              </Text>
              <TextInput
                value={password}
                onChangeText={setPassword}
                secureTextEntry
                autoComplete="current-password"
                returnKeyType="done"
                onSubmitEditing={handleLogin}
                className="bg-panel border border-edge rounded-input px-3 py-3 text-bone text-[13px]"
                placeholderTextColor={colors.fog}
                style={{ fontFamily: "Inter_400Regular" }}
              />
            </View>

            {/* Error */}
            {error && (
              <View className="bg-danger-night border border-brick/40 rounded-input px-3 py-2.5 mb-4">
                <Text className="text-brick text-[13px]">{error}</Text>
              </View>
            )}

            {/* Submit */}
            <TouchableOpacity
              onPress={handleLogin}
              disabled={busy || !username.trim() || !password}
              className="bg-ember rounded-input py-3 items-center"
              style={{ opacity: busy || !username.trim() || !password ? 0.55 : 1 }}
              accessibilityRole="button"
              accessibilityLabel="Sign in"
            >
              {busy ? (
                <ActivityIndicator color={colors.abyss} size="small" />
              ) : (
                <Text
                  className="text-black font-medium text-[13px]"
                  style={{ fontFamily: "Inter_500Medium" }}
                >
                  Take the helm
                </Text>
              )}
            </TouchableOpacity>
          </View>
        </ScrollView>
      </KeyboardAvoidingView>
    </SafeAreaView>
  );
}
