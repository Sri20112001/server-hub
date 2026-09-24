import axios from "axios";
import * as SecureStore from "expo-secure-store";

export const API_BASE =
  (process.env.EXPO_PUBLIC_API_URL as string | undefined) ?? "http://localhost:4000";

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

// Normalize errors
client.interceptors.response.use(
  (res) => res,
  (error) => {
    const status: number = error.response?.status ?? 0;
    const msg: string =
      error.response?.data?.error ??
      error.message ??
      `Request failed (${status})`;
    return Promise.reject(new ApiError(status, msg));
  },
);
