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

  const stopCamera = useCallback(() => {
    streamRef.current?.getTracks().forEach((track) => track.stop());
    streamRef.current = null;
    if (videoRef.current) videoRef.current.srcObject = null;
    setCameraActive(false);
    setState("idle");
  }, []);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      streamRef.current?.getTracks().forEach((track) => track.stop());
      streamRef.current = null;
    };
  }, []);

  async function startCamera() {
    setState("requesting_permission");
    setError("");
    try {
      if (!navigator.mediaDevices?.getUserMedia) {
        throw new Error("Camera access is not supported by this browser.");
      }
      const stream = await navigator.mediaDevices.getUserMedia({
        audio: false,
        video: { facingMode: "user", width: { ideal: 1280 } },
      });
      if (!mountedRef.current) {
        stream.getTracks().forEach((track) => track.stop());
        return;
      }
      streamRef.current = stream;
      setCameraActive(true);
      if (videoRef.current) {
        videoRef.current.srcObject = stream;
        await videoRef.current.play();
      }
      setState("live");
    } catch (cameraError) {
      streamRef.current?.getTracks().forEach((track) => track.stop());
      streamRef.current = null;
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
          muted
          playsInline
          className={`size-full object-cover ${cameraActive ? "block" : "hidden"}`}
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
