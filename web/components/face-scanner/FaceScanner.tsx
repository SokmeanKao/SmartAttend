"use client";

import { Camera, CameraOff, RefreshCw } from "lucide-react";
import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";

import { Button } from "@/components/ui/button";

import { createJpegFromVideo } from "./camera/createJpeg";
import { useCameraStream } from "./camera/useCameraStream";
import {
  createCaptureCoordinator,
  type ScannerCaptureState,
} from "./captureCoordinator";
import { mapLandmarksToGuidance } from "./guidance/mapLandmarksToGuidance";
import {
  createFaceLandmarker,
  type LandmarkerHandle,
} from "./guidance/mediapipeLandmarker";
import {
  createGuidanceLoop,
  type GuidanceLoop,
} from "./guidance/runGuidanceLoop";
import { OvalOverlay } from "./overlay/OvalOverlay";
import { guidanceMessage } from "./overlay/guidanceCopy";
import {
  resolveFrameState,
  type ScannerFrameOutcome,
} from "./overlay/frameState";
import type {
  CaptureCandidate,
  CaptureResult,
  EnrollmentPose,
  FaceGuidance,
  GuidancePose,
  NormalizedLandmark,
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
  active?: boolean;
  autoStart?: boolean;
  /** Backend result — green/red only when set. Never from MediaPipe alone. */
  frameOutcome?: ScannerFrameOutcome | null;
  onCapture: (candidate: CaptureCandidate) => Promise<CaptureResult>;
  instructionSlot?: ReactNode;
  progressSlot?: ReactNode;
};

export function FaceScanner({
  requiredPose,
  active = true,
  autoStart: _autoStart = false,
  frameOutcome = null,
  onCapture,
  instructionSlot,
  progressSlot,
}: FaceScannerProps) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const coordinatorRef = useRef(createCaptureCoordinator({ cooldownMs: 400 }));
  const landmarkerRef = useRef<LandmarkerHandle | null>(null);
  const loopRef = useRef<GuidanceLoop | null>(null);
  const prevPoseRef = useRef<GuidancePose>("UNKNOWN");
  const stableSinceRef = useRef<number | null>(null);
  const requiredPoseRef = useRef(requiredPose);
  const activeRef = useRef(active);

  const [captureState, setCaptureState] =
    useState<ScannerCaptureState>("IDLE");
  const [busy, setBusy] = useState(false);
  const [localError, setLocalError] = useState("");
  const [guidance, setGuidance] = useState<FaceGuidance>(IDLE_GUIDANCE);
  const [guidanceDegraded, setGuidanceDegraded] = useState(false);

  requiredPoseRef.current = requiredPose;
  activeRef.current = active;

  const {
    cameraLive,
    cameras,
    selectedDeviceId,
    activeLabel,
    error: cameraError,
    requesting,
    waitSeconds,
    embedded,
    startCamera,
    stopCamera,
    switchCamera,
    formatCameraLabel,
  } = useCameraStream(videoRef);

  const refreshState = useCallback(() => {
    setCaptureState(coordinatorRef.current.getState());
  }, []);

  useEffect(() => {
    const coordinator = coordinatorRef.current;
    return () => {
      loopRef.current?.stop();
      landmarkerRef.current?.close();
      landmarkerRef.current = null;
      coordinator.dispose();
    };
  }, []);

  // Reset stability when required pose changes (FACE-GUIDE-04).
  useEffect(() => {
    stableSinceRef.current = null;
    prevPoseRef.current = "UNKNOWN";
    setGuidance((g) => ({ ...g, stable: false }));
  }, [requiredPose]);

  // autoStart is intentionally a no-op: getUserMedia must run from a click
  // (user gesture) or Chrome/Edge leave the permission promise pending forever.

  const runCapture = useCallback(
    async (source: "AUTO" | "MANUAL", guidanceSnapshot: FaceGuidance) => {
      if (!activeRef.current) return;
      if (!cameraLive || !videoRef.current) return;
      if (!coordinatorRef.current.beginCapture()) return;

      refreshState();
      setBusy(true);
      setLocalError("");
      stableSinceRef.current = null;

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
    [cameraLive, onCapture, refreshState],
  );

  const tryAutoCapture = useCallback(
    (g: FaceGuidance) => {
      if (!activeRef.current) return;
      if (coordinatorRef.current.getState() !== "IDLE") return;
      if (!g.stable || g.pose !== requiredPoseRef.current) return;
      void runCapture("AUTO", g);
    },
    [runCapture],
  );

  // Init / dispose MediaPipe with camera lifecycle.
  useEffect(() => {
    if (!cameraLive) {
      loopRef.current?.stop();
      loopRef.current = null;
      landmarkerRef.current?.close();
      landmarkerRef.current = null;
      setGuidance(IDLE_GUIDANCE);
      return;
    }

    let cancelled = false;

    void (async () => {
      try {
        const handle = await createFaceLandmarker();
        if (cancelled) {
          handle.close();
          return;
        }
        landmarkerRef.current = handle;
        setGuidanceDegraded(false);

        const loop = createGuidanceLoop({
          getVideo: () => videoRef.current,
          isActive: () => activeRef.current && !document.hidden,
          detect: (video, ts) => handle.detectForVideo(video, ts),
          onGuidance: (result, video) => {
            const faces =
              (
                result as {
                  faceLandmarks?: NormalizedLandmark[][];
                }
              ).faceLandmarks ?? [];
            const landmarks = faces[0];
            const mapped = mapLandmarksToGuidance({
              landmarks,
              frameW: video.videoWidth,
              frameH: video.videoHeight,
              requiredPose: requiredPoseRef.current,
              prevPose: prevPoseRef.current,
              nowMs: performance.now(),
              stableSince: stableSinceRef.current,
            });
            prevPoseRef.current = mapped.nextPrevPose;
            stableSinceRef.current = mapped.nextStableSince;
            setGuidance(mapped.guidance);
            tryAutoCapture(mapped.guidance);
          },
          onError: () => {
            setGuidanceDegraded(true);
          },
          targetIntervalMs: 80,
        });
        loopRef.current = loop;
        if (activeRef.current) loop.start();
      } catch {
        if (!cancelled) {
          setGuidanceDegraded(true);
          landmarkerRef.current = null;
        }
      }
    })();

    return () => {
      cancelled = true;
      loopRef.current?.stop();
      loopRef.current = null;
      landmarkerRef.current?.close();
      landmarkerRef.current = null;
    };
  }, [cameraLive, tryAutoCapture]);

  // Pause / resume on active + visibility.
  useEffect(() => {
    const loop = loopRef.current;
    if (!loop || !cameraLive) return;

    function sync() {
      if (!loopRef.current) return;
      if (!active || document.hidden) {
        loopRef.current.pause();
        stableSinceRef.current = null;
      } else {
        loopRef.current.resume();
      }
    }

    sync();
    document.addEventListener("visibilitychange", sync);
    return () => document.removeEventListener("visibilitychange", sync);
  }, [active, cameraLive]);

  const manualEnabled =
    active && cameraLive && captureState === "IDLE" && !busy && !requesting;

  const displayError = localError || cameraError;
  const hint =
    cameraLive && active
      ? guidanceMessage(guidance, requiredPose)
      : cameraLive
        ? "Camera paused"
        : "";
  const frameState = resolveFrameState({
    cameraLive,
    active,
    guidance,
    requiredPose,
    captureState,
    submitting: busy,
    outcome: frameOutcome,
  });

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
        {cameraLive && (
          <OvalOverlay frameState={frameState} detailHint={hint} />
        )}
        {!cameraLive && (
          <div className="absolute inset-0 flex flex-col items-center justify-center gap-3 text-zinc-300">
            <CameraOff className="size-10" />
            <p className="max-w-xs text-center text-sm">
              Click{" "}
              <span className="font-medium text-zinc-100">Start camera</span>{" "}
              below and allow access when the browser asks.
            </p>
          </div>
        )}
        {instructionSlot}
        {progressSlot}
      </div>

      {embedded && (
        <p role="status" className="mt-3 rounded-md bg-amber-50 px-3 py-2 text-sm text-amber-900">
          This looks like an embedded preview. Camera access often hangs here —
          open{" "}
          <a className="underline" href="http://localhost:3000" target="_blank" rel="noreferrer">
            http://localhost:3000
          </a>{" "}
          in Chrome or Edge instead.
        </p>
      )}

      {guidanceDegraded && cameraLive && (
        <p className="mt-2 text-xs text-amber-700">
          Face guidance is unavailable. Use Capture manually — server checks
          still apply.
        </p>
      )}

      {cameraLive && activeLabel && (
        <p className="mt-2 text-xs text-muted-foreground">
          Using: {activeLabel}
        </p>
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
                {formatCameraLabel(camera)}
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
          <>
            <Button
              type="button"
              disabled={requesting}
              onClick={() => void startCamera(selectedDeviceId || undefined)}
            >
              {requesting ? <RefreshCw className="animate-spin" /> : <Camera />}
              {requesting
                ? `Waiting for camera… ${waitSeconds}s`
                : "Start camera"}
            </Button>
            {requesting && (
              <Button type="button" variant="outline" onClick={stopCamera}>
                Cancel
              </Button>
            )}
          </>
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
