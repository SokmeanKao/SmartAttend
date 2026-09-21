"use client";

import Link from "next/link";
import { FormEvent, use, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { ArrowLeft } from "lucide-react";

import {
  EmployeeFields,
  employeePayload,
} from "@/components/employee-fields";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { ApiError, apiFetch, type Employee } from "@/lib/api";

export default function EmployeeDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = use(params);
  const router = useRouter();
  const [employee, setEmployee] = useState<Employee | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [statusChanging, setStatusChanging] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [confirmOpen, setConfirmOpen] = useState(false);

  useEffect(() => {
    let active = true;
    apiFetch<Employee>(`/api/v1/employees/${id}`)
      .then((result) => {
        if (active) setEmployee(result);
      })
      .catch((requestError: unknown) => {
        if (!active) return;
        setError(
          requestError instanceof ApiError
            ? requestError.message
            : "Unable to load this employee.",
        );
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [id]);

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSaving(true);
    setError("");
    setNotice("");
    try {
      const updated = await apiFetch<Employee>(`/api/v1/employees/${id}`, {
        method: "PATCH",
        body: JSON.stringify(employeePayload(new FormData(event.currentTarget))),
      });
      setEmployee(updated);
      setNotice("Employee details saved.");
    } catch (requestError) {
      setError(
        requestError instanceof ApiError
          ? requestError.message
          : "Unable to save this employee.",
      );
    } finally {
      setSaving(false);
    }
  }

  async function changeStatus(status: "ACTIVE" | "INACTIVE") {
    setStatusChanging(true);
    setError("");
    setNotice("");
    try {
      const updated =
        status === "INACTIVE"
          ? await apiFetch<Employee>(`/api/v1/employees/${id}`, {
              method: "DELETE",
            })
          : await apiFetch<Employee>(`/api/v1/employees/${id}`, {
              method: "PATCH",
              body: JSON.stringify({ status }),
            });
      setEmployee(updated);
      setNotice(
        status === "ACTIVE"
          ? "Employee reactivated."
          : "Employee deactivated.",
      );
      setConfirmOpen(false);
      router.refresh();
    } catch (requestError) {
      setError(
        requestError instanceof ApiError
          ? requestError.message
          : "Unable to update employee status.",
      );
    } finally {
      setStatusChanging(false);
    }
  }

  if (loading) {
    return <p className="text-sm text-muted-foreground">Loading employee…</p>;
  }

  if (!employee) {
    return (
      <div>
        <h1 className="text-2xl font-semibold">Employee unavailable</h1>
        <p className="mt-2 text-sm text-destructive">{error}</p>
        <Link
          href="/employees"
          className="mt-5 inline-block text-sm font-medium text-emerald-700 hover:underline"
        >
          Return to employees
        </Link>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-4xl">
      <Link
        href="/employees"
        className="inline-flex items-center gap-2 text-sm text-muted-foreground hover:text-foreground"
      >
        <ArrowLeft className="size-4" /> Employees
      </Link>
      <div className="mt-5 flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="flex items-center gap-3">
            <h1 className="text-3xl font-semibold tracking-tight">
              {employee.first_name} {employee.last_name}
            </h1>
            <Badge
              variant="outline"
              className={
                employee.status === "ACTIVE"
                  ? "border-emerald-200 bg-emerald-50 text-emerald-800"
                  : ""
              }
            >
              {employee.status === "ACTIVE" ? "Active" : "Inactive"}
            </Badge>
          </div>
          <p className="mt-2 text-muted-foreground">
            {employee.employee_code} ·{" "}
            {employee.enrollment_status === "ENROLLED"
              ? "Face enrolled"
              : "Face not enrolled"}
          </p>
        </div>
        {employee.status === "ACTIVE" ? (
          <AlertDialog open={confirmOpen} onOpenChange={setConfirmOpen}>
            <AlertDialogTrigger className="inline-flex h-8 items-center justify-center rounded-lg bg-destructive/10 px-3 text-sm font-medium text-destructive hover:bg-destructive/20">
              Deactivate
            </AlertDialogTrigger>
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>Deactivate this employee?</AlertDialogTitle>
                <AlertDialogDescription>
                  They will no longer be eligible for attendance or face
                  enrollment. Their existing face templates will be retained.
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel disabled={statusChanging}>
                  Cancel
                </AlertDialogCancel>
                <AlertDialogAction
                  variant="destructive"
                  disabled={statusChanging}
                  onClick={() => void changeStatus("INACTIVE")}
                >
                  {statusChanging ? "Deactivating…" : "Deactivate"}
                </AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
        ) : (
          <Button
            variant="outline"
            disabled={statusChanging}
            onClick={() => void changeStatus("ACTIVE")}
          >
            {statusChanging ? "Reactivating…" : "Reactivate"}
          </Button>
        )}
      </div>

      <Card className="mt-8">
        <CardContent className="py-6">
          <form onSubmit={save}>
            <EmployeeFields key={employee.updated_at} employee={employee} />
            {error && (
              <p role="alert" className="mt-5 text-sm text-destructive">
                {error}
              </p>
            )}
            {notice && (
              <p role="status" className="mt-5 text-sm text-emerald-700">
                {notice}
              </p>
            )}
            <div className="mt-7 flex justify-end border-t pt-5">
              <Button disabled={saving}>
                {saving ? "Saving…" : "Save changes"}
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
