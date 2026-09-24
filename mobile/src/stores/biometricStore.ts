import { create } from "zustand";
import * as SecureStore from "expo-secure-store";
import * as LocalAuthentication from "expo-local-authentication";

const BIOMETRIC_KEY = "serverhub_biometric_enabled";

interface BiometricState {
  enabled: boolean;
  supported: boolean;
  load: () => Promise<void>;
  setEnabled: (v: boolean) => Promise<void>;
  authenticate: () => Promise<boolean>;
}

export const useBiometricStore = create<BiometricState>((set, get) => ({
  enabled: false,
  supported: false,

  load: async () => {
    const supported = await LocalAuthentication.hasHardwareAsync();
    const enrolled = await LocalAuthentication.isEnrolledAsync();
    const raw = await SecureStore.getItemAsync(BIOMETRIC_KEY);
    set({ supported: supported && enrolled, enabled: raw === "true" });
  },

  setEnabled: async (v) => {
    await SecureStore.setItemAsync(BIOMETRIC_KEY, v ? "true" : "false");
    set({ enabled: v });
  },

  authenticate: async () => {
    if (!get().enabled) return true;
    const result = await LocalAuthentication.authenticateAsync({
      promptMessage: "Authenticate to access Server Hub",
      fallbackLabel: "Use password",
    });
    return result.success;
  },
}));
