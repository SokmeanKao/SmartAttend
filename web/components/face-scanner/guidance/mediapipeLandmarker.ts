"use client";

import {
  FaceLandmarker,
  FilesetResolver,
  type FaceLandmarkerResult,
} from "@mediapipe/tasks-vision";

import manifest from "@/mediapipe.manifest.json";

export type LandmarkerHandle = {
  detectForVideo: (
    video: HTMLVideoElement,
    timestamp: number,
  ) => FaceLandmarkerResult;
  close: () => void;
};

async function createWithDelegate(
  wasmPath: string,
  modelPath: string,
  delegate: "GPU" | "CPU",
): Promise<LandmarkerHandle> {
  const vision = await FilesetResolver.forVisionTasks(wasmPath);
  const landmarker = await FaceLandmarker.createFromOptions(vision, {
    baseOptions: {
      modelAssetPath: modelPath,
      delegate,
    },
    runningMode: "VIDEO",
    numFaces: 1,
  });

  return {
    detectForVideo: (video, timestamp) =>
      landmarker.detectForVideo(video, timestamp),
    close: () => landmarker.close(),
  };
}

/**
 * GPU preferred; CPU retry. Throws if both fail (caller → manual-primary).
 */
export async function createFaceLandmarker(opts?: {
  wasmPath?: string;
  modelPath?: string;
}): Promise<LandmarkerHandle> {
  const wasmPath = opts?.wasmPath ?? manifest.wasmPath;
  const modelPath = opts?.modelPath ?? manifest.modelPath;

  if (wasmPath.includes("@latest") || modelPath.includes("@latest")) {
    throw new Error("MediaPipe asset paths must not use @latest");
  }

  try {
    return await createWithDelegate(wasmPath, modelPath, "GPU");
  } catch {
    return await createWithDelegate(wasmPath, modelPath, "CPU");
  }
}
