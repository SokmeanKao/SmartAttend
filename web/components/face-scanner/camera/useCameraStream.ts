"use client";

import { useCallback, useEffect, useRef, useState } from "react";

export type CameraOption = {
  deviceId: string;
  label: string;
};

async function listCameraOptions(): Promise<CameraOption[]> {
  if (!navigator.mediaDevices?.enumerateDevices) return [];
  const devices = await navigator.mediaDevices.enumerateDevices();
  return devices
    .filter((device) => device.kind === "videoinput")
    .map((device, index) => ({
      deviceId: device.deviceId,
      label: device.label.trim() || `Camera ${index + 1}`,
    }));
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

  // Default facingMode user where supported — no physical/virtual security filter.
  return navigator.mediaDevices.getUserMedia({
    audio: false,
    video: {
      facingMode: "user",
      width: { ideal: 1280 },
      height: { ideal: 720 },
    },
  });
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

export function useCameraStream(videoRef: React.RefObject<HTMLVideoElement | null>) {
  const streamRef = useRef<MediaStream | null>(null);
  const mountedRef = useRef(true);
  const [cameraLive, setCameraLive] = useState(false);
  const [cameras, setCameras] = useState<CameraOption[]>([]);
  const [selectedDeviceId, setSelectedDeviceId] = useState("");
  const [activeLabel, setActiveLabel] = useState("");
  const [error, setError] = useState("");
  const [requesting, setRequesting] = useState(false);

  const stopTracks = useCallback(() => {
    streamRef.current?.getTracks().forEach((track) => track.stop());
    streamRef.current = null;
  }, []);

  const stopCamera = useCallback(() => {
    stopTracks();
    if (videoRef.current) {
      videoRef.current.srcObject = null;
    }
    setCameraLive(false);
    setActiveLabel("");
  }, [stopTracks, videoRef]);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      streamRef.current?.getTracks().forEach((track) => track.stop());
      streamRef.current = null;
    };
  }, []);

  const startCamera = useCallback(
    async (deviceId?: string) => {
      setRequesting(true);
      setError("");
      try {
        if (!navigator.mediaDevices?.getUserMedia) {
          throw new Error("Camera access is not supported by this browser.");
        }

        let stream = await openCameraStream(deviceId);
        const options = await listCameraOptions();
        if (mountedRef.current) setCameras(options);

        const preferred =
          deviceId ||
          selectedDeviceId ||
          options[0]?.deviceId ||
          undefined;
        const currentId = stream.getVideoTracks()[0]?.getSettings().deviceId;
        if (preferred && preferred !== currentId && options.length > 0) {
          const match = options.find((o) => o.deviceId === preferred);
          if (match) {
            stream.getTracks().forEach((t) => t.stop());
            stream = await openCameraStream(match.deviceId);
          }
        }

        if (!mountedRef.current) {
          stream.getTracks().forEach((t) => t.stop());
          return;
        }

        const video = videoRef.current;
        if (!video) {
          stream.getTracks().forEach((t) => t.stop());
          throw new Error("Camera preview is not available.");
        }

        stopTracks();
        streamRef.current = stream;
        const track = stream.getVideoTracks()[0];
        setSelectedDeviceId(track?.getSettings().deviceId ?? preferred ?? "");
        setActiveLabel(track?.label || "Camera");
        setCameraLive(true);
        await waitForVideo(video, stream);
      } catch (cameraError) {
        stopTracks();
        setCameraLive(false);
        setActiveLabel("");
        setError(
          cameraError instanceof Error
            ? cameraError.message
            : "Unable to access the camera.",
        );
      } finally {
        if (mountedRef.current) setRequesting(false);
      }
    },
    [selectedDeviceId, stopTracks, videoRef],
  );

  const switchCamera = useCallback(
    async (nextDeviceId: string) => {
      setSelectedDeviceId(nextDeviceId);
      if (!cameraLive) return;
      stopCamera();
      await startCamera(nextDeviceId);
    },
    [cameraLive, startCamera, stopCamera],
  );

  return {
    cameraLive,
    cameras,
    selectedDeviceId,
    activeLabel,
    error,
    requesting,
    startCamera,
    stopCamera,
    switchCamera,
    setError,
  };
}
