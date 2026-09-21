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

type CameraOption = {
  deviceId: string;
  label: string;
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
    value.includes("windows hello") ||
    value.includes("rgbcamerair") ||
    value.includes("camera ir") ||
    /\bir\b/.test(value)
  );
}

function isLikelyVirtualLabel(label: string): boolean {
  const value = label.toLowerCase();
  return (
    value.includes("virtual") ||
    value.includes("obs") ||
    value.includes("manycam") ||
    value.includes("droidcam") ||
    value.includes("snap camera") ||
    value.includes("nvidia broadcast") ||
    value.includes("xsplit") ||
    value.includes("iriun") ||
    value.includes("epoccam") ||
    value.includes("unity video")
  );
}

function rankCamera(label: string): number {
  if (isLikelyVirtualLabel(label)) return 30;
  if (isLikelyInfraredLabel(label)) return 20;
  return 0;
}

async function listCameraOptions(): Promise<CameraOption[]> {
  if (!navigator.mediaDevices?.enumerateDevices) return [];
  const devices = await navigator.mediaDevices.enumerateDevices();
  return devices
    .filter((device) => device.kind === "videoinput")
    .map((device, index) => ({
      deviceId: device.deviceId,
      label: device.label.trim() || `Camera ${index + 1}`,
    }))
    .sort((a, b) => rankCamera(a.label) - rankCamera(b.label));
}

function pickPreferredDeviceId(
  options: CameraOption[],
  preferredId?: string,
): string | undefined {
  if (preferredId && options.some((option) => option.deviceId === preferredId)) {
    return preferredId;
  }
  const preferred = options.find(
    (option) =>
      !isLikelyVirtualLabel(option.label) &&
      !isLikelyInfraredLabel(option.label),
  );
  return (preferred ?? options[0])?.deviceId;
}

async function openCameraStream(deviceId?: string): Promise<MediaStream> {
  if (deviceId) {
    return navigator.mediaDevices.getUserMedia({
      audio: false,
      video: {
        deviceId: { exact: deviceId },
        width: { ideal: 1280 },
        height: { ideal: 720 },
      },
    });
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

function waitForVideo(video: HTMLVideoElement, stream: MediaStream): Promise<void> {
  return new Promise((resolve, reject) => {
    const timeout = window.setTimeout(() => {
      cleanup();
      reject(
        new Error(
          "The camera opened but no video frames arrived. Try another camera.",
        ),
      );
    }, 8000);

    function cleanup() {
      window.clearTimeout(timeout);
      video.removeEventListener("loadeddata", onReady);
      video.removeEventListener("error", onError);
    }

    function onReady() {
      if (video.videoWidth > 0 && video.videoHeight > 0) {
        cleanup();
        resolve();
      }
    }

    function onError() {
      cleanup();
      reject(new Error("The camera stream failed to play."));
    }

    video.addEventListener("loadeddata", onReady);
    video.addEventListener("error", onError);
    video.srcObject = stream;
    video.muted = true;
    video.playsInline = true;

    const playPromise = video.play();
    if (playPromise !== undefined) {
      void playPromise.then(onReady).catch(() => {
        // Autoplay can reject; loadeddata may still arrive after a gesture.
      });
    }

    // Already have frames (common when switching devices).
    if (video.readyState >= 2 && video.videoWidth > 0) {
      onReady();
    }
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
  const [activeLabel, setActiveLabel] = useState("");
  const [cameras, setCameras] = useState<CameraOption[]>([]);
  const [selectedDeviceId, setSelectedDeviceId] = useState<string>("");

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
    setActiveLabel("");
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

  async function refreshCameraList(): Promise<CameraOption[]> {
    const options = await listCameraOptions();
    if (!mountedRef.current) return options;
    setCameras(options);
    return options;
  }

  async function startCamera(deviceId?: string) {
    setState("requesting_permission");
    setError("");
    try {
      if (!navigator.mediaDevices?.getUserMedia) {
        throw new Error("Camera access is not supported by this browser.");
      }

      // Permission first (labels are often empty until a stream is granted).
      let stream = await openCameraStream(deviceId);
      const options = await refreshCameraList();
      const preferredId = pickPreferredDeviceId(
        options,
        deviceId || selectedDeviceId || undefined,
      );
      const currentId = stream.getVideoTracks()[0]?.getSettings().deviceId;
      const currentLabel = stream.getVideoTracks()[0]?.label ?? "";

      // Reopen if we landed on a virtual/IR device and a better one exists.
      if (
        preferredId &&
        preferredId !== currentId &&
        (isLikelyVirtualLabel(currentLabel) ||
          isLikelyInfraredLabel(currentLabel) ||
          !deviceId)
      ) {
        stream.getTracks().forEach((track) => track.stop());
        stream = await openCameraStream(preferredId);
      }

      if (!mountedRef.current) {
        stream.getTracks().forEach((track) => track.stop());
        return;
      }

      const track = stream.getVideoTracks()[0];
      const label = track?.label || "Camera";
      if (isLikelyVirtualLabel(label)) {
        stream.getTracks().forEach((t) => t.stop());
        throw new Error(
          `Selected "${label}", which is a virtual camera (often black). Choose HD Webcam or another physical camera.`,
        );
      }

      const video = videoRef.current;
      if (!video) {
        stream.getTracks().forEach((t) => t.stop());
        throw new Error("Camera preview is not available.");
      }

      stopTracks();
      streamRef.current = stream;
      setSelectedDeviceId(track?.getSettings().deviceId ?? preferredId ?? "");
      setActiveLabel(label);
      setCameraActive(true);
      setState("live");

      await waitForVideo(video, stream);
      if (!mountedRef.current) return;
    } catch (cameraError) {
      stopTracks();
      setCameraActive(false);
      setActiveLabel("");
      setError(
        cameraError instanceof Error
          ? cameraError.message
          : "Unable to access the camera.",
      );
      setState("error");
    }
  }

  async function switchCamera(nextDeviceId: string) {
    setSelectedDeviceId(nextDeviceId);
    if (!cameraActive) return;
    stopTracks();
    if (videoRef.current) videoRef.current.srcObject = null;
    await startCamera(nextDeviceId);
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

      {cameraActive && activeLabel && (
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
            disabled={busy}
            onChange={(event) => void switchCamera(event.target.value)}
          >
            {cameras.map((camera) => (
              <option key={camera.deviceId} value={camera.deviceId}>
                {camera.label}
                {isLikelyVirtualLabel(camera.label) ? " (virtual)" : ""}
                {isLikelyInfraredLabel(camera.label) ? " (IR)" : ""}
              </option>
            ))}
          </select>
        </label>
      )}

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
            onClick={() => void startCamera(selectedDeviceId || undefined)}
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
