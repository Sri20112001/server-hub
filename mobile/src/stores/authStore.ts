import { create } from "zustand";
import * as SecureStore from "expo-secure-store";
import { authApi } from "../api/auth";
import { TOKEN_KEY, USER_KEY } from "../api/client";
import type { User } from "../types";

interface AuthState {
  user: User | null;
  token: string | null;
  checked: boolean;
  busy: boolean;
  error: string | null;
  hydrate: () => Promise<void>;
  login: (username: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  clearError: () => void;
}

export const useAuthStore = create<AuthState>((set) => ({
  user: null,
  token: null,
  checked: false,
  busy: false,
  error: null,

  hydrate: async () => {
    try {
      const [token, userJson] = await Promise.all([
        SecureStore.getItemAsync(TOKEN_KEY),
        SecureStore.getItemAsync(USER_KEY),
      ]);
      if (token && userJson) {
        // Verify token is still valid, use fresh user data from server
        try {
          const freshUser = await authApi.me();
          await SecureStore.setItemAsync(USER_KEY, JSON.stringify(freshUser));
          set({ user: freshUser, token, checked: true });
        } catch {
          await SecureStore.deleteItemAsync(TOKEN_KEY);
          await SecureStore.deleteItemAsync(USER_KEY);
          set({ user: null, token: null, checked: true });
        }
      } else {
        set({ checked: true });
      }
    } catch {
      set({ checked: true });
    }
  },

  login: async (username, password) => {
    set({ busy: true, error: null });
    try {
      const res = await authApi.login(username, password);
      const user: User = { username: res.username, role: res.role };
      await SecureStore.setItemAsync(TOKEN_KEY, res.token);
      await SecureStore.setItemAsync(USER_KEY, JSON.stringify(user));
      set({ user, token: res.token, busy: false });
    } catch (e) {
      set({
        busy: false,
        error: e instanceof Error ? e.message : "Login failed",
      });
      throw e;
    }
  },

  logout: async () => {
    try {
      await authApi.logout();
    } catch {
      // ignore — clear local state regardless
    } finally {
      await SecureStore.deleteItemAsync(TOKEN_KEY);
      await SecureStore.deleteItemAsync(USER_KEY);
      set({ user: null, token: null });
    }
  },

  clearError: () => set({ error: null }),
}));
