"use client";

import Link from "next/link";
import { FormEvent, useCallback, useEffect, useState } from "react";
import { Plus, Search, Users } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { ApiError, apiFetch, type Employee } from "@/lib/api";

export default function EmployeesPage() {
  const [employees, setEmployees] = useState<Employee[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [query, setQuery] = useState("");

  const loadEmployees = useCallback(async (search = "") => {
    setLoading(true);
    setError("");
    try {
      const path = search
        ? `/api/v1/employees?q=${encodeURIComponent(search)}`
        : "/api/v1/employees";
      setEmployees(await apiFetch<Employee[]>(path));
    } catch (requestError) {
      setError(
        requestError instanceof ApiError
          ? requestError.message
          : "Unable to load employees.",
      );
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    let active = true;
    apiFetch<Employee[]>("/api/v1/employees")
      .then((result) => {
        if (active) setEmployees(result);
      })
      .catch((requestError: unknown) => {
        if (!active) return;
        setError(
          requestError instanceof ApiError
            ? requestError.message
            : "Unable to load employees.",
        );
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, []);

  function search(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    void loadEmployees(query.trim());
  }

  return (
    <div className="mx-auto max-w-6xl">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <p className="text-sm font-medium text-emerald-700">Directory</p>
          <h1 className="mt-1 text-3xl font-semibold tracking-tight">
            Employees
          </h1>
          <p className="mt-2 text-muted-foreground">
            Manage employee details and account status.
          </p>
        </div>
        <Link
          href="/employees/new"
          className="inline-flex h-9 items-center justify-center gap-2 rounded-lg bg-primary px-3 text-sm font-medium text-primary-foreground hover:bg-primary/80"
        >
          <Plus className="size-4" /> Add employee
        </Link>
      </div>

      <form className="mt-8 flex max-w-md gap-2" onSubmit={search}>
        <Input
          aria-label="Search employees"
          placeholder="Search by code or name"
          value={query}
          onChange={(event) => setQuery(event.target.value)}
        />
        <Button variant="outline" aria-label="Search" disabled={loading}>
          <Search />
        </Button>
      </form>

      <Card className="mt-5 overflow-hidden py-0">
        <CardContent className="p-0">
          {loading ? (
            <p className="p-8 text-center text-sm text-muted-foreground">
              Loading employees…
            </p>
          ) : error ? (
            <div className="p-8 text-center">
              <p className="text-sm text-destructive">{error}</p>
              <Button
                variant="outline"
                className="mt-4"
                onClick={() => loadEmployees(query)}
              >
                Try again
              </Button>
            </div>
          ) : employees.length === 0 ? (
            <div className="flex flex-col items-center p-12 text-center">
              <div className="rounded-full bg-muted p-3">
                <Users className="size-5 text-muted-foreground" />
              </div>
              <p className="mt-4 font-medium">No employees found</p>
              <p className="mt-1 text-sm text-muted-foreground">
                {query
                  ? "Try a different search."
                  : "Add your first employee to get started."}
              </p>
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="pl-5">Employee</TableHead>
                  <TableHead>Department</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Face enrollment</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {employees.map((employee) => (
                  <TableRow key={employee.id}>
                    <TableCell className="pl-5">
                      <Link
                        href={`/employees/${employee.id}`}
                        className="font-medium hover:text-emerald-700 hover:underline"
                      >
                        {employee.first_name} {employee.last_name}
                      </Link>
                      <p className="text-xs text-muted-foreground">
                        {employee.employee_code}
                      </p>
                    </TableCell>
                    <TableCell>{employee.department ?? "—"}</TableCell>
                    <TableCell>
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
                    </TableCell>
                    <TableCell>
                      {employee.enrollment_status === "ENROLLED"
                        ? "Enrolled"
                        : "Not enrolled"}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
