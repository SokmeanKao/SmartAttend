"use client";

import { useCallback, useEffect, useRef, useState } from "react";

export type CameraOption = {
  deviceId: string;
  label: string;
};

const PER_DEVICE_TIMEOUT_MS = 4_000;
const TOTAL_ATTEMPT_TIMEOUT_MS = 12_000;

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
    value.includes("epoccam")
  );
}

function rankCamera(label: string): number {
  if (!label.trim()) return 10;
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

function withTimeout<T>(
  promise: Promise<T>,
  ms: number,
  message: string,
): Promise<T> {
  return new Promise((resolve, reject) => {
    const timer = window.setTimeout(() => reject(new Error(message)), ms);
    promise.then(
      (value) => {
        window.clearTimeout(timer);
        resolve(value);
      },
      (error) => {
        window.clearTimeout(timer);
        reject(error);
      },
    );
  });
}

function mapGetUserMediaError(error: unknown): Error {
  if (error instanceof DOMException) {
    if (error.name === "NotAllowedError" || error.name === "PermissionDeniedError") {
      return new Error(
        "Camera permission was blocked. In the address bar, set Camera to Allow for this site, close this tab, and open http://localhost:3000 in Chrome or Edge.",
      );
    }
    if (error.name === "NotFoundError" || error.name === "DevicesNotFoundError") {
      return new Error("No camera was found on this device.");
    }
    if (error.name === "NotReadableError" || error.name === "TrackStartError") {
      return new Error(
        "The camera is in use by another app (or a stuck browser tab). Close other camera apps/tabs and try again.",
      );
    }
  }
  if (error instanceof Error) return error;
  return new Error("Unable to access the camera.");
}

async function getUserMediaRaw(
  constraints: MediaStreamConstraints,
): Promise<MediaStream> {
  if (!navigator.mediaDevices?.getUserMedia) {
    throw new Error("Camera access is not supported by this browser.");
  }
  if (!window.isSecureContext) {
    throw new Error(
      "Camera requires a secure context. Use http://localhost:3000 (not an IP) or HTTPS.",
    );
  }
  try {
    return await navigator.mediaDevices.getUserMedia(constraints);
  } catch (error) {
    throw mapGetUserMediaError(error);
  }
}

/**
 * Open a camera without hanging forever on Windows Virtual Camera Device.
 * Tries preferred deviceId first, then each enumerated device with a short
 * timeout, then a bare `{ video: true }` fallback.
 */
async function openCameraStream(deviceId?: string): Promise<MediaStream> {
  const deadline = Date.now() + TOTAL_ATTEMPT_TIMEOUT_MS;

  async function tryOnce(
    constraints: MediaStreamConstraints,
    label: string,
  ): Promise<MediaStream> {
    const remaining = Math.max(1_000, deadline - Date.now());
    const budget = Math.min(PER_DEVICE_TIMEOUT_MS, remaining);
    return withTimeout(
      getUserMediaRaw(constraints),
      budget,
      `Timed out opening ${label}.`,
    );
  }

  if (deviceId) {
    return tryOnce(
      { audio: false, video: { deviceId: { exact: deviceId } } },
      "selected camera",
    );
  }

  const devices = (await navigator.mediaDevices.enumerateDevices()).filter(
    (d) => d.kind === "videoinput" && d.deviceId,
  );
  const ordered = [...devices].sort(
    (a, b) => rankCamera(a.label) - rankCamera(b.label),
  );

  const preferred = ordered.filter(
    (d) => !isLikelyVirtualLabel(d.label) && !isLikelyInfraredLabel(d.label),
  );
  const candidates = preferred.length > 0 ? preferred : ordered;

  const errors: string[] = [];
  for (const device of candidates) {
    if (Date.now() >= deadline) break;
    const name = device.label.trim() || "camera";
    try {
      return await tryOnce(
        { audio: false, video: { deviceId: { exact: device.deviceId } } },
        name,
      );
    } catch (error) {
      errors.push(error instanceof Error ? error.message : String(error));
    }
  }

  if (Date.now() < deadline) {
    try {
      // Bare constraints — no facingMode (hangs on some Windows setups).
      return await tryOnce({ audio: false, video: true }, "default camera");
    } catch (error) {
      errors.push(error instanceof Error ? error.message : String(error));
    }
  }

  throw new Error(
    errors[errors.length - 1] ||
      "Could not open a camera. Close other tabs using the camera, allow Camera for localhost, and retry in Chrome/Edge.",
  );
}

function waitForVideo(
  video: HTMLVideoElement,
  stream: MediaStream,
): Promise<void> {
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
      void playPromise.then(onReady).catch(() => {});
    }

    if (video.readyState >= 2 && video.videoWidth > 0) {
      onReady();
    }
  });
}

function formatCameraLabel(option: CameraOption): string {
  if (isLikelyVirtualLabel(option.label)) return `${option.label} (virtual)`;
  if (isLikelyInfraredLabel(option.label)) return `${option.label} (IR)`;
  return option.label;
}

export function useCameraStream(
  videoRef: React.RefObject<HTMLVideoElement | null>,
) {
  const streamRef = useRef<MediaStream | null>(null);
  const mountedRef = useRef(true);
  const startGenerationRef = useRef(0);
  const [cameraLive, setCameraLive] = useState(false);
  const [cameras, setCameras] = useState<CameraOption[]>([]);
  const [selectedDeviceId, setSelectedDeviceId] = useState("");
  const [activeLabel, setActiveLabel] = useState("");
  const [error, setError] = useState("");
  const [requesting, setRequesting] = useState(false);
  const [waitSeconds, setWaitSeconds] = useState(0);
  const [embedded, setEmbedded] = useState(false);

  const stopTracks = useCallback(() => {
    streamRef.current?.getTracks().forEach((track) => track.stop());
    streamRef.current = null;
  }, []);

  const stopCamera = useCallback(() => {
    startGenerationRef.current += 1;
    stopTracks();
    if (videoRef.current) {
      videoRef.current.srcObject = null;
    }
    setCameraLive(false);
    setActiveLabel("");
    setRequesting(false);
    setWaitSeconds(0);
  }, [stopTracks, videoRef]);

  useEffect(() => {
    mountedRef.current = true;
    try {
      setEmbedded(window.self !== window.top);
    } catch {
      setEmbedded(true);
    }
    return () => {
      mountedRef.current = false;
      startGenerationRef.current += 1;
      streamRef.current?.getTracks().forEach((track) => track.stop());
      streamRef.current = null;
    };
  }, []);

  useEffect(() => {
    if (!requesting) {
      setWaitSeconds(0);
      return;
    }
    setWaitSeconds(0);
    const started = Date.now();
    const tick = window.setInterval(() => {
      setWaitSeconds(Math.floor((Date.now() - started) / 1000));
    }, 250);
    const failSafe = window.setTimeout(() => {
      if (!mountedRef.current) return;
      startGenerationRef.current += 1;
      setRequesting(false);
      setWaitSeconds(0);
      setError(
        "Camera did not respond. Close this tab completely, then open http://localhost:3000 in Chrome or Edge (not an embedded preview). Allow Camera when prompted.",
      );
    }, TOTAL_ATTEMPT_TIMEOUT_MS + 2_000);
    return () => {
      window.clearInterval(tick);
      window.clearTimeout(failSafe);
    };
  }, [requesting]);

  const startCamera = useCallback(
    async (deviceId?: string) => {
      const generation = ++startGenerationRef.current;
      setRequesting(true);
      setError("");
      try {
        let stream = await openCameraStream(deviceId);
        if (!mountedRef.current || generation !== startGenerationRef.current) {
          stream.getTracks().forEach((t) => t.stop());
          return;
        }

        const options = await listCameraOptions();
        if (mountedRef.current) setCameras(options);

        const track = stream.getVideoTracks()[0];
        const label = track?.label || "Camera";
        const currentId = track?.getSettings().deviceId;

        // If we landed on virtual/IR and a better labeled device exists, switch.
        const better = options.find(
          (o) =>
            !isLikelyVirtualLabel(o.label) &&
            !isLikelyInfraredLabel(o.label) &&
            o.deviceId !== currentId,
        );
        if (
          better &&
          (isLikelyVirtualLabel(label) || isLikelyInfraredLabel(label))
        ) {
          stream.getTracks().forEach((t) => t.stop());
          stream = await openCameraStream(better.deviceId);
          if (!mountedRef.current || generation !== startGenerationRef.current) {
            stream.getTracks().forEach((t) => t.stop());
            return;
          }
        }

        const finalTrack = stream.getVideoTracks()[0];
        const finalLabel = finalTrack?.label || "Camera";
        if (isLikelyVirtualLabel(finalLabel) && options.length > 1) {
          stream.getTracks().forEach((t) => t.stop());
          throw new Error(
            `Selected "${finalLabel}" (virtual — often black). Choose HD Webcam in the camera list.`,
          );
        }

        const video = videoRef.current;
        if (!video) {
          stream.getTracks().forEach((t) => t.stop());
          throw new Error("Camera preview is not available.");
        }

        stopTracks();
        streamRef.current = stream;
        setSelectedDeviceId(
          finalTrack?.getSettings().deviceId ?? better?.deviceId ?? deviceId ?? "",
        );
        setActiveLabel(finalLabel);
        setCameraLive(true);
        await waitForVideo(video, stream);
      } catch (cameraError) {
        if (generation !== startGenerationRef.current) return;
        stopTracks();
        setCameraLive(false);
        setActiveLabel("");
        setError(mapGetUserMediaError(cameraError).message);
      } finally {
        if (mountedRef.current && generation === startGenerationRef.current) {
          setRequesting(false);
          setWaitSeconds(0);
        }
      }
    },
    [stopTracks, videoRef],
  );

  const switchCamera = useCallback(
    async (nextDeviceId: string) => {
      setSelectedDeviceId(nextDeviceId);
      if (!cameraLive && !requesting) return;
      stopCamera();
      await startCamera(nextDeviceId);
    },
    [cameraLive, requesting, startCamera, stopCamera],
  );

  return {
    cameraLive,
    cameras,
    selectedDeviceId,
    activeLabel,
    error,
    requesting,
    waitSeconds,
    embedded,
    startCamera,
    stopCamera,
    switchCamera,
    setError,
    formatCameraLabel,
  };
}
