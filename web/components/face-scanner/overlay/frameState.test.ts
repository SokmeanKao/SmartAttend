import { describe, expect, it } from "vitest";

import type { FaceGuidance } from "../types";
import {
  frameStatusCopy,
  isGuidanceReady,
  resolveFrameState,
} from "./frameState";

const readyGuidance: FaceGuidance = {
  faceDetected: true,
  centered: true,
  distance: "GOOD",
  pose: "FRONT",
  stable: true,
};

const base = {
  cameraLive: true,
  active: true,
  guidance: readyGuidance,
  requiredPose: "FRONT" as const,
  captureState: "IDLE" as const,
  submitting: false,
  outcome: null,
};

describe("resolveFrameState", () => {
  it("never returns MATCHED from guidance alone", () => {
    const state = resolveFrameState(base);
    expect(state).toBe("READY");
    expect(state).not.toBe("MATCHED");
  });

  it("returns MATCHED only when outcome is MATCHED", () => {
    expect(
      resolveFrameState({ ...base, outcome: "MATCHED", submitting: true }),
    ).toBe("MATCHED");
  });

  it("returns NO_MATCH / ERROR from outcome over guidance", () => {
    expect(resolveFrameState({ ...base, outcome: "NO_MATCH" })).toBe(
      "NO_MATCH",
    );
    expect(resolveFrameState({ ...base, outcome: "ERROR" })).toBe("ERROR");
  });

  it("maps no face to IDLE (gray)", () => {
    expect(
      resolveFrameState({
        ...base,
        guidance: { ...readyGuidance, faceDetected: false },
      }),
    ).toBe("IDLE");
  });

  it("maps detected-but-not-ready to GUIDE_ADJUST (yellow)", () => {
    expect(
      resolveFrameState({
        ...base,
        guidance: { ...readyGuidance, centered: false, stable: false },
      }),
    ).toBe("GUIDE_ADJUST");
  });

  it("maps ready guidance to READY (blue), not green", () => {
    expect(resolveFrameState(base)).toBe("READY");
  });

  it("maps submitting / capture to VERIFYING (blue pulse)", () => {
    expect(resolveFrameState({ ...base, submitting: true })).toBe("VERIFYING");
    expect(
      resolveFrameState({ ...base, captureState: "SUBMITTING" }),
    ).toBe("VERIFYING");
  });
});

describe("isGuidanceReady", () => {
  it("requires pose match and stability", () => {
    expect(isGuidanceReady(readyGuidance, "FRONT")).toBe(true);
    expect(isGuidanceReady({ ...readyGuidance, pose: "LEFT" }, "FRONT")).toBe(
      false,
    );
    expect(isGuidanceReady({ ...readyGuidance, stable: false }, "FRONT")).toBe(
      false,
    );
  });
});

describe("frameStatusCopy", () => {
  it("provides non-color labels for each state", () => {
    expect(frameStatusCopy("READY", "Hold still…").label).toBe("Ready");
    expect(frameStatusCopy("MATCHED", "").label).toBe("Verified");
    expect(frameStatusCopy("NO_MATCH", "").label).toBe("Not recognized");
  });
});
