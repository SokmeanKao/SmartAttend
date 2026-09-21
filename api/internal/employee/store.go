package employee

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/smartattend/api/internal/face"
)

const (
	StatusActive          = "ACTIVE"
	StatusInactive        = "INACTIVE"
	EnrollmentNotEnrolled = "NOT_ENROLLED"
	EnrollmentEnrolled    = "ENROLLED"
)

var (
	ErrNotFound           = errors.New("employee not found")
	ErrEmployeeCodeExists = errors.New("employee code already exists")
	ErrInactive           = errors.New("employee inactive")
)

type Employee struct {
	ID               string    `json:"id"`
	EmployeeCode     string    `json:"employee_code"`
	FirstName        string    `json:"first_name"`
	LastName         string    `json:"last_name"`
	Email            *string   `json:"email"`
	Department       *string   `json:"department"`
	Position         *string   `json:"position"`
	Status           string    `json:"status"`
	EnrollmentStatus string    `json:"enrollment_status"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type CreateParams struct {
	EmployeeCode string
	FirstName    string
	LastName     string
	Email        *string
	Department   *string
	Position     *string
}

type UpdateParams struct {
	EmployeeCode *string
	FirstName    *string
	LastName     *string
	Email        **string
	Department   **string
	Position     **string
	Status       *string
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

const employeeColumns = `id, employee_code, first_name, last_name, email, department,
	position, status, enrollment_status, created_at, updated_at`

func (s *Store) Create(ctx context.Context, params CreateParams) (Employee, error) {
	row := s.pool.QueryRow(ctx, `
		INSERT INTO employees (
			employee_code, first_name, last_name, email, department, position,
			status, enrollment_status
		)
		VALUES ($1, $2, $3, $4, $5, $6, 'ACTIVE', 'NOT_ENROLLED')
		RETURNING `+employeeColumns,
		NormalizeEmployeeCode(params.EmployeeCode),
		params.FirstName,
		params.LastName,
		params.Email,
		params.Department,
		params.Position,
	)
	employee, err := scanEmployee(row)
	return employee, mapStoreError(err)
}

func (s *Store) List(ctx context.Context, query string) ([]Employee, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+employeeColumns+`
		FROM employees
		WHERE $1 = ''
		   OR employee_code ILIKE '%' || $1 || '%'
		   OR first_name ILIKE '%' || $1 || '%'
		   OR last_name ILIKE '%' || $1 || '%'
		   OR COALESCE(email, '') ILIKE '%' || $1 || '%'
		ORDER BY created_at DESC, id DESC
	`, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	employees := make([]Employee, 0)
	for rows.Next() {
		employee, err := scanEmployee(rows)
		if err != nil {
			return nil, err
		}
		employees = append(employees, employee)
	}
	return employees, rows.Err()
}

func (s *Store) Get(ctx context.Context, id string) (Employee, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT `+employeeColumns+`
		FROM employees
		WHERE id = $1
	`, id)
	employee, err := scanEmployee(row)
	return employee, mapStoreError(err)
}

func (s *Store) Update(ctx context.Context, id string, params UpdateParams) (Employee, error) {
	emailSet, email := nullableUpdate(params.Email)
	departmentSet, department := nullableUpdate(params.Department)
	positionSet, position := nullableUpdate(params.Position)

	row := s.pool.QueryRow(ctx, `
		UPDATE employees
		SET employee_code = CASE WHEN $2::boolean THEN $3::text ELSE employee_code END,
			first_name = CASE WHEN $4::boolean THEN $5::text ELSE first_name END,
			last_name = CASE WHEN $6::boolean THEN $7::text ELSE last_name END,
			email = CASE WHEN $8::boolean THEN $9::text ELSE email END,
			department = CASE WHEN $10::boolean THEN $11::text ELSE department END,
			position = CASE WHEN $12::boolean THEN $13::text ELSE position END,
			status = CASE WHEN $14::boolean THEN $15::text ELSE status END,
			updated_at = now()
		WHERE id = $1
		RETURNING `+employeeColumns,
		id,
		params.EmployeeCode != nil, normalizedValue(params.EmployeeCode),
		params.FirstName != nil, stringValue(params.FirstName),
		params.LastName != nil, stringValue(params.LastName),
		emailSet, email,
		departmentSet, department,
		positionSet, position,
		params.Status != nil, stringValue(params.Status),
	)
	employee, err := scanEmployee(row)
	return employee, mapStoreError(err)
}

func (s *Store) Deactivate(ctx context.Context, id string) (Employee, error) {
	row := s.pool.QueryRow(ctx, `
		UPDATE employees
		SET status = 'INACTIVE', updated_at = now()
		WHERE id = $1
		RETURNING `+employeeColumns,
		id,
	)
	employee, err := scanEmployee(row)
	return employee, mapStoreError(err)
}

func (s *Store) ActiveTemplateIDs(ctx context.Context, employeeID string) (map[face.Pose]uuid.UUID, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT pose, id
		FROM face_templates
		WHERE employee_id = $1 AND revoked_at IS NULL
	`, employeeID)
	if err != nil {
		return nil, mapStoreError(err)
	}
	defer rows.Close()

	templates := make(map[face.Pose]uuid.UUID)
	for rows.Next() {
		var pose face.Pose
		var id uuid.UUID
		if err := rows.Scan(&pose, &id); err != nil {
			return nil, err
		}
		templates[pose] = id
	}
	return templates, rows.Err()
}

func (s *Store) CommitEnrollment(
	ctx context.Context,
	employeeID string,
	templates map[face.Pose]face.StagedTemplate,
) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM employees WHERE id = $1`, employeeID).Scan(&status); err != nil {
		return mapStoreError(err)
	}
	if status != StatusActive {
		return ErrInactive
	}
	if _, err := tx.Exec(ctx, `
		UPDATE face_templates SET revoked_at = now()
		WHERE employee_id = $1 AND revoked_at IS NULL
	`, employeeID); err != nil {
		return err
	}
	for _, pose := range []face.Pose{face.PoseFront, face.PoseLeft, face.PoseRight} {
		tmpl, ok := templates[pose]
		if !ok {
			return face.ErrEnrollmentIncomplete
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO face_templates (
				employee_id, pose, embedding, embedding_dim,
				model_name, model_version, quality_score
			) VALUES ($1, $2, $3, $4, $5, $6, $7)
		`, employeeID, pose, tmpl.Embedding, tmpl.EmbeddingDim,
			tmpl.ModelName, tmpl.ModelVersion, tmpl.QualityScore); err != nil {
			return err
		}
	}
	result, err := tx.Exec(ctx, `
		UPDATE employees
		SET enrollment_status = 'ENROLLED', updated_at = now()
		WHERE id = $1
	`, employeeID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return tx.Commit(ctx)
}

func (s *Store) DeleteFace(ctx context.Context, employeeID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		UPDATE face_templates SET revoked_at = now()
		WHERE employee_id = $1 AND revoked_at IS NULL
	`, employeeID); err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `
		UPDATE employees
		SET enrollment_status = 'NOT_ENROLLED', updated_at = now()
		WHERE id = $1
	`, employeeID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return tx.Commit(ctx)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanEmployee(row rowScanner) (Employee, error) {
	var employee Employee
	err := row.Scan(
		&employee.ID,
		&employee.EmployeeCode,
		&employee.FirstName,
		&employee.LastName,
		&employee.Email,
		&employee.Department,
		&employee.Position,
		&employee.Status,
		&employee.EnrollmentStatus,
		&employee.CreatedAt,
		&employee.UpdatedAt,
	)
	return employee, err
}

func mapStoreError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgError *pgconn.PgError
	if errors.As(err, &pgError) {
		if pgError.Code == "23505" && pgError.ConstraintName == "employees_employee_code_key" {
			return ErrEmployeeCodeExists
		}
		if pgError.Code == "22P02" {
			return ErrNotFound
		}
	}
	return err
}

func nullableUpdate(value **string) (bool, *string) {
	if value == nil {
		return false, nil
	}
	return true, *value
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func normalizedValue(value *string) string {
	if value == nil {
		return ""
	}
	return NormalizeEmployeeCode(*value)
}
