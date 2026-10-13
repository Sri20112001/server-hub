import { afterEach, describe, expect, it, vi } from "vitest";
import * as ep from "@serverhub/shared";
import { ApiError, dbBackupsApi } from "./api";
import type { VgAgent, VgConnection } from "@serverhub/shared";

// Phase 4 contract: endpoint paths must match server/cmd/server/main.go
// (/server-hub/api/db-backups), and the client must surface server errors
// without ever shipping secrets (redaction is server-enforced; these tests
// pin the shapes both sides agree on).
describe("dbBackups endpoints", () => {
  it("builds management paths under /server-hub/api/db-backups", () => {
    expect(ep.dbBackupsDashboard()).toBe("/server-hub/api/db-backups/dashboard");
    expect(ep.dbBackupsTargets()).toBe("/server-hub/api/db-backups/storage-targets");
    expect(ep.dbBackupsTarget("a/b")).toBe("/server-hub/api/db-backups/storage-targets/a%2Fb");
    expect(ep.dbBackupsTargetTest()).toBe("/server-hub/api/db-backups/storage-targets/test");
    expect(ep.dbBackupsConnections()).toBe("/server-hub/api/db-backups/connections");
    expect(ep.dbBackupsConnection("x")).toBe("/server-hub/api/db-backups/connections/x");
    expect(ep.dbBackupsConnectionTest()).toBe("/server-hub/api/db-backups/connections/test");
    expect(ep.dbBackupsPolicies()).toBe("/server-hub/api/db-backups/policies");
    expect(ep.dbBackupsPolicy("p")).toBe("/server-hub/api/db-backups/policies/p");
    expect(ep.dbBackupsJobs()).toBe("/server-hub/api/db-backups/jobs");
    expect(ep.dbBackupsJob("j")).toBe("/server-hub/api/db-backups/jobs/j");
    expect(ep.dbBackupsJobEnable("j")).toBe("/server-hub/api/db-backups/jobs/j/enable");
    expect(ep.dbBackupsJobDisable("j")).toBe("/server-hub/api/db-backups/jobs/j/disable");
    expect(ep.dbBackupsJobRun("j")).toBe("/server-hub/api/db-backups/jobs/j/run");
    expect(ep.dbBackupsJobPreflight("j")).toBe("/server-hub/api/db-backups/jobs/j/preflight");
    expect(ep.dbBackupsRun("r")).toBe("/server-hub/api/db-backups/runs/r");
    expect(ep.dbBackupsRunArtifacts("r")).toBe("/server-hub/api/db-backups/runs/r/artifacts");
    expect(ep.dbBackupsRunCancel("r")).toBe("/server-hub/api/db-backups/runs/r/cancel");
    expect(ep.dbBackupsRunVerify("r")).toBe("/server-hub/api/db-backups/runs/r/verify");
    expect(ep.dbBackupsRestores()).toBe("/server-hub/api/db-backups/restores");
    expect(ep.dbBackupsRestore("r")).toBe("/server-hub/api/db-backups/restores/r");
    expect(ep.dbBackupsAgents()).toBe("/server-hub/api/db-backups/agents");
    expect(ep.dbBackupsAgent("a")).toBe("/server-hub/api/db-backups/agents/a");
    expect(ep.dbBackupsAgentToken()).toBe("/server-hub/api/db-backups/agents/token");
    expect(ep.dbBackupsAgentRotate("a")).toBe("/server-hub/api/db-backups/agents/a/rotate-token");
    expect(ep.dbBackupsAgentRevoke("a")).toBe("/server-hub/api/db-backups/agents/a/revoke");
    expect(ep.dbBackupsEnrollTokens()).toBe("/server-hub/api/db-backups/enrollment-tokens");
    expect(ep.dbBackupsEnrollToken("e")).toBe("/server-hub/api/db-backups/enrollment-tokens/e");
    expect(ep.dbBackupsMachines()).toBe("/server-hub/api/db-backups/machines");
    expect(ep.dbBackupsMachine("m")).toBe("/server-hub/api/db-backups/machines/m");
  });

  it("builds run list queries with filters and pagination", () => {
    const url = ep.dbBackupsRuns({ job_id: "j", status: "FAILED", search: "nightly", page: 2, limit: 20, sort: "started", order: "asc" });
    expect(url.startsWith("/server-hub/api/db-backups/runs?")).toBe(true);
    for (const part of ["job_id=j", "status=FAILED", "search=nightly", "page=2", "limit=20", "sort=started", "order=asc"]) {
      expect(url).toContain(part);
    }
    expect(ep.dbBackupsRuns()).toBe("/server-hub/api/db-backups/runs");
  });
});

describe("dbBackupsApi client", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  function stubFetch(handler: (url: string, init?: RequestInit) => Response) {
    const calls: { url: string; init?: RequestInit }[] = [];
    vi.stubGlobal("fetch", vi.fn(async (url: string, init?: RequestInit) => {
      calls.push({ url, init });
      return handler(url, init);
    }));
    return calls;
  }

  const json = (body: unknown, status = 200) =>
    new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });

  it("sends credentials and parses lists", async () => {
    const calls = stubFetch((url) => {
      expect(url).toBe("http://localhost:4000/server-hub/api/db-backups/jobs");
      return json([{ id: "j1", name: "nightly" }]);
    });
    const jobs = await dbBackupsApi.jobs();
    expect(jobs).toHaveLength(1);
    expect(calls[0].init?.credentials).toBe("include");
  });

  it("uses the right HTTP methods for mutations", async () => {
    const calls = stubFetch(() => json({ ok: true }));
    await dbBackupsApi.updateJob("j", { name: "x" });
    await dbBackupsApi.deleteJob("j");
    await dbBackupsApi.enableJob("j");
    await dbBackupsApi.runJob("j");
    await dbBackupsApi.createRestore({ backup_run_id: "r", agent_id: "a", confirmed: true });
    const methods = calls.map((c) => (c.init?.method ?? "GET") + " " + c.url);
    expect(methods).toContain("PUT http://localhost:4000/server-hub/api/db-backups/jobs/j");
    expect(methods).toContain("DELETE http://localhost:4000/server-hub/api/db-backups/jobs/j");
    expect(methods).toContain("POST http://localhost:4000/server-hub/api/db-backups/jobs/j/enable");
    expect(methods).toContain("POST http://localhost:4000/server-hub/api/db-backups/jobs/j/run");
    const restoreBody = JSON.parse(calls[4].init?.body as string);
    expect(restoreBody.confirmed).toBe(true);
  });

  it("surfaces server error messages as ApiError", async () => {
    stubFetch(() => json({ error: "only COMPLETED runs can be restored" }, 400));
    await expect(dbBackupsApi.createRestore({ backup_run_id: "r", agent_id: "a", confirmed: true }))
      .rejects.toMatchObject({ status: 400 } as Partial<ApiError>);
    try {
      await dbBackupsApi.jobs();
      expect.unreachable("should have thrown");
    } catch (e) {
      expect(e).toBeInstanceOf(ApiError);
      expect((e as Error).message).toBe("only COMPLETED runs can be restored");
    }
  });
});

describe("secret redaction contract", () => {
  const FORBIDDEN = [
    "password", "token_hash", "agent_token", "registration_key",
    "secret_key", "access_key", "encrypted_password", "encrypted_access_key",
    "encrypted_secret_key", "data_key", "data_key_encrypted",
  ];

  it("connection payloads carry no secrets", () => {
    const conn = {
      id: "c1", agent_id: "a1", name: "prod", type: "POSTGRES",
      host: "db", port: 5432, username: "u", status: "UNKNOWN",
      last_checked_at: null, databases: [], database_count: 0,
      created_at: "", updated_at: "",
    } satisfies VgConnection;
    for (const key of Object.keys(conn)) {
      expect(FORBIDDEN).not.toContain(key);
    }
  });

  it("agent payloads carry no credentials", () => {
    const agent = {
      id: "a1", name: "db01", platform: "", architecture: "",
      machine_name: "", status: "OFFLINE", lifecycle: "PENDING",
      version: "", last_seen_at: null, installed_at: null,
      revoked_at: null, created_at: "",
    } satisfies VgAgent;
    for (const key of Object.keys(agent)) {
      expect(FORBIDDEN).not.toContain(key);
    }
  });
});
