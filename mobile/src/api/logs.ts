import { client } from "./client";
import { logs } from "@serverhub/shared";
import type { AppLog, LogsParams } from "@serverhub/shared";

export const logsApi = {
  list: (params?: LogsParams) =>
    client.get<AppLog[]>(logs(params)).then((r) => r.data),
};
