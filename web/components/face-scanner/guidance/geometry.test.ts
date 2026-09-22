import { describe, expect, it } from "vitest";

import { classifyDistance, isCentered, ovalForFrame } from "./geometry";

describe("geometry", () => {
  it("classifies distance bands", () => {
    expect(classifyDistance(0.4 * 100, 100)).toBe("TOO_FAR");
    expect(classifyDistance(0.7 * 100, 100)).toBe("GOOD");
    expect(classifyDistance(1.1 * 100, 100)).toBe("TOO_CLOSE");
  });

  it("centered uses inner oval", () => {
    const oval = ovalForFrame(640, 480);
    expect(isCentered({ centerX: oval.cx, centerY: oval.cy }, oval)).toBe(true);
    expect(isCentered({ centerX: 0, centerY: 0 }, oval)).toBe(false);
  });

  it("oval scales with frame", () => {
    const oval = ovalForFrame(640, 480);
    expect(oval.cx).toBe(320);
    expect(oval.cy).toBe(240);
    expect(oval.rx).toBeCloseTo(0.55 * 480 * 0.5);
    expect(oval.ry).toBeCloseTo(oval.rx * 1.25);
  });
});
