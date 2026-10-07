import { client } from "./client";
import { operation, operations } from "@serverhub/shared";
import type { Operation } from "@serverhub/shared";

export const operationsApi = {
  list: (limit = 20) =>
    client
      .get<Operation[]>(operations(limit))
      .then((r) => r.data),

  get: (id: string) =>
    client
      .get<Operation>(operation(id))
      .then((r) => r.data),
};
