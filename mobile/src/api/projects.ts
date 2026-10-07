import { client } from "./client";
import { project, projectAction, projectDeployments, projectServices, projects } from "@serverhub/shared";
import type { Project, Service, Deployment } from "@serverhub/shared";

export const projectsApi = {
  list: () =>
    client.get<Project[]>(projects()).then((r) => r.data),

  get: (id: number) =>
    client.get<Project>(project(id)).then((r) => r.data),

  services: (projectId: number) =>
    client
      .get<Service[]>(projectServices(projectId))
      .then((r) => r.data),

  deployments: (projectId: number) =>
    client
      .get<Deployment[]>(projectDeployments(projectId))
      .then((r) => r.data),

  action: (
    id: number,
    action: "start" | "stop" | "restart",
    confirm = false,
  ) =>
    client
      .post<{ ok: boolean; logs?: string }>(
        projectAction(id, action, confirm),
      )
      .then((r) => r.data),
};
