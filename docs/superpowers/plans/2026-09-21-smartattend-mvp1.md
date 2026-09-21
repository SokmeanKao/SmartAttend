# SmartAttend MVP-1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build SmartAttend MVP-1 end-to-end: Docker Compose monorepo with Next.js admin/kiosk UI, Go API (auth, employees, enrollment orchestration, verify receipts, attendance), Python YuNet/SFace face-service, and PostgreSQL — through the frozen DoD journey.

**Architecture:** Browser talks only to Go (`:8080`). Go owns Postgres state and sessions. Face-service (`:8090`, internal) is a stateless ML worker for embed/verify. Public `/verify` uses rate-limited 1:1 verification + short-lived receipts; check-in/out consume receipts without a second face capture.

**Tech Stack:** Next.js (App Router) + TypeScript + Tailwind + shadcn/ui + Maven Pro; Go 1.23+; FastAPI + `opencv-python-headless==4.10.0.84`; PostgreSQL 16; Docker Compose.

**Spec:** `docs/superpowers/specs/2026-09-21-smartattend-mvp1-design.md`

## Global Constraints

- Requirement IDs from the frozen spec are binding (AUTH-01, HTTP-01, DATA-01…06, ENROLL-02/03, FACE-*, UI-*, ATTENDANCE-01, VERIFY-RECEIPT-*).
- Never return embeddings or raw images to the browser; never log them.
- `WEB_ORIGIN=http://localhost:3000` — use hostname `localhost` only (not `127.0.0.1`).
- Credentialed CORS: exact `WEB_ORIGIN`, `Allow-Credentials: true`, never `*`.
- Session: opaque ≥256-bit `crypto/rand` token; no `SESSION_SECRET`.
- Models (M4+): YuNet `face_detection_yunet_2023mar.onnx` SHA256 `8F2383E4DD3CFBB4553EA8718107FC0423210DC964F9F4280604804ED2552FA4`; SFace `face_recognition_sface_2021dec.onnx` SHA256 `0BA9FBFA01B5270C96627C4EF784DA859931E02F04419C829E83484087C34E79`; threshold `0.363`.
- Business day timezone: `Asia/Phnom_Penh`. Timestamps UTC in DB.
- Hard rule: do not start M5 until M4 enrollment tests pass.
- Every milestone: `go test ./...`, `go vet ./...`, Next typecheck/lint/build, Python tests/lint, no secrets in logs.
- Commits: one logical commit per completed task.

---

## File structure (create across milestones)

```text
smartattend/
├── docker-compose.yml
├── Makefile
├── README.md
├── .env.example
├── .gitignore
├── api/
│   ├── Dockerfile
│   ├── go.mod
│   ├── cmd/server/main.go
│   ├── internal/
│   │   ├── config/config.go
│   │   ├── database/database.go
│   │   ├── httpapi/
│   │   │   ├── router.go
│   │   │   ├── middleware_auth.go
│   │   │   ├── middleware_cors.go
│   │   │   ├── errors.go
│   │   │   ├── auth_handlers.go
│   │   │   ├── employee_handlers.go
│   │   │   ├── enroll_handlers.go
│   │   │   ├── verify_handlers.go
│   │   │   └── attendance_handlers.go
│   │   ├── auth/session.go
│   │   ├── employee/store.go
│   │   ├── face/
│   │   │   ├── client.go
│   │   │   ├── enrollment.go
│   │   │   └── receipt.go
│   │   └── attendance/engine.go
│   ├── migrations/
│   │   └── 001_init.sql
│   └── internal/.../*_test.go
├── face-service/
│   ├── Dockerfile
│   ├── requirements.txt
│   ├── pyproject.toml / ruff config
│   ├── app/
│   │   ├── main.py
│   │   ├── config.py
│   │   ├── api/routes.py
│   │   ├── detection/yunet.py
│   │   ├── recognition/sface.py
│   │   ├── quality/checks.py
│   │   ├── pose/yaw_score.py
│   │   └── embedding/codec.py
│   ├── scripts/download_models.py
│   ├── models/          # gitignored; downloaded in image build
│   └── tests/
├── web/
│   ├── Dockerfile
│   ├── package.json
│   ├── app/
│   │   ├── layout.tsx
│   │   ├── (public)/login/page.tsx
│   │   ├── (public)/verify/page.tsx
│   │   └── (admin)/...
│   ├── components/camera/CameraCapture.tsx
│   ├── features/...
│   └── lib/api.ts
└── deployments/docker/
    ├── api.Dockerfile      # or use api/Dockerfile
    └── nginx/              # empty until later
```

---

### Task 1: Monorepo skeleton + Compose + Postgres migration

**Milestone:** M1  
**Requirements:** Foundation, DATA-01 schema start  
**Files:**
- Create: `docker-compose.yml`, `Makefile`, `.gitignore`, `.env.example`, `README.md`
- Create: `api/migrations/001_init.sql`
- Create: `api/Dockerfile`, `api/go.mod`, `api/cmd/server/main.go` (health only)
- Create: `face-service/Dockerfile`, `face-service/app/main.py` (health only), `face-service/requirements.txt`
- Create: `web/package.json` minimal Next app shell (or scaffold via create-next-app)

**Interfaces:**
- Produces: Compose services `web`, `api`, `face-service`, `postgres`; `GET /healthz` on api `:8080` and face-service `:8090`

- [ ] **Step 1: Write migration `api/migrations/001_init.sql`**

```sql
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
```

- [ ] **Step 2: Write `docker-compose.yml`**

```yaml
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: smartattend
      POSTGRES_PASSWORD: smartattend
      POSTGRES_DB: smartattend
    volumes:
      - pgdata:/var/lib/postgresql/data
      - ./api/migrations/001_init.sql:/docker-entrypoint-initdb.d/001_init.sql:ro
    # no host ports in production-like mode; for local tools optionally publish 5432
    expose:
      - "5432"
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U smartattend"]
      interval: 5s
      timeout: 5s
      retries: 10

  api:
    build: ./api
    environment:
      DATABASE_URL: postgres://smartattend:smartattend@postgres:5432/smartattend?sslmode=disable
      WEB_ORIGIN: http://localhost:3000
      FACE_SERVICE_URL: http://face-service:8090
      COOKIE_SECURE: "false"
      ADMIN_USERNAME: admin
      ADMIN_PASSWORD_HASH: "${ADMIN_PASSWORD_HASH}"
      SESSION_TTL: 8h
      BUSINESS_TIMEZONE: Asia/Phnom_Penh
    ports:
      - "8080:8080"
    depends_on:
      postgres:
        condition: service_healthy
      face-service:
        condition: service_started

  face-service:
    build: ./face-service
    expose:
      - "8090"
    environment:
      FACE_MATCH_THRESHOLD: "0.363"

  web:
    build: ./web
    environment:
      NEXT_PUBLIC_API_BASE_URL: http://localhost:8080
    ports:
      - "3000:3000"
    depends_on:
      - api

volumes:
  pgdata:
```

- [ ] **Step 3: Minimal Go health server**

`api/cmd/server/main.go` listens `:8080`, `GET /healthz` → `200` `{"status":"ok"}`.

- [ ] **Step 4: Minimal FastAPI health**

`GET /healthz` → `{"status":"ok"}` on `:8090`.

- [ ] **Step 5: Scaffold Next.js app**

```bash
npx create-next-app@latest web --typescript --tailwind --eslint --app --src-dir=false --import-alias "@/*" --use-npm
```

Add a root page that renders `SmartAttend` so Compose “web loads” is obvious.

- [ ] **Step 6: `.env.example` + Makefile targets `up`, `down`, `test-api`, `test-web`, `test-face`**

Generate admin hash for docs (document command):

```bash
# example helper later in api: go run ./cmd/hashpwd 'changeme'
ADMIN_PASSWORD_HASH=<argon2id encoded hash>
```

- [ ] **Step 7: Run Compose acceptance**

```bash
docker compose up --build -d
curl -s http://localhost:8080/healthz
curl -s http://localhost:8090/healthz   # only if temporarily publishing for host curl; otherwise docker compose exec
curl -s -o /dev/null -w "%{http_code}" http://localhost:3000
```

Expected: health `200`; web `200`. Verify migration applied:

```bash
docker compose exec postgres psql -U smartattend -c '\dt'
```

- [ ] **Step 8: Commit**

```bash
git add docker-compose.yml Makefile .gitignore .env.example README.md api face-service web
git commit -m "chore: scaffold SmartAttend monorepo with Compose and schema"
```

---

### Task 2: Go config, CORS (HTTP-01), error envelope

**Milestone:** M1→M2 foundation for HTTP  
**Files:**
- Create: `api/internal/config/config.go`
- Create: `api/internal/httpapi/errors.go`, `middleware_cors.go`, `router.go`
- Test: `api/internal/httpapi/cors_test.go`

**Interfaces:**
- Produces: `config.Load() Config` with `WEB_ORIGIN`, `DATABASE_URL`, `FACE_SERVICE_URL`, `COOKIE_SECURE`, `ADMIN_USERNAME`, `ADMIN_PASSWORD_HASH`, `SESSION_TTL`, `BUSINESS_TIMEZONE`
- Produces: `WriteError(w, status, code, message)`, CORS middleware

- [ ] **Step 1: Write failing CORS test**

```go
func TestCORSPreflightAllowsConfiguredOrigin(t *testing.T) {
    // GET /healthz with Origin: http://localhost:3000
    // expect Access-Control-Allow-Origin: http://localhost:3000
    // expect Access-Control-Allow-Credentials: true
}
func TestCORSRejectsOtherOrigin(t *testing.T) {
    // Origin: http://evil.example → no allow-origin of evil (or omit / 403 on unsafe)
}
```

- [ ] **Step 2: Implement config + CORS + error helper; make tests pass**

- [ ] **Step 3: Commit**

```bash
git commit -m "feat(api): add config, CORS HTTP-01, and error envelope"
```

---

### Task 3: AUTH-01 session store + login/me/logout

**Milestone:** M2  
**Files:**
- Create: `api/internal/auth/session.go`, `password.go`
- Create: `api/internal/httpapi/auth_handlers.go`, `middleware_auth.go`
- Create: `api/cmd/hashpwd/main.go`
- Test: `api/internal/auth/session_test.go`, `api/internal/httpapi/auth_handlers_test.go`

**Interfaces:**
- Produces:

```go
type SessionStore interface {
    Create(username string, ttl time.Duration) (token string, expiresAt time.Time, err error)
    Get(token string) (username string, ok bool)
    Delete(token string)
}
func VerifyPassword(encodedHash, password string) bool
```

Cookie name: `smartattend_session`.

- [ ] **Step 1: Failing tests for login success sets HttpOnly cookie; bad creds → 401 `INVALID_CREDENTIALS`; `/me` without cookie → 401; logout clears session**

- [ ] **Step 2: Implement Argon2id verify (use `github.com/alexedwards/argon2id` or `golang.org/x/crypto/argon2` with encoded hash format consistent with `hashpwd`)**

- [ ] **Step 3: Wire routes; CSRF Origin check on POST login/logout**

- [ ] **Step 4: `go test ./...` + race on session package**

```bash
go test ./internal/auth/... -race
go test ./...
```

- [ ] **Step 5: Commit**

```bash
git commit -m "feat(api): add admin session auth AUTH-01"
```

---

### Task 4: Employee CRUD (DATA-06)

**Milestone:** M2  
**Files:**
- Create: `api/internal/employee/store.go`, `normalize.go`
- Create: `api/internal/httpapi/employee_handlers.go`
- Test: `api/internal/employee/store_test.go` (integration with testcontainers or Compose postgres)

**Interfaces:**
- Produces:

```go
func NormalizeEmployeeCode(s string) string // trim + strings.ToUpper
type Employee struct { /* fields matching schema */ }
```

- DELETE = set `status=INACTIVE` + invalidate enrollments (stub no-op until Task 6 wires enrollment store).
- PATCH must ignore/reject `enrollment_status` changes.

- [ ] **Step 1: Failing tests — create, list, duplicate code 409, normalize ` emp001 ` → `EMP001`, soft deactivate**

- [ ] **Step 2: Implement with `pgx`/`database/sql`**

- [ ] **Step 3: Commit**

```bash
git commit -m "feat(api): employee CRUD with soft deactivate"
```

---

### Task 5: Next.js auth shell + employee pages (UI-01 admin)

**Milestone:** M2  
**Files:**
- Create: `web/lib/api.ts` (`credentials: 'include'`, base URL)
- Create: `web/app/(public)/login/page.tsx`
- Create: `web/app/(admin)/layout.tsx` (auth bootstrap via `/api/v1/auth/me`)
- Create: `web/app/(admin)/dashboard/page.tsx` (placeholder)
- Create: `web/app/(admin)/employees/page.tsx`, `new/page.tsx`, `[id]/page.tsx`
- Add Maven Pro via `next/font/google`; shadcn/ui init

**Interfaces:**
- Consumes: auth + employee JSON APIs
- Produces: working admin login → employees CRUD UI

- [ ] **Step 1: Configure `lib/api.ts` with credentialed fetch to `http://localhost:8080`**

- [ ] **Step 2: Implement login + admin layout redirect on 401**

- [ ] **Step 3: Employees list/create/detail/deactivate confirmation**

- [ ] **Step 4: `npm run lint && npx tsc --noEmit && npm run build`**

- [ ] **Step 5: Commit**

```bash
git commit -m "feat(web): admin login and employee CRUD UI"
```

---

### Task 6: Enrollment staging API + face client stub (ENROLL-02/03)

**Milestone:** M3  
**Files:**
- Create: `api/internal/face/client.go` (HTTP client to face-service)
- Create: `api/internal/face/enrollment.go` (in-memory enrollment sessions)
- Create: `api/internal/httpapi/enroll_handlers.go`
- Create: `face-service/app/api/routes.py` stub `/internal/v1/faces/embed` returning fixed 128-dim unit vector
- Test: `api/internal/face/enrollment_test.go`

**Interfaces:**
- Produces:

```go
type EnrollmentStore interface {
    Start(employeeID uuid.UUID, baseTemplateIDs map[Pose]uuid.UUID) (enrollmentID string, err error)
    Capture(enrollmentID string, pose Pose, tmpl StagedTemplate) error
    Get(enrollmentID string) (*Enrollment, error)
    Abort(enrollmentID string)
    InvalidateEmployee(employeeID uuid.UUID)
}
```

Start/commit require ACTIVE (ENROLL-03). Capture calls face client stub.

- [ ] **Step 1: Failing tests — start returns enrollment_id; capture FRONT twice keeps one; commit incomplete → ENROLLMENT_INCOMPLETE; TTL expiry; InvalidateEmployee drops sessions; inactive start rejected**

- [ ] **Step 2: Implement staging + stub face-service embed**

- [ ] **Step 3: Wire routes from Section 3**

- [ ] **Step 4: Commit**

```bash
git commit -m "feat: enrollment staging API with face-service stub"
```

---

### Task 7: Camera component + enrollment UI (UI-02, UI-05, UI-06)

**Milestone:** M3  
**Files:**
- Create: `web/components/camera/CameraCapture.tsx`
- Create: `web/app/(admin)/employees/[id]/face/page.tsx`
- Create: `web/features/enrollment/*`

**Interfaces:**
- Produces: user-triggered JPEG capture (resize before upload); FRONT→LEFT→RIGHT wizard; Complete enrollment → commit; best-effort abort on unmount

- [ ] **Step 1: CameraCapture with states idle/requesting_permission/live/capturing/submitting/error; stop tracks on unmount**

- [ ] **Step 2: Enrollment page calling start/capture/commit APIs**

- [ ] **Step 3: Manual check — capture reaches Go (stub accepts)**

- [ ] **Step 4: Commit**

```bash
git commit -m "feat(web): guided face enrollment camera UI"
```

---

### Task 8: Real YuNet/SFace pipeline + model pin (FACE-BIO-01…06)

**Milestone:** M4  
**Files:**
- Create: `face-service/scripts/download_models.py`
- Create: `face-service/app/detection/yunet.py`, `recognition/sface.py`, `quality/checks.py`, `pose/yaw_score.py`, `embedding/codec.py`
- Modify: `face-service/Dockerfile` to download + verify SHA256 at build
- Modify: `face-service/app/api/routes.py` real embed
- Test: unit tests with fixture images (synthetic / sample faces under `face-service/tests/fixtures/`)

**Interfaces:**
- Produces: embed JSON with `embedding_encoding=float32-le-base64`, `model_name=sface`, `model_version=2021dec`, `embedding_dim=128`, pose `{label, yaw_score}`

- [ ] **Step 1: Download script verifies SHA256 and fails loudly on mismatch**

- [ ] **Step 2: Implement BGR decode, EXIF orient, dimension limits, YuNet qualifying faces, quality, alignCrop, SFace, L2 normalize, base64 codec**

- [ ] **Step 3: Pose bands from config; `INVALID_POSE` when out of band**

- [ ] **Step 4: Tests for FACE_NOT_FOUND, MULTIPLE_FACES, codec round-trip, SHA pin**

- [ ] **Step 5: Commit**

```bash
git commit -m "feat(face-service): YuNet/SFace embed pipeline with pinned models"
```

---

### Task 9: Atomic enrollment commit + re-enroll conflict (DATA-03/04, ENROLL-02/03)

**Milestone:** M4  
**Files:**
- Modify: `api/internal/face/enrollment.go`, employee face delete
- Create: SQL helpers for revoke+insert in transaction + `pg_advisory_xact_lock`
- Test: concurrency conflict test; failed commit preserves prior; inactive commit rejected

**Interfaces:**
- Commit revalidates ACTIVE under lock; compares `base_templates`; sets `enrollment_status=ENROLLED`

- [ ] **Step 1: Write failing tests listed in M4 acceptance**

- [ ] **Step 2: Implement commit transaction**

- [ ] **Step 3: Deactivate + DELETE face call `InvalidateEmployee`**

- [ ] **Step 4: `go test ./... -race`**

- [ ] **Step 5: Commit**

```bash
git commit -m "feat(api): atomic face enrollment commit and ENROLL-03 guards"
```

---

### Task 10: Face verify + receipts (FACE-VERIFY-01/02, VERIFY-RECEIPT-*)

**Milestone:** M5  
**Files:**
- Create: `api/internal/face/receipt.go`
- Modify: face-service `/internal/v1/faces/verify`
- Create: `api/internal/httpapi/verify_handlers.go`
- Test: receipt claim concurrency, expiry, no-match no token, public `VERIFICATION_UNAVAILABLE`

**Interfaces:**
- Produces:

```go
type ReceiptState string // UNUSED, IN_FLIGHT, CONSUMED
func (s *ReceiptStore) Issue(...) (token string, err error)
func (s *ReceiptStore) Claim(token string) (*Receipt, error) // UNUSED→IN_FLIGHT or error
func (s *ReceiptStore) Release(token string) // IN_FLIGHT→UNUSED if not expired
func (s *ReceiptStore) Consume(token string)
```

- Rate limit verify by IP (e.g. 20/min).
- Match → 200 + token; no-match → 200 `{matched:false}`; never expose embeddings.

- [ ] **Step 1: Face-service verify with max cosine vs refs + threshold 0.363**

- [ ] **Step 2: Go verify handler + receipt store tests (including concurrent Claim)**

- [ ] **Step 3: Commit**

```bash
git commit -m "feat: 1:1 verify with verification receipts"
```

---

### Task 11: Public verify UI (UI-03, UI-04)

**Milestone:** M5  
**Files:**
- Create: `web/app/(public)/verify/page.tsx` (no admin shell)
- States per Section 6 verify machine

- [ ] **Step 1: Implement employee code → capture → verify → show Check In/Out buttons without second capture (buttons can be disabled until M6 wires attendance)**

- [ ] **Step 2: Hide scores unless debug; never render embeddings**

- [ ] **Step 3: Commit**

```bash
git commit -m "feat(web): public verify kiosk UI"
```

---

### Task 12: Attendance engine + dashboard (ATTENDANCE-01, UI-07)

**Milestone:** M6  
**Files:**
- Create: `api/internal/attendance/engine.go`
- Create: `api/internal/httpapi/attendance_handlers.go`
- Modify: dashboard page
- Test: consume-on-success; duplicate leaves receipt usable; concurrent inserts; Phnom Penh day bounds

**Interfaces:**
- Body only `{verification_token}`; identity from receipt
- No sequence rules beyond same-event cooldown (document in code comment)

- [ ] **Step 1: Failing attendance tests from M6 acceptance + DoD steps 11–15**

- [ ] **Step 2: Implement claim → lock → cooldown → insert → consume**

- [ ] **Step 3: `GET /attendance/today` with timezone window**

- [ ] **Step 4: Wire verify UI Check In/Out; dashboard counts (checked-in-today, active, events today)**

- [ ] **Step 5: Full quality gate + manual DoD walkthrough**

- [ ] **Step 6: Commit**

```bash
git commit -m "feat: attendance check-in/out and dashboard today view"
```

---

### Task 13: End-to-end DoD verification + README

**Milestone:** M6 closeout  
**Files:**
- Modify: `README.md` with run instructions, env vars, model pins, localhost hostname warning, licensing note

- [ ] **Step 1: Execute DoD steps 1–17 on Compose**

- [ ] **Step 2: Confirm Security DoD (inspect Network tab + logs)**

- [ ] **Step 3: Commit README**

```bash
git commit -m "docs: document MVP-1 runbook and DoD verification"
```

---

## Spec coverage checklist

| Spec area | Tasks |
| --------- | ----- |
| HTTP-01 CORS | 2 |
| AUTH-01 | 3, 5 |
| Employees / DATA-* schema | 1, 4 |
| ENROLL-02/03 staging+commit | 6, 9 |
| FACE-BIO pins + pipeline | 8 |
| FACE-VERIFY + receipts | 10, 11 |
| ATTENDANCE-01 + dashboard | 12 |
| UI-01…07 | 5, 7, 11, 12 |
| DoD / non-goals documented | 13 |

## Placeholder / consistency self-review

- No TBD steps; model SHA256 and OpenCV pin copied from amended spec.
- Receipt states UNUSED/IN_FLIGHT/CONSUMED named consistently across Tasks 10–12.
- `enrollment_status` persisted values only NOT_ENROLLED/ENROLLED.
- SESSION_SECRET absent from env lists.
- YuNet artifact is `2023mar` (OpenCV 4.10), not `2026may`.
