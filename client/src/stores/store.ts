import { create } from "zustand";
import { api } from "../lib/api";
import type { User } from "../lib/types";

interface AuthState {
  user: User | null;
  checked: boolean;
  busy: boolean;
  error: string | null;
  check: () => Promise<void>;
  login: (username: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
}

export const useAuth = create<AuthState>((set) => ({
  user: null,
  checked: false,
  busy: false,
  error: null,

  check: async () => {
    try {
      const me = await api.me();
      set({ user: me, checked: true });
    } catch {
      set({ user: null, checked: true });
    }
  },

  login: async (username, password) => {
    set({ busy: true, error: null });
    try {
      const res = await api.login(username, password);
      set({ user: { username: res.username, role: res.role }, busy: false });
    } catch (e) {
      set({ busy: false, error: e instanceof Error ? e.message : "Login failed" });
      throw e;
    }
  },

  logout: async () => {
    try {
      await api.logout();
    } finally {
      set({ user: null });
    }
  },
}));

export interface Toast {
  id: number;
  text: string;
  bad?: boolean;
}

interface UiState {
  toasts: Toast[];
  paletteOpen: boolean;
  pushToast: (text: string, bad?: boolean) => void;
  dismissToast: (id: number) => void;
  setPaletteOpen: (open: boolean) => void;
}

let toastId = 0;

export const useUi = create<UiState>((set) => ({
  toasts: [],
  paletteOpen: false,
  pushToast: (text, bad) => {
    const id = ++toastId;
    set((s) => ({ toasts: [...s.toasts, { id, text, bad }] }));
    window.setTimeout(() => {
      set((s) => ({ toasts: s.toasts.filter((t) => t.id !== id) }));
    }, 4200);
  },
  dismissToast: (id) => set((s) => ({ toasts: s.toasts.filter((t) => t.id !== id) })),
  setPaletteOpen: (open) => set({ paletteOpen: open }),
}));
