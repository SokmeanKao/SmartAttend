# SmartAttend

Face-recognition attendance system (MVP-1). Monorepo with Next.js web UI, Go API, Python face-service, and PostgreSQL.

## Prerequisites

- Docker Desktop with Compose v2
- Node.js 20+ (local web development only)
- Go 1.23+ (local API development only)

## Quick start

```bash
cp .env.example .env
make up
```

Services:

| Service       | URL / port              | Notes                          |
|---------------|-------------------------|--------------------------------|
| web           | http://localhost:3000   | Next.js admin/kiosk shell      |
| api           | http://localhost:8080   | Go REST API                    |
| face-service  | internal :8090          | FastAPI ML worker (no host port) |
| postgres      | internal :5432          | PostgreSQL 16                  |

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

The example hash in `.env.example` corresponds to password `changeme` (escape `$` as `$$` in `.env` for Compose). Regenerate with (added in a later milestone):

```bash
go run ./cmd/hashpwd 'your-password'
```

## Database

Initial schema is applied via `api/migrations/001_init.sql` on first Postgres startup. Verify:

```bash
docker compose exec postgres psql -U smartattend -c '\dt'
```

## Stop

```bash
make down
```

## Documentation

- Design spec: `docs/superpowers/specs/2026-09-21-smartattend-mvp1-design.md`
- Implementation plan: `docs/superpowers/plans/2026-09-21-smartattend-mvp1.md`
