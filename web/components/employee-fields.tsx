import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import type { Employee } from "@/lib/api";

export function EmployeeFields({ employee }: { employee?: Employee }) {
  return (
    <div className="grid gap-5 sm:grid-cols-2">
      <div className="space-y-2">
        <Label htmlFor="employee_code">Employee code</Label>
        <Input
          id="employee_code"
          name="employee_code"
          defaultValue={employee?.employee_code}
          placeholder="EMP001"
          required
        />
      </div>
      <div className="space-y-2">
        <Label htmlFor="department">Department</Label>
        <Input
          id="department"
          name="department"
          defaultValue={employee?.department ?? ""}
          placeholder="Operations"
        />
      </div>
      <div className="space-y-2">
        <Label htmlFor="first_name">First name</Label>
        <Input
          id="first_name"
          name="first_name"
          defaultValue={employee?.first_name}
          required
        />
      </div>
      <div className="space-y-2">
        <Label htmlFor="last_name">Last name</Label>
        <Input
          id="last_name"
          name="last_name"
          defaultValue={employee?.last_name}
          required
        />
      </div>
      <div className="space-y-2">
        <Label htmlFor="email">Email</Label>
        <Input
          id="email"
          name="email"
          type="email"
          defaultValue={employee?.email ?? ""}
          placeholder="name@company.com"
        />
      </div>
      <div className="space-y-2">
        <Label htmlFor="position">Position</Label>
        <Input
          id="position"
          name="position"
          defaultValue={employee?.position ?? ""}
          placeholder="Team member"
        />
      </div>
    </div>
  );
}

export function employeePayload(form: FormData) {
  const optional = (name: string) => {
    const value = String(form.get(name) ?? "").trim();
    return value || null;
  };

  return {
    employee_code: String(form.get("employee_code") ?? "").trim(),
    first_name: String(form.get("first_name") ?? "").trim(),
    last_name: String(form.get("last_name") ?? "").trim(),
    email: optional("email"),
    department: optional("department"),
    position: optional("position"),
  };
}
