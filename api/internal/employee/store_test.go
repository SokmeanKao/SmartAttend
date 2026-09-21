package employee

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/smartattend/api/internal/face"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestStoreEmployeeLifecycle(t *testing.T) {
	store := newIntegrationStore(t)
	ctx := context.Background()

	created, err := store.Create(ctx, CreateParams{
		EmployeeCode: " emp001 ",
		FirstName:    "Sokmean",
		LastName:     "Chea",
		Email:        stringPointer("sokmean@example.com"),
		Department:   stringPointer("Operations"),
		Position:     stringPointer("Officer"),
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.EmployeeCode != "EMP001" {
		t.Errorf("employee code = %q, want EMP001", created.EmployeeCode)
	}
	if created.Status != StatusActive || created.EnrollmentStatus != EnrollmentNotEnrolled {
		t.Errorf("initial states = %q/%q, want ACTIVE/NOT_ENROLLED", created.Status, created.EnrollmentStatus)
	}

	if _, err := store.Create(ctx, CreateParams{
		EmployeeCode: "EMP001",
		FirstName:    "Duplicate",
		LastName:     "Employee",
	}); !errors.Is(err, ErrEmployeeCodeExists) {
		t.Fatalf("duplicate Create() error = %v, want ErrEmployeeCodeExists", err)
	}

	list, err := store.List(ctx, "sokmean")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("List() = %#v, want created employee", list)
	}

	firstName := "Sophea"
	status := StatusInactive
	updateInvalidated := false
	updated, err := store.Update(ctx, created.ID, UpdateParams{
		FirstName: &firstName,
		Status:    &status,
	}, func() {
		assertEmployeeAdvisoryLockHeld(t, store, created.ID)
		updateInvalidated = true
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.FirstName != firstName || updated.Status != StatusInactive {
		t.Errorf("updated employee = %#v", updated)
	}
	if !updateInvalidated {
		t.Error("Update(INACTIVE) did not invalidate enrollment sessions")
	}

	deactivateInvalidated := false
	deactivated, err := store.Deactivate(ctx, created.ID, func() {
		assertEmployeeAdvisoryLockHeld(t, store, created.ID)
		deactivateInvalidated = true
	})
	if err != nil {
		t.Fatalf("Deactivate() error = %v", err)
	}
	if deactivated.Status != StatusInactive {
		t.Errorf("status = %q, want INACTIVE", deactivated.Status)
	}
	if deactivated.EnrollmentStatus != EnrollmentNotEnrolled {
		t.Errorf("enrollment status changed to %q", deactivated.EnrollmentStatus)
	}
	if !deactivateInvalidated {
		t.Error("Deactivate() did not invalidate enrollment sessions")
	}

	got, err := store.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("Get().ID = %q, want %q", got.ID, created.ID)
	}
}

func TestStoreReturnsNotFound(t *testing.T) {
	store := newIntegrationStore(t)
	_, err := store.Get(context.Background(), "00000000-0000-0000-0000-000000000000")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() error = %v, want ErrNotFound", err)
	}
}

func TestConcurrentEnrollmentCommitsRejectStaleBaseTemplates(t *testing.T) {
	store := newIntegrationStore(t)
	ctx := context.Background()
	created := createTestEmployee(t, store, "CONCURRENT")
	base, err := store.ActiveTemplateIDs(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}

	results := make(chan error, 2)
	start := make(chan struct{})
	var ready sync.WaitGroup
	ready.Add(2)
	for _, marker := range []byte{1, 2} {
		marker := marker
		go func() {
			ready.Done()
			<-start
			results <- store.CommitEnrollment(
				ctx,
				created.ID,
				testEnrollmentLoader(created.ID, base, stagedTemplates(marker)),
			)
		}()
	}
	ready.Wait()
	close(start)

	var succeeded, conflicted int
	for range 2 {
		switch err := <-results; {
		case err == nil:
			succeeded++
		case errors.Is(err, face.ErrEnrollmentConflict):
			conflicted++
		default:
			t.Fatalf("CommitEnrollment() error = %v", err)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("commit results = %d success, %d conflict; want 1 each", succeeded, conflicted)
	}
	assertActiveTemplateSet(t, store, created.ID, 3)
}

func TestFailedEnrollmentCommitPreservesPriorEnrollment(t *testing.T) {
	store := newIntegrationStore(t)
	ctx := context.Background()
	created := createTestEmployee(t, store, "ROLLBACK")
	if err := store.CommitEnrollment(
		ctx,
		created.ID,
		testEnrollmentLoader(created.ID, nil, stagedTemplates(1)),
	); err != nil {
		t.Fatalf("initial CommitEnrollment() error = %v", err)
	}
	base, err := store.ActiveTemplateIDs(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}

	invalid := stagedTemplates(2)
	bad := invalid[face.PoseRight]
	bad.QualityScore = 2
	invalid[face.PoseRight] = bad
	if err := store.CommitEnrollment(
		ctx,
		created.ID,
		testEnrollmentLoader(created.ID, base, invalid),
	); err == nil {
		t.Fatal("CommitEnrollment() error = nil, want insert failure")
	}

	current, err := store.ActiveTemplateIDs(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !templateIDsEqual(current, base) {
		t.Fatalf("active template IDs changed after failed commit: got %v, want %v", current, base)
	}
	assertActiveTemplateSet(t, store, created.ID, 3)
}

func TestEnrollmentCommitRejectsInactiveEmployee(t *testing.T) {
	store := newIntegrationStore(t)
	ctx := context.Background()
	created := createTestEmployee(t, store, "INACTIVE")
	if _, err := store.Deactivate(ctx, created.ID, nil); err != nil {
		t.Fatal(err)
	}

	err := store.CommitEnrollment(
		ctx,
		created.ID,
		testEnrollmentLoader(created.ID, nil, stagedTemplates(1)),
	)
	if !errors.Is(err, ErrInactive) {
		t.Fatalf("CommitEnrollment() error = %v, want ErrInactive", err)
	}
	assertActiveTemplateSet(t, store, created.ID, 0)
}

func TestEnrollmentCommitWaitsForConcurrentDeactivate(t *testing.T) {
	store := newIntegrationStore(t)
	ctx := context.Background()
	created := createTestEmployee(t, store, "DEACTIVATE_RACE")

	blocker, err := store.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback(ctx)
	var lockedID string
	if err := blocker.QueryRow(ctx,
		`SELECT id FROM employees WHERE id = $1 FOR UPDATE`,
		created.ID,
	).Scan(&lockedID); err != nil {
		t.Fatal(err)
	}

	deactivateResult := make(chan error, 1)
	go func() {
		_, err := store.Deactivate(ctx, created.ID, nil)
		deactivateResult <- err
	}()
	waitForAdvisoryLock(t, store)

	commitResult := make(chan error, 1)
	go func() {
		commitResult <- store.CommitEnrollment(
			ctx,
			created.ID,
			testEnrollmentLoader(created.ID, nil, stagedTemplates(1)),
		)
	}()

	if err := blocker.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-deactivateResult; err != nil {
		t.Fatalf("Deactivate() error = %v", err)
	}
	if err := <-commitResult; !errors.Is(err, ErrInactive) {
		t.Fatalf("CommitEnrollment() error = %v, want ErrInactive", err)
	}

	found, err := store.Get(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if found.Status != StatusInactive || found.EnrollmentStatus != EnrollmentNotEnrolled {
		t.Fatalf("employee states = %s/%s, want INACTIVE/NOT_ENROLLED",
			found.Status, found.EnrollmentStatus)
	}
	assertActiveTemplateSet(t, store, created.ID, 0)
}

func TestEnrollmentCommitFailsAfterConcurrentFaceDeleteInvalidatesSession(t *testing.T) {
	store := newIntegrationStore(t)
	ctx := context.Background()
	created := createTestEmployee(t, store, "FACE_DELETE_RACE")
	employeeID := uuid.MustParse(created.ID)
	enrollments := face.NewMemoryEnrollmentStore(10*time.Minute, time.Now)
	enrollmentID, err := enrollments.Start(employeeID, nil)
	if err != nil {
		t.Fatal(err)
	}
	for pose, tmpl := range stagedTemplates(1) {
		if err := enrollments.Capture(enrollmentID, pose, tmpl); err != nil {
			t.Fatal(err)
		}
	}

	blocker, err := store.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback(ctx)
	var lockedID string
	if err := blocker.QueryRow(ctx,
		`SELECT id FROM employees WHERE id = $1 FOR UPDATE`,
		created.ID,
	).Scan(&lockedID); err != nil {
		t.Fatal(err)
	}

	deleteResult := make(chan error, 1)
	go func() {
		deleteResult <- store.DeleteFace(ctx, created.ID, func() {
			enrollments.InvalidateEmployee(employeeID)
		})
	}()
	waitForAdvisoryLock(t, store)

	commitResult := make(chan error, 1)
	go func() {
		commitResult <- store.CommitEnrollment(ctx, created.ID, func() (*face.Enrollment, error) {
			return enrollments.Get(enrollmentID)
		})
	}()

	if err := blocker.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-deleteResult; err != nil {
		t.Fatalf("DeleteFace() error = %v", err)
	}
	if err := <-commitResult; !errors.Is(err, face.ErrEnrollmentExpired) {
		t.Fatalf("CommitEnrollment() error = %v, want ErrEnrollmentExpired", err)
	}
	assertActiveTemplateSet(t, store, created.ID, 0)
}

func TestDeactivateInvalidatesBeforeReactivationCanCommit(t *testing.T) {
	store := newIntegrationStore(t)
	ctx := context.Background()
	created := createTestEmployee(t, store, "DEACTIVATE_INVALIDATE")
	employeeID := uuid.MustParse(created.ID)
	enrollments := face.NewMemoryEnrollmentStore(10*time.Minute, time.Now)
	enrollmentID, err := enrollments.Start(employeeID, nil)
	if err != nil {
		t.Fatal(err)
	}
	for pose, tmpl := range stagedTemplates(1) {
		if err := enrollments.Capture(enrollmentID, pose, tmpl); err != nil {
			t.Fatal(err)
		}
	}

	invalidating := make(chan struct{})
	releaseInvalidator := make(chan struct{})
	deactivateResult := make(chan error, 1)
	go func() {
		_, err := store.Deactivate(ctx, created.ID, func() {
			enrollments.InvalidateEmployee(employeeID)
			close(invalidating)
			<-releaseInvalidator
		})
		deactivateResult <- err
	}()
	<-invalidating

	active := StatusActive
	reactivateResult := make(chan error, 1)
	go func() {
		_, err := store.Update(ctx, created.ID, UpdateParams{Status: &active}, nil)
		reactivateResult <- err
	}()
	select {
	case err := <-reactivateResult:
		t.Fatalf("reactivation completed before invalidator released: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(releaseInvalidator)
	if err := <-deactivateResult; err != nil {
		t.Fatalf("Deactivate() error = %v", err)
	}
	if err := <-reactivateResult; err != nil {
		t.Fatalf("Update(ACTIVE) error = %v", err)
	}

	err = store.CommitEnrollment(ctx, created.ID, func() (*face.Enrollment, error) {
		return enrollments.Get(enrollmentID)
	})
	if !errors.Is(err, face.ErrEnrollmentExpired) {
		t.Fatalf("CommitEnrollment() error = %v, want ErrEnrollmentExpired", err)
	}
}

func TestEnrollmentStartWaitsForConcurrentDeactivate(t *testing.T) {
	store := newIntegrationStore(t)
	ctx := context.Background()
	created := createTestEmployee(t, store, "START_DEACTIVATE_RACE")
	employeeID := uuid.MustParse(created.ID)
	enrollments := face.NewMemoryEnrollmentStore(10*time.Minute, time.Now)

	blocker, err := store.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback(ctx)
	var lockedID string
	if err := blocker.QueryRow(ctx,
		`SELECT id FROM employees WHERE id = $1 FOR UPDATE`,
		created.ID,
	).Scan(&lockedID); err != nil {
		t.Fatal(err)
	}

	deactivateResult := make(chan error, 1)
	go func() {
		_, err := store.Deactivate(ctx, created.ID, func() {
			enrollments.InvalidateEmployee(employeeID)
		})
		deactivateResult <- err
	}()
	waitForAdvisoryLock(t, store)

	var startCalled bool
	startResult := make(chan error, 1)
	go func() {
		_, err := store.StartEnrollment(ctx, created.ID, func(base map[face.Pose]uuid.UUID) (string, error) {
			startCalled = true
			_, err := enrollments.Start(employeeID, base)
			return "", err
		})
		startResult <- err
	}()

	if err := blocker.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-deactivateResult; err != nil {
		t.Fatalf("Deactivate() error = %v", err)
	}
	if err := <-startResult; !errors.Is(err, ErrInactive) {
		t.Fatalf("StartEnrollment() error = %v, want ErrInactive", err)
	}
	if startCalled {
		t.Fatal("StartEnrollment callback ran for inactive employee")
	}
}

func TestIncompleteEnrollmentCannotCommit(t *testing.T) {
	store := newIntegrationStore(t)
	ctx := context.Background()
	created := createTestEmployee(t, store, "INCOMPLETE")
	templates := stagedTemplates(1)
	delete(templates, face.PoseRight)

	err := store.CommitEnrollment(
		ctx,
		created.ID,
		testEnrollmentLoader(created.ID, nil, templates),
	)
	if !errors.Is(err, face.ErrEnrollmentIncomplete) {
		t.Fatalf("CommitEnrollment() error = %v, want ErrEnrollmentIncomplete", err)
	}
	assertActiveTemplateSet(t, store, created.ID, 0)
}

func TestEnrollmentWithMixedStagedModelsCannotCommit(t *testing.T) {
	store := newIntegrationStore(t)
	ctx := context.Background()
	created := createTestEmployee(t, store, "MIXEDMODEL")
	templates := stagedTemplates(1)
	right := templates[face.PoseRight]
	right.ModelVersion = "next-model"
	templates[face.PoseRight] = right

	err := store.CommitEnrollment(
		ctx,
		created.ID,
		testEnrollmentLoader(created.ID, nil, templates),
	)
	if !errors.Is(err, face.ErrModelVersionMismatch) {
		t.Fatalf("CommitEnrollment() error = %v, want ErrModelVersionMismatch", err)
	}
	assertActiveTemplateSet(t, store, created.ID, 0)
}

func waitForAdvisoryLock(t *testing.T, store *Store) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var count int
		if err := store.pool.QueryRow(context.Background(), `
			SELECT count(*)
			FROM pg_locks
			WHERE locktype = 'advisory' AND granted
		`).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for employee advisory lock")
}

func assertEmployeeAdvisoryLockHeld(t *testing.T, store *Store, employeeID string) {
	t.Helper()
	var acquired bool
	if err := store.pool.QueryRow(context.Background(),
		`SELECT pg_try_advisory_xact_lock(hashtextextended($1::text, 0))`,
		employeeID,
	).Scan(&acquired); err != nil {
		t.Fatal(err)
	}
	if acquired {
		t.Fatal("enrollment invalidator ran without employee advisory lock")
	}
}

func newIntegrationStore(t *testing.T) *Store {
	t.Helper()
	ctx := context.Background()
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
	return NewStore(pool)
}

func createTestEmployee(t *testing.T, store *Store, code string) Employee {
	t.Helper()
	created, err := store.Create(context.Background(), CreateParams{
		EmployeeCode: code,
		FirstName:    "Test",
		LastName:     "Employee",
	})
	if err != nil {
		t.Fatal(err)
	}
	return created
}

func stagedTemplates(marker byte) map[face.Pose]face.StagedTemplate {
	result := make(map[face.Pose]face.StagedTemplate, 3)
	for _, pose := range []face.Pose{face.PoseFront, face.PoseLeft, face.PoseRight} {
		result[pose] = face.StagedTemplate{
			Embedding:    []byte{marker, byte(len(pose)), 0, 0},
			EmbeddingDim: 1,
			ModelName:    "sface",
			ModelVersion: "2021dec",
			QualityScore: 0.9,
		}
	}
	return result
}

func testEnrollmentLoader(
	employeeID string,
	baseTemplateIDs map[face.Pose]uuid.UUID,
	templates map[face.Pose]face.StagedTemplate,
) EnrollmentLoader {
	return func() (*face.Enrollment, error) {
		return &face.Enrollment{
			EmployeeID:      uuid.MustParse(employeeID),
			BaseTemplateIDs: baseTemplateIDs,
			Templates:       templates,
		}, nil
	}
}

func assertActiveTemplateSet(t *testing.T, store *Store, employeeID string, want int) {
	t.Helper()
	rows, err := store.pool.Query(context.Background(), `
		SELECT embedding
		FROM face_templates
		WHERE employee_id = $1 AND revoked_at IS NULL
	`, employeeID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var count int
	var marker byte
	for rows.Next() {
		var embedding []byte
		if err := rows.Scan(&embedding); err != nil {
			t.Fatal(err)
		}
		if len(embedding) == 0 {
			t.Fatal("active template has empty embedding")
		}
		if count == 0 {
			marker = embedding[0]
		} else if !bytes.Equal(embedding[:1], []byte{marker}) {
			t.Fatalf("active set mixes commit markers: first %d, got %d", marker, embedding[0])
		}
		count++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("active template count = %d, want %d", count, want)
	}
}

func templateIDsEqual(left, right map[face.Pose]uuid.UUID) bool {
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

func stringPointer(value string) *string {
	return &value
}
