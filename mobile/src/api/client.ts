import axios from "axios";
import * as SecureStore from "expo-secure-store";

const envUrl = (process.env.EXPO_PUBLIC_API_URL as string | undefined)?.trim();

// Build-time default. Prefer the env value, but never crash the app at
// import time if it is missing — the user can set the server address on the
// login screen (stored in SecureStore, see serverUrl.ts).
export const DEFAULT_API_BASE =
  envUrl && envUrl.length > 0 ? envUrl : "http://192.168.0.111:4000";

export const API_BASE = DEFAULT_API_BASE;

export const TOKEN_KEY = "serverhub_token";
export const USER_KEY = "serverhub_user";

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

export const client = axios.create({
  baseURL: API_BASE,
  timeout: 15000,
  headers: { "Content-Type": "application/json" },
});

// Inject stored JWT on every request
client.interceptors.request.use(async (config) => {
  const token = await SecureStore.getItemAsync(TOKEN_KEY);
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

// Normalize errors — and make "can't reach server" actionable by naming
// the base URL the app is actually pointing at.
client.interceptors.response.use(
  (res) => res,
  (error) => {
    const status: number = error.response?.status ?? 0;
    let msg: string =
      error.response?.data?.error ??
      error.message ??
      `Request failed (${status})`;
    if (status === 0) {
      const base = (client.defaults.baseURL as string | undefined) ?? API_BASE;
      msg = `Can't reach the server at ${base}. Check the server URL, that the backend is running, and that phone + PC share the same Wi-Fi.`;
    }
    return Promise.reject(new ApiError(status, msg));
  },
);
