import type { EnrollmentPose, FaceGuidance } from "../types";
import type { ScannerCaptureState } from "../captureCoordinator";

/**
 * Oval / frame chrome for FaceScanner.
 * Guidance colors are advisory only. Green/red are reserved for backend results.
 */
export type ScannerFrameState =
  | "IDLE"
  | "GUIDE_ADJUST"
  | "READY"
  | "VERIFYING"
  | "MATCHED"
  | "NO_MATCH"
  | "ERROR";

/** Backend / verify outcome — never derived from MediaPipe alone. */
export type ScannerFrameOutcome = "MATCHED" | "NO_MATCH" | "ERROR";

export type ResolveFrameStateInput = {
  cameraLive: boolean;
  active: boolean;
  guidance: FaceGuidance;
  requiredPose: EnrollmentPose;
  captureState: ScannerCaptureState;
  /** true while onCapture promise is in flight */
  submitting: boolean;
  /** Server/result override — wins over live guidance */
  outcome?: ScannerFrameOutcome | null;
};

export function isGuidanceReady(
  guidance: FaceGuidance,
  requiredPose: EnrollmentPose,
): boolean {
  return (
    guidance.faceDetected &&
    guidance.centered &&
    guidance.distance === "GOOD" &&
    guidance.pose === requiredPose &&
    guidance.stable
  );
}

/**
 * Resolve oval border state.
 * Hard rule: MATCHED (green) only when outcome === "MATCHED" from the API.
 */
export function resolveFrameState(
  input: ResolveFrameStateInput,
): ScannerFrameState {
  const { outcome } = input;
  if (outcome === "MATCHED") return "MATCHED";
  if (outcome === "NO_MATCH") return "NO_MATCH";
  if (outcome === "ERROR") return "ERROR";

  if (!input.cameraLive) return "IDLE";

  if (
    input.submitting ||
    input.captureState === "CAPTURING" ||
    input.captureState === "SUBMITTING"
  ) {
    return "VERIFYING";
  }

  if (!input.active) return "IDLE";

  if (!input.guidance.faceDetected) return "IDLE";

  if (isGuidanceReady(input.guidance, input.requiredPose)) return "READY";

  return "GUIDE_ADJUST";
}

export const frameBorderClass: Record<ScannerFrameState, string> = {
  IDLE: "border-zinc-400",
  GUIDE_ADJUST: "border-amber-400",
  READY: "border-blue-500",
  VERIFYING: "border-blue-500 animate-pulse",
  MATCHED: "border-emerald-500",
  NO_MATCH: "border-red-500",
  ERROR: "border-red-500",
};

export const frameScrimClass: Record<ScannerFrameState, string> = {
  IDLE: "shadow-[0_0_0_9999px_rgba(0,0,0,0.55)]",
  GUIDE_ADJUST: "shadow-[0_0_0_9999px_rgba(0,0,0,0.50)]",
  READY: "shadow-[0_0_0_9999px_rgba(0,0,0,0.42)]",
  VERIFYING: "shadow-[0_0_0_9999px_rgba(0,0,0,0.42)]",
  MATCHED: "shadow-[0_0_0_9999px_rgba(16,185,129,0.22)]",
  NO_MATCH: "shadow-[0_0_0_9999px_rgba(239,68,68,0.22)]",
  ERROR: "shadow-[0_0_0_9999px_rgba(239,68,68,0.22)]",
};

export type FrameStatusCopy = {
  label: string;
  detail: string;
};

/** Accessible status — never color-only. */
export function frameStatusCopy(
  state: ScannerFrameState,
  guidanceMessage: string,
): FrameStatusCopy {
  switch (state) {
    case "IDLE":
      return {
        label: "Waiting",
        detail: guidanceMessage || "Position your face in the frame",
      };
    case "GUIDE_ADJUST":
      return {
        label: "Adjust",
        detail: guidanceMessage || "Adjust your position",
      };
    case "READY":
      return {
        label: "Ready",
        detail: guidanceMessage || "Hold still",
      };
    case "VERIFYING":
      return {
        label: "Verifying",
        detail: "Checking with the server…",
      };
    case "MATCHED":
      return {
        label: "Verified",
        detail: "Face matched successfully",
      };
    case "NO_MATCH":
      return {
        label: "Not recognized",
        detail: "Face did not match this employee code",
      };
    case "ERROR":
      return {
        label: "Failed",
        detail: guidanceMessage || "Verification could not complete",
      };
  }
}
