# Task 12 Report: Attendance engine + dashboard

## Status

Complete. The strict concurrency gate passed.

## Implemented

- Added public `POST /api/v1/attendance/check-in` and `/check-out` endpoints with a strict `{verification_token}`-only JSON body.
- Added receipt claim → trusted metadata extraction → ACTIVE validation → employee advisory lock → same-event 60-second cooldown → UTC insert → commit → receipt consume flow.
- Business failures, including `DUPLICATE_ATTENDANCE`, release unexpired receipts back to `UNUSED`; successful inserts consume receipts.
- Kept intentional MVP semantics: no check-in/check-out sequence rules beyond same employee + same event type cooldown.
- Added authenticated `GET /api/v1/attendance/today` using `Asia/Phnom_Penh` calendar-day bounds.
- Added dashboard facts for checked-in-today, active employees, and events today, plus the day's event list.
- Connected the existing verify-page Check In / Check Out actions to the new API routes through the shared in-memory receipt store.

## Concurrency and receipt coverage

- Successful attendance consumes its receipt.
- A duplicate leaves the receipt reusable for the other event type.
- Concurrent same-employee/same-event inserts using separate valid receipts produce exactly one event.
- Receipt-store concurrent claim coverage continues to guarantee only one `IN_FLIGHT` claimant.
- Phnom Penh day-bound and fact-count coverage added.

## Verification

- `go test ./...` — pass
- `go vet ./...` — pass
- `go test -race -count=1 ./internal/attendance ./internal/face` in Docker with PostgreSQL — pass
- `npm run lint` — pass
- `npm run build` — pass
- IDE lint diagnostics — none
