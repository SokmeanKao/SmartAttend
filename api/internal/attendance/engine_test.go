package attendance

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/smartattend/api/internal/face"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestEngineReceiptLifecycleAndConcurrentCooldown(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 22, 2, 30, 0, 0, time.FixedZone("ICT", 7*60*60))
	receipts := face.NewReceiptStore(time.Minute, func() time.Time { return now })
	engine := NewEngine(pool, receipts, time.Minute, func() time.Time { return now })

	employeeID := insertEmployee(t, pool, "EMP001", "ACTIVE")

	firstToken := issueReceipt(t, receipts, employeeID)
	first, err := engine.Record(ctx, firstToken, EventCheckIn)
	if err != nil {
		t.Fatalf("first Record() error = %v", err)
	}
	if first.EmployeeID != employeeID || first.EventType != EventCheckIn {
		t.Fatalf("first event = %#v", first)
	}
	if !first.OccurredAt.Equal(now.UTC()) || first.OccurredAt.Location() != time.UTC {
		t.Fatalf("occurred_at = %v, want server UTC %v", first.OccurredAt, now.UTC())
	}
	if _, err := receipts.Claim(firstToken); !errors.Is(err, face.ErrReceiptInvalid) {
		t.Fatalf("successful receipt Claim() error = %v, want invalid/consumed", err)
	}

	duplicateToken := issueReceipt(t, receipts, employeeID)
	if _, err := engine.Record(ctx, duplicateToken, EventCheckIn); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("duplicate Record() error = %v, want ErrDuplicate", err)
	}
	if _, err := engine.Record(ctx, duplicateToken, EventCheckOut); err != nil {
		t.Fatalf("released duplicate receipt was not reusable for check-out: %v", err)
	}
	if got := eventCount(t, pool, employeeID, EventCheckIn); got != 1 {
		t.Fatalf("check-in count = %d, want 1", got)
	}

	concurrentEmployeeID := insertEmployee(t, pool, "EMP002", "ACTIVE")
	tokens := []string{
		issueReceipt(t, receipts, concurrentEmployeeID),
		issueReceipt(t, receipts, concurrentEmployeeID),
	}
	start := make(chan struct{})
	results := make(chan error, len(tokens))
	var ready sync.WaitGroup
	ready.Add(len(tokens))
	for _, token := range tokens {
		token := token
		go func() {
			ready.Done()
			<-start
			_, err := engine.Record(ctx, token, EventCheckIn)
			results <- err
		}()
	}
	ready.Wait()
	close(start)

	var succeeded, duplicated int
	for range tokens {
		switch err := <-results; {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrDuplicate):
			duplicated++
		default:
			t.Fatalf("concurrent Record() error = %v", err)
		}
	}
	if succeeded != 1 || duplicated != 1 {
		t.Fatalf("concurrent results = %d success, %d duplicate; want 1 each", succeeded, duplicated)
	}
	if got := eventCount(t, pool, concurrentEmployeeID, EventCheckIn); got != 1 {
		t.Fatalf("concurrent check-in count = %d, want 1", got)
	}
}

func TestPhnomPenhDayBounds(t *testing.T) {
	location, err := time.LoadLocation("Asia/Phnom_Penh")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 21, 18, 30, 0, 0, time.UTC)

	start, end := DayBounds(now, location)

	wantStart := time.Date(2026, 9, 21, 17, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2026, 9, 22, 17, 0, 0, 0, time.UTC)
	if !start.Equal(wantStart) || !end.Equal(wantEnd) {
		t.Fatalf("bounds = [%v, %v), want [%v, %v)", start, end, wantStart, wantEnd)
	}
}

func TestTodayUsesBusinessDayAndFactOnlyCounts(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	location, err := time.LoadLocation("Asia/Phnom_Penh")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 21, 18, 30, 0, 0, time.UTC)
	engine := NewEngine(pool, face.NewReceiptStore(time.Minute, time.Now), time.Minute, func() time.Time {
		return now
	})
	activeOne := insertEmployee(t, pool, "ACTIVE1", "ACTIVE")
	activeTwo := insertEmployee(t, pool, "ACTIVE2", "ACTIVE")
	inactive := insertEmployee(t, pool, "INACTIVE1", "INACTIVE")
	insertEvent(t, pool, activeOne, EventCheckIn, now.Add(-30*time.Minute))
	insertEvent(t, pool, activeOne, EventCheckOut, now.Add(-15*time.Minute))
	insertEvent(t, pool, inactive, EventCheckIn, now.Add(-10*time.Minute))
	insertEvent(t, pool, activeTwo, EventCheckIn, now.Add(-25*time.Hour))

	today, err := engine.Today(ctx, location)
	if err != nil {
		t.Fatalf("Today() error = %v", err)
	}
	if today.CheckedInToday != 1 || today.ActiveEmployees != 2 || today.EventsToday != 3 {
		t.Fatalf("today counts = %#v, want checked-in=1 active=2 events=3", today)
	}
	if len(today.Events) != 3 {
		t.Fatalf("event list length = %d, want 3", len(today.Events))
	}
	if today.Events[0].OccurredAt.Before(today.Events[1].OccurredAt) {
		t.Fatal("events are not newest first")
	}
}

func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	if dsn := os.Getenv("TEST_DATABASE_URL"); dsn != "" {
		pool, err := pgxpool.New(ctx, dsn)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(pool.Close)
		if err := pool.Ping(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx,
			`TRUNCATE attendance_events, face_templates, employees CASCADE`,
		); err != nil {
			t.Fatal(err)
		}
		return pool
	}
	migration, err := filepath.Abs(filepath.Join("..", "..", "migrations", "001_init.sql"))
	if err != nil {
		t.Fatal(err)
	}
	container, err := postgres.Run(
		ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("smartattend_test"),
		postgres.WithUsername("smartattend"),
		postgres.WithPassword("smartattend"),
		postgres.WithInitScripts(migration),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Skipf("Postgres container unavailable: %v", err)
	}
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(container) })
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	return pool
}

func insertEmployee(t *testing.T, pool *pgxpool.Pool, code, status string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO employees (
			employee_code, first_name, last_name, status, enrollment_status
		) VALUES ($1, 'Test', 'Employee', $2, 'ENROLLED')
		RETURNING id
	`, code, status).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func issueReceipt(t *testing.T, receipts *face.ReceiptStore, employeeID string) string {
	t.Helper()
	token, err := receipts.Issue(face.Receipt{
		EmployeeID:      employeeID,
		SimilarityScore: 0.91,
		MatchedPose:     face.PoseFront,
		ModelName:       "sface",
		ModelVersion:    "2021dec",
	})
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func eventCount(t *testing.T, pool *pgxpool.Pool, employeeID string, eventType EventType) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), `
		SELECT count(*) FROM attendance_events
		WHERE employee_id = $1 AND event_type = $2
	`, employeeID, eventType).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func insertEvent(
	t *testing.T,
	pool *pgxpool.Pool,
	employeeID string,
	eventType EventType,
	occurredAt time.Time,
) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO attendance_events (
			employee_id, event_type, verification_method, similarity_score,
			matched_pose, model_name, model_version, occurred_at
		) VALUES ($1, $2, 'FACE_1_TO_1', 0.91, 'FRONT', 'sface', '2021dec', $3)
	`, employeeID, eventType, occurredAt); err != nil {
		t.Fatal(err)
	}
}
