"use client";

import { ChevronLeft } from "lucide-react";
import Link from "next/link";
import { use, useEffect, useRef, useState } from "react";

import { EnrollmentFlow } from "@/components/enrollment/EnrollmentFlow";
import { Badge } from "@/components/ui/badge";
import { buttonVariants } from "@/components/ui/button";
import {
  abortEnrollment,
  startEnrollment,
} from "@/features/enrollment/api";
import { ApiError, apiFetch, type Employee } from "@/lib/api";

export default function FaceEnrollmentPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = use(params);
  const enrollmentRef = useRef<string | null>(null);
  const committedRef = useRef(false);
  const [employee, setEmployee] = useState<Employee | null>(null);
  const [enrollmentId, setEnrollmentId] = useState("");
  const [loading, setLoading] = useState(true);
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
          Guided face enrollment
        </h1>
        <p className="mt-2 text-muted-foreground">
          Move your head slowly through front, left, and right. Photos are
          processed for enrollment and are not stored.
        </p>
      </div>

      <div className="mt-8">
        <EnrollmentFlow
          employeeId={id}
          enrollmentId={enrollmentId}
          onCommitted={() => {
            committedRef.current = true;
            enrollmentRef.current = null;
          }}
        />
      </div>
    </div>
  );
}
