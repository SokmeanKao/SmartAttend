"use client";

type OvalOverlayProps = {
  /** true when face is centered and distance is GOOD */
  aligned?: boolean;
};

/**
 * CSS oval mask. Preview video is CSS-mirrored; oval is centered on the
 * display box (symmetric) so mirroring does not invert the guide shape.
 */
export function OvalOverlay({ aligned = false }: OvalOverlayProps) {
  return (
    <div className="pointer-events-none absolute inset-0 flex items-center justify-center">
      <div
        className={`aspect-[4/5] h-[72%] rounded-[50%] border-2 ${
          aligned
            ? "border-emerald-400 shadow-[0_0_0_9999px_rgba(0,0,0,0.45)]"
            : "border-white/80 shadow-[0_0_0_9999px_rgba(0,0,0,0.55)]"
        }`}
        aria-hidden
      />
    </div>
  );
}
