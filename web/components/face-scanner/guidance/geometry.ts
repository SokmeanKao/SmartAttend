import type { FaceMetrics, NormalizedLandmark, Oval } from "../types";

/** MediaPipe Face Landmarker indices used for bbox/pose proxies. */
export const LM = {
  NOSE_TIP: 1,
  LEFT_EYE_OUTER: 33,
  RIGHT_EYE_OUTER: 263,
  CHIN: 152,
  FOREHEAD: 10,
} as const;

/**
 * Oval in unmirrored source-frame pixels (spec §4.4).
 * width ≈ 0.55 × min(W,H); height ≈ width × 1.25
 */
export function ovalForFrame(w: number, h: number): Oval {
  const rx = 0.55 * Math.min(w, h) * 0.5;
  const ry = rx * 1.25;
  return { cx: w / 2, cy: h / 2, rx, ry };
}

/**
 * Face metrics from normalized landmarks (0–1) → source pixels.
 * Landmarks must be in unmirrored sensor space.
 */
export function faceMetricsFromLandmarks(
  landmarks: NormalizedLandmark[],
  frameW: number,
  frameH: number,
): FaceMetrics | null {
  const leftEye = landmarks[LM.LEFT_EYE_OUTER];
  const rightEye = landmarks[LM.RIGHT_EYE_OUTER];
  const chin = landmarks[LM.CHIN];
  const forehead = landmarks[LM.FOREHEAD];
  if (!leftEye || !rightEye || !chin || !forehead) return null;

  const centerX = ((leftEye.x + rightEye.x) / 2) * frameW;
  const centerY = ((forehead.y + chin.y) / 2) * frameH;
  const faceHeight = Math.abs(chin.y - forehead.y) * frameH;
  if (faceHeight <= 0) return null;

  return { centerX, centerY, faceHeight };
}

/** Face center inside inner oval (~85% of radii). */
export function isCentered(
  face: { centerX: number; centerY: number },
  oval: Oval,
  innerScale = 0.85,
): boolean {
  const dx = (face.centerX - oval.cx) / (oval.rx * innerScale);
  const dy = (face.centerY - oval.cy) / (oval.ry * innerScale);
  return dx * dx + dy * dy <= 1;
}

/**
 * Distance from faceHeight / ovalHeight (2*ry).
 * < 0.55 TOO_FAR; > 0.95 TOO_CLOSE; else GOOD
 */
export function classifyDistance(
  faceHeight: number,
  ovalHeight: number,
): "TOO_CLOSE" | "GOOD" | "TOO_FAR" {
  if (ovalHeight <= 0) return "TOO_FAR";
  const ratio = faceHeight / ovalHeight;
  if (ratio < 0.55) return "TOO_FAR";
  if (ratio > 0.95) return "TOO_CLOSE";
  return "GOOD";
}
