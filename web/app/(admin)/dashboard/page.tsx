import Link from "next/link";
import { ArrowRight, Users } from "lucide-react";

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

export default function DashboardPage() {
  return (
    <div className="mx-auto max-w-6xl">
      <p className="text-sm font-medium text-emerald-700">Overview</p>
      <h1 className="mt-1 text-3xl font-semibold tracking-tight">Dashboard</h1>
      <p className="mt-2 text-muted-foreground">
        Today&apos;s attendance summary will appear here in the next milestone.
      </p>

      <div className="mt-8 grid gap-4 sm:grid-cols-3">
        {["Checked in today", "Active employees", "Events today"].map(
          (label) => (
            <Card key={label}>
              <CardHeader className="pb-2">
                <CardTitle className="text-sm font-medium text-muted-foreground">
                  {label}
                </CardTitle>
              </CardHeader>
              <CardContent>
                <p className="text-3xl font-semibold">—</p>
              </CardContent>
            </Card>
          ),
        )}
      </div>

      <Card className="mt-6">
        <CardContent className="flex flex-col items-start gap-4 py-6 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-center gap-3">
            <div className="rounded-lg bg-emerald-50 p-3 text-emerald-700">
              <Users className="size-5" />
            </div>
            <div>
              <p className="font-medium">Manage your employee directory</p>
              <p className="text-sm text-muted-foreground">
                Add employees and update their details.
              </p>
            </div>
          </div>
          <Link
            href="/employees"
            className="inline-flex items-center gap-2 text-sm font-medium text-emerald-700 hover:underline"
          >
            View employees <ArrowRight className="size-4" />
          </Link>
        </CardContent>
      </Card>
    </div>
  );
}
