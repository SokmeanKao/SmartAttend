// Prefer same-origin (empty base) so cookies stay first-party via Next rewrites.
// Override only when intentionally calling the API host directly.
const API_BASE_URL = (process.env.NEXT_PUBLIC_API_BASE_URL ?? "").trim();

type ApiErrorEnvelope = {
  error?: {
    code?: string;
    message?: string;
  };
};

export class ApiError extends Error {
  constructor(
    public readonly status: number,
    public readonly code: string,
    message: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

export type Employee = {
  id: string;
  employee_code: string;
  first_name: string;
  last_name: string;
  email: string | null;
  department: string | null;
  position: string | null;
  status: "ACTIVE" | "INACTIVE";
  enrollment_status: "NOT_ENROLLED" | "ENROLLED";
  created_at: string;
  updated_at: string;
};

export type EmployeePayload = {
  employee_code: string;
  first_name: string;
  last_name: string;
  email: string | null;
  department: string | null;
  position: string | null;
};

export async function apiFetch<T>(
  path: string,
  init: RequestInit = {},
): Promise<T> {
  const headers = new Headers(init.headers);
  headers.set("Accept", "application/json");
  if (init.body && !(init.body instanceof FormData)) {
    headers.set("Content-Type", "application/json");
  }

  const response = await fetch(`${API_BASE_URL}${path}`, {
    ...init,
    headers,
    credentials: "include",
  });

  if (!response.ok) {
    const payload = (await response.json().catch(() => ({}))) as ApiErrorEnvelope;
    throw new ApiError(
      response.status,
      payload.error?.code ?? "REQUEST_FAILED",
      payload.error?.message ?? "Something went wrong. Please try again.",
    );
  }

  if (response.status === 204) {
    return undefined as T;
  }

  return response.json() as Promise<T>;
}
