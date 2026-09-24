import { client } from "./client";
import type { Project, Service, Deployment } from "../types";

export const projectsApi = {
  list: () =>
    client.get<Project[]>("/server-hub/api/projects").then((r) => r.data),

  get: (id: number) =>
    client.get<Project>(`/server-hub/api/projects/${id}`).then((r) => r.data),

  services: (projectId: number) =>
    client
      .get<Service[]>(`/server-hub/api/projects/${projectId}/services`)
      .then((r) => r.data),

  deployments: (projectId: number) =>
    client
      .get<Deployment[]>(`/server-hub/api/projects/${projectId}/deployments`)
      .then((r) => r.data),

  action: (
    id: number,
    action: "start" | "stop" | "restart",
    confirm = false,
  ) =>
    client
      .post<{ ok: boolean; logs?: string }>(
        `/server-hub/api/projects/${id}/${action}${confirm ? "?confirm=true" : ""}`,
      )
      .then((r) => r.data),
};
