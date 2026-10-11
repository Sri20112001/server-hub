import { describe, expect, it } from "vitest";
import {
  describeAlert,
  fleetLabel,
  formatAge,
  STALE_AFTER_MS,
  toBusinessStatus,
  toneRank,
  type BusinessTone,
} from "@serverhub/shared";

// Phase 1B plain-language contract: every machine status word maps onto
// five owner-readable tones, and user-facing labels contain no nautical or
// DevOps metaphors.
describe("toBusinessStatus", () => {
  it("maps working states", () => {
    for (const raw of ["sailing", "healthy", "OK", "Success", "running", "UP", "online", "nominal", "resolved"]) {
      expect(toBusinessStatus(raw)).toEqual({ tone: "working", label: "Working" });
    }
  });

  it("maps degraded states to needs-attention", () => {
    for (const raw of ["choppy", "degraded", "WARNING", "watch"]) {
      expect(toBusinessStatus(raw)).toEqual({ tone: "attention", label: "Needs attention" });
    }
  });

  it("maps failure states to down", () => {
    for (const raw of [
      "lost", "down", "FAILED", "error", "critical", "offline",
      "disconnected", "firing", "triggered", "unreachable",
    ]) {
      expect(toBusinessStatus(raw)).toEqual({ tone: "down", label: "Down" });
    }
  });

  it("maps deliberately-stopped states, which are not problems", () => {
    for (const raw of ["docked", "stopped", "paused", "disabled"]) {
      expect(toBusinessStatus(raw)).toEqual({ tone: "stopped", label: "Stopped" });
    }
  });

  it("maps unknown, empty, and garbage input to unknown — never working", () => {
    for (const raw of [undefined, "", "   ", "blorple", "sailing-ish", "up-ish"]) {
      expect(toBusinessStatus(raw)).toEqual({ tone: "unknown", label: "Unknown" });
    }
  });

  it("orders tones most-severe-first for needs-attention lists", () => {
    const tones: BusinessTone[] = ["working", "unknown", "stopped", "attention", "down"];
    expect(tones.sort((a, b) => toneRank(a) - toneRank(b))).toEqual([
      "down",
      "attention",
      "unknown",
      "stopped",
      "working",
    ]);
  });
});

describe("fleetLabel", () => {
  it("uses plain words everywhere the pill renders", () => {
    expect(fleetLabel).toEqual({
      sailing: "Working",
      choppy: "Needs attention",
      lost: "Down",
      docked: "Stopped",
    });
  });
});

describe("formatAge", () => {
  it("reads fresh data as just now", () => {
    expect(formatAge(0)).toBe("just now");
    expect(formatAge(9_999)).toBe("just now");
  });

  it("reads clock skew and garbage as just now, never a future age", () => {
    expect(formatAge(-5_000)).toBe("just now");
    expect(formatAge(Number.NaN)).toBe("just now");
  });

  it("escalates through seconds, minutes, hours", () => {
    expect(formatAge(20_000)).toBe("20s ago");
    expect(formatAge(5 * 60_000)).toBe("5 min ago");
    expect(formatAge(2 * 3_600_000)).toBe("2 hr ago");
  });

  it("flags readings older than the stale threshold", () => {
    expect(STALE_AFTER_MS).toBe(90_000);
    expect(formatAge(STALE_AFTER_MS + 1)).not.toBe("just now");
  });
});

describe("describeAlert", () => {
  it("explains an offline server with a next step", () => {
    const d = describeAlert({ condition: "agent_offline", message: "Server X offline" });
    expect(d.what).toMatch(/stopped reporting/i);
    expect(d.next).toMatch(/powered on|agent/i);
    expect(d.next).toMatch(/doesn't fix the cause/);
  });

  it("explains CPU, memory, disk, and failed checks distinctly", () => {
    expect(describeAlert({ condition: "cpu_high" }).what).toMatch(/processor|working very hard/i);
    expect(describeAlert({ condition: "mem_high" }).what).toMatch(/memory/i);
    expect(describeAlert({ condition: "disk_low" }).what).toMatch(/disk space/i);
    expect(describeAlert({ condition: "health_http" }).what).toMatch(/didn't respond/i);
    expect(describeAlert({ condition: "health_http" }).next).toMatch(/restart/i);
  });

  it("includes the alert limit when a threshold is set", () => {
    expect(describeAlert({ condition: "cpu_high", threshold: 85 }).what).toMatch(/85/);
  });

  it("attributes external-monitor alerts without pretending local knowledge", () => {
    const d = describeAlert({ condition: "Watchdog", source: "alertmanager" });
    expect(d.what).toMatch(/external monitor/i);
    expect(d.next).toMatch(/Monitoring/);
  });

  it("falls back to the message with a resolve warning, never an empty card", () => {
    const d = describeAlert({ condition: "mystery", message: "Something odd" });
    expect(d.what).toBe("Something odd");
    expect(d.next).toMatch(/doesn't fix the cause/);
  });
});
