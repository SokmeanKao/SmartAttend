"use client";

import { CalendarCheck2, ListChecks, RefreshCw, Users } from "lucide-react";
import { useCallback, useEffect, useState } from "react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { apiFetch } from "@/lib/api";

type AttendanceEvent = {
  id: string;
  employee_id: string;
  employee_code: string;
  employee_first_name: string;
  employee_last_name: string;
  event_type: "CHECK_IN" | "CHECK_OUT";
  occurred_at: string;
};

type TodaySummary = {
  date: string;
  checked_in_today: number;
  active_employees: number;
  events_today: number;
  events: AttendanceEvent[];
};

const metricDefinitions = [
  {
    key: "checked_in_today" as const,
    label: "Checked in today",
    icon: CalendarCheck2,
  },
  {
    key: "active_employees" as const,
    label: "Active employees",
    icon: Users,
  },
  {
    key: "events_today" as const,
    label: "Events today",
    icon: ListChecks,
  },
];

const phnomPenhTime = new Intl.DateTimeFormat("en-GB", {
  timeZone: "Asia/Phnom_Penh",
  hour: "2-digit",
  minute: "2-digit",
  second: "2-digit",
});

export default function DashboardPage() {
  const [summary, setSummary] = useState<TodaySummary | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);

  const loadToday = useCallback(async () => {
    try {
      const result = await apiFetch<TodaySummary>("/api/v1/attendance/today");
      setSummary(result);
      setError("");
    } catch {
      setError("Unable to load today’s attendance. Please try again.");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    let active = true;
    apiFetch<TodaySummary>("/api/v1/attendance/today")
      .then((result) => {
        if (active) setSummary(result);
      })
      .catch(() => {
        if (active) {
          setError("Unable to load today’s attendance. Please try again.");
        }
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, []);

  return (
    <div className="mx-auto max-w-6xl">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <p className="text-sm font-medium text-emerald-700">Overview</p>
          <h1 className="mt-1 text-3xl font-semibold tracking-tight">
            Dashboard
          </h1>
          <p className="mt-2 text-muted-foreground">
            Attendance facts for {summary?.date ?? "today"} in Phnom Penh.
          </p>
        </div>
        <Button
          variant="outline"
          disabled={loading}
          onClick={() => {
            setLoading(true);
            void loadToday();
          }}
        >
          <RefreshCw className={loading ? "animate-spin" : ""} />
          Refresh
        </Button>
      </div>

      <div className="mt-8 grid gap-4 sm:grid-cols-3">
        {metricDefinitions.map(({ key, label, icon: Icon }) => (
          <Card key={key}>
            <CardHeader className="pb-2">
              <div className="flex items-center justify-between">
                <CardTitle className="text-sm font-medium text-muted-foreground">
                  {label}
                </CardTitle>
                <Icon className="size-4 text-emerald-700" />
              </div>
            </CardHeader>
            <CardContent>
              <p className="text-3xl font-semibold">
                {summary ? summary[key] : "—"}
              </p>
            </CardContent>
          </Card>
        ))}
      </div>

      <Card className="mt-6">
        <CardHeader>
          <CardTitle>Today&apos;s events</CardTitle>
        </CardHeader>
        <CardContent>
          {error ? (
            <div
              role="alert"
              className="rounded-lg bg-red-50 px-4 py-3 text-sm text-destructive"
            >
              {error}
            </div>
          ) : !loading && summary?.events.length === 0 ? (
            <p className="py-8 text-center text-sm text-muted-foreground">
              No attendance events have been recorded today.
            </p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Employee</TableHead>
                  <TableHead>Code</TableHead>
                  <TableHead>Event</TableHead>
                  <TableHead className="text-right">Time</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {summary?.events.map((event) => (
                  <TableRow key={event.id}>
                    <TableCell className="font-medium">
                      {event.employee_first_name} {event.employee_last_name}
                    </TableCell>
                    <TableCell>{event.employee_code}</TableCell>
                    <TableCell>
                      {event.event_type === "CHECK_IN"
                        ? "Check in"
                        : "Check out"}
                    </TableCell>
                    <TableCell className="text-right">
                      {phnomPenhTime.format(new Date(event.occurred_at))}
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
