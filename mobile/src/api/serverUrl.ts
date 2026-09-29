import * as SecureStore from "expo-secure-store";
import { client, DEFAULT_API_BASE } from "./client";

export const SERVER_URL_KEY = "serverhub_api_url";

/** Strip whitespace/trailing slashes so `http://host:4000/` and `http://host:4000` behave the same. */
export function normalizeUrl(url: string): string {
  return url.trim().replace(/\/+$/, "");
}

/** Build-time default baked via EXPO_PUBLIC_API_URL (see mobile/.env). */
export function defaultBaseUrl(): string {
  return normalizeUrl(DEFAULT_API_BASE);
}

/** Current base URL in effect (may be an override set at runtime). */
export function currentBaseUrl(): string {
  return normalizeUrl(
    (client.defaults.baseURL as string | undefined) ?? DEFAULT_API_BASE,
  );
}

/** Apply a base URL to the shared axios instance immediately. */
export function applyBaseUrl(url: string): string {
  const clean = normalizeUrl(url);
  client.defaults.baseURL = clean;
  return clean;
}

/** Load a previously saved override (if any) and apply it. Call at startup. */
export async function initStoredBaseUrl(): Promise<string> {
  try {
    const stored = await SecureStore.getItemAsync(SERVER_URL_KEY);
    if (stored && stored.trim()) {
      return applyBaseUrl(stored);
    }
  } catch {
    // SecureStore unavailable — fall through to the build-time default.
  }
  return applyBaseUrl(defaultBaseUrl());
}

/** Persist a user-provided server URL and switch to it without a rebuild. */
export async function saveBaseUrl(url: string): Promise<string> {
  const clean = normalizeUrl(url);
  if (!/^https?:\/\/.+/.test(clean)) {
    throw new Error("Server URL must start with http:// or https://");
  }
  await SecureStore.setItemAsync(SERVER_URL_KEY, clean);
  return applyBaseUrl(clean);
}

/** Forget the override and return to the build-time default. */
export async function resetBaseUrl(): Promise<string> {
  try {
    await SecureStore.deleteItemAsync(SERVER_URL_KEY);
  } catch {
    // ignore
  }
  return applyBaseUrl(defaultBaseUrl());
}

/** Quick reachability probe used by the UI before saving. */
export async function probeBaseUrl(url: string, timeoutMs = 8000): Promise<void> {
  const clean = normalizeUrl(url);
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  try {
    const res = await fetch(`${clean}/health`, { signal: controller.signal });
    if (!res.ok) {
      throw new Error(`Server answered with HTTP ${res.status}`);
    }
  } catch (e) {
    if (e instanceof Error && e.name === "AbortError") {
      throw new Error(
        "Timed out reaching the server. Check the IP, port 4000, and that phone + PC share the same Wi-Fi.",
      );
    }
    throw e instanceof Error
      ? e
      : new Error("Could not reach the server at that address.");
  } finally {
    clearTimeout(timer);
  }
}
