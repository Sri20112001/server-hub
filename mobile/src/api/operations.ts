import { client } from "./client";
import type { Operation } from "../types";

export const operationsApi = {
  list: (limit = 20) =>
    client
      .get<Operation[]>(`/server-hub/api/operations?limit=${limit}`)
      .then((r) => r.data),

  get: (id: string) =>
    client
      .get<Operation>(`/server-hub/api/operations/${id}`)
      .then((r) => r.data),
};
