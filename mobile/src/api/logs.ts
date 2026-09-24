import { client } from "./client";
import type { AppLog } from "../types";

interface LogParams {
  level?: string;
  source?: string;
  search?: string;
  projectId?: number;
  limit?: number;
  offset?: number;
}

export const logsApi = {
  list: (params?: LogParams) => {
    const q = new URLSearchParams();
    if (params?.level) q.set("level", params.level);
    if (params?.source) q.set("source", params.source);
    if (params?.search) q.set("search", params.search);
    if (params?.projectId !== undefined)
      q.set("projectId", String(params.projectId));
    if (params?.limit !== undefined) q.set("limit", String(params.limit));
    if (params?.offset !== undefined) q.set("offset", String(params.offset));
    const suffix = q.toString() ? `?${q.toString()}` : "";
    return client
      .get<AppLog[]>(`/server-hub/api/logs${suffix}`)
      .then((r) => r.data);
  },
};
