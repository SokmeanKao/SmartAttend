package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/smartattend/api/internal/employee"
)

type employeeStore interface {
	Create(context.Context, employee.CreateParams) (employee.Employee, error)
	List(context.Context, string) ([]employee.Employee, error)
	Get(context.Context, string) (employee.Employee, error)
	Update(context.Context, string, employee.UpdateParams) (employee.Employee, error)
	Deactivate(context.Context, string) (employee.Employee, error)
}

type employeeInvalidator interface {
	InvalidateEmployee(string)
}

type noOpEmployeeInvalidator struct{}

func (noOpEmployeeInvalidator) InvalidateEmployee(string) {}

type employeeHandlers struct {
	store       employeeStore
	invalidator employeeInvalidator
}

func (h employeeHandlers) list(w http.ResponseWriter, r *http.Request) {
	employees, err := h.store.List(r.Context(), strings.TrimSpace(r.URL.Query().Get("q")))
	if err != nil {
		writeEmployeeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, employees)
}

func (h employeeHandlers) create(w http.ResponseWriter, r *http.Request) {
	var request struct {
		EmployeeCode string  `json:"employee_code"`
		FirstName    string  `json:"first_name"`
		LastName     string  `json:"last_name"`
		Email        *string `json:"email"`
		Department   *string `json:"department"`
		Position     *string `json:"position"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeValidationError(w, "Invalid employee payload")
		return
	}
	request.EmployeeCode = employee.NormalizeEmployeeCode(request.EmployeeCode)
	request.FirstName = strings.TrimSpace(request.FirstName)
	request.LastName = strings.TrimSpace(request.LastName)
	if request.EmployeeCode == "" || request.FirstName == "" || request.LastName == "" {
		writeValidationError(w, "employee_code, first_name, and last_name are required")
		return
	}

	created, err := h.store.Create(r.Context(), employee.CreateParams{
		EmployeeCode: request.EmployeeCode,
		FirstName:    request.FirstName,
		LastName:     request.LastName,
		Email:        request.Email,
		Department:   request.Department,
		Position:     request.Position,
	})
	if err != nil {
		writeEmployeeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h employeeHandlers) get(w http.ResponseWriter, r *http.Request) {
	found, err := h.store.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeEmployeeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, found)
}

func (h employeeHandlers) patch(w http.ResponseWriter, r *http.Request) {
	body := http.MaxBytesReader(w, r.Body, 1<<20)
	payload, err := readAllJSON(body)
	if err != nil {
		writeValidationError(w, "Invalid employee payload")
		return
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		writeValidationError(w, "Invalid employee payload")
		return
	}
	if _, exists := fields["enrollment_status"]; exists {
		writeValidationError(w, "enrollment_status cannot be changed by employee CRUD")
		return
	}

	var request struct {
		EmployeeCode stringPatch         `json:"employee_code"`
		FirstName    stringPatch         `json:"first_name"`
		LastName     stringPatch         `json:"last_name"`
		Email        nullableStringPatch `json:"email"`
		Department   nullableStringPatch `json:"department"`
		Position     nullableStringPatch `json:"position"`
		Status       stringPatch         `json:"status"`
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || len(fields) == 0 {
		writeValidationError(w, "Invalid employee payload")
		return
	}

	params := employee.UpdateParams{
		EmployeeCode: request.EmployeeCode.pointer(),
		FirstName:    request.FirstName.pointer(),
		LastName:     request.LastName.pointer(),
		Email:        request.Email.pointer(),
		Department:   request.Department.pointer(),
		Position:     request.Position.pointer(),
		Status:       request.Status.pointer(),
	}
	if params.EmployeeCode != nil {
		normalized := employee.NormalizeEmployeeCode(*params.EmployeeCode)
		params.EmployeeCode = &normalized
	}
	if invalidRequiredPatch(params.EmployeeCode) ||
		invalidRequiredPatch(params.FirstName) ||
		invalidRequiredPatch(params.LastName) {
		writeValidationError(w, "employee_code, first_name, and last_name cannot be empty")
		return
	}
	if params.Status != nil && *params.Status != employee.StatusActive && *params.Status != employee.StatusInactive {
		writeValidationError(w, "status must be ACTIVE or INACTIVE")
		return
	}

	updated, err := h.store.Update(r.Context(), r.PathValue("id"), params)
	if err != nil {
		writeEmployeeError(w, err)
		return
	}
	if updated.Status == employee.StatusInactive {
		h.invalidator.InvalidateEmployee(updated.ID)
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h employeeHandlers) deactivate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	deactivated, err := h.store.Deactivate(r.Context(), id)
	if err != nil {
		writeEmployeeError(w, err)
		return
	}
	h.invalidator.InvalidateEmployee(id)
	writeJSON(w, http.StatusOK, deactivated)
}

type stringPatch struct {
	Set   bool
	Value string
}

func (p *stringPatch) UnmarshalJSON(data []byte) error {
	p.Set = true
	return json.Unmarshal(data, &p.Value)
}

func (p stringPatch) pointer() *string {
	if !p.Set {
		return nil
	}
	return &p.Value
}

type nullableStringPatch struct {
	Set   bool
	Value *string
}

func (p *nullableStringPatch) UnmarshalJSON(data []byte) error {
	p.Set = true
	if bytes.Equal(data, []byte("null")) {
		p.Value = nil
		return nil
	}
	return json.Unmarshal(data, &p.Value)
}

func (p nullableStringPatch) pointer() **string {
	if !p.Set {
		return nil
	}
	return &p.Value
}

func invalidRequiredPatch(value *string) bool {
	return value != nil && strings.TrimSpace(*value) == ""
}

func decodeJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("request body must contain one JSON value")
		}
		return err
	}
	return nil
}

func readAllJSON(body io.Reader) ([]byte, error) {
	var raw json.RawMessage
	decoder := json.NewDecoder(body)
	if err := decoder.Decode(&raw); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("request body must contain one JSON value")
		}
		return nil, err
	}
	return raw, nil
}

func writeEmployeeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, employee.ErrNotFound):
		WriteError(w, http.StatusNotFound, "EMPLOYEE_NOT_FOUND", "Employee not found")
	case errors.Is(err, employee.ErrEmployeeCodeExists):
		WriteError(w, http.StatusConflict, "EMPLOYEE_CODE_EXISTS", "Employee code already exists")
	default:
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
	}
}

func writeValidationError(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", message)
}

type unavailableEmployeeStore struct{}

func (unavailableEmployeeStore) Create(context.Context, employee.CreateParams) (employee.Employee, error) {
	return employee.Employee{}, errors.New("employee store unavailable")
}
func (unavailableEmployeeStore) List(context.Context, string) ([]employee.Employee, error) {
	return nil, errors.New("employee store unavailable")
}
func (unavailableEmployeeStore) Get(context.Context, string) (employee.Employee, error) {
	return employee.Employee{}, errors.New("employee store unavailable")
}
func (unavailableEmployeeStore) Update(context.Context, string, employee.UpdateParams) (employee.Employee, error) {
	return employee.Employee{}, errors.New("employee store unavailable")
}
func (unavailableEmployeeStore) Deactivate(context.Context, string) (employee.Employee, error) {
	return employee.Employee{}, errors.New("employee store unavailable")
}
