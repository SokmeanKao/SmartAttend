import { describe, expect, it, vi } from "vitest";

import { createCaptureCoordinator } from "./captureCoordinator";

describe("captureCoordinator", () => {
  it("rejects second beginCapture while SUBMITTING", () => {
    const c = createCaptureCoordinator({ cooldownMs: 0 });
    expect(c.beginCapture()).toBe(true);
    c.markSubmitting();
    expect(c.getState()).toBe("SUBMITTING");
    expect(c.beginCapture()).toBe(false);
  });

  it("returns to IDLE after settle + cooldown", () => {
    vi.useFakeTimers();
    const c = createCaptureCoordinator({
      cooldownMs: 10,
      schedule: (fn, ms) => setTimeout(fn, ms) as unknown as number,
      cancel: (id) => clearTimeout(id),
    });
    expect(c.beginCapture()).toBe(true);
    c.markSubmitting();
    c.settle({ status: "ACCEPTED" });
    expect(c.getState()).toBe("COOLDOWN");
    vi.advanceTimersByTime(10);
    expect(c.getState()).toBe("IDLE");
    vi.useRealTimers();
  });

  it("manual and auto share the same in-flight lock", () => {
    const c = createCaptureCoordinator({ cooldownMs: 0 });
    expect(c.beginCapture()).toBe(true);
    expect(c.canStartCapture()).toBe(false);
    c.markSubmitting();
    c.settle({ status: "REJECTED" });
    expect(c.getState()).toBe("IDLE");
    expect(c.beginCapture()).toBe(true);
  });
});
