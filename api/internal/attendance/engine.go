package attendance

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/smartattend/api/internal/employee"
	"github.com/smartattend/api/internal/face"
)

type EventType string

const (
	EventCheckIn  EventType = "CHECK_IN"
	EventCheckOut EventType = "CHECK_OUT"
)

var (
	ErrDuplicate = errors.New("duplicate attendance")
	ErrInactive  = errors.New("employee inactive")
)

type Event struct {
	ID                 string    `json:"id"`
	EmployeeID         string    `json:"employee_id"`
	EmployeeCode       string    `json:"employee_code,omitempty"`
	EmployeeFirstName  string    `json:"employee_first_name,omitempty"`
	EmployeeLastName   string    `json:"employee_last_name,omitempty"`
	EventType          EventType `json:"event_type"`
	VerificationMethod string    `json:"verification_method"`
	SimilarityScore    float32   `json:"similarity_score"`
	MatchedPose        face.Pose `json:"matched_pose,omitempty"`
	ModelName          string    `json:"model_name"`
	ModelVersion       string    `json:"model_version"`
	OccurredAt         time.Time `json:"occurred_at"`
	CreatedAt          time.Time `json:"created_at"`
}

type TodaySummary struct {
	Date            string  `json:"date"`
	CheckedInToday  int     `json:"checked_in_today"`
	ActiveEmployees int     `json:"active_employees"`
	EventsToday     int     `json:"events_today"`
	Events          []Event `json:"events"`
}

type receiptStore interface {
	Claim(string) (*face.Receipt, error)
	Release(string)
	Consume(string)
}

type Engine struct {
	pool     *pgxpool.Pool
	receipts receiptStore
	cooldown time.Duration
	now      func() time.Time
}

func NewEngine(
	pool *pgxpool.Pool,
	receipts receiptStore,
	cooldown time.Duration,
	now func() time.Time,
) *Engine {
	return &Engine{pool: pool, receipts: receipts, cooldown: cooldown, now: now}
}

func (e *Engine) Record(
	ctx context.Context,
	token string,
	eventType EventType,
) (Event, error) {
	receipt, err := e.receipts.Claim(token)
	if err != nil {
		return Event{}, err
	}
	release := true
	defer func() {
		if release {
			e.receipts.Release(token)
		}
	}()

	tx, err := e.pool.Begin(ctx)
	if err != nil {
		return Event{}, err
	}
	defer tx.Rollback(ctx)

	if err := requireActive(ctx, tx, receipt.EmployeeID); err != nil {
		return Event{}, err
	}
	if _, err := tx.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1::text, 0))`,
		receipt.EmployeeID,
	); err != nil {
		return Event{}, err
	}
	// Recheck after the employee-scoped lock so deactivation and attendance
	// have a single, deterministic ordering.
	if err := requireActive(ctx, tx, receipt.EmployeeID); err != nil {
		return Event{}, err
	}

	occurredAt := e.now().UTC()
	var duplicate bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM attendance_events
			WHERE employee_id = $1
			  AND event_type = $2
			  AND occurred_at >= $3
		)
	`, receipt.EmployeeID, eventType, occurredAt.Add(-e.cooldown)).Scan(&duplicate); err != nil {
		return Event{}, err
	}
	if duplicate {
		return Event{}, ErrDuplicate
	}

	// MVP-1 intentionally has no check-in/check-out sequence rules. Only the
	// same-employee, same-event-type cooldown is enforced.
	event := Event{
		EmployeeID:         receipt.EmployeeID,
		EventType:          eventType,
		VerificationMethod: "FACE_1_TO_1",
		SimilarityScore:    receipt.SimilarityScore,
		MatchedPose:        receipt.MatchedPose,
		ModelName:          receipt.ModelName,
		ModelVersion:       receipt.ModelVersion,
		OccurredAt:         occurredAt,
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO attendance_events (
			employee_id, event_type, verification_method, similarity_score,
			matched_pose, model_name, model_version, occurred_at
		) VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, $7, $8)
		RETURNING id, created_at
	`, event.EmployeeID, event.EventType, event.VerificationMethod,
		event.SimilarityScore, event.MatchedPose, event.ModelName,
		event.ModelVersion, event.OccurredAt,
	).Scan(&event.ID, &event.CreatedAt); err != nil {
		return Event{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Event{}, err
	}

	e.receipts.Consume(token)
	release = false
	return event, nil
}

func requireActive(ctx context.Context, tx pgx.Tx, employeeID string) error {
	var status string
	if err := tx.QueryRow(ctx,
		`SELECT status FROM employees WHERE id = $1`,
		employeeID,
	).Scan(&status); err != nil {
		return err
	}
	if status != employee.StatusActive {
		return ErrInactive
	}
	return nil
}

func DayBounds(now time.Time, location *time.Location) (time.Time, time.Time) {
	localNow := now.In(location)
	start := time.Date(
		localNow.Year(), localNow.Month(), localNow.Day(),
		0, 0, 0, 0, location,
	)
	return start.UTC(), start.AddDate(0, 0, 1).UTC()
}

func (e *Engine) Today(ctx context.Context, location *time.Location) (TodaySummary, error) {
	now := e.now()
	start, end := DayBounds(now, location)
	summary := TodaySummary{
		Date:   now.In(location).Format("2006-01-02"),
		Events: make([]Event, 0),
	}
	if err := e.pool.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM employees WHERE status = 'ACTIVE'),
			(SELECT count(DISTINCT ae.employee_id)
			 FROM attendance_events ae
			 JOIN employees e ON e.id = ae.employee_id
			 WHERE e.status = 'ACTIVE'
			   AND ae.event_type = 'CHECK_IN'
			   AND ae.occurred_at >= $1 AND ae.occurred_at < $2),
			(SELECT count(*) FROM attendance_events
			 WHERE occurred_at >= $1 AND occurred_at < $2)
	`, start, end).Scan(
		&summary.ActiveEmployees,
		&summary.CheckedInToday,
		&summary.EventsToday,
	); err != nil {
		return TodaySummary{}, err
	}

	rows, err := e.pool.Query(ctx, `
		SELECT ae.id, ae.employee_id, e.employee_code, e.first_name, e.last_name,
			ae.event_type, ae.verification_method, ae.similarity_score,
			COALESCE(ae.matched_pose, ''), ae.model_name, ae.model_version,
			ae.occurred_at, ae.created_at
		FROM attendance_events ae
		JOIN employees e ON e.id = ae.employee_id
		WHERE ae.occurred_at >= $1 AND ae.occurred_at < $2
		ORDER BY ae.occurred_at DESC, ae.id DESC
	`, start, end)
	if err != nil {
		return TodaySummary{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var event Event
		if err := rows.Scan(
			&event.ID,
			&event.EmployeeID,
			&event.EmployeeCode,
			&event.EmployeeFirstName,
			&event.EmployeeLastName,
			&event.EventType,
			&event.VerificationMethod,
			&event.SimilarityScore,
			&event.MatchedPose,
			&event.ModelName,
			&event.ModelVersion,
			&event.OccurredAt,
			&event.CreatedAt,
		); err != nil {
			return TodaySummary{}, err
		}
		summary.Events = append(summary.Events, event)
	}
	if err := rows.Err(); err != nil {
		return TodaySummary{}, err
	}
	return summary, nil
}
