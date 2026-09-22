"use client";

type GuidanceBannerProps = {
  message: string;
};

export function GuidanceBanner({ message }: GuidanceBannerProps) {
  return (
    <div className="absolute inset-x-4 bottom-4 rounded-lg bg-black/65 px-4 py-3 text-center text-sm font-medium text-white">
      {message}
    </div>
  );
}
