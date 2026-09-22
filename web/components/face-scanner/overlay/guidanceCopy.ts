import type { EnrollmentPose, FaceGuidance } from "../types";

const POSE_HINT: Record<EnrollmentPose, string> = {
  FRONT: "Look forward",
  LEFT: "Turn slightly left",
  RIGHT: "Turn slightly right",
};

/** First failing readiness condition wins. */
export function guidanceMessage(
  g: FaceGuidance,
  requiredPose: EnrollmentPose,
): string {
  if (!g.faceDetected) return "Position your face inside the frame";
  if (!g.centered) return "Center your face in the oval";
  if (g.distance === "TOO_FAR") return "Move closer";
  if (g.distance === "TOO_CLOSE") return "Move back";
  if (g.pose !== requiredPose) return POSE_HINT[requiredPose];
  if (!g.stable) return "Hold still";
  return "Hold still…";
}
