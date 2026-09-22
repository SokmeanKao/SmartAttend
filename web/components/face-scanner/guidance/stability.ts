import { DEFAULT_STABILITY_MS } from "../types";

export function updateStability(args: {
  ready: boolean;
  nowMs: number;
  stableSince: number | null;
  windowMs?: number;
}): { stable: boolean; stableSince: number | null } {
  const windowMs = args.windowMs ?? DEFAULT_STABILITY_MS;

  if (!args.ready) {
    return { stable: false, stableSince: null };
  }

  const stableSince = args.stableSince ?? args.nowMs;
  const stable = args.nowMs - stableSince >= windowMs;
  return { stable, stableSince };
}
