import {
  classifyDistance,
  faceMetricsFromLandmarks,
  isCentered,
  ovalForFrame,
} from "./geometry";
import { updateStability } from "./stability";
import { classifyYaw, yawProxyFromLandmarks } from "./yaw";
import type {
  EnrollmentPose,
  FaceGuidance,
  GuidancePose,
  NormalizedLandmark,
} from "../types";

export function mapLandmarksToGuidance(input: {
  landmarks: NormalizedLandmark[] | null | undefined;
  frameW: number;
  frameH: number;
  requiredPose: EnrollmentPose;
  prevPose: GuidancePose;
  nowMs: number;
  stableSince: number | null;
}): {
  guidance: FaceGuidance;
  nextStableSince: number | null;
  nextPrevPose: GuidancePose;
} {
  const { landmarks, frameW, frameH, requiredPose, prevPose, nowMs, stableSince } =
    input;

  if (!landmarks || landmarks.length === 0 || frameW <= 0 || frameH <= 0) {
    return {
      guidance: {
        faceDetected: false,
        centered: false,
        distance: "TOO_FAR",
        pose: "UNKNOWN",
        stable: false,
      },
      nextStableSince: null,
      nextPrevPose: "UNKNOWN",
    };
  }

  const metrics = faceMetricsFromLandmarks(landmarks, frameW, frameH);
  if (!metrics) {
    return {
      guidance: {
        faceDetected: false,
        centered: false,
        distance: "TOO_FAR",
        pose: "UNKNOWN",
        stable: false,
      },
      nextStableSince: null,
      nextPrevPose: "UNKNOWN",
    };
  }

  const oval = ovalForFrame(frameW, frameH);
  const centered = isCentered(metrics, oval);
  const distance = classifyDistance(metrics.faceHeight, oval.ry * 2);
  const yaw = yawProxyFromLandmarks(landmarks);
  const pose =
    yaw === null ? ("UNKNOWN" as GuidancePose) : classifyYaw(yaw, prevPose);

  const ready =
    centered && distance === "GOOD" && pose === requiredPose;

  const stability = updateStability({
    ready,
    nowMs,
    stableSince,
  });

  return {
    guidance: {
      faceDetected: true,
      centered,
      distance,
      pose,
      stable: stability.stable,
    },
    nextStableSince: stability.stableSince,
    nextPrevPose: pose,
  };
}
