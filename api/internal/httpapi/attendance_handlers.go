package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/smartattend/api/internal/attendance"
	"github.com/smartattend/api/internal/face"
)

type attendanceService interface {
	Record(context.Context, string, attendance.EventType) (attendance.Event, error)
	Today(context.Context, *time.Location) (attendance.TodaySummary, error)
}

type unavailableAttendanceService struct{}

func (unavailableAttendanceService) Record(
	context.Context,
	string,
	attendance.EventType,
) (attendance.Event, error) {
	return attendance.Event{}, errors.New("attendance service unavailable")
}

func (unavailableAttendanceService) Today(
	context.Context,
	*time.Location,
) (attendance.TodaySummary, error) {
	return attendance.TodaySummary{}, errors.New("attendance service unavailable")
}

type attendanceHandlers struct {
	service  attendanceService
	location *time.Location
}

func (h attendanceHandlers) checkIn(w http.ResponseWriter, r *http.Request) {
	h.record(w, r, attendance.EventCheckIn)
}

func (h attendanceHandlers) checkOut(w http.ResponseWriter, r *http.Request) {
	h.record(w, r, attendance.EventCheckOut)
}

func (h attendanceHandlers) record(
	w http.ResponseWriter,
	r *http.Request,
	eventType attendance.EventType,
) {
	var request struct {
		VerificationToken string `json:"verification_token"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeValidationError(w, "Body must contain only verification_token")
		return
	}
	request.VerificationToken = strings.TrimSpace(request.VerificationToken)
	if request.VerificationToken == "" {
		writeValidationError(w, "verification_token is required")
		return
	}

	event, err := h.service.Record(r.Context(), request.VerificationToken, eventType)
	if err != nil {
		writeAttendanceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, event)
}

func (h attendanceHandlers) today(w http.ResponseWriter, r *http.Request) {
	summary, err := h.service.Today(r.Context(), h.location)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func writeAttendanceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, face.ErrReceiptInvalid):
		WriteError(
			w,
			http.StatusUnprocessableEntity,
			"VERIFICATION_TOKEN_INVALID",
			"Verification token is invalid or expired",
		)
	case errors.Is(err, attendance.ErrDuplicate):
		WriteError(
			w,
			http.StatusConflict,
			"DUPLICATE_ATTENDANCE",
			"A recent attendance event of this type already exists",
		)
	case errors.Is(err, attendance.ErrInactive):
		WriteError(w, http.StatusConflict, "EMPLOYEE_INACTIVE", "Employee is inactive")
	default:
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
	}
}
