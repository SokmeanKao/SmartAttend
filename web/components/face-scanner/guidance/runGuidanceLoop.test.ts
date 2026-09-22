import { describe, expect, it, vi } from "vitest";

import { createGuidanceLoop } from "./runGuidanceLoop";

describe("createGuidanceLoop", () => {
  it("never overlaps detect calls and adapts interval when slow", () => {
    let now = 1000;
    const frames: FrameRequestCallback[] = [];
    const detect = vi.fn(() => {
      now += 120; // slow inference
      return { ok: true };
    });
    const onGuidance = vi.fn();

    const video = {
      readyState: 4,
      videoWidth: 640,
      videoHeight: 480,
      currentTime: 0,
    } as HTMLVideoElement;

    const loop = createGuidanceLoop({
      getVideo: () => video,
      isActive: () => true,
      detect,
      onGuidance,
      targetIntervalMs: 80,
      now: () => now,
      requestFrame: (cb) => {
        frames.push(cb);
        return frames.length;
      },
      cancelFrame: () => {},
    });

    loop.start();
    expect(frames.length).toBe(1);

    video.currentTime = 0.1;
    const first = frames.shift()!;
    first(0);
    expect(detect).toHaveBeenCalledTimes(1);
    expect(onGuidance).toHaveBeenCalledTimes(1);

    // Next scheduled frame arrives too soon — skip detect
    const second = frames.shift()!;
    video.currentTime = 0.15;
    second(40);
    expect(detect).toHaveBeenCalledTimes(1);

    // After adapted interval, detect again
    const third = frames.shift()!;
    video.currentTime = 0.3;
    third(200);
    expect(detect).toHaveBeenCalledTimes(2);
  });

  it("skips work when inactive", () => {
    const onGuidance = vi.fn();
    let active = true;
    const video = {
      readyState: 4,
      videoWidth: 640,
      videoHeight: 480,
      currentTime: 1,
    } as HTMLVideoElement;

    const frames: FrameRequestCallback[] = [];
    const loop = createGuidanceLoop({
      getVideo: () => video,
      isActive: () => active,
      detect: () => ({ ok: true }),
      onGuidance,
      targetIntervalMs: 0,
      now: () => 0,
      requestFrame: (cb) => {
        frames.push(cb);
        return frames.length;
      },
      cancelFrame: () => {},
    });

    loop.start();
    const first = frames.shift()!;
    first(0);
    expect(onGuidance).toHaveBeenCalledTimes(1);

    active = false;
    video.currentTime = 2;
    const next = frames.shift()!;
    next(50);
    expect(onGuidance).toHaveBeenCalledTimes(1);
  });
});
