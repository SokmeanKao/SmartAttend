"use client";

import Link from "next/link";
import { FormEvent, useState } from "react";
import { useRouter } from "next/navigation";
import { ArrowLeft } from "lucide-react";

import {
  EmployeeFields,
  employeePayload,
} from "@/components/employee-fields";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { ApiError, apiFetch, type Employee } from "@/lib/api";

export default function NewEmployeePage() {
  const router = useRouter();
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSubmitting(true);
    setError("");

    try {
      const created = await apiFetch<Employee>("/api/v1/employees", {
        method: "POST",
        body: JSON.stringify(employeePayload(new FormData(event.currentTarget))),
      });
      router.push(`/employees/${created.id}`);
    } catch (requestError) {
      setError(
        requestError instanceof ApiError
          ? requestError.message
          : "Unable to create this employee.",
      );
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="mx-auto max-w-4xl">
      <Link
        href="/employees"
        className="inline-flex items-center gap-2 text-sm text-muted-foreground hover:text-foreground"
      >
        <ArrowLeft className="size-4" /> Employees
      </Link>
      <h1 className="mt-5 text-3xl font-semibold tracking-tight">
        Add employee
      </h1>
      <p className="mt-2 text-muted-foreground">
        Create a record for a new team member.
      </p>

      <Card className="mt-8">
        <CardContent className="py-6">
          <form onSubmit={handleSubmit}>
            <EmployeeFields />
            {error && (
              <p role="alert" className="mt-5 text-sm text-destructive">
                {error}
              </p>
            )}
            <div className="mt-7 flex justify-end gap-3 border-t pt-5">
              <Link
                href="/employees"
                className="inline-flex h-8 items-center rounded-lg border px-3 text-sm font-medium hover:bg-muted"
              >
                Cancel
              </Link>
              <Button disabled={submitting}>
                {submitting ? "Creating…" : "Create employee"}
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
