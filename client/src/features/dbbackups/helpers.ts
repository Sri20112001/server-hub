import { useAuth } from "../../stores/store";

// Role gating mirrors the backend groups (viewer reads, operator mutates,
// admin destroys/mints). The server owns authorization; these flags only
// decide which controls to render.
export function useDbBackupsRole() {
  const { user } = useAuth();
  const isAdmin = user?.role === "admin";
  const canMutate = user?.role === "operator" || isAdmin;
  return { canMutate, isAdmin };
}

export function timeStr(iso: string | null | undefined): string {
  if (!iso) return "—";
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? "—" : d.toLocaleString();
}

const DOT: Record<string, string> = {
  // runs
  PENDING: "bg-outline",
  RUNNING: "bg-primary",
  UPLOADING: "bg-primary",
  VERIFYING: "bg-tertiary",
  COMPLETED: "bg-primary",
  FAILED: "bg-error",
  CANCELLED: "bg-outline",
  // agents / checks
  ONLINE: "bg-primary",
  OFFLINE: "bg-error",
  UP: "bg-primary",
  DOWN: "bg-error",
  UNKNOWN: "bg-outline",
  // generic
  CONNECTED: "bg-primary",
  DISCONNECTED: "bg-error",
  ERROR: "bg-error",
};

export function statusDot(status: string | undefined): string {
  return DOT[(status ?? "").toUpperCase()] ?? "bg-outline";
}

export const inputCls =
  "w-full bg-surface-container-low border border-outline-variant/30 rounded-xl px-3.5 py-2.5 text-on-surface text-sm outline-none focus:border-primary placeholder:text-on-surface-variant/40 transition-colors";

export const labelCls = "text-on-surface-variant text-[11px] font-semibold uppercase tracking-wider mb-1.5 block";

export const primaryBtn =
  "px-5 py-2 text-sm bg-primary text-on-primary font-semibold rounded-full disabled:opacity-50 hover:bg-primary/90 transition-all cursor-pointer shadow-sm";

export const ghostBtn =
  "px-4 py-2 text-sm text-on-surface-variant hover:text-on-surface border border-outline-variant/30 rounded-full cursor-pointer hover:bg-surface-container transition-colors";

export const dangerIconBtn =
  "text-on-surface-variant hover:text-error p-1.5 cursor-pointer rounded-full transition-colors";
