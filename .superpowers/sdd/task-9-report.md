# Task 9 status: complete
- Atomic commit now locks per employee, revalidates `ACTIVE`, and compares current IDs with `base_templates`.
- Stale re-enrollment returns `ENROLLMENT_CONFLICT`; incomplete enrollment cannot commit.
- Revoke, FRONT/LEFT/RIGHT inserts, and `ENROLLED` update share one transaction.
- Failed inserts roll back and preserve the prior active enrollment.
- Face deletion uses the same employee lock; deactivation and face deletion invalidate pending sessions.
- Added DB concurrency, rollback-preservation, inactive, incomplete, HTTP conflict, and invalidation tests.
- `go test ./... -count=1`: passed.
- Docker CGO race gate for employee/face/httpapi packages: passed.

## Important finding follow-up
- Commit now locks the employee row with `FOR UPDATE`; deactivate and status-changing update use the same advisory lock.
- Deterministic commit-vs-deactivate test proves a waiting commit rejects the now-inactive employee without inserting templates.
- `go test ./internal/employee/... ./internal/httpapi/... -count=1`: passed.
- Docker CGO `-race` for employee/httpapi packages: passed.

## M4 gate follow-up
- Commit reloads the enrollment session only after obtaining the employee advisory and row locks.
- Face deletion invalidates pending sessions while holding the same advisory lock.
- Deterministic face-delete-vs-commit test proves the waiting initial commit returns `ENROLLMENT_EXPIRED`.
- `go test ./... -count=1` and Docker CGO employee/httpapi `-race`: passed.
