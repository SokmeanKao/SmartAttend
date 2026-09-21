"use client";

import { Check, ChevronLeft, RefreshCw } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { use, useEffect, useRef, useState } from "react";

import { CameraCapture } from "@/components/camera/CameraCapture";
import { Badge } from "@/components/ui/badge";
import { Button, buttonVariants } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  abortEnrollment,
  captureEnrollmentPose,
  commitEnrollment,
  ENROLLMENT_POSES,
  type EnrollmentPose,
  startEnrollment,
} from "@/features/enrollment/api";
import { ApiError, apiFetch, type Employee } from "@/lib/api";

const POSE_GUIDANCE: Record<
  EnrollmentPose,
  { title: string; instruction: string }
> = {
  FRONT: {
    title: "Look straight ahead",
    instruction: "Center your face and look directly at the camera.",
  },
  LEFT: {
    title: "Turn to your left",
    instruction: "Slowly turn your head to your left and hold still.",
  },
  RIGHT: {
    title: "Turn to your right",
    instruction: "Slowly turn your head to your right and hold still.",
  },
};

export default function FaceEnrollmentPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = use(params);
  const router = useRouter();
  const enrollmentRef = useRef<string | null>(null);
  const committedRef = useRef(false);
  const [employee, setEmployee] = useState<Employee | null>(null);
  const [enrollmentId, setEnrollmentId] = useState("");
  const [currentPose, setCurrentPose] = useState<EnrollmentPose>("FRONT");
  const [captured, setCaptured] = useState<Set<EnrollmentPose>>(new Set());
  const [loading, setLoading] = useState(true);
  const [committing, setCommitting] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    let active = true;

    void (async () => {
      try {
        const loadedEmployee = await apiFetch<Employee>(
          `/api/v1/employees/${id}`,
        );
        if (!active) return;
        setEmployee(loadedEmployee);

        const started = await startEnrollment(id);
        if (!active) {
          void abortEnrollment(id, started.enrollment_id).catch(() => {});
          return;
        }
        enrollmentRef.current = started.enrollment_id;
        setEnrollmentId(started.enrollment_id);
      } catch (requestError) {
        if (!active) return;
        setError(
          requestError instanceof ApiError
            ? requestError.message
            : "Unable to start face enrollment.",
        );
      } finally {
        if (active) setLoading(false);
      }
    })();

    return () => {
      active = false;
      const pendingEnrollment = enrollmentRef.current;
      if (pendingEnrollment && !committedRef.current) {
        void abortEnrollment(id, pendingEnrollment).catch(() => {});
      }
    };
  }, [id]);

  async function capturePose(image: Blob) {
    if (!enrollmentId) return;
    setError("");
    try {
      const result = await captureEnrollmentPose(
        id,
        enrollmentId,
        currentPose,
        image,
      );
      if (!result.accepted || result.pose !== currentPose) {
        throw new Error("The capture was not accepted. Please try again.");
      }

      const nextCaptured = new Set(captured).add(currentPose);
      setCaptured(nextCaptured);
      const nextPose = ENROLLMENT_POSES.find(
        (pose) => !nextCaptured.has(pose),
      );
      if (nextPose) setCurrentPose(nextPose);
    } catch (requestError) {
      const message =
        requestError instanceof Error
          ? requestError.message
          : "Unable to upload this capture.";
      setError(message);
      throw new Error(message);
    }
  }

  async function completeEnrollment() {
    if (!enrollmentId || captured.size !== ENROLLMENT_POSES.length) return;
    setCommitting(true);
    setError("");
    try {
      await commitEnrollment(id, enrollmentId);
      committedRef.current = true;
      enrollmentRef.current = null;
      router.push(`/employees/${id}`);
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

  if (loading) {
    return (
      <p className="text-sm text-muted-foreground">
        Preparing face enrollment…
      </p>
    );
  }

  if (!employee || !enrollmentId) {
    return (
      <div className="mx-auto max-w-2xl">
        <h1 className="text-2xl font-semibold">Face enrollment unavailable</h1>
        <p role="alert" className="mt-2 text-sm text-destructive">
          {error || "Unable to start face enrollment."}
        </p>
        <Link
          href={`/employees/${id}`}
          className={buttonVariants({ variant: "outline", className: "mt-5" })}
        >
          Return to employee
        </Link>
      </div>
    );
  }

  const guidance = POSE_GUIDANCE[currentPose];
  const complete = captured.size === ENROLLMENT_POSES.length;

  return (
    <div className="mx-auto max-w-5xl">
      <Link
        href={`/employees/${id}`}
        className="inline-flex items-center gap-2 text-sm text-muted-foreground hover:text-foreground"
      >
        <ChevronLeft className="size-4" />
        {employee.first_name} {employee.last_name}
      </Link>

      <div className="mt-5">
        <Badge variant="outline">Face enrollment</Badge>
        <h1 className="mt-3 text-3xl font-semibold tracking-tight">
          Capture three face angles
        </h1>
        <p className="mt-2 text-muted-foreground">
          Photos are processed for enrollment and are not stored.
        </p>
      </div>

      <div className="mt-8 grid gap-6 lg:grid-cols-[minmax(0,1fr)_20rem]">
        <Card>
          <CardHeader>
            <CardTitle>{guidance.title}</CardTitle>
            <CardDescription>
              Step {ENROLLMENT_POSES.indexOf(currentPose) + 1} of 3 ·{" "}
              {captured.has(currentPose) ? "Retake this angle" : currentPose}
            </CardDescription>
          </CardHeader>
          <CardContent>
            <CameraCapture
              disabled={committing}
              captureLabel={
                captured.has(currentPose)
                  ? `Retake ${currentPose.toLowerCase()}`
                  : `Capture ${currentPose.toLowerCase()}`
              }
              instruction={guidance.instruction}
              onCapture={capturePose}
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
              Select a completed angle to capture it again.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <ol className="space-y-2">
              {ENROLLMENT_POSES.map((pose, index) => {
                const isCaptured = captured.has(pose);
                const isCurrent = currentPose === pose;
                return (
                  <li key={pose}>
                    <button
                      type="button"
                      disabled={committing || (!isCaptured && !isCurrent)}
                      onClick={() => setCurrentPose(pose)}
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
                          {isCaptured ? "Captured · click to retake" : "Pending"}
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
              You can complete after all three angles are accepted.
            </p>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
