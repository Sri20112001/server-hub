import React, { useState } from "react";
import {
  View,
  Text,
  ScrollView,
  TouchableOpacity,
  Switch,
  TextInput,
  Alert,
  ActivityIndicator,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { router } from "expo-router";
import * as Haptics from "expo-haptics";
import { ArrowRight, X } from "lucide-react-native";
import { useAuthStore } from "../../../src/stores/authStore";
import { useBiometricStore } from "../../../src/stores/biometricStore";
import { authApi } from "../../../src/api/auth";
import { colors } from "../../../src/theme/colors";

function SectionHeader({ title }: { title: string }) {
  return (
    <Text className="text-[10px] font-semibold uppercase tracking-widest text-fog px-4 pt-5 pb-2">
      {title}
    </Text>
  );
}

function SettingRow({
  label,
  hint,
  right,
}: {
  label: string;
  hint?: string;
  right?: React.ReactNode;
}) {
  return (
    <View className="flex-row items-center justify-between px-4 py-3.5 border-b border-edge">
      <View className="flex-1 mr-4">
        <Text className="text-bone text-[14px]">{label}</Text>
        {hint && <Text className="text-fog text-[12px] mt-0.5">{hint}</Text>}
      </View>
      {right}
    </View>
  );
}

export default function SettingsScreen() {
  const { user, logout } = useAuthStore();
  const { enabled: biometricEnabled, supported, setEnabled } = useBiometricStore();
  const [changingPassword, setChangingPassword] = useState(false);
  const [currentPw, setCurrentPw] = useState("");
  const [newPw, setNewPw] = useState("");
  const [pwError, setPwError] = useState<string | null>(null);
  const [pwBusy, setPwBusy] = useState(false);

  const handleLogout = () => {
    Alert.alert("Sign out", "Are you sure you want to sign out?", [
      { text: "Cancel", style: "cancel" },
      {
        text: "Sign out",
        style: "destructive",
        onPress: async () => {
          void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Warning);
          await logout();
          router.replace("/(auth)/login");
        },
      },
    ]);
  };

  const handleChangePassword = async () => {
    if (!currentPw || !newPw) return;
    if (newPw.length < 8) {
      setPwError("New password must be at least 8 characters.");
      return;
    }
    setPwBusy(true);
    setPwError(null);
    try {
      await authApi.changePassword(currentPw, newPw);
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
      setChangingPassword(false);
      setCurrentPw("");
      setNewPw("");
    } catch (e) {
      setPwError(e instanceof Error ? e.message : "Failed to change password");
    } finally {
      setPwBusy(false);
    }
  };

  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: colors.abyss }}>
      <View className="px-4 pt-3 pb-3 border-b border-edge">
        <Text className="font-head font-bold text-[20px] text-bone">Settings</Text>
      </View>

      <ScrollView className="flex-1" contentContainerStyle={{ paddingBottom: 40 }}>
        <SectionHeader title="Account" />
        <View className="bg-panel border-t border-b border-edge">
          <SettingRow
            label="Username"
            hint={user?.username}
            right={
              <View className="bg-emboss border border-edge rounded-md px-2 py-0.5">
                <Text className="font-mono text-[11px] text-fog">{user?.role}</Text>
              </View>
            }
          />
          <TouchableOpacity
            onPress={() => setChangingPassword(!changingPassword)}
            className="flex-row items-center justify-between px-4 py-3.5 border-b border-edge"
            accessibilityRole="button"
          >
            <Text className="text-bone text-[14px]">Change password</Text>
            <View className="flex-row items-center gap-1">
              <Text className="text-ember text-[13px]">
                {changingPassword ? "Cancel" : "Change"}
              </Text>
              {changingPassword ? (
                <X size={14} color={colors.ember} />
              ) : (
                <ArrowRight size={14} color={colors.ember} />
              )}
            </View>
          </TouchableOpacity>

          {changingPassword && (
            <View className="px-4 py-4 border-b border-edge">
              <TextInput
                value={currentPw}
                onChangeText={setCurrentPw}
                placeholder="Current password"
                placeholderTextColor={colors.fog}
                secureTextEntry
                className="bg-emboss border border-edge rounded-input px-3 py-2.5 text-bone text-[13px] mb-3"
                style={{ fontFamily: "Inter_400Regular" }}
              />
              <TextInput
                value={newPw}
                onChangeText={setNewPw}
                placeholder="New password (min 8 chars)"
                placeholderTextColor={colors.fog}
                secureTextEntry
                className="bg-emboss border border-edge rounded-input px-3 py-2.5 text-bone text-[13px] mb-3"
                style={{ fontFamily: "Inter_400Regular" }}
              />
              {pwError && (
                <Text className="text-brick text-[12px] mb-3">{pwError}</Text>
              )}
              <TouchableOpacity
                onPress={handleChangePassword}
                disabled={pwBusy || !currentPw || !newPw}
                className="bg-ember rounded-input py-2.5 items-center"
                style={{ opacity: pwBusy || !currentPw || !newPw ? 0.55 : 1 }}
                accessibilityRole="button"
              >
                {pwBusy ? (
                  <ActivityIndicator color={colors.abyss} size="small" />
                ) : (
                  <Text className="text-black font-medium text-[13px]">
                    Update password
                  </Text>
                )}
              </TouchableOpacity>
            </View>
          )}
        </View>

        <SectionHeader title="Security" />
        <View className="bg-panel border-t border-b border-edge">
          <SettingRow
            label="Biometric lock"
            hint={
              supported
                ? "Require fingerprint or face to open the app"
                : "No biometric hardware enrolled on this device"
            }
            right={
              supported ? (
                <Switch
                  value={biometricEnabled}
                  onValueChange={(v) => void setEnabled(v)}
                  trackColor={{ false: colors.edge, true: colors.ember }}
                  thumbColor={colors.bone}
                  accessibilityLabel="Toggle biometric lock"
                />
              ) : undefined
            }
          />
        </View>

        <SectionHeader title="Connection" />
        <View className="bg-panel border-t border-b border-edge">
          <SettingRow
            label="API endpoint"
            hint={
              (process.env.EXPO_PUBLIC_API_URL as string | undefined) ??
              "http://localhost:4000"
            }
          />
        </View>

        <SectionHeader title="About" />
        <View className="bg-panel border-t border-b border-edge">
          <SettingRow label="App" hint="Server Hub Mobile" />
          <SettingRow label="Version" hint="1.0.0" />
          <SettingRow label="Package" hint="com.serverhub.mobile" />
        </View>

        <View className="px-4 mt-6">
          <TouchableOpacity
            onPress={handleLogout}
            className="border border-brick rounded-input py-3 items-center"
            accessibilityRole="button"
            accessibilityLabel="Sign out"
          >
            <Text className="text-brick font-medium text-[14px]">Sign out</Text>
          </TouchableOpacity>
        </View>
      </ScrollView>
    </SafeAreaView>
  );
}
