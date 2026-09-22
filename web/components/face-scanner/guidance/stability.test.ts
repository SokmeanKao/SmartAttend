import { describe, expect, it } from "vitest";

import { updateStability } from "./stability";

describe("stability", () => {
  it("becomes stable after window and resets when not ready", () => {
    let s = updateStability({
      ready: true,
      nowMs: 0,
      stableSince: null,
      windowMs: 400,
    });
    expect(s.stable).toBe(false);
    expect(s.stableSince).toBe(0);

    s = updateStability({
      ready: true,
      nowMs: 400,
      stableSince: s.stableSince,
      windowMs: 400,
    });
    expect(s.stable).toBe(true);

    s = updateStability({
      ready: false,
      nowMs: 450,
      stableSince: s.stableSince,
      windowMs: 400,
    });
    expect(s.stable).toBe(false);
    expect(s.stableSince).toBeNull();
  });

  it("resets timer when readiness flickers", () => {
    let s = updateStability({
      ready: true,
      nowMs: 100,
      stableSince: null,
      windowMs: 400,
    });
    s = updateStability({
      ready: false,
      nowMs: 200,
      stableSince: s.stableSince,
      windowMs: 400,
    });
    s = updateStability({
      ready: true,
      nowMs: 300,
      stableSince: s.stableSince,
      windowMs: 400,
    });
    expect(s.stable).toBe(false);
    expect(s.stableSince).toBe(300);
  });
});
