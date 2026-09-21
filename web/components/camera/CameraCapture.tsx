"use client";

import { Camera, CameraOff, RefreshCw } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";

import { Button } from "@/components/ui/button";

export type CameraState =
  | "idle"
  | "requesting_permission"
  | "live"
  | "capturing"
  | "submitting"
  | "error";

const MAX_IMAGE_BYTES = 5 * 1024 * 1024;
const MAX_IMAGE_DIMENSION = 1280;

type CameraCaptureProps = {
  disabled?: boolean;
  captureLabel?: string;
  instruction?: string;
  onCapture: (image: Blob) => Promise<void>;
};

function canvasToBlob(
  canvas: HTMLCanvasElement,
  quality: number,
): Promise<Blob> {
  return new Promise((resolve, reject) => {
    canvas.toBlob(
      (blob) =>
        blob
          ? resolve(blob)
          : reject(new Error("Unable to create an image from the camera.")),
      "image/jpeg",
      quality,
    );
  });
}

async function createJpeg(video: HTMLVideoElement): Promise<Blob> {
  if (!video.videoWidth || !video.videoHeight) {
    throw new Error("The camera is not ready yet. Please try again.");
  }

  const scale = Math.min(
    1,
    MAX_IMAGE_DIMENSION / Math.max(video.videoWidth, video.videoHeight),
  );
  const canvas = document.createElement("canvas");
  canvas.width = Math.round(video.videoWidth * scale);
  canvas.height = Math.round(video.videoHeight * scale);

  const context = canvas.getContext("2d");
  if (!context) {
    throw new Error("Camera capture is not supported by this browser.");
  }
  context.drawImage(video, 0, 0, canvas.width, canvas.height);

  for (const quality of [0.9, 0.8, 0.7, 0.6, 0.5]) {
    const blob = await canvasToBlob(canvas, quality);
    if (blob.size <= MAX_IMAGE_BYTES) return blob;
  }
  throw new Error("The captured image is larger than 5 MiB. Please retry.");
}

function isLikelyInfraredLabel(label: string): boolean {
  const value = label.toLowerCase();
  return (
    value.includes("ir camera") ||
    value.includes("infrared") ||
    value.includes(" windows hello") ||
    value.includes("rgbcamerair") ||
    /\bir\b/.test(value)
  );
}

async function pickPreferredVideoDeviceId(): Promise<string | undefined> {
  if (!navigator.mediaDevices?.enumerateDevices) return undefined;
  const devices = await navigator.mediaDevices.enumerateDevices();
  const videoInputs = devices.filter((device) => device.kind === "videoinput");
  if (videoInputs.length === 0) return undefined;

  const labeled = videoInputs.filter((device) => device.label.trim() !== "");
  if (labeled.length === 0) return undefined;

  const visible = labeled.find((device) => !isLikelyInfraredLabel(device.label));
  return (visible ?? labeled[0])?.deviceId;
}

async function openCameraStream(): Promise<MediaStream> {
  const preferredDeviceId = await pickPreferredVideoDeviceId();

  if (preferredDeviceId) {
    try {
      return await navigator.mediaDevices.getUserMedia({
        audio: false,
        video: {
          deviceId: { exact: preferredDeviceId },
          width: { ideal: 1280 },
          height: { ideal: 720 },
        },
      });
    } catch {
      // Fall through to a generic facingMode request.
    }
  }

  return navigator.mediaDevices.getUserMedia({
    audio: false,
    video: {
      facingMode: "user",
      width: { ideal: 1280 },
      height: { ideal: 720 },
    },
  });
}

export function CameraCapture({
  disabled = false,
  captureLabel = "Capture photo",
  instruction,
  onCapture,
}: CameraCaptureProps) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const streamRef = useRef<MediaStream | null>(null);
  const mountedRef = useRef(true);
  const [state, setState] = useState<CameraState>("idle");
  const [cameraActive, setCameraActive] = useState(false);
  const [error, setError] = useState("");

  const stopTracks = useCallback(() => {
    streamRef.current?.getTracks().forEach((track) => track.stop());
    streamRef.current = null;
  }, []);

  const stopCamera = useCallback(() => {
    stopTracks();
    if (videoRef.current) {
      videoRef.current.srcObject = null;
    }
    setCameraActive(false);
    setState("idle");
  }, [stopTracks]);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      streamRef.current?.getTracks().forEach((track) => track.stop());
      streamRef.current = null;
    };
  }, []);

  // Attach/play only after the <video> is visible. Starting playback while
  // display:none often yields a black frame on Chromium/Windows.
  useEffect(() => {
    const video = videoRef.current;
    const stream = streamRef.current;
    if (!cameraActive || !video || !stream) return;

    video.srcObject = stream;
    video.muted = true;
    const playPromise = video.play();
    if (playPromise !== undefined) {
      void playPromise.catch(() => {
        // Autoplay may reject briefly; a later user gesture / retry handles it.
      });
    }
  }, [cameraActive]);

  async function startCamera() {
    setState("requesting_permission");
    setError("");
    try {
      if (!navigator.mediaDevices?.getUserMedia) {
        throw new Error("Camera access is not supported by this browser.");
      }

      // First call may only grant permission (labels empty). Then reopen with
      // a preferred non-IR device when labels become available.
      let stream = await openCameraStream();
      const labeledAfterGrant = (await navigator.mediaDevices.enumerateDevices())
        .filter((device) => device.kind === "videoinput")
        .some((device) => device.label.trim() !== "");
      if (labeledAfterGrant) {
        const preferred = await pickPreferredVideoDeviceId();
        const currentId = stream.getVideoTracks()[0]?.getSettings().deviceId;
        if (preferred && preferred !== currentId) {
          stream.getTracks().forEach((track) => track.stop());
          stream = await openCameraStream();
        }
      }

      if (!mountedRef.current) {
        stream.getTracks().forEach((track) => track.stop());
        return;
      }

      streamRef.current = stream;
      setCameraActive(true);
      setState("live");
    } catch (cameraError) {
      stopTracks();
      setCameraActive(false);
      setError(
        cameraError instanceof Error
          ? cameraError.message
          : "Unable to access the camera.",
      );
      setState("error");
    }
  }

  async function capture() {
    if (
      !videoRef.current ||
      (state !== "live" && !(state === "error" && cameraActive))
    ) {
      return;
    }
    setState("capturing");
    setError("");
    try {
      // Ensure we have frames before drawing.
      if (!videoRef.current.videoWidth) {
        await videoRef.current.play();
        await new Promise((resolve) => setTimeout(resolve, 150));
      }
      const image = await createJpeg(videoRef.current);
      setState("submitting");
      await onCapture(image);
      setState("live");
    } catch (captureError) {
      setError(
        captureError instanceof Error
          ? captureError.message
          : "Unable to capture the photo.",
      );
      setState("error");
    }
  }

  const busy =
    state === "requesting_permission" ||
    state === "capturing" ||
    state === "submitting";

  return (
    <div>
      <div className="relative aspect-4/3 overflow-hidden rounded-xl bg-zinc-950">
        <video
          ref={videoRef}
          autoPlay
          muted
          playsInline
          // Keep in layout (opacity) instead of display:none so frames decode.
          className={`size-full object-cover scale-x-[-1] transition-opacity ${
            cameraActive ? "opacity-100" : "pointer-events-none opacity-0"
          }`}
        />
        {!cameraActive && (
          <div className="absolute inset-0 flex flex-col items-center justify-center gap-3 text-zinc-300">
            <CameraOff className="size-10" />
            <p className="max-w-xs text-center text-sm">
              Start the camera when you are ready.
            </p>
          </div>
        )}
        {instruction && cameraActive && (
          <div className="absolute inset-x-4 bottom-4 rounded-lg bg-black/65 px-4 py-3 text-center text-sm font-medium text-white">
            {instruction}
          </div>
        )}
      </div>

      {error && (
        <p role="alert" className="mt-3 text-sm text-destructive">
          {error}
        </p>
      )}

      <div className="mt-4 flex flex-wrap gap-2">
        {!cameraActive ? (
          <Button
            type="button"
            disabled={disabled || state === "requesting_permission"}
            onClick={() => void startCamera()}
          >
            {state === "requesting_permission" ? (
              <RefreshCw className="animate-spin" />
            ) : (
              <Camera />
            )}
            {state === "requesting_permission"
              ? "Requesting permission…"
              : state === "error"
                ? "Try camera again"
                : "Start camera"}
          </Button>
        ) : (
          <>
            <Button
              type="button"
              disabled={disabled || busy}
              onClick={() => void capture()}
            >
              {busy ? <RefreshCw className="animate-spin" /> : <Camera />}
              {state === "capturing"
                ? "Capturing…"
                : state === "submitting"
                  ? "Uploading…"
                  : captureLabel}
            </Button>
            <Button
              type="button"
              variant="outline"
              disabled={busy}
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
