package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/smartattend/api/internal/config"
	employeeapi "github.com/smartattend/api/internal/employee"
	faceapi "github.com/smartattend/api/internal/face"
)

func TestEmployeeRoutesRequireAuthentication(t *testing.T) {
	handler := NewRouter(authTestConfig(), &stubEmployeeStore{})

	for _, route := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/employees"},
		{http.MethodPost, "/api/v1/employees"},
		{http.MethodGet, "/api/v1/employees/00000000-0000-0000-0000-000000000001"},
		{http.MethodPatch, "/api/v1/employees/00000000-0000-0000-0000-000000000001"},
		{http.MethodDelete, "/api/v1/employees/00000000-0000-0000-0000-000000000001"},
	} {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			req := httptest.NewRequest(route.method, route.path, nil)
			if route.method != http.MethodGet {
				req.Header.Set("Origin", authTestConfig().WebOrigin)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusUnauthorized, rec.Body.String())
			}
		})
	}
}

func TestEmployeeCRUDRoutes(t *testing.T) {
	existing := sampleEmployee()
	store := &stubEmployeeStore{employees: []employeeapi.Employee{existing}}
	cfg := authTestConfig()
	handler := NewRouter(cfg, store)
	cookie := login(t, handler, cfg)

	createRec := authenticatedRequest(t, handler, cfg, cookie, http.MethodPost, "/api/v1/employees",
		`{"employee_code":" emp002 ","first_name":"Dara","last_name":"Sok","email":null}`)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d; body = %s", createRec.Code, http.StatusCreated, createRec.Body.String())
	}
	if store.created.EmployeeCode != "EMP002" {
		t.Errorf("Create employee code = %q, want EMP002", store.created.EmployeeCode)
	}

	listRec := authenticatedRequest(t, handler, cfg, cookie, http.MethodGet, "/api/v1/employees?q=EMP001", "")
	if listRec.Code != http.StatusOK || store.listQuery != "EMP001" {
		t.Fatalf("list status/query = %d/%q; body = %s", listRec.Code, store.listQuery, listRec.Body.String())
	}

	getRec := authenticatedRequest(t, handler, cfg, cookie, http.MethodGet, "/api/v1/employees/"+existing.ID, "")
	if getRec.Code != http.StatusOK || store.gotID != existing.ID {
		t.Fatalf("get status/id = %d/%q; body = %s", getRec.Code, store.gotID, getRec.Body.String())
	}

	patchRec := authenticatedRequest(t, handler, cfg, cookie, http.MethodPatch, "/api/v1/employees/"+existing.ID,
		`{"first_name":"Sophea","status":"INACTIVE"}`)
	if patchRec.Code != http.StatusOK {
		t.Fatalf("patch status = %d, want %d; body = %s", patchRec.Code, http.StatusOK, patchRec.Body.String())
	}
	if store.updated.FirstName == nil || *store.updated.FirstName != "Sophea" {
		t.Errorf("Update params = %#v, want first_name Sophea", store.updated)
	}

	deleteRec := authenticatedRequest(t, handler, cfg, cookie, http.MethodDelete, "/api/v1/employees/"+existing.ID, "")
	if deleteRec.Code != http.StatusOK || store.deactivatedID != existing.ID {
		t.Fatalf("delete status/id = %d/%q; body = %s", deleteRec.Code, store.deactivatedID, deleteRec.Body.String())
	}
}

func TestCreateEmployeeMapsDuplicateCodeToConflict(t *testing.T) {
	store := &stubEmployeeStore{createErr: employeeapi.ErrEmployeeCodeExists}
	cfg := authTestConfig()
	handler := NewRouter(cfg, store)
	cookie := login(t, handler, cfg)

	rec := authenticatedRequest(t, handler, cfg, cookie, http.MethodPost, "/api/v1/employees",
		`{"employee_code":"EMP001","first_name":"Dara","last_name":"Sok"}`)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
	assertErrorCode(t, rec, "EMPLOYEE_CODE_EXISTS")
}

func TestPatchEmployeeRejectsEnrollmentStatus(t *testing.T) {
	store := &stubEmployeeStore{}
	cfg := authTestConfig()
	handler := NewRouter(cfg, store)
	cookie := login(t, handler, cfg)

	rec := authenticatedRequest(t, handler, cfg, cookie, http.MethodPatch,
		"/api/v1/employees/00000000-0000-0000-0000-000000000001",
		`{"enrollment_status":"ENROLLED"}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	assertErrorCode(t, rec, "VALIDATION_ERROR")
	if store.updateCalls != 0 {
		t.Fatalf("Update() calls = %d, want 0", store.updateCalls)
	}
}

func authenticatedRequest(
	t *testing.T,
	handler http.Handler,
	cfg config.Config,
	cookie *http.Cookie,
	method string,
	path string,
	body string,
) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.AddCookie(cookie)
	if method != http.MethodGet {
		req.Header.Set("Origin", cfg.WebOrigin)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

type stubEmployeeStore struct {
	employees          []employeeapi.Employee
	created            employeeapi.CreateParams
	createErr          error
	listQuery          string
	gotID              string
	updated            employeeapi.UpdateParams
	updateCalls        int
	deactivatedID      string
	committedTemplates map[faceapi.Pose]faceapi.StagedTemplate
	commitErr          error
	deletedFaceID      string
}

func (s *stubEmployeeStore) Create(_ context.Context, params employeeapi.CreateParams) (employeeapi.Employee, error) {
	s.created = params
	if s.createErr != nil {
		return employeeapi.Employee{}, s.createErr
	}
	result := sampleEmployee()
	result.EmployeeCode = params.EmployeeCode
	result.FirstName = params.FirstName
	result.LastName = params.LastName
	return result, nil
}

func (s *stubEmployeeStore) List(_ context.Context, query string) ([]employeeapi.Employee, error) {
	s.listQuery = query
	return s.employees, nil
}

func (s *stubEmployeeStore) Get(_ context.Context, id string) (employeeapi.Employee, error) {
	s.gotID = id
	if len(s.employees) == 0 {
		return employeeapi.Employee{}, employeeapi.ErrNotFound
	}
	return s.employees[0], nil
}

func (s *stubEmployeeStore) Update(_ context.Context, _ string, params employeeapi.UpdateParams) (employeeapi.Employee, error) {
	s.updateCalls++
	s.updated = params
	result := sampleEmployee()
	if params.Status != nil {
		result.Status = *params.Status
	}
	return result, nil
}

func (s *stubEmployeeStore) Deactivate(_ context.Context, id string) (employeeapi.Employee, error) {
	s.deactivatedID = id
	result := sampleEmployee()
	result.Status = employeeapi.StatusInactive
	return result, nil
}

func (s *stubEmployeeStore) ActiveTemplateIDs(
	_ context.Context,
	_ string,
) (map[faceapi.Pose]uuid.UUID, error) {
	return map[faceapi.Pose]uuid.UUID{}, nil
}

func (s *stubEmployeeStore) CommitEnrollment(
	_ context.Context,
	_ string,
	loadEnrollment employeeapi.EnrollmentLoader,
) error {
	enrollment, err := loadEnrollment()
	if err != nil {
		return err
	}
	if err := enrollment.ValidateComplete(); err != nil {
		return err
	}
	s.committedTemplates = enrollment.Templates
	return s.commitErr
}

func (s *stubEmployeeStore) DeleteFace(_ context.Context, id string, invalidate func()) error {
	invalidate()
	s.deletedFaceID = id
	return nil
}

func sampleEmployee() employeeapi.Employee {
	return employeeapi.Employee{
		ID:               "00000000-0000-0000-0000-000000000001",
		EmployeeCode:     "EMP001",
		FirstName:        "Sokmean",
		LastName:         "Chea",
		Status:           employeeapi.StatusActive,
		EnrollmentStatus: employeeapi.EnrollmentNotEnrolled,
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
	}
}
