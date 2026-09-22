export type GuidanceLoop = {
  start: () => void;
  stop: () => void;
  pause: () => void;
  resume: () => void;
};

/**
 * Adaptive rAF guidance loop: never overlaps detect; skips frames when busy;
 * lengthens interval when inference is slow.
 */
export function createGuidanceLoop(opts: {
  getVideo: () => HTMLVideoElement | null;
  isActive: () => boolean;
  detect: (video: HTMLVideoElement, ts: number) => unknown;
  onGuidance: (result: unknown, video: HTMLVideoElement, ts: number) => void;
  onError?: (error: unknown) => void;
  /** Starting target interval ms (~66–100 for 10–15 FPS). */
  targetIntervalMs?: number;
  now?: () => number;
  requestFrame?: (cb: FrameRequestCallback) => number;
  cancelFrame?: (id: number) => void;
}): GuidanceLoop {
  const now = opts.now ?? (() => performance.now());
  const requestFrame =
    opts.requestFrame ?? ((cb) => requestAnimationFrame(cb));
  const cancelFrame = opts.cancelFrame ?? ((id) => cancelAnimationFrame(id));

  let running = false;
  let paused = false;
  let rafId: number | null = null;
  let inFlight = false;
  let lastTs = -1;
  let intervalMs = opts.targetIntervalMs ?? 80;
  let lastFrameTime = Number.NEGATIVE_INFINITY;

  function tick(frameTime: number) {
    rafId = null;
    if (!running || paused) return;

    rafId = requestFrame(tick);

    if (!opts.isActive()) return;
    if (inFlight) return;
    if (frameTime - lastFrameTime < intervalMs) return;

    const video = opts.getVideo();
    if (!video || video.readyState < 2 || video.videoWidth <= 0) return;

    // Skip duplicate timestamps when the video element has not advanced.
    const mediaTs = video.currentTime;
    if (mediaTs === lastTs && lastTs >= 0) return;

    inFlight = true;
    lastFrameTime = frameTime;
    const started = now();
    try {
      const ts = Math.floor(started);
      const result = opts.detect(video, ts);
      lastTs = mediaTs;
      opts.onGuidance(result, video, ts);
    } catch (error) {
      opts.onError?.(error);
    } finally {
      inFlight = false;
      const elapsed = now() - started;
      // Adapt downward when slow; recover gradually toward target.
      const target = opts.targetIntervalMs ?? 80;
      if (elapsed > intervalMs) {
        intervalMs = Math.min(200, Math.max(intervalMs, elapsed * 1.2));
      } else {
        intervalMs = Math.max(target, intervalMs * 0.9);
      }
    }
  }

  return {
    start: () => {
      if (running) return;
      running = true;
      paused = false;
      rafId = requestFrame(tick);
    },
    stop: () => {
      running = false;
      paused = false;
      if (rafId !== null) {
        cancelFrame(rafId);
        rafId = null;
      }
      inFlight = false;
    },
    pause: () => {
      paused = true;
    },
    resume: () => {
      if (!running) return;
      paused = false;
      if (rafId === null) {
        rafId = requestFrame(tick);
      }
    },
  };
}
