package employee

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
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
	updated, err := store.Update(ctx, created.ID, UpdateParams{
		FirstName: &firstName,
		Status:    &status,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.FirstName != firstName || updated.Status != StatusInactive {
		t.Errorf("updated employee = %#v", updated)
	}

	deactivated, err := store.Deactivate(ctx, created.ID)
	if err != nil {
		t.Fatalf("Deactivate() error = %v", err)
	}
	if deactivated.Status != StatusInactive {
		t.Errorf("status = %q, want INACTIVE", deactivated.Status)
	}
	if deactivated.EnrollmentStatus != EnrollmentNotEnrolled {
		t.Errorf("enrollment status changed to %q", deactivated.EnrollmentStatus)
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

func stringPointer(value string) *string {
	return &value
}
