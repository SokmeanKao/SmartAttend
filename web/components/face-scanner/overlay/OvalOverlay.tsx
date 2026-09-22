"use client";

import type { ComponentType } from "react";
import {
  AlertTriangle,
  CheckCircle2,
  CircleDashed,
  Loader2,
  ScanFace,
  XCircle,
} from "lucide-react";

import type { ScannerFrameState } from "./frameState";
import {
  frameBorderClass,
  frameScrimClass,
  frameStatusCopy,
} from "./frameState";

type OvalOverlayProps = {
  frameState: ScannerFrameState;
  /** Live guidance helper text (ignored for result states' primary label). */
  detailHint?: string;
};

const statusIcon: Record<
  ScannerFrameState,
  ComponentType<{ className?: string }>
> = {
  IDLE: CircleDashed,
  GUIDE_ADJUST: AlertTriangle,
  READY: ScanFace,
  VERIFYING: Loader2,
  MATCHED: CheckCircle2,
  NO_MATCH: XCircle,
  ERROR: XCircle,
};

/**
 * CSS oval mask. Preview video is CSS-mirrored; oval is centered on the
 * display box (symmetric) so mirroring does not invert the guide shape.
 *
 * Border colors: gray/amber/blue = advisory guidance; green/red = API result only.
 */
export function OvalOverlay({
  frameState,
  detailHint = "",
}: OvalOverlayProps) {
  const copy = frameStatusCopy(frameState, detailHint);
  const Icon = statusIcon[frameState];

  return (
    <div className="pointer-events-none absolute inset-0 flex items-center justify-center">
      <div
        className={`aspect-[4/5] h-[72%] rounded-[50%] border-[3px] transition-colors duration-200 ${frameBorderClass[frameState]} ${frameScrimClass[frameState]}`}
        aria-hidden
      />
      <div
        role="status"
        aria-live="polite"
        className="absolute bottom-3 left-1/2 z-10 flex max-w-[92%] -translate-x-1/2 items-center gap-2 rounded-full bg-black/70 px-3 py-1.5 text-xs font-medium text-white backdrop-blur-sm"
      >
        <Icon
          className={`size-3.5 shrink-0 ${
            frameState === "VERIFYING" ? "animate-spin" : ""
          }`}
          aria-hidden
        />
        <span className="shrink-0 whitespace-nowrap">{copy.label}</span>
        <span className="truncate text-white/75">{copy.detail}</span>
      </div>
    </div>
  );
}
