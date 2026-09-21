# SmartAttend

Face-recognition attendance system (MVP-1). Monorepo with Next.js web UI, Go API, Python face-service, and PostgreSQL.

## Prerequisites

- Docker Desktop with Compose v2
- GNU Make (optional; every target below is a Docker Compose or `curl` shortcut)
- Node.js 20+ (local web development and verification only)
- Go 1.23+ (local API development only)
- Python 3.12+ (local face-service development only)

## Quick start

```bash
cp .env.example .env
make up
```

On PowerShell, use `Copy-Item .env.example .env` instead of `cp`. The equivalent command without Make is:

```bash
docker compose up --build -d
```

Open `http://localhost:3000/login` and sign in with `admin` / `changeme` when using the example password hash.

> **Use `localhost`, not `127.0.0.1`.** The API only accepts the configured `http://localhost:3000` web origin, cookies are host-scoped, and browsers grant development camera access to the `localhost` secure-context exception. Mixing hostnames causes CORS/cookie failures and may prevent webcam access.

Services:

| Service | URL / port | Exposure |
| --- | --- | --- |
| web | `http://localhost:3000` | Host port `3000` |
| api | `http://localhost:8080` | Host port `8080` |
| face-service | `http://face-service:8090` | Compose network only |
| postgres | `postgres:5432` | Compose network only |

## Environment variables

Copy `.env.example` to `.env` and adjust these values:

| Variable | Purpose | Example/default |
| --- | --- | --- |
| `ADMIN_USERNAME` | Single MVP admin login | `admin` |
| `ADMIN_PASSWORD_HASH` | Argon2id admin password hash | Example accepts `changeme` |
| `WEB_ORIGIN` | Exact browser origin allowed by the API | `http://localhost:3000` |
| `DATABASE_URL` | API PostgreSQL connection string | Compose `postgres` service |
| `FACE_SERVICE_URL` | API-to-face-service URL | `http://face-service:8090` |
| `COOKIE_SECURE` | Require HTTPS for the session cookie | `false` for localhost |
| `SESSION_TTL` | In-memory admin session lifetime, maximum 8 hours | `8h` |
| `BUSINESS_TIMEZONE` | Attendance business-day timezone | `Asia/Phnom_Penh` |
| `NEXT_PUBLIC_API_BASE_URL` | Browser-visible API base URL | `http://localhost:8080` |
| `FACE_MATCH_THRESHOLD` | SFace cosine match threshold | `0.363` |

Sessions use random server-side tokens and are held in API memory. Do not commit `.env` or real credentials.

## Make targets

| Target | Action |
| --- | --- |
| `make up` | Build images and start the Compose stack in the background |
| `make down` | Stop and remove the Compose stack (preserves the named DB volume) |
| `make test-api` | Request the API health endpoint |
| `make test-web` | Request the web root and print its HTTP status |
| `make test-face` | Request face-service health from inside its container |

## Health checks

```bash
make test-api    # curl http://localhost:8080/healthz
make test-web    # HTTP 200 from http://localhost:3000
make test-face   # exec into face-service container
```

Face-service is expose-only on the host. Use `make test-face` or:

```bash
docker compose exec face-service python -c "import urllib.request; print(urllib.request.urlopen('http://localhost:8090/healthz').read().decode())"
```

## Admin password hash

The example hash in `.env.example` corresponds to password `changeme` (escape `$` as `$$` in `.env` for Compose). Regenerate by running the helper and entering the password on standard input:

```bash
cd api
go run ./cmd/hashpwd
```

## Model pins

The face-service image downloads these exact OpenCV Zoo artifacts during build and verifies their SHA256 digests again at startup:

| Model | Pinned artifact | SHA256 |
| --- | --- | --- |
| YuNet | `face_detection_yunet_2023mar.onnx` | `8F2383E4DD3CFBB4553EA8718107FC0423210DC964F9F4280604804ED2552FA4` |
| SFace | `face_recognition_sface_2021dec.onnx` | `0BA9FBFA01B5270C96627C4EF784DA859931E02F04419C829E83484087C34E79` |

The application stores SFace templates with model version `2021dec`; changing models requires an explicit compatibility and re-enrollment plan.

### Licensing and privacy

OpenCV Zoo code is Apache-2.0/MIT per its directories, but pretrained-weight licensing and training-data provenance are separate concerns. The pinned weights are permitted for local MVP-1 development only in this project. Before commercial or production employee-biometric use, complete legal review of the weights and a privacy/compliance review covering biometric consent, retention, access, and deletion. Do not treat SFace `2021dec` as automatically cleared for commercial deployment.

## Database

Initial schema is applied via `api/migrations/001_init.sql` on first Postgres startup. Verify:

```bash
docker compose exec postgres psql -U smartattend -c '\dt'
```

The `pgdata` named volume preserves employees, templates, and attendance across API/container restarts. Admin sessions, enrollment staging, and verification receipts are in API memory and are cleared when the API restarts.

## Local verification

```bash
cd api && go test ./... && go vet ./...
cd ../face-service && python -m pytest -q
cd ../web && npm run lint && npm run build
```

The complete biometric journey still requires a human using a webcam at `http://localhost:3000`: enroll FRONT/LEFT/RIGHT captures, verify the live face, and exercise check-in/check-out. Raw captures are not intended to be persisted.

## Stop

Run `make down` (or `docker compose down`). Add `--volumes` only when you intentionally want to delete local database data.

## Documentation

- Design spec: `docs/superpowers/specs/2026-09-21-smartattend-mvp1-design.md`
- Implementation plan: `docs/superpowers/plans/2026-09-21-smartattend-mvp1.md`
