# SmartAttend MVP-1 Design

**Status:** Frozen (Sections 1–7)  
**Date:** 2026-09-21  
**Scope:** Employee CRUD, guided 3-pose enrollment, 1:1 face verification, verification receipts, basic check-in/check-out

---

## 1. System boundaries

SmartAttend MVP-1 is a monorepo with four runtime services:

| Service | Technology | Responsibility | Exposure |
| ------- | ---------- | -------------- | -------- |
| `web` | Next.js + TypeScript + shadcn/ui + Maven Pro | Admin UI, enrollment UI, camera capture, public verify kiosk | `:3000` |
| `api` | Go | Authentication, employees, face-template persistence, verification orchestration, attendance | `:8080` |
| `face-service` | Python | Face detection, quality validation, alignment, embedding generation, multi-template comparison | `:8090`, internal only |
| `postgres` | PostgreSQL | Persistent application data | `:5432`, internal only |

```text
Browser
   │
   │ application API requests
   ▼
Go API
   │
   ├──────────────► PostgreSQL
   │
   └──────────────► Face Service
```

### Hard boundaries

- Browser application requests never call `face-service` directly.
- `face-service` is stateless and never reads from or writes to PostgreSQL.
- Go owns employee, face-template, and attendance persistence.
- For enrollment, Go sends the captured image; face-service returns a validated embedding; Go persists it.
- For verification, Go loads the employee's active compatible templates and sends them with the captured image to face-service.
- Face-service owns detection, quality checks, alignment, embedding generation, similarity calculations, threshold evaluation, and match/no-match decisions.
- Recognition thresholds are face-service configuration, not Go business configuration.
- Go may persist recognition result metadata (similarity score, model version, outcome).
- Raw face images and embeddings are never returned to the browser.
- Raw images and embeddings must never appear in application logs.
- PostgreSQL and `face-service` are internal services and are not publicly exposed.
- Model compatibility is mandatory: templates carry `model_name`, `model_version`, and `embedding_dim`. Face-service rejects incompatible or mixed template sets.

### Locked requirements (boundaries)

| ID | Requirement |
| -- | ----------- |
| **AUTH-01** | Single environment-configured administrator authenticated with Argon2id password verification and an opaque server-side session cookie. No user management or RBAC. |
| **FACE-ENROLL-01** | Enrollment uses three quality-controlled guided captures: `FRONT`, `LEFT`, `RIGHT`. Each successful capture produces one independently stored active face template. |
| **FACE-VERIFY-01** | 1:1 verification compares the live embedding against all compatible active templates for the selected employee. The highest valid similarity score drives the recognition decision. |
| **FACE-ARCH-01** | Face-service owns biometric computation. Go owns application state, persistence, authentication, employees, and attendance. |

---

## 2. Data model

Timestamps are `TIMESTAMPTZ`, stored/generated as UTC. Business-day presentation uses `Asia/Phnom_Penh`.

### `employees`

| Column | Type | Constraint |
| ------ | ---- | ---------- |
| `id` | UUID | PK |
| `employee_code` | TEXT | UNIQUE NOT NULL (trim + uppercase before persist) |
| `first_name` | TEXT | NOT NULL |
| `last_name` | TEXT | NOT NULL |
| `email` | TEXT | NULL |
| `department` | TEXT | NULL |
| `position` | TEXT | NULL |
| `status` | TEXT | CHECK `ACTIVE` / `INACTIVE` |
| `enrollment_status` | TEXT | CHECK `NOT_ENROLLED` / `ENROLLED` only |
| `created_at` | TIMESTAMPTZ | NOT NULL |
| `updated_at` | TIMESTAMPTZ | NOT NULL |

- `enrollment_status` is a controlled cached field: only the enrollment commit path may change it. Employee CRUD cannot set it.
- Persisted values are only `NOT_ENROLLED` and `ENROLLED`. Transient `ENROLLING` exists only in memory during staging.
- Prefer deactivation (`INACTIVE`) over destructive delete once biometric or attendance history exists.
- Deactivation does **not** clear enrollment; templates remain until explicit face deletion.

### `face_templates`

| Column | Type | Constraint |
| ------ | ---- | ---------- |
| `id` | UUID | PK |
| `employee_id` | UUID | FK → employees, NOT NULL, ON DELETE RESTRICT |
| `pose` | TEXT | CHECK `FRONT` / `LEFT` / `RIGHT` |
| `embedding` | BYTEA | NOT NULL; float32 LE; length = `embedding_dim * 4` |
| `embedding_dim` | INT | CHECK `> 0` |
| `model_name` | TEXT | NOT NULL |
| `model_version` | TEXT | NOT NULL |
| `quality_score` | REAL | NOT NULL; CHECK `>= 0 AND <= 1` |
| `created_at` | TIMESTAMPTZ | NOT NULL |
| `revoked_at` | TIMESTAMPTZ | NULL = active |

Partial unique index:

```sql
CREATE UNIQUE INDEX face_templates_active_pose_uidx
ON face_templates (employee_id, pose)
WHERE revoked_at IS NULL;
```

One active FRONT, LEFT, and RIGHT per employee. Active set must share the same `model_name`, `model_version`, and `embedding_dim`.

**Embedding serialization contract:** exactly `embedding_dim` IEEE-754 float32 values, little-endian, consecutive. Example: dim 128 → 512 bytes. Go rejects malformed embeddings before insert. Values must be finite; post-normalization L2 norm ≈ 1.

### `attendance_events`

| Column | Type | Constraint |
| ------ | ---- | ---------- |
| `id` | UUID | PK |
| `employee_id` | UUID | FK → employees, NOT NULL, ON DELETE RESTRICT |
| `event_type` | TEXT | CHECK `CHECK_IN` / `CHECK_OUT` |
| `verification_method` | TEXT | CHECK `FACE_1_TO_1` |
| `similarity_score` | REAL | NOT NULL |
| `matched_pose` | TEXT | NULL or FRONT / LEFT / RIGHT |
| `model_name` | TEXT | NOT NULL |
| `model_version` | TEXT | NOT NULL |
| `occurred_at` | TIMESTAMPTZ | NOT NULL (server clock; never client-supplied) |
| `created_at` | TIMESTAMPTZ | NOT NULL |

Do not store recognition threshold or capture photos on attendance rows in MVP-1.

### Data requirements

| ID | Requirement |
| -- | ----------- |
| **DATA-01** | All persisted timestamps use TIMESTAMPTZ and are generated/stored as UTC. |
| **DATA-02** | Face embeddings use a fixed float32 little-endian binary serialization contract. |
| **DATA-03** | An ENROLLED employee must have exactly one active FRONT/LEFT/RIGHT compatible template set. |
| **DATA-04** | Re-enrollment replacement is atomic; failure preserves the previous active enrollment. |
| **DATA-05** | Attendance duplicate suppression is atomic per employee (advisory lock + cooldown). |
| **DATA-06** | Employees with dependent biometric or attendance records are deactivated rather than destructively deleted. |

### Explicitly out of schema

`users`, `roles`, `devices`, `shifts`, `audit_logs`, photo retention, pgvector.

---

## 3. API contracts & error model

Error envelope:

```json
{
  "error": {
    "code": "FACE_NOT_FOUND",
    "message": "No face detected in the image."
  }
}
```

Success payloads are resource-shaped (no generic wrapper).

### Auth

| Method | Path | Auth |
| ------ | ---- | ---- |
| `POST` | `/api/v1/auth/login` | Public |
| `GET` | `/api/v1/auth/me` | Session |
| `POST` | `/api/v1/auth/logout` | Session |
| `GET` | `/healthz` | Public |

- Env: `ADMIN_USERNAME`, `ADMIN_PASSWORD_HASH` (Argon2id), `SESSION_SECRET`, `COOKIE_SECURE`.
- Opaque HttpOnly cookie `smartattend_session`; SameSite=Lax; Secure from `COOKIE_SECURE`.
- In-memory sessions; absolute TTL 8 hours (not sliding); API restart clears sessions.
- CSRF: validate `Origin` on unsafe methods against configured web origin.
- Cap in-memory session count; periodically purge expired sessions.

### Employees (admin session)

| Method | Path | Notes |
| ------ | ---- | ----- |
| `GET` | `/api/v1/employees` | Optional `?q=` |
| `POST` | `/api/v1/employees` | Normalize `employee_code` |
| `GET` | `/api/v1/employees/{id}` | |
| `PATCH` | `/api/v1/employees/{id}` | Cannot set `enrollment_status` |
| `DELETE` | `/api/v1/employees/{id}` | Soft-deactivate → `INACTIVE` |

### Enrollment (admin session)

| Method | Path | Notes |
| ------ | ---- | ----- |
| `POST` | `/api/v1/employees/{id}/face/enroll` | Start; returns `enrollment_id`, required poses, TTL 600s |
| `POST` | `/api/v1/employees/{id}/face/enroll/{enrollment_id}/{pose}` | Multipart `image`; pose FRONT\|LEFT\|RIGHT; idempotent replace per pose |
| `POST` | `/api/v1/employees/{id}/face/enroll/{enrollment_id}/commit` | Atomic revoke+insert when all 3 staged |
| `POST` | `/api/v1/employees/{id}/face/enroll/{enrollment_id}/abort` | Discard staging |
| `DELETE` | `/api/v1/employees/{id}/face` | Revoke all active; `NOT_ENROLLED` |

Browser enrollment responses never include embeddings:

```json
{ "pose": "FRONT", "accepted": true, "quality_score": 0.91 }
```

### Public verify & attendance

| Method | Path | Auth |
| ------ | ---- | ---- |
| `POST` | `/api/v1/face/verify` | Public (rate-limited) |
| `POST` | `/api/v1/attendance/check-in` | Public (requires valid receipt) |
| `POST` | `/api/v1/attendance/check-out` | Public (requires valid receipt) |
| `GET` | `/api/v1/attendance/today` | Admin session |

Verify request: `employee_code` + `image` (multipart). Max image 5 MiB.

Match:

```json
{
  "matched": true,
  "employee": {
    "id": "uuid",
    "employee_code": "EMP001",
    "first_name": "Sokmean",
    "last_name": "..."
  },
  "verification_token": "...",
  "expires_in_seconds": 60
}
```

No match: `200` `{ "matched": false }` — **not** an error code.

Check-in / check-out body:

```json
{ "verification_token": "..." }
```

Employee identity and biometric metadata come only from the server-side receipt. Client must not supply `employee_id`, scores, or `occurred_at`.

“Today” for attendance list = calendar day in configured business timezone (`Asia/Phnom_Penh`).

### Internal face-service

| Method | Path |
| ------ | ---- |
| `GET` | `/healthz` |
| `POST` | `/internal/v1/faces/embed` |
| `POST` | `/internal/v1/faces/verify` |

Embed response (JSON + base64 embedding):

```json
{
  "quality_score": 0.91,
  "pose": { "label": "FRONT", "yaw_score": -0.04 },
  "embedding": "<base64>",
  "embedding_encoding": "float32-le-base64",
  "embedding_dim": 128,
  "model_name": "sface",
  "model_version": "2021dec"
}
```

Verify: multipart `image` + JSON `references.templates[]`.

### Error codes

| Code | HTTP | Exposure |
| ---- | ---: | -------- |
| `INVALID_CREDENTIALS` | 401 | Admin auth |
| `UNAUTHORIZED` | 401 | Admin session |
| `VALIDATION_ERROR` | 400 | All |
| `EMPLOYEE_NOT_FOUND` | 404 | Admin |
| `EMPLOYEE_CODE_EXISTS` | 409 | Admin |
| `EMPLOYEE_INACTIVE` | 409 | Admin |
| `NOT_ENROLLED` | 409 | Admin |
| `ENROLLMENT_INCOMPLETE` | 409 | Enrollment |
| `ENROLLMENT_EXPIRED` | 410 | Enrollment |
| `ENROLLMENT_CONFLICT` | 409 | Enrollment |
| `FACE_NOT_FOUND` | 422 | Face ops |
| `MULTIPLE_FACES` | 422 | Face ops |
| `FACE_TOO_SMALL` | 422 | Face ops |
| `FACE_TOO_BLURRY` | 422 | Face ops |
| `FACE_QUALITY_TOO_LOW` | 422 | Face ops |
| `INVALID_POSE` | 422 | Enrollment |
| `MODEL_VERSION_MISMATCH` | 422 | Internal/admin |
| `INVALID_REFERENCE_TEMPLATES` | 422 | Internal |
| `VERIFICATION_UNAVAILABLE` | 409 | Public verify (generic) |
| `VERIFICATION_TOKEN_INVALID` | 422 | Attendance |
| `DUPLICATE_ATTENDANCE` | 409 | Attendance |
| `REQUEST_TOO_LARGE` | 413 | Image upload |
| `RATE_LIMITED` | 429 | Public verify |
| `FACE_SERVICE_UNAVAILABLE` | 503 | Go external API |
| `FACE_SERVICE_TIMEOUT` | 504 | Go external API |
| `INTERNAL_ERROR` | 500 | All |

`NO_MATCH` is not an error code. Public verify maps missing/inactive/unenrolled employees to `VERIFICATION_UNAVAILABLE`.

### Verification receipt requirement

| ID | Requirement |
| -- | ----------- |
| **FACE-VERIFY-02** | Successful 1:1 verification creates an opaque, short-lived, one-use server-side verification receipt. Attendance operations consume that receipt and derive employee identity and biometric metadata exclusively from server-side receipt state. Check-in/check-out do not perform a second biometric verification. |

---

## 4. Workflows & state machines

### Admin session

Login → in-memory session (8h absolute TTL) → logout / expiry / API restart → unauthenticated.

### Enrollment

- Start creates opaque `enrollment_id` (TTL 10 minutes) and records `base_templates` (active template IDs at start; empty for first enroll).
- Staging is in-memory only; DB stays `NOT_ENROLLED` or `ENROLLED` until commit.
- Re-capture replaces the staged value for that pose only.
- Commit: employee advisory lock → compare current active IDs to `base_templates` → if changed return `ENROLLMENT_CONFLICT` → else revoke old, insert three, set `ENROLLED`.
- Abort / TTL / API restart: discard staging; DB unchanged.
- Navigation abort is best-effort; TTL is authoritative.

| ID | Requirement |
| -- | ----------- |
| **ENROLL-02** | Enrollment staging is ephemeral and identified by an opaque `enrollment_id`. Commit uses employee-scoped serialization and optimistic comparison against the enrollment's starting active template set. Concurrent stale commits return `ENROLLMENT_CONFLICT`. |

### Verification receipt

States: `UNUSED` → `IN_FLIGHT` → `CONSUMED`. Business failure before successful insert returns receipt to `UNUSED` if not expired. Expiry checked at claim time. Token: ≥256 bits crypto random. TTL 60s. Action-neutral (authorizes CHECK_IN or CHECK_OUT).

| ID | Requirement |
| -- | ----------- |
| **VERIFY-RECEIPT-01** | A successful face verification creates a cryptographically random, 60-second, server-side, action-neutral receipt authorizing exactly one successful attendance operation. |
| **VERIFY-RECEIPT-02** | Receipts transition UNUSED → IN_FLIGHT → CONSUMED. Business failures before attendance insertion return the receipt to UNUSED if it has not expired. Concurrent use of the same receipt is prohibited. |
| **ATTENDANCE-01** | Attendance creation is serialized per employee. The receipt is consumed only after a successful attendance insert. |

### Attendance processing order

1. Atomically claim receipt UNUSED → IN_FLIGHT  
2. Extract trusted employee_id + biometric metadata  
3. Confirm employee ACTIVE  
4. BEGIN; advisory lock(employee)  
5. Cooldown check (default 60s same employee + same event_type)  
6. On duplicate: release IN_FLIGHT → UNUSED; return 409  
7. Insert with server `occurred_at`  
8. COMMIT  
9. Mark receipt CONSUMED  

### Employee lifecycle

`ACTIVE + NOT_ENROLLED` → enroll commit → `ACTIVE + ENROLLED` → deactivate → `INACTIVE + ENROLLED` (templates retained). Reactivation restores verify eligibility without re-enrollment. Explicit face delete → revoke templates → `NOT_ENROLLED`.

---

## 5. Face-service biometric contract

### Models (pinned artifacts)

| Role | Artifact (illustrative pin) | Identity stored in DB |
| ---- | --------------------------- | --------------------- |
| Detector | `face_detection_yunet_*.onnx` + SHA256 | — |
| Recognizer | `face_recognition_sface_2021dec.onnx` + SHA256 | `model_name=sface`, `model_version=2021dec`, `embedding_dim=128` |

Model upgrades are intentional deployments, never automatic Zoo pulls.

### Image intake

- Formats: JPEG, PNG  
- Encoded max: 5 MiB  
- Decoded max: configurable pixel/dimension caps (e.g. 16 MP, ≤4096 per side)  
- Shortest side ≥ 160 px  
- Decode → EXIF orientation → canonical OpenCV **BGR** uint8  
- Go → face-service hard deadline: **5 seconds**

### Pipeline

```text
encoded-size validation
  → decode + EXIF orientation
  → decoded-dimension validation
  → BGR uint8
  → YuNet (confidence + NMS config)
  → filter qualifying faces (confidence + min face px)
  → exactly one qualifying face
  → face-region quality (size, sharpness, exposure)
  → 5-landmark alignment
  → SFace feature
  → finite check + L2 normalize
  → enroll: landmark-derived pose proxy vs expected_pose
  → verify: cosine/dot vs refs → max → threshold
```

Pose uses landmark-derived `yaw_score` / `label`, **not** physical degrees unless a dedicated head-pose estimator is added later.

Quality score is a SmartAttend heuristic in `[0,1]`, not an SFace-native score.

### Threshold

- Owned exclusively by face-service.  
- Development baseline: OpenCV SFace reference cosine threshold **0.363**.  
- Production threshold requires empirical calibration on SmartAttend cameras/population.  
- Do not ship `0.62` as default without calibration.

### Logging

Allowed: `request_id`, `template_count`, `model_version`, `matched`, `best_score`, `processing_ms`, `error_code`.  
Prohibited: embeddings, images.

### Biometric requirements

| ID | Requirement |
| -- | ----------- |
| **FACE-BIO-01** | YuNet provides detection + 5 landmarks; SFace provides the descriptor. Embeddings are finite float32 L2-normalized vectors serialized as IEEE-754 little-endian bytes. |
| **FACE-BIO-02** | Recognition threshold belongs exclusively to face-service. OpenCV SFace reference threshold is the initial development baseline; production threshold requires empirical calibration. |
| **FACE-BIO-03** | Enrollment pose validation is configuration-driven and based on landmark-derived pose metrics. MVP does not claim physical yaw/pitch degrees. |
| **FACE-BIO-04** | Input accepts JPEG/PNG up to 5 MiB plus decoded-pixel/dimension limits. Go imposes a 5-second face-service deadline. |
| **FACE-BIO-05** | Detector and recognizer artifacts are pinned by artifact/version and checksum. Updating a model is intentional change management. |
| **FACE-BIO-06** | Images and embeddings are prohibited from browser responses, persistent logs, and attendance records. |

### Out of MVP-1 biometrics

Liveness, 1:N search, GPU, InsightFace, DeepFace, storing capture photos.

---

## 6. UI pages & UX

### Routes

| Route | Access | Purpose |
| ----- | ------ | ------- |
| `/login` | Public | Admin authentication |
| `/verify` | Public kiosk | Face verify → attendance |
| `/dashboard` | Admin | Today's activity |
| `/employees` | Admin | List/search |
| `/employees/new` | Admin | Create |
| `/employees/[id]` | Admin | Detail |
| `/employees/[id]/face` | Admin | Enrollment / re-enrollment |

App Router: `(public)` for login/verify; `(admin)` layout for shell, auth bootstrap, logout.

### Dashboard metrics (MVP facts only)

- Checked in today (distinct active employees with ≥1 CHECK_IN in Phnom Penh business day)  
- Active employees  
- Events today  

No Late / Absent / Present HR semantics.

### Enrollment UX

FRONT → LEFT → RIGHT guided captures; retake replaces staged pose; final CTA **Complete enrollment** performs commit. Camera states: idle / requesting_permission / live / capturing / submitting / error. Stop media tracks on release.

### Verify UX

Employee code → camera → verifying → match (Check In / Check Out) or no-match retry. `DUPLICATE_ATTENDANCE` keeps verified card while receipt valid. `VERIFICATION_TOKEN_INVALID` clears verified state.

### Debug UI

`NEXT_PUBLIC_SHOW_BIOMETRIC_DEBUG` only controls rendering. Go must not expose embeddings in any environment. Dev-only score metadata may be included by Go in non-production builds; the public flag is not a security boundary.

### Camera capture

User-triggered snapshot → limit dimensions → JPEG encode → verify blob ≤ 5 MiB → upload. No continuous upload.

### UI requirements

| ID | Requirement |
| -- | ----------- |
| **UI-01** | Seven MVP routes. Public: `/login`, `/verify`. Remaining routes use shadcn/ui + Maven Pro admin shell. |
| **UI-02** | Enrollment uses FRONT → LEFT → RIGHT guided captures; re-capture replaces that staged pose; completion commits. |
| **UI-03** | Successful verification creates a receipt and exposes Check In / Check Out without another face capture. |
| **UI-04** | Normal UI does not display biometric scores. Debug presentation is development-only and is not a security boundary. Embeddings never enter browser payloads. |
| **UI-05** | MVP camera capture is user-triggered. No continuous image upload. |
| **UI-06** | Enrollment cleanup on navigation is best-effort; server 10-minute staging TTL is authoritative. |
| **UI-07** | Dashboard statistics describe only MVP-supported facts: active employees, checked-in-today, today's event count. |

---

## 7. Non-goals, DoD, milestones

### Non-goals

- 1:N identification / walk-up without employee code  
- Liveness / anti-spoof  
- Shifts, late, absent, overtime, leave, holidays  
- Reports, CSV export, payroll, HR sync  
- Multi-admin, RBAC, JWT, MFA, SSO  
- Device registry, Redis  
- Storing attendance photos  
- pgvector / InsightFace / DeepFace  
- Mobile apps / geolocation  
- Continuous video recognition streams  
- Production TLS/reverse-proxy deployment (MVP-1 acceptance runs on Docker Compose / localhost; production HTTPS is required later for real kiosk camera use)

### Definition of Done (user journey)

1. Admin logs in (`/login`)  
2. Creates EMP001 Sokmean  
3. Opens face enrollment  
4. Webcam starts  
5. Captures FRONT / LEFT / RIGHT; all pass quality/pose validation  
6. Commit succeeds → ENROLLED  
7. Opens public `/verify`  
8. Enters EMP001  
9. Captures live face  
10. Match succeeds → verification receipt issued  
11. Check In succeeds → receipt consumed  
12. Dashboard shows today's CHECK_IN  
13. Re-verify → new receipt  
14. Check In again within cooldown → `409 DUPLICATE_ATTENDANCE` → receipt remains usable  
15. Use that same receipt for Check Out → succeeds → receipt consumed  
16. Deactivate employee → verification returns `VERIFICATION_UNAVAILABLE`  
17. API restart clears sessions/staging/receipts; DB employee, enrollment, templates, and attendance persist  

### Security DoD (cross-cutting)

- Embeddings never appear in browser responses  
- Embeddings/images never appear in application logs  
- Raw capture images are not persisted  

### Milestone quality gate (every milestone)

- Go: `go test ./...`, `go vet ./...`, race tests where concurrency is introduced  
- Next.js: typecheck, lint, production build  
- Python: unit tests, lint/static checks  
- Relevant Docker Compose integration tests  
- No secrets, embeddings, or raw images in logs  

### Milestones

| Milestone | Delivers | Acceptance |
| --------- | -------- | ---------- |
| **M1 Foundation** | Monorepo, Compose, Postgres migrations, Go/Next/face-service skeletons, healthz | Compose up; health endpoints; web loads; migrations apply |
| **M2 Auth + Employees** | AUTH-01, employee CRUD, soft deactivate | Login cookie; CRUD; duplicate code rejected; enrollment_status not editable |
| **M3 Camera + enroll staging** | Camera capture → Go → face embed (stub OK); enrollment_id flow | 3 poses stage; abort/TTL; no DB until commit |
| **M4 Real YuNet/SFace enroll** | FACE-BIO-*; atomic commit; re-enroll conflict | Quality/pose rejects; ENROLLED with 3 templates; incomplete cannot commit; failed commit preserves prior; `ENROLLMENT_CONFLICT` |
| **M5 Verify + receipt** | FACE-VERIFY-01/02; rate limit; generic public errors | match → 200 + receipt; no-match → 200 matched=false; expiry; concurrent claim single IN_FLIGHT; biometric metadata server-side |
| **M6 Attendance** | check-in/out, cooldown, dashboard | Only `verification_token`; identity from receipt; consume-on-success; duplicate leaves receipt usable; concurrent duplicates cannot double-insert; Phnom Penh business day |

### Hard planning rule

> Do not start M5 biometric verification until M4 enrollment is reliable and its failure/concurrency tests pass.

### Implementation order

```text
M1 Foundation
  → M2 Auth + Employees
  → M3 Camera + Enrollment Staging
  → M4 Real YuNet/SFace Enrollment
  → M5 Verification + Receipt
  → M6 Attendance + Dashboard
```

### Suggested repository layout

```text
smartattend/
├── web/
├── api/
├── face-service/
├── deployments/docker/
├── docs/
├── docker-compose.yml
├── Makefile
└── README.md
```

---

## Requirement ID index

| ID | Section |
| -- | ------- |
| AUTH-01 | 1, 3, 4 |
| FACE-ENROLL-01 | 1, 4, 6 |
| FACE-VERIFY-01 | 1, 3, 5 |
| FACE-VERIFY-02 | 3, 4 |
| FACE-ARCH-01 | 1, 5 |
| DATA-01 … DATA-06 | 2 |
| ENROLL-02 | 4 |
| VERIFY-RECEIPT-01, VERIFY-RECEIPT-02 | 4 |
| ATTENDANCE-01 | 4 |
| FACE-BIO-01 … FACE-BIO-06 | 5 |
| UI-01 … UI-07 | 6 |

---

## References

- OpenCV Zoo SFace: reference cosine threshold and demo artifacts  
- OpenCV Zoo YuNet: detection, confidence/NMS, five landmarks  
- MDN `getUserMedia`: secure contexts (HTTPS; localhost exception for development)
