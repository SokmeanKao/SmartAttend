"use client";

import { Check, RefreshCw } from "lucide-react";
import { useRouter } from "next/navigation";
import { useState } from "react";

import { FaceScanner } from "@/components/face-scanner/FaceScanner";
import type {
  CaptureCandidate,
  CaptureResult,
  EnrollmentPose as ScannerPose,
} from "@/components/face-scanner/types";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  captureEnrollmentPose,
  commitEnrollment,
  ENROLLMENT_POSES,
  type EnrollmentPose,
} from "@/features/enrollment/api";
import { ApiError } from "@/lib/api";

type EnrollmentFlowProps = {
  employeeId: string;
  enrollmentId: string;
  disabled?: boolean;
  onCommitted?: () => void;
};

export function EnrollmentFlow({
  employeeId,
  enrollmentId,
  disabled = false,
  onCommitted,
}: EnrollmentFlowProps) {
  const router = useRouter();
  const [requiredPose, setRequiredPose] = useState<EnrollmentPose>("FRONT");
  const [captured, setCaptured] = useState<Set<EnrollmentPose>>(new Set());
  const [committing, setCommitting] = useState(false);
  const [error, setError] = useState("");

  const complete = captured.size === ENROLLMENT_POSES.length;
  const progressPercent = Math.round(
    (captured.size / ENROLLMENT_POSES.length) * 100,
  );

  async function handleCapture(
    candidate: CaptureCandidate,
  ): Promise<CaptureResult> {
    setError("");
    try {
      // guidance is advisory only — never sent as trusted pose metadata
      const result = await captureEnrollmentPose(
        employeeId,
        enrollmentId,
        requiredPose,
        candidate.blob,
      );
      if (!result.accepted || result.pose !== requiredPose) {
        setError("The capture was not accepted. Please try again.");
        return { status: "REJECTED" };
      }

      const nextCaptured = new Set(captured).add(requiredPose);
      setCaptured(nextCaptured);
      const nextPose = ENROLLMENT_POSES.find((pose) => !nextCaptured.has(pose));
      if (nextPose) setRequiredPose(nextPose);
      return { status: "ACCEPTED" };
    } catch (requestError) {
      const message =
        requestError instanceof ApiError
          ? requestError.message
          : requestError instanceof Error
            ? requestError.message
            : "Unable to upload this capture.";
      setError(message);
      return { status: "REJECTED" };
    }
  }

  async function completeEnrollment() {
    if (!complete) return;
    setCommitting(true);
    setError("");
    try {
      await commitEnrollment(employeeId, enrollmentId);
      onCommitted?.();
      router.push(`/employees/${employeeId}`);
      router.refresh();
    } catch (requestError) {
      setError(
        requestError instanceof ApiError
          ? requestError.message
          : "Unable to complete enrollment.",
      );
      setCommitting(false);
    }
  }

  return (
    <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_20rem]">
      <Card>
        <CardHeader>
          <CardTitle>Guided face enrollment</CardTitle>
          <CardDescription>
            Step {ENROLLMENT_POSES.indexOf(requiredPose) + 1} of 3 ·{" "}
            {captured.has(requiredPose) ? "Retake this angle" : requiredPose} ·{" "}
            {progressPercent}%
          </CardDescription>
        </CardHeader>
        <CardContent>
          <FaceScanner
            requiredPose={requiredPose as ScannerPose}
            active={!disabled && !committing}
            autoStart
            onCapture={handleCapture}
          />
          {error && (
            <p role="alert" className="mt-4 text-sm text-destructive">
              {error}
            </p>
          )}
        </CardContent>
      </Card>

      <Card className="h-fit">
        <CardHeader>
          <CardTitle>Enrollment progress</CardTitle>
          <CardDescription>
            Progress advances only after the server accepts each pose. Select a
            completed angle to capture it again.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <ol className="space-y-2">
            {ENROLLMENT_POSES.map((pose, index) => {
              const isCaptured = captured.has(pose);
              const isCurrent = requiredPose === pose;
              return (
                <li key={pose}>
                  <button
                    type="button"
                    disabled={committing || (!isCaptured && !isCurrent)}
                    onClick={() => setRequiredPose(pose)}
                    className={`flex w-full items-center gap-3 rounded-lg border px-3 py-3 text-left transition-colors ${
                      isCurrent
                        ? "border-emerald-300 bg-emerald-50"
                        : "border-border"
                    } disabled:cursor-not-allowed disabled:opacity-60`}
                  >
                    <span
                      className={`flex size-7 shrink-0 items-center justify-center rounded-full text-xs font-semibold ${
                        isCaptured
                          ? "bg-emerald-700 text-white"
                          : "bg-muted text-muted-foreground"
                      }`}
                    >
                      {isCaptured ? <Check className="size-4" /> : index + 1}
                    </span>
                    <span>
                      <span className="block font-medium">{pose}</span>
                      <span className="block text-xs text-muted-foreground">
                        {isCaptured
                          ? "Accepted · click to retake"
                          : isCurrent
                            ? "In progress"
                            : "Pending"}
                      </span>
                    </span>
                  </button>
                </li>
              );
            })}
          </ol>

          <Button
            className="mt-5 w-full"
            size="lg"
            disabled={!complete || committing}
            onClick={() => void completeEnrollment()}
          >
            {committing && <RefreshCw className="animate-spin" />}
            {committing ? "Completing…" : "Complete enrollment"}
          </Button>
          <p className="mt-3 text-center text-xs text-muted-foreground">
            You can complete after all three angles are accepted by the server.
          </p>
        </CardContent>
      </Card>
    </div>
  );
}
