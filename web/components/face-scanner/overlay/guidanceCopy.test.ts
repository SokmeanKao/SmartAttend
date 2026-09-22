import { describe, expect, it } from "vitest";

import type { FaceGuidance } from "../types";
import { guidanceMessage } from "./guidanceCopy";

const base: FaceGuidance = {
  faceDetected: true,
  centered: true,
  distance: "GOOD",
  pose: "FRONT",
  stable: true,
};

describe("guidanceMessage", () => {
  it("prioritizes no face first", () => {
    expect(
      guidanceMessage({ ...base, faceDetected: false }, "FRONT"),
    ).toBe("Position your face inside the frame");
  });

  it("asks for pose before stability", () => {
    expect(
      guidanceMessage({ ...base, pose: "LEFT", stable: false }, "RIGHT"),
    ).toBe("Turn slightly right");
  });

  it("asks to hold still when ready but not stable", () => {
    expect(guidanceMessage({ ...base, stable: false }, "FRONT")).toBe(
      "Hold still",
    );
  });
});
