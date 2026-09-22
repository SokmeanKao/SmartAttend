const MAX_IMAGE_BYTES = 5 * 1024 * 1024;
const MAX_IMAGE_DIMENSION = 1280;

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

/** JPEG from video in unmirrored sensor orientation (no CSS mirror on canvas). */
export async function createJpegFromVideo(
  video: HTMLVideoElement,
): Promise<Blob> {
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
