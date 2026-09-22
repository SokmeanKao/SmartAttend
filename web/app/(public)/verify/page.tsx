"use client";

import {
  ArrowLeft,
  CheckCircle2,
  Clock3,
  Fingerprint,
  RefreshCw,
  ShieldCheck,
  UserRoundCheck,
} from "lucide-react";
import { FormEvent, useEffect, useState } from "react";

import { FaceScanner } from "@/components/face-scanner/FaceScanner";
import type {
  CaptureCandidate,
  CaptureResult,
} from "@/components/face-scanner/types";
import type { ScannerFrameOutcome } from "@/components/face-scanner/overlay/frameState";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ApiError, apiFetch } from "@/lib/api";

type VerifyEmployee = {
  id: string;
  employee_code: string;
  first_name: string;
  last_name: string;
};

type VerifyResponse =
  | { matched: false }
  | {
      matched: true;
      employee: VerifyEmployee;
      verification_token: string;
      expires_in_seconds: number;
      similarity_score?: number;
      best_score?: number;
    };

type VerifiedResult = Extract<VerifyResponse, { matched: true }>;
type VerifyStage =
  | "employee_code"
  | "camera"
  | "verifying"
  | "no_match"
  | "verified"
  | "attendance_recorded";
type AttendanceAction = "check-in" | "check-out";

const SHOW_BIOMETRIC_DEBUG =
  process.env.NODE_ENV !== "production" &&
  process.env.NEXT_PUBLIC_SHOW_BIOMETRIC_DEBUG === "true";

function verificationErrorMessage(error: unknown) {
  if (!(error instanceof ApiError)) {
    return "Unable to reach SmartAttend. Please try again.";
  }

  switch (error.code) {
    case "VERIFICATION_UNAVAILABLE":
      return "Verification is not available for this employee. Check the code or ask an administrator.";
    case "FACE_NOT_FOUND":
      return "No face was detected. Face the camera and try again.";
    case "MULTIPLE_FACES":
      return "More than one face was detected. Make sure only you are in view.";
    case "FACE_TOO_SMALL":
      return "Move closer to the camera and try again.";
    case "FACE_TOO_BLURRY":
    case "FACE_QUALITY_TOO_LOW":
      return "The image was not clear enough. Hold still in good lighting and try again.";
    case "RATE_LIMITED":
      return "Too many attempts. Wait a moment before trying again.";
    case "FACE_SERVICE_TIMEOUT":
    case "FACE_SERVICE_UNAVAILABLE":
      return "Face verification is temporarily unavailable. Please try again.";
    default:
      return error.message;
  }
}

export default function VerifyPage() {
  const [stage, setStage] = useState<VerifyStage>("employee_code");
  const [employeeCode, setEmployeeCode] = useState("");
  const [verified, setVerified] = useState<VerifiedResult | null>(null);
  const [error, setError] = useState("");
  const [attendanceAction, setAttendanceAction] =
    useState<AttendanceAction | null>(null);
  const [attendanceMessage, setAttendanceMessage] = useState("");
  const [recordedAction, setRecordedAction] =
    useState<AttendanceAction | null>(null);
  const [scannerActive, setScannerActive] = useState(true);
  const [frameOutcome, setFrameOutcome] = useState<ScannerFrameOutcome | null>(
    null,
  );

  useEffect(() => {
    if (!verified || stage !== "verified") return;

    const timeout = window.setTimeout(() => {
      setVerified(null);
      setStage("employee_code");
      setAttendanceMessage("");
      setFrameOutcome(null);
      setError("Your verification expired. Please verify your face again.");
    }, verified.expires_in_seconds * 1000);

    return () => window.clearTimeout(timeout);
  }, [stage, verified]);

  function continueToCamera(event?: FormEvent<HTMLFormElement>) {
    event?.preventDefault();
    const normalizedCode = employeeCode.trim().toUpperCase();
    if (!normalizedCode) {
      setError("Enter your employee code to continue.");
      return;
    }

    setEmployeeCode(normalizedCode);
    setError("");
    setFrameOutcome(null);
    setScannerActive(true);
    setStage("camera");
  }

  async function verifyFace(
    candidate: CaptureCandidate,
  ): Promise<CaptureResult> {
    setError("");
    setFrameOutcome(null);
    setStage("verifying");

    const form = new FormData();
    form.append("employee_code", employeeCode);
    // guidance is advisory only — never sent as trusted pose metadata
    form.append("image", candidate.blob, "verification.jpg");

    try {
      const result = await apiFetch<VerifyResponse>("/api/v1/face/verify", {
        method: "POST",
        body: form,
      });

      // Settled outcomes deactivate scanner so no auto-recapture loop.
      setScannerActive(false);

      if (!result.matched) {
        setVerified(null);
        setFrameOutcome("NO_MATCH");
        setStage("no_match");
        return { status: "ACCEPTED" };
      }

      setVerified(result);
      setAttendanceMessage("");
      setFrameOutcome("MATCHED");
      // Brief green border on the oval before switching to the action card.
      await new Promise((resolve) => window.setTimeout(resolve, 900));
      setStage("verified");
      return { status: "ACCEPTED" };
    } catch (requestError) {
      const message = verificationErrorMessage(requestError);
      setError(message);
      setFrameOutcome("ERROR");
      setStage("camera");
      setScannerActive(true);
      return { status: "REJECTED" };
    }
  }

  function resetVerification() {
    setVerified(null);
    setError("");
    setAttendanceMessage("");
    setAttendanceAction(null);
    setRecordedAction(null);
    setFrameOutcome(null);
    setScannerActive(true);
    setStage("employee_code");
  }

  function retryFaceVerification() {
    setFrameOutcome(null);
    setError("");
    setScannerActive(true);
    setStage("camera");
  }

  async function recordAttendance(action: AttendanceAction) {
    if (!verified || attendanceAction) return;

    setAttendanceAction(action);
    setAttendanceMessage("");
    try {
      await apiFetch<unknown>(`/api/v1/attendance/${action}`, {
        method: "POST",
        body: JSON.stringify({
          verification_token: verified.verification_token,
        }),
      });
      setRecordedAction(action);
      setVerified(null);
      setStage("attendance_recorded");
    } catch (requestError) {
      if (
        requestError instanceof ApiError &&
        requestError.code === "DUPLICATE_ATTENDANCE"
      ) {
        setAttendanceMessage(
          `A recent ${action === "check-in" ? "check-in" : "check-out"} already exists. You can choose the other action while this verification is valid.`,
        );
      } else if (
        requestError instanceof ApiError &&
        requestError.code === "VERIFICATION_TOKEN_INVALID"
      ) {
        setVerified(null);
        setStage("employee_code");
        setError("Your verification is no longer valid. Please verify again.");
      } else {
        setAttendanceMessage(
          requestError instanceof ApiError
            ? requestError.message
            : "Unable to record attendance. Please try again.",
        );
      }
    } finally {
      setAttendanceAction(null);
    }
  }

  const debugScore = verified?.similarity_score ?? verified?.best_score;

  return (
    <main className="min-h-screen bg-[radial-gradient(circle_at_top_left,#d1fae5,transparent_35%),linear-gradient(to_bottom_right,#f8fafc,#ecfdf5)] px-4 py-6 sm:px-6 lg:py-10">
      <div className="mx-auto flex min-h-[calc(100vh-3rem)] max-w-5xl flex-col">
        <header className="flex items-center justify-between">
          <div className="flex items-center gap-2 text-xl font-bold">
            <span className="flex size-9 items-center justify-center rounded-xl bg-emerald-800 text-white">
              <Fingerprint className="size-5" />
            </span>
            <span>
              Smart<span className="text-emerald-700">Attend</span>
            </span>
          </div>
          <div className="flex items-center gap-2 rounded-full border border-emerald-200 bg-white/80 px-3 py-1.5 text-xs font-medium text-emerald-800 shadow-sm backdrop-blur">
            <ShieldCheck className="size-4" />
            Secure attendance
          </div>
        </header>

        <div className="flex flex-1 items-center justify-center py-8">
          <div className="w-full max-w-2xl">
            {stage === "employee_code" && (
              <Card className="border-0 bg-white/90 shadow-xl shadow-emerald-950/5 ring-1 ring-emerald-950/10 backdrop-blur">
                <CardHeader className="text-center">
                  <div className="mx-auto mb-3 flex size-14 items-center justify-center rounded-2xl bg-emerald-100 text-emerald-800">
                    <Fingerprint className="size-7" />
                  </div>
                  <CardTitle className="text-2xl">Record attendance</CardTitle>
                  <CardDescription>
                    Enter your employee code to begin face verification.
                  </CardDescription>
                </CardHeader>
                <CardContent>
                  <form
                    className="mx-auto max-w-md space-y-5"
                    onSubmit={continueToCamera}
                  >
                    <div className="space-y-2">
                      <Label htmlFor="employee-code">Employee code</Label>
                      <Input
                        id="employee-code"
                        name="employee_code"
                        value={employeeCode}
                        onChange={(event) =>
                          setEmployeeCode(event.target.value.toUpperCase())
                        }
                        autoComplete="off"
                        autoCapitalize="characters"
                        placeholder="e.g. EMP001"
                        className="h-12 text-center text-lg font-semibold tracking-wider uppercase"
                        required
                        autoFocus
                      />
                    </div>
                    {error && (
                      <p
                        role="alert"
                        className="rounded-lg bg-red-50 px-3 py-2 text-center text-sm text-destructive"
                      >
                        {error}
                      </p>
                    )}
                    <Button
                      type="button"
                      className="h-11 w-full"
                      size="lg"
                      onClick={() => continueToCamera()}
                    >
                      Continue
                    </Button>
                  </form>
                </CardContent>
              </Card>
            )}

            {(stage === "camera" ||
              stage === "verifying" ||
              stage === "no_match") && (
              <Card className="border-0 bg-white/95 shadow-xl shadow-emerald-950/5 ring-1 ring-emerald-950/10">
                <CardHeader>
                  <button
                    type="button"
                    onClick={resetVerification}
                    className="mb-2 inline-flex w-fit items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
                  >
                    <ArrowLeft className="size-4" />
                    Change employee code
                  </button>
                  <CardTitle className="text-2xl">Verify your face</CardTitle>
                  <CardDescription>
                    Employee {employeeCode} · Look directly at the camera in
                    good lighting.
                  </CardDescription>
                </CardHeader>
                <CardContent>
                  <FaceScanner
                    requiredPose="FRONT"
                    active={scannerActive && stage === "camera"}
                    frameOutcome={
                      stage === "no_match" ? "NO_MATCH" : frameOutcome
                    }
                    onCapture={verifyFace}
                  />
                  {stage === "verifying" && (
                    <p
                      aria-live="polite"
                      className="mt-4 text-center text-sm text-muted-foreground"
                    >
                      Verifying with the server — please wait.
                    </p>
                  )}
                  {stage === "no_match" && (
                    <div className="mt-4 space-y-3">
                      <p
                        role="alert"
                        className="rounded-lg bg-red-50 px-3 py-2 text-center text-sm text-destructive"
                      >
                        Face not matched. Check lighting and try again.
                      </p>
                      <div className="flex flex-col justify-center gap-3 sm:flex-row">
                        <Button onClick={retryFaceVerification}>
                          Try face verification again
                        </Button>
                        <Button variant="outline" onClick={resetVerification}>
                          Use a different code
                        </Button>
                      </div>
                    </div>
                  )}
                  {error && stage === "camera" && (
                    <p
                      role="alert"
                      className="mt-4 rounded-lg bg-red-50 px-3 py-2 text-sm text-destructive"
                    >
                      {error}
                    </p>
                  )}
                  {frameOutcome === "ERROR" && stage === "camera" && (
                    <div className="mt-3 flex justify-center">
                      <Button
                        type="button"
                        variant="outline"
                        onClick={() => {
                          setFrameOutcome(null);
                          setError("");
                        }}
                      >
                        Dismiss and retry
                      </Button>
                    </div>
                  )}
                </CardContent>
              </Card>
            )}

            {stage === "verified" && verified && (
              <Card className="border-0 bg-white/95 shadow-xl shadow-emerald-950/5 ring-1 ring-emerald-950/10">
                <CardHeader className="text-center">
                  <div className="mx-auto mb-3 flex size-16 items-center justify-center rounded-full bg-emerald-100 text-emerald-700">
                    <UserRoundCheck className="size-8" />
                  </div>
                  <CardTitle className="text-2xl">
                    Welcome, {verified.employee.first_name}
                  </CardTitle>
                  <CardDescription>
                    {verified.employee.first_name}{" "}
                    {verified.employee.last_name} ·{" "}
                    {verified.employee.employee_code}
                  </CardDescription>
                </CardHeader>
                <CardContent>
                  <div className="mx-auto max-w-lg">
                    <div className="flex items-center justify-center gap-2 rounded-lg bg-emerald-50 px-3 py-2 text-sm font-medium text-emerald-800">
                      <Clock3 className="size-4" />
                      Face verified · choose one attendance action
                    </div>

                    {SHOW_BIOMETRIC_DEBUG &&
                      typeof debugScore === "number" && (
                        <p className="mt-3 text-center font-mono text-xs text-muted-foreground">
                          Debug similarity: {debugScore.toFixed(4)}
                        </p>
                      )}

                    {attendanceMessage && (
                      <p
                        role="alert"
                        className="mt-4 rounded-lg bg-amber-50 px-3 py-2 text-center text-sm text-amber-800"
                      >
                        {attendanceMessage}
                      </p>
                    )}

                    <div className="mt-6 grid gap-3 sm:grid-cols-2">
                      <Button
                        className="h-14 text-base"
                        disabled={attendanceAction !== null}
                        onClick={() => void recordAttendance("check-in")}
                      >
                        {attendanceAction === "check-in" && (
                          <RefreshCw className="animate-spin" />
                        )}
                        Check In
                      </Button>
                      <Button
                        variant="outline"
                        className="h-14 text-base"
                        disabled={attendanceAction !== null}
                        onClick={() => void recordAttendance("check-out")}
                      >
                        {attendanceAction === "check-out" && (
                          <RefreshCw className="animate-spin" />
                        )}
                        Check Out
                      </Button>
                    </div>
                    <button
                      type="button"
                      onClick={resetVerification}
                      disabled={attendanceAction !== null}
                      className="mx-auto mt-5 block text-sm text-muted-foreground hover:text-foreground disabled:pointer-events-none disabled:opacity-50"
                    >
                      Cancel and start over
                    </button>
                  </div>
                </CardContent>
              </Card>
            )}

            {stage === "attendance_recorded" && recordedAction && (
              <Card className="border-0 bg-white/95 py-8 text-center shadow-xl shadow-emerald-950/5 ring-1 ring-emerald-950/10">
                <CardContent>
                  <CheckCircle2 className="mx-auto size-16 text-emerald-700" />
                  <h1 className="mt-5 text-2xl font-semibold">
                    {recordedAction === "check-in"
                      ? "Checked in"
                      : "Checked out"}
                  </h1>
                  <p className="mt-2 text-muted-foreground">
                    Your attendance was recorded successfully.
                  </p>
                  <Button className="mt-6" onClick={resetVerification}>
                    Done
                  </Button>
                </CardContent>
              </Card>
            )}
          </div>
        </div>

        <footer className="text-center text-xs text-emerald-950/50">
          Captured images are processed for verification and are not stored.
        </footer>
      </div>
    </main>
  );
}
