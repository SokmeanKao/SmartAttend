export type GuidancePose = "LEFT" | "FRONT" | "RIGHT" | "UNKNOWN";
export type EnrollmentPose = "LEFT" | "FRONT" | "RIGHT";

export type FaceGuidance = {
  faceDetected: boolean;
  centered: boolean;
  distance: "TOO_CLOSE" | "GOOD" | "TOO_FAR";
  pose: GuidancePose;
  stable: boolean;
};

export type CaptureCandidate = {
  blob: Blob;
  source: "AUTO" | "MANUAL";
  guidance: FaceGuidance;
};

export type CaptureResult =
  | { status: "ACCEPTED" }
  | { status: "REJECTED" };

export type NormalizedLandmark = {
  x: number;
  y: number;
  z?: number;
};

export type Oval = {
  cx: number;
  cy: number;
  rx: number;
  ry: number;
};

export type FaceMetrics = {
  centerX: number;
  centerY: number;
  faceHeight: number;
};

export type YawOptions = {
  frontMax: number;
  sideMin: number;
  epsilon: number;
};

export const DEFAULT_YAW_OPTIONS: YawOptions = {
  frontMax: 0.15,
  sideMin: 0.25,
  epsilon: 0.05,
};

export const DEFAULT_STABILITY_MS = 400;
