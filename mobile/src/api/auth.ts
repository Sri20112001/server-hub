import { client } from "./client";
import type { User } from "../types";

export const authApi = {
  login: (username: string, password: string) =>
    client
      .post<{ username: string; role: string; token: string }>(
        "/server-hub/api/auth/login",
        { username, password },
      )
      .then((r) => r.data),

  logout: () =>
    client.post<{ ok: boolean }>("/server-hub/api/auth/logout").then((r) => r.data),

  me: () =>
    client.get<User>("/server-hub/api/auth/me").then((r) => r.data),

  changePassword: (currentPassword: string, newPassword: string) =>
    client
      .put<{ ok: boolean }>("/server-hub/api/auth/password", {
        currentPassword,
        newPassword,
      })
      .then((r) => r.data),
};
