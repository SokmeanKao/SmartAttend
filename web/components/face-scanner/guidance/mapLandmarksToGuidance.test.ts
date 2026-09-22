import { describe, expect, it } from "vitest";

import type { NormalizedLandmark } from "../types";
import { LM } from "./geometry";
import { mapLandmarksToGuidance } from "./mapLandmarksToGuidance";

function centeredLandmarks(): NormalizedLandmark[] {
  const points: NormalizedLandmark[] = Array.from({ length: 478 }, () => ({
    x: 0.5,
    y: 0.5,
  }));
  points[LM.LEFT_EYE_OUTER] = { x: 0.42, y: 0.42 };
  points[LM.RIGHT_EYE_OUTER] = { x: 0.58, y: 0.42 };
  points[LM.NOSE_TIP] = { x: 0.5, y: 0.5 };
  points[LM.FOREHEAD] = { x: 0.5, y: 0.28 };
  points[LM.CHIN] = { x: 0.5, y: 0.72 };
  return points;
}

describe("mapLandmarksToGuidance", () => {
  it("marks no face when landmarks missing", () => {
    const mapped = mapLandmarksToGuidance({
      landmarks: null,
      frameW: 640,
      frameH: 480,
      requiredPose: "FRONT",
      prevPose: "UNKNOWN",
      nowMs: 0,
      stableSince: null,
    });
    expect(mapped.guidance.faceDetected).toBe(false);
    expect(mapped.guidance.stable).toBe(false);
  });

  it("becomes stable after window when FRONT-ready", () => {
    const landmarks = centeredLandmarks();
    let mapped = mapLandmarksToGuidance({
      landmarks,
      frameW: 640,
      frameH: 480,
      requiredPose: "FRONT",
      prevPose: "UNKNOWN",
      nowMs: 0,
      stableSince: null,
    });
    expect(mapped.guidance.faceDetected).toBe(true);
    expect(mapped.guidance.pose).toBe("FRONT");
    expect(mapped.guidance.stable).toBe(false);

    mapped = mapLandmarksToGuidance({
      landmarks,
      frameW: 640,
      frameH: 480,
      requiredPose: "FRONT",
      prevPose: mapped.nextPrevPose,
      nowMs: 400,
      stableSince: mapped.nextStableSince,
    });
    expect(mapped.guidance.stable).toBe(true);
  });
});
