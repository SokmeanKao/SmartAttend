import { apiFetch } from "@/lib/api";

export const ENROLLMENT_POSES = ["FRONT", "LEFT", "RIGHT"] as const;

export type EnrollmentPose = (typeof ENROLLMENT_POSES)[number];

type StartEnrollmentResponse = {
  enrollment_id: string;
  required_poses: EnrollmentPose[];
  expires_in_seconds: number;
};

type CaptureResponse = {
  pose: EnrollmentPose;
  accepted: boolean;
};

type CommitResponse = {
  enrollment_status: "ENROLLED";
};

function enrollmentPath(employeeId: string, enrollmentId?: string) {
  const base = `/api/v1/employees/${employeeId}/face/enroll`;
  return enrollmentId ? `${base}/${enrollmentId}` : base;
}

export function startEnrollment(employeeId: string) {
  return apiFetch<StartEnrollmentResponse>(enrollmentPath(employeeId), {
    method: "POST",
  });
}

export function captureEnrollmentPose(
  employeeId: string,
  enrollmentId: string,
  pose: EnrollmentPose,
  image: Blob,
) {
  const form = new FormData();
  form.append("image", image, `${pose.toLowerCase()}.jpg`);
  return apiFetch<CaptureResponse>(
    `${enrollmentPath(employeeId, enrollmentId)}/${pose}`,
    { method: "POST", body: form },
  );
}

export function commitEnrollment(employeeId: string, enrollmentId: string) {
  return apiFetch<CommitResponse>(
    `${enrollmentPath(employeeId, enrollmentId)}/commit`,
    { method: "POST" },
  );
}

export function abortEnrollment(employeeId: string, enrollmentId: string) {
  return apiFetch<void>(
    `${enrollmentPath(employeeId, enrollmentId)}/abort`,
    { method: "POST", keepalive: true },
  );
}
