import { describe, expect, it } from "vitest";

import type { NormalizedLandmark } from "../types";
import { LM } from "./geometry";
import { classifyYaw, yawProxyFromLandmarks } from "./yaw";

const opts = { frontMax: 0.15, sideMin: 0.25, epsilon: 0.05 };

function landmarksWithNoseOffset(offsetNorm: number): NormalizedLandmark[] {
  const points: NormalizedLandmark[] = Array.from({ length: 478 }, () => ({
    x: 0.5,
    y: 0.5,
  }));
  points[LM.LEFT_EYE_OUTER] = { x: 0.4, y: 0.45 };
  points[LM.RIGHT_EYE_OUTER] = { x: 0.6, y: 0.45 };
  const mid = 0.5;
  // offsetNorm is desired yawProxy (neg = LEFT). Invert the proxy formula.
  // yaw = -(nose.x - mid) / iod  => nose.x = mid - yaw * iod
  const iod = 0.2;
  points[LM.NOSE_TIP] = { x: mid - offsetNorm * iod, y: 0.5 };
  return points;
}

describe("yaw", () => {
  it("negative yaw is LEFT", () => {
    expect(classifyYaw(-0.4, "UNKNOWN", opts)).toBe("LEFT");
    expect(classifyYaw(0.4, "UNKNOWN", opts)).toBe("RIGHT");
    expect(classifyYaw(0.05, "UNKNOWN", opts)).toBe("FRONT");
  });

  it("hysteresis avoids chatter near threshold", () => {
    const left = classifyYaw(-0.3, "LEFT", opts);
    expect(left).toBe("LEFT");
    // Still slightly past the softened threshold while labeled LEFT
    expect(classifyYaw(-0.22, "LEFT", opts)).toBe("LEFT");
  });

  it("yawProxyFromLandmarks uses locked LEFT-negative convention", () => {
    const leftish = yawProxyFromLandmarks(landmarksWithNoseOffset(-0.4));
    expect(leftish).not.toBeNull();
    expect(leftish!).toBeCloseTo(-0.4, 5);
    expect(classifyYaw(leftish!, "UNKNOWN", opts)).toBe("LEFT");

    const rightish = yawProxyFromLandmarks(landmarksWithNoseOffset(0.4));
    expect(classifyYaw(rightish!, "UNKNOWN", opts)).toBe("RIGHT");
  });
});
