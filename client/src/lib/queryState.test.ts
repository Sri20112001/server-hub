import { describe, expect, it } from "vitest";
import { queryState } from "@serverhub/shared";

// Phase 1A honesty contract: a failed fetch must surface as "error" (with
// retry in the UI), never as an endless skeleton and never as
// healthy-looking empty content. Both dashboard pages branch on this.
describe("queryState", () => {
  it("is ready whenever data exists, even mid-refetch or with a stale error", () => {
    expect(queryState({ hasData: true, isLoading: false, isError: false })).toBe("ready");
    expect(queryState({ hasData: true, isLoading: true, isError: false })).toBe("ready");
    expect(queryState({ hasData: true, isLoading: false, isError: true })).toBe("ready");
  });

  it("is loading on first fetch and while retrying after an error", () => {
    expect(queryState({ hasData: false, isLoading: true, isError: false })).toBe("loading");
    expect(queryState({ hasData: false, isLoading: true, isError: true })).toBe("loading");
  });

  it("is error only once loading has settled without data", () => {
    expect(queryState({ hasData: false, isLoading: false, isError: true })).toBe("error");
  });

  it("reads an indeterminate first frame as loading, not error", () => {
    expect(queryState({ hasData: false, isLoading: false, isError: false })).toBe("loading");
  });
});
