import type { CaptureResult } from "./types";

export type ScannerCaptureState =
  | "IDLE"
  | "CAPTURING"
  | "SUBMITTING"
  | "COOLDOWN";

export type CaptureCoordinator = {
  getState: () => ScannerCaptureState;
  canStartCapture: () => boolean;
  beginCapture: () => boolean;
  markSubmitting: () => void;
  settle: (result: CaptureResult) => void;
  /** Test hook — advance past cooldown immediately. */
  flushCooldown: () => void;
  dispose: () => void;
};

export function createCaptureCoordinator(opts?: {
  cooldownMs?: number;
  now?: () => number;
  schedule?: (fn: () => void, ms: number) => number;
  cancel?: (id: number) => void;
}): CaptureCoordinator {
  const cooldownMs = opts?.cooldownMs ?? 400;
  const schedule = opts?.schedule ?? ((fn, ms) => setTimeout(fn, ms) as unknown as number);
  const cancel = opts?.cancel ?? ((id) => clearTimeout(id));

  let state: ScannerCaptureState = "IDLE";
  let cooldownTimer: number | null = null;

  function clearCooldown() {
    if (cooldownTimer !== null) {
      cancel(cooldownTimer);
      cooldownTimer = null;
    }
  }

  function enterIdle() {
    clearCooldown();
    state = "IDLE";
  }

  return {
    getState: () => state,
    canStartCapture: () => state === "IDLE",
    beginCapture: () => {
      if (state !== "IDLE") return false;
      state = "CAPTURING";
      return true;
    },
    markSubmitting: () => {
      if (state === "CAPTURING") {
        state = "SUBMITTING";
      }
    },
    settle: (_result: CaptureResult) => {
      clearCooldown();
      state = "COOLDOWN";
      if (cooldownMs <= 0) {
        state = "IDLE";
        return;
      }
      cooldownTimer = schedule(() => {
        cooldownTimer = null;
        state = "IDLE";
      }, cooldownMs);
    },
    flushCooldown: () => {
      if (state === "COOLDOWN") {
        enterIdle();
      }
    },
    dispose: () => {
      clearCooldown();
      state = "IDLE";
    },
  };
}
