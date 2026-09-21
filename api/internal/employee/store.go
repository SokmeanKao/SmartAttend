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

type VerificationCandidate struct {
	Employee  Employee
	Templates []face.ReferenceTemplate
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

type EnrollmentLoader func() (*face.Enrollment, error)
type EnrollmentStarter func(map[face.Pose]uuid.UUID) (string, error)

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

func (s *Store) FindVerificationCandidate(
	ctx context.Context,
	employeeCode string,
) (VerificationCandidate, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT `+employeeColumns+`
		FROM employees
		WHERE employee_code = $1
	`, NormalizeEmployeeCode(employeeCode))
	found, err := scanEmployee(row)
	if err != nil {
		return VerificationCandidate{}, mapStoreError(err)
	}
	rows, err := s.pool.Query(ctx, `
		SELECT pose, embedding, embedding_dim, model_name, model_version, quality_score
		FROM face_templates
		WHERE employee_id = $1 AND revoked_at IS NULL
		ORDER BY pose
	`, found.ID)
	if err != nil {
		return VerificationCandidate{}, err
	}
	defer rows.Close()
	templates := make([]face.ReferenceTemplate, 0, 3)
	for rows.Next() {
		var template face.ReferenceTemplate
		if err := rows.Scan(
			&template.Pose,
			&template.Embedding,
			&template.EmbeddingDim,
			&template.ModelName,
			&template.ModelVersion,
			&template.QualityScore,
		); err != nil {
			return VerificationCandidate{}, err
		}
		templates = append(templates, template)
	}
	if err := rows.Err(); err != nil {
		return VerificationCandidate{}, err
	}
	return VerificationCandidate{Employee: found, Templates: templates}, nil
}

func (s *Store) Update(
	ctx context.Context,
	id string,
	params UpdateParams,
	invalidate func(),
) (Employee, error) {
	if params.Status == nil {
		return s.update(ctx, s.pool, id, params)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Employee{}, err
	}
	defer tx.Rollback(ctx)
	if err := lockEmployee(ctx, tx, id); err != nil {
		return Employee{}, err
	}
	updated, err := s.update(ctx, tx, id, params)
	if err != nil {
		return Employee{}, err
	}
	if updated.Status == StatusInactive && invalidate != nil {
		invalidate()
	}
	if err := tx.Commit(ctx); err != nil {
		return Employee{}, err
	}
	return updated, nil
}

type employeeQueryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (s *Store) update(
	ctx context.Context,
	db employeeQueryRower,
	id string,
	params UpdateParams,
) (Employee, error) {
	emailSet, email := nullableUpdate(params.Email)
	departmentSet, department := nullableUpdate(params.Department)
	positionSet, position := nullableUpdate(params.Position)

	row := db.QueryRow(ctx, `
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

func (s *Store) Deactivate(ctx context.Context, id string, invalidate func()) (Employee, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Employee{}, err
	}
	defer tx.Rollback(ctx)
	if err := lockEmployee(ctx, tx, id); err != nil {
		return Employee{}, err
	}
	row := tx.QueryRow(ctx, `
		UPDATE employees
		SET status = 'INACTIVE', updated_at = now()
		WHERE id = $1
		RETURNING `+employeeColumns,
		id,
	)
	employee, err := scanEmployee(row)
	if err != nil {
		return Employee{}, mapStoreError(err)
	}
	if invalidate != nil {
		invalidate()
	}
	if err := tx.Commit(ctx); err != nil {
		return Employee{}, err
	}
	return employee, nil
}

func (s *Store) StartEnrollment(
	ctx context.Context,
	employeeID string,
	start EnrollmentStarter,
) (string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	if err := lockEmployee(ctx, tx, employeeID); err != nil {
		return "", err
	}

	var status string
	if err := tx.QueryRow(ctx,
		`SELECT status FROM employees WHERE id = $1 FOR UPDATE`,
		employeeID,
	).Scan(&status); err != nil {
		return "", mapStoreError(err)
	}
	if status != StatusActive {
		return "", ErrInactive
	}

	rows, err := tx.Query(ctx, `
		SELECT pose, id
		FROM face_templates
		WHERE employee_id = $1 AND revoked_at IS NULL
	`, employeeID)
	if err != nil {
		return "", err
	}
	baseTemplateIDs := make(map[face.Pose]uuid.UUID)
	for rows.Next() {
		var pose face.Pose
		var id uuid.UUID
		if err := rows.Scan(&pose, &id); err != nil {
			rows.Close()
			return "", err
		}
		baseTemplateIDs[pose] = id
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return "", err
	}
	rows.Close()

	enrollmentID, err := start(baseTemplateIDs)
	if err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return enrollmentID, nil
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
	loadEnrollment EnrollmentLoader,
) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := lockEmployee(ctx, tx, employeeID); err != nil {
		return err
	}

	var status string
	if err := tx.QueryRow(ctx,
		`SELECT status FROM employees WHERE id = $1 FOR UPDATE`,
		employeeID,
	).Scan(&status); err != nil {
		return mapStoreError(err)
	}
	if status != StatusActive {
		return ErrInactive
	}

	enrollment, err := loadEnrollment()
	if err != nil {
		return err
	}
	if enrollment == nil || enrollment.EmployeeID.String() != employeeID {
		return face.ErrEnrollmentExpired
	}
	if err := enrollment.ValidateComplete(); err != nil {
		return err
	}
	if err := enrollment.ValidateModelSet(); err != nil {
		return err
	}

	rows, err := tx.Query(ctx, `
		SELECT pose, id
		FROM face_templates
		WHERE employee_id = $1 AND revoked_at IS NULL
	`, employeeID)
	if err != nil {
		return err
	}
	activeTemplateIDs := make(map[face.Pose]uuid.UUID)
	for rows.Next() {
		var pose face.Pose
		var id uuid.UUID
		if err := rows.Scan(&pose, &id); err != nil {
			rows.Close()
			return err
		}
		activeTemplateIDs[pose] = id
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if !sameTemplateIDs(activeTemplateIDs, enrollment.BaseTemplateIDs) {
		return face.ErrEnrollmentConflict
	}

	if _, err := tx.Exec(ctx, `
		UPDATE face_templates SET revoked_at = now()
		WHERE employee_id = $1 AND revoked_at IS NULL
	`, employeeID); err != nil {
		return err
	}
	for _, pose := range []face.Pose{face.PoseFront, face.PoseLeft, face.PoseRight} {
		tmpl := enrollment.Templates[pose]
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

func sameTemplateIDs(left, right map[face.Pose]uuid.UUID) bool {
	if len(left) != len(right) {
		return false
	}
	for pose, id := range left {
		if right[pose] != id {
			return false
		}
	}
	return true
}

func lockEmployee(ctx context.Context, tx pgx.Tx, employeeID string) error {
	_, err := tx.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1::text, 0))`,
		employeeID,
	)
	return err
}

func (s *Store) DeleteFace(ctx context.Context, employeeID string, invalidate func()) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := lockEmployee(ctx, tx, employeeID); err != nil {
		return err
	}
	if invalidate != nil {
		invalidate()
	}
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
