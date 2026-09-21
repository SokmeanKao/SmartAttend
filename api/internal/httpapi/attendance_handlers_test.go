package httpapi

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/smartattend/api/internal/attendance"
	"github.com/smartattend/api/internal/face"
)

type attendanceServiceStub struct {
	event       attendance.Event
	todayResult attendance.TodaySummary
	err         error
	token       string
	eventType   attendance.EventType
}

func (s *attendanceServiceStub) Record(
	_ context.Context,
	token string,
	eventType attendance.EventType,
) (attendance.Event, error) {
	s.token = token
	s.eventType = eventType
	return s.event, s.err
}

func (s *attendanceServiceStub) Today(
	context.Context,
	*time.Location,
) (attendance.TodaySummary, error) {
	return s.todayResult, s.err
}

func TestAttendanceCreateAcceptsOnlyVerificationToken(t *testing.T) {
	service := &attendanceServiceStub{event: attendance.Event{
		ID:        "event-1",
		EventType: attendance.EventCheckIn,
	}}
	handler := attendanceHandlers{service: service, location: time.UTC}
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/attendance/check-in",
		bytes.NewBufferString(`{"verification_token":"receipt-1"}`),
	)
	response := httptest.NewRecorder()

	handler.checkIn(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", response.Code, response.Body.String())
	}
	if service.token != "receipt-1" || service.eventType != attendance.EventCheckIn {
		t.Fatalf("Record() args = %q/%q", service.token, service.eventType)
	}

	for name, body := range map[string]string{
		"missing token": `{"employee_id":"employee-1"}`,
		"extra field":   `{"verification_token":"receipt-1","occurred_at":"2026-09-22T00:00:00Z"}`,
		"trailing JSON": `{"verification_token":"receipt-1"} {}`,
	} {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.checkIn(recorder, httptest.NewRequest(
				http.MethodPost,
				"/api/v1/attendance/check-in",
				bytes.NewBufferString(body),
			))
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body = %s", recorder.Code, recorder.Body.String())
			}
			assertErrorCode(t, recorder, "VALIDATION_ERROR")
		})
	}
}

func TestAttendanceErrorsUsePublicContract(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"invalid receipt", face.ErrReceiptInvalid, http.StatusUnprocessableEntity, "VERIFICATION_TOKEN_INVALID"},
		{"duplicate", attendance.ErrDuplicate, http.StatusConflict, "DUPLICATE_ATTENDANCE"},
		{"inactive", attendance.ErrInactive, http.StatusConflict, "EMPLOYEE_INACTIVE"},
		{"internal", errors.New("database unavailable"), http.StatusInternalServerError, "INTERNAL_ERROR"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := attendanceHandlers{
				service:  &attendanceServiceStub{err: test.err},
				location: time.UTC,
			}
			response := httptest.NewRecorder()
			handler.checkOut(response, httptest.NewRequest(
				http.MethodPost,
				"/api/v1/attendance/check-out",
				bytes.NewBufferString(`{"verification_token":"receipt-1"}`),
			))
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d", response.Code, test.status)
			}
			assertErrorCode(t, response, test.code)
		})
	}
}

func TestAttendanceTodayReturnsFactSummary(t *testing.T) {
	service := &attendanceServiceStub{todayResult: attendance.TodaySummary{
		Date:            "2026-09-22",
		CheckedInToday:  4,
		ActiveEmployees: 7,
		EventsToday:     9,
		Events:          []attendance.Event{},
	}}
	handler := attendanceHandlers{service: service, location: time.UTC}
	response := httptest.NewRecorder()

	handler.today(response, httptest.NewRequest(
		http.MethodGet,
		"/api/v1/attendance/today",
		nil,
	))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body.String())
	}
	for _, expected := range [][]byte{
		[]byte(`"checked_in_today":4`),
		[]byte(`"active_employees":7`),
		[]byte(`"events_today":9`),
	} {
		if !bytes.Contains(response.Body.Bytes(), expected) {
			t.Fatalf("body %s does not contain %s", response.Body.String(), expected)
		}
	}
}
