CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE employees (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  employee_code TEXT NOT NULL,
  first_name TEXT NOT NULL,
  last_name TEXT NOT NULL,
  email TEXT NULL,
  department TEXT NULL,
  position TEXT NULL,
  status TEXT NOT NULL CHECK (status IN ('ACTIVE', 'INACTIVE')),
  enrollment_status TEXT NOT NULL CHECK (enrollment_status IN ('NOT_ENROLLED', 'ENROLLED')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT employees_employee_code_key UNIQUE (employee_code)
);

CREATE TABLE face_templates (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  employee_id UUID NOT NULL REFERENCES employees(id) ON DELETE RESTRICT,
  pose TEXT NOT NULL CHECK (pose IN ('FRONT', 'LEFT', 'RIGHT')),
  embedding BYTEA NOT NULL,
  embedding_dim INT NOT NULL CHECK (embedding_dim > 0),
  model_name TEXT NOT NULL,
  model_version TEXT NOT NULL,
  quality_score REAL NOT NULL CHECK (quality_score >= 0 AND quality_score <= 1),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  revoked_at TIMESTAMPTZ NULL
);

CREATE UNIQUE INDEX face_templates_active_pose_uidx
  ON face_templates (employee_id, pose)
  WHERE revoked_at IS NULL;

CREATE TABLE attendance_events (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  employee_id UUID NOT NULL REFERENCES employees(id) ON DELETE RESTRICT,
  event_type TEXT NOT NULL CHECK (event_type IN ('CHECK_IN', 'CHECK_OUT')),
  verification_method TEXT NOT NULL CHECK (verification_method IN ('FACE_1_TO_1')),
  similarity_score REAL NOT NULL,
  matched_pose TEXT NULL CHECK (
    matched_pose IS NULL OR matched_pose IN ('FRONT', 'LEFT', 'RIGHT')
  ),
  model_name TEXT NOT NULL,
  model_version TEXT NOT NULL,
  occurred_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX attendance_events_employee_occurred_idx
  ON attendance_events (employee_id, occurred_at DESC);
