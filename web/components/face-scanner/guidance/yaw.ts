import {
  DEFAULT_YAW_OPTIONS,
  type GuidancePose,
  type NormalizedLandmark,
  type YawOptions,
} from "../types";
import { LM } from "./geometry";

/**
 * Yaw proxy in unmirrored sensor space.
 * Nose tip horizontal offset from eye midline, normalized by inter-ocular distance.
 * Locked convention: negative = user's LEFT, positive = user's RIGHT.
 */
export function yawProxyFromLandmarks(
  landmarks: NormalizedLandmark[],
): number | null {
  const nose = landmarks[LM.NOSE_TIP];
  const leftEye = landmarks[LM.LEFT_EYE_OUTER];
  const rightEye = landmarks[LM.RIGHT_EYE_OUTER];
  if (!nose || !leftEye || !rightEye) return null;

  const midX = (leftEye.x + rightEye.x) / 2;
  const iod = Math.abs(rightEye.x - leftEye.x);
  if (iod < 1e-6) return null;

  // In MediaPipe image coords, +x is toward the subject's left (viewer's right)
  // when facing the camera. Nose left of midline → subject turned to their RIGHT
  // from our locked convention perspective:
  //   User turns head LEFT → nose moves toward camera-right (+x) relative to eyes
  //   → (nose.x - midX) / iod is positive for LEFT? Let's lock tests to:
  // Spec: negative yaw = user's LEFT.
  // Empirically for facing camera: when user turns left, their right cheek shows
  // more; nose tip shifts in image toward +x (right side of frame from viewer).
  // So: yaw = -(nose.x - midX) / iod  → turn left yields negative.
  return -((nose.x - midX) / iod);
}

export function classifyYaw(
  yaw: number,
  prev: GuidancePose,
  opts: YawOptions = DEFAULT_YAW_OPTIONS,
): GuidancePose {
  const { frontMax, sideMin, epsilon } = opts;

  if (Math.abs(yaw) <= frontMax) {
    if (prev === "LEFT" || prev === "RIGHT") {
      // Leave side band only after crossing back past sideMin - epsilon via front
      // Already inside frontMax → FRONT
    }
    return "FRONT";
  }

  if (prev === "LEFT") {
    if (yaw <= -(sideMin - epsilon)) return "LEFT";
  } else if (prev === "RIGHT") {
    if (yaw >= sideMin - epsilon) return "RIGHT";
  }

  if (yaw <= -sideMin) return "LEFT";
  if (yaw >= sideMin) return "RIGHT";
  return "UNKNOWN";
}
