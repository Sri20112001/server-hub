import { client } from "./client";
import { authLogout, authMe, authPassword, authToken } from "@serverhub/shared";
import type { User } from "@serverhub/shared";

export const authApi = {
  login: (username: string, password: string) =>
    client
      .post<{ username: string; role: string; token: string; refreshToken?: string }>(
        authToken(),
        { username, password },
      )
      .then((r) => r.data),

  logout: () =>
    client.post<{ ok: boolean }>(authLogout()).then((r) => r.data),

  me: () =>
    client.get<User>(authMe()).then((r) => r.data),

  changePassword: (currentPassword: string, newPassword: string) =>
    client
      .put<{ ok: boolean }>(authPassword(), {
        currentPassword,
        newPassword,
      })
      .then((r) => r.data),
};
