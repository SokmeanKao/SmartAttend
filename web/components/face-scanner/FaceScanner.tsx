"use client";

import { Camera, CameraOff, RefreshCw } from "lucide-react";
import { useCallback, useEffect, useRef, useState, type ReactNode } from "react";

import { Button } from "@/components/ui/button";

import { createJpegFromVideo } from "./camera/createJpeg";
import { useCameraStream } from "./camera/useCameraStream";
import {
  createCaptureCoordinator,
  type ScannerCaptureState,
} from "./captureCoordinator";
import type {
  CaptureCandidate,
  CaptureResult,
  EnrollmentPose,
  FaceGuidance,
} from "./types";

const IDLE_GUIDANCE: FaceGuidance = {
  faceDetected: false,
  centered: false,
  distance: "TOO_FAR",
  pose: "UNKNOWN",
  stable: false,
};

export type FaceScannerProps = {
  requiredPose: EnrollmentPose;
  /** When false, pause auto-capture attempts (verify settle). Default true. */
  active?: boolean;
  autoStart?: boolean;
  onCapture: (candidate: CaptureCandidate) => Promise<CaptureResult>;
  instructionSlot?: ReactNode;
  progressSlot?: ReactNode;
  /** Latest advisory guidance (Task 4 wires MediaPipe). */
  guidance?: FaceGuidance;
  /** Optional hook for Task 4 auto path. */
  onTryAutoCapture?: (fn: (g: FaceGuidance) => void) => void;
};

export function FaceScanner({
  requiredPose,
  active = true,
  autoStart = false,
  onCapture,
  instructionSlot,
  progressSlot,
  guidance = IDLE_GUIDANCE,
  onTryAutoCapture,
}: FaceScannerProps) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const coordinatorRef = useRef(createCaptureCoordinator({ cooldownMs: 400 }));
  const [captureState, setCaptureState] =
    useState<ScannerCaptureState>("IDLE");
  const [busy, setBusy] = useState(false);
  const [localError, setLocalError] = useState("");
  const autoStartAttemptedRef = useRef(false);

  const {
    cameraLive,
    cameras,
    selectedDeviceId,
    activeLabel,
    error: cameraError,
    requesting,
    startCamera,
    stopCamera,
    switchCamera,
  } = useCameraStream(videoRef);

  const refreshState = useCallback(() => {
    setCaptureState(coordinatorRef.current.getState());
  }, []);

  useEffect(() => {
    const coordinator = coordinatorRef.current;
    return () => coordinator.dispose();
  }, []);

  useEffect(() => {
    if (!autoStart || autoStartAttemptedRef.current) return;
    autoStartAttemptedRef.current = true;
    void startCamera();
  }, [autoStart, startCamera]);

  const runCapture = useCallback(
    async (source: "AUTO" | "MANUAL", guidanceSnapshot: FaceGuidance) => {
      if (!active) return;
      if (!cameraLive || !videoRef.current) return;
      if (!coordinatorRef.current.beginCapture()) return;

      refreshState();
      setBusy(true);
      setLocalError("");

      try {
        const blob = await createJpegFromVideo(videoRef.current);
        coordinatorRef.current.markSubmitting();
        refreshState();

        const candidate: CaptureCandidate = {
          blob,
          source,
          guidance: guidanceSnapshot,
        };
        const result = await onCapture(candidate);
        coordinatorRef.current.settle(result);
        refreshState();

        // Cooldown → IDLE is async; poll briefly for UI.
        window.setTimeout(refreshState, 450);
      } catch (err) {
        coordinatorRef.current.settle({ status: "REJECTED" });
        refreshState();
        setLocalError(
          err instanceof Error ? err.message : "Unable to capture the photo.",
        );
        window.setTimeout(refreshState, 450);
      } finally {
        setBusy(false);
      }
    },
    [active, cameraLive, onCapture, refreshState],
  );

  const tryAutoCapture = useCallback(
    (g: FaceGuidance) => {
      if (!active) return;
      if (coordinatorRef.current.getState() !== "IDLE") return;
      if (!g.stable || g.pose !== requiredPose) return;
      void runCapture("AUTO", g);
    },
    [active, requiredPose, runCapture],
  );

  useEffect(() => {
    onTryAutoCapture?.(tryAutoCapture);
  }, [onTryAutoCapture, tryAutoCapture]);

  // Task 4 will drive MediaPipe; until then auto only if parent pushes stable guidance.
  useEffect(() => {
    if (!active || !cameraLive) return;
    tryAutoCapture(guidance);
  }, [active, cameraLive, guidance, tryAutoCapture]);

  const manualEnabled =
    active && cameraLive && captureState === "IDLE" && !busy && !requesting;

  const displayError = localError || cameraError;

  return (
    <div>
      <div className="relative aspect-4/3 overflow-hidden rounded-xl bg-zinc-950">
        <video
          ref={videoRef}
          autoPlay
          muted
          playsInline
          className={`size-full object-cover scale-x-[-1] transition-opacity ${
            cameraLive ? "opacity-100" : "pointer-events-none opacity-0"
          }`}
        />
        {!cameraLive && (
          <div className="absolute inset-0 flex flex-col items-center justify-center gap-3 text-zinc-300">
            <CameraOff className="size-10" />
            <p className="max-w-xs text-center text-sm">
              Start the camera when you are ready.
            </p>
          </div>
        )}
        {instructionSlot}
        {progressSlot}
      </div>

      {cameraLive && activeLabel && (
        <p className="mt-2 text-xs text-muted-foreground">Using: {activeLabel}</p>
      )}

      {cameras.length > 1 && (
        <label className="mt-3 flex flex-col gap-1.5 text-sm">
          <span className="text-muted-foreground">Camera</span>
          <select
            className="h-9 rounded-md border border-input bg-background px-3 text-sm"
            value={selectedDeviceId}
            disabled={busy || requesting}
            onChange={(event) => void switchCamera(event.target.value)}
          >
            {cameras.map((camera) => (
              <option key={camera.deviceId} value={camera.deviceId}>
                {camera.label}
              </option>
            ))}
          </select>
        </label>
      )}

      {displayError && (
        <p role="alert" className="mt-3 text-sm text-destructive">
          {displayError}
        </p>
      )}

      <div className="mt-4 flex flex-wrap gap-2">
        {!cameraLive ? (
          <Button
            type="button"
            disabled={requesting}
            onClick={() => void startCamera(selectedDeviceId || undefined)}
          >
            {requesting ? <RefreshCw className="animate-spin" /> : <Camera />}
            {requesting ? "Requesting permission…" : "Start camera"}
          </Button>
        ) : (
          <>
            <Button
              type="button"
              disabled={!manualEnabled}
              onClick={() => void runCapture("MANUAL", guidance)}
            >
              {busy ? <RefreshCw className="animate-spin" /> : <Camera />}
              Capture manually
            </Button>
            <Button
              type="button"
              variant="outline"
              disabled={busy || requesting}
              onClick={stopCamera}
            >
              Stop camera
            </Button>
          </>
        )}
      </div>
    </div>
  );
}
