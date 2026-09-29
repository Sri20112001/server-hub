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
import { Cog, Server } from "lucide-react-native";
import { useAuthStore } from "../../src/stores/authStore";
import {
  currentBaseUrl,
  probeBaseUrl,
  saveBaseUrl,
} from "../../src/api/serverUrl";
import { colors } from "../../src/theme/colors";

export default function LoginScreen() {
  const { login, busy, error, clearError } = useAuthStore();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [serverUrl, setServerUrl] = useState(() => currentBaseUrl());
  const [editingServer, setEditingServer] = useState(false);
  const [serverBusy, setServerBusy] = useState(false);
  const [serverMsg, setServerMsg] = useState<string | null>(null);

  const handleSaveServer = async () => {
    if (!serverUrl.trim()) return;
    setServerBusy(true);
    setServerMsg(null);
    try {
      await probeBaseUrl(serverUrl);
      const clean = await saveBaseUrl(serverUrl);
      setServerUrl(clean);
      setEditingServer(false);
      setServerMsg(`Connected to ${clean}`);
    } catch (e) {
      setServerMsg(e instanceof Error ? e.message : "Could not reach that server.");
    } finally {
      setServerBusy(false);
    }
  };

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
              <Cog size={28} color={colors.abyss} />
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

          {/* Server address — must be reachable from the phone (LAN IP, not localhost) */}
          <View className="bg-panel border border-edge rounded-card p-4 mt-4">
            <TouchableOpacity
              onPress={() => setEditingServer((v) => !v)}
              className="flex-row items-center justify-between"
              accessibilityRole="button"
              accessibilityLabel="Change server address"
            >
              <View className="flex-row items-center gap-2 flex-1 mr-3">
                <Server size={14} color={colors.fog} />
                <Text
                  className="text-fog text-[12px] flex-1"
                  numberOfLines={1}
                  ellipsizeMode="middle"
                >
                  {serverUrl || "…"}
                </Text>
              </View>
              <Text className="text-ember text-[12px]">
                {editingServer ? "Cancel" : "Change"}
              </Text>
            </TouchableOpacity>
            {editingServer && (
              <View className="mt-3">
                <TextInput
                  value={serverUrl}
                  onChangeText={setServerUrl}
                  autoCapitalize="none"
                  autoCorrect={false}
                  keyboardType="url"
                  placeholder="http://192.168.0.111:4000"
                  placeholderTextColor={colors.fog}
                  className="bg-panel border border-edge rounded-input px-3 py-3 text-bone text-[13px] mb-3"
                  style={{ fontFamily: "Inter_400Regular" }}
                />
                {serverMsg && (
                  <Text className="text-fog text-[12px] mb-3">{serverMsg}</Text>
                )}
                <TouchableOpacity
                  onPress={handleSaveServer}
                  disabled={serverBusy || !serverUrl.trim()}
                  className="bg-ember rounded-input py-2.5 items-center"
                  style={{ opacity: serverBusy || !serverUrl.trim() ? 0.55 : 1 }}
                  accessibilityRole="button"
                  accessibilityLabel="Save server address"
                >
                  {serverBusy ? (
                    <ActivityIndicator color={colors.abyss} size="small" />
                  ) : (
                    <Text
                      className="text-black font-medium text-[13px]"
                      style={{ fontFamily: "Inter_500Medium" }}
                    >
                      Test & save
                    </Text>
                  )}
                </TouchableOpacity>
                <Text className="text-fog text-[11px] mt-2">
                  Use your PC&apos;s LAN IP (e.g. 192.168.0.111), not localhost.
                  Phone + PC must share the same Wi-Fi.
                </Text>
              </View>
            )}
          </View>
        </ScrollView>
      </KeyboardAvoidingView>
    </SafeAreaView>
  );
}
