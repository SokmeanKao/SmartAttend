package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/smartattend/api/internal/employee"
	"github.com/smartattend/api/internal/face"
)

const maxEnrollmentImageBytes = 5 << 20

type faceTemplateStore interface {
	StartEnrollment(context.Context, string, employee.EnrollmentStarter) (string, error)
	CommitEnrollment(
		context.Context,
		string,
		employee.EnrollmentLoader,
	) error
	DeleteFace(context.Context, string, func()) error
}

type faceEmbedder interface {
	Embed(context.Context, []byte, string, string, face.Pose) (face.EmbedResult, error)
}

type enrollHandlers struct {
	employees   employeeStore
	templates   faceTemplateStore
	enrollments face.EnrollmentStore
	faceClient  faceEmbedder
}

func (h enrollHandlers) start(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	employeeID, err := uuid.Parse(id)
	if err != nil {
		writeEmployeeError(w, employee.ErrNotFound)
		return
	}
	enrollmentID, err := h.templates.StartEnrollment(
		r.Context(),
		id,
		func(baseTemplates map[face.Pose]uuid.UUID) (string, error) {
			return h.enrollments.Start(employeeID, baseTemplates)
		},
	)
	if err != nil {
		writeEnrollmentError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"enrollment_id":      enrollmentID,
		"required_poses":     []face.Pose{face.PoseFront, face.PoseLeft, face.PoseRight},
		"expires_in_seconds": 600,
	})
}

func (h enrollHandlers) capture(w http.ResponseWriter, r *http.Request) {
	enrollment, employeeID, ok := h.enrollmentForRequest(w, r)
	if !ok {
		return
	}
	pose := face.Pose(strings.ToUpper(r.PathValue("pose")))
	if !pose.Valid() {
		writeEnrollmentError(w, face.ErrInvalidPose)
		return
	}
	if enrollment.EmployeeID != employeeID {
		WriteError(w, http.StatusGone, "ENROLLMENT_EXPIRED", "Enrollment expired")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxEnrollmentImageBytes+(1<<20))
	if err := r.ParseMultipartForm(maxEnrollmentImageBytes); err != nil {
		WriteError(w, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE", "Image is too large")
		return
	}
	file, header, err := r.FormFile("image")
	if err != nil {
		writeValidationError(w, "image is required")
		return
	}
	defer file.Close()
	image, err := io.ReadAll(io.LimitReader(file, maxEnrollmentImageBytes+1))
	if err != nil {
		writeValidationError(w, "Unable to read image")
		return
	}
	if len(image) == 0 {
		writeValidationError(w, "image is required")
		return
	}
	if len(image) > maxEnrollmentImageBytes {
		WriteError(w, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE", "Image is too large")
		return
	}

	result, err := h.faceClient.Embed(
		r.Context(), image, header.Filename, header.Header.Get("Content-Type"), pose,
	)
	if err != nil {
		writeEnrollmentError(w, err)
		return
	}
	if result.Pose.Label != pose {
		writeEnrollmentError(w, face.ErrInvalidPose)
		return
	}
	if err := h.enrollments.Capture(enrollment.ID, pose, result.Template); err != nil {
		writeEnrollmentError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"pose":          pose,
		"accepted":      true,
		"quality_score": result.QualityScore,
	})
}

func (h enrollHandlers) commit(w http.ResponseWriter, r *http.Request) {
	employeeID, _, ok := h.activeEmployee(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	enrollmentID := r.PathValue("enrollment_id")
	if err := h.templates.CommitEnrollment(
		r.Context(),
		r.PathValue("id"),
		func() (*face.Enrollment, error) {
			enrollment, err := h.enrollments.Get(enrollmentID)
			if err != nil {
				return nil, err
			}
			if enrollment.EmployeeID != employeeID {
				return nil, face.ErrEnrollmentExpired
			}
			return enrollment, nil
		},
	); err != nil {
		writeEnrollmentError(w, err)
		return
	}
	h.enrollments.Abort(enrollmentID)
	writeJSON(w, http.StatusOK, map[string]any{"enrollment_status": employee.EnrollmentEnrolled})
}

func (h enrollHandlers) abort(w http.ResponseWriter, r *http.Request) {
	enrollment, _, ok := h.enrollmentForRequest(w, r)
	if !ok {
		return
	}
	h.enrollments.Abort(enrollment.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (h enrollHandlers) deleteFace(w http.ResponseWriter, r *http.Request) {
	found, err := h.employees.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeEmployeeError(w, err)
		return
	}
	employeeID, parseErr := uuid.Parse(found.ID)
	if err := h.templates.DeleteFace(r.Context(), found.ID, func() {
		if parseErr == nil {
			h.enrollments.InvalidateEmployee(employeeID)
		}
	}); err != nil {
		writeEnrollmentError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h enrollHandlers) activeEmployee(
	w http.ResponseWriter,
	r *http.Request,
	id string,
) (uuid.UUID, employee.Employee, bool) {
	employeeID, err := uuid.Parse(id)
	if err != nil {
		writeEmployeeError(w, employee.ErrNotFound)
		return uuid.Nil, employee.Employee{}, false
	}
	found, err := h.employees.Get(r.Context(), id)
	if err != nil {
		writeEmployeeError(w, err)
		return uuid.Nil, employee.Employee{}, false
	}
	if found.Status != employee.StatusActive {
		WriteError(w, http.StatusConflict, "EMPLOYEE_INACTIVE", "Employee is inactive")
		return uuid.Nil, employee.Employee{}, false
	}
	return employeeID, found, true
}

func (h enrollHandlers) enrollmentForRequest(
	w http.ResponseWriter,
	r *http.Request,
) (*face.Enrollment, uuid.UUID, bool) {
	employeeID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeEmployeeError(w, employee.ErrNotFound)
		return nil, uuid.Nil, false
	}
	enrollment, err := h.enrollments.Get(r.PathValue("enrollment_id"))
	if err != nil {
		writeEnrollmentError(w, err)
		return nil, uuid.Nil, false
	}
	if enrollment.EmployeeID != employeeID {
		writeEnrollmentError(w, face.ErrEnrollmentExpired)
		return nil, uuid.Nil, false
	}
	return enrollment, employeeID, true
}

func writeEnrollmentError(w http.ResponseWriter, err error) {
	if writeFaceServiceError(w, err) {
		return
	}
	switch {
	case errors.Is(err, employee.ErrNotFound):
		writeEmployeeError(w, err)
	case errors.Is(err, employee.ErrInactive):
		WriteError(w, http.StatusConflict, "EMPLOYEE_INACTIVE", "Employee is inactive")
	case errors.Is(err, face.ErrEnrollmentIncomplete):
		WriteError(w, http.StatusConflict, "ENROLLMENT_INCOMPLETE", "All required poses must be captured")
	case errors.Is(err, face.ErrEnrollmentConflict):
		WriteError(w, http.StatusConflict, "ENROLLMENT_CONFLICT", "Enrollment changed; start again")
	case errors.Is(err, face.ErrEnrollmentExpired):
		WriteError(w, http.StatusGone, "ENROLLMENT_EXPIRED", "Enrollment expired")
	case errors.Is(err, face.ErrInvalidPose):
		WriteError(w, http.StatusUnprocessableEntity, "INVALID_POSE", "Captured face does not match the expected pose")
	case errors.Is(err, face.ErrModelVersionMismatch):
		WriteError(w, http.StatusUnprocessableEntity, "MODEL_VERSION_MISMATCH", "Captured templates use incompatible face models")
	case errors.Is(err, face.ErrValidation):
		WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Captured templates are invalid")
	case errors.Is(err, face.ErrFaceServiceTimeout):
		WriteError(w, http.StatusGatewayTimeout, "FACE_SERVICE_TIMEOUT", "Face service timed out")
	case errors.Is(err, face.ErrFaceServiceUnavailable):
		WriteError(w, http.StatusServiceUnavailable, "FACE_SERVICE_UNAVAILABLE", "Face service unavailable")
	default:
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
	}
}

func writeFaceServiceError(w http.ResponseWriter, err error) bool {
	var serviceErr *face.ServiceError
	if !errors.As(err, &serviceErr) {
		return false
	}
	switch serviceErr.Code {
	case "VALIDATION_ERROR":
		WriteError(w, http.StatusBadRequest, serviceErr.Code, serviceErr.Message)
	case "REQUEST_TOO_LARGE":
		WriteError(w, http.StatusRequestEntityTooLarge, serviceErr.Code, serviceErr.Message)
	case "FACE_NOT_FOUND", "MULTIPLE_FACES", "FACE_TOO_SMALL", "FACE_TOO_BLURRY",
		"FACE_QUALITY_TOO_LOW", "INVALID_POSE", "MODEL_VERSION_MISMATCH",
		"INVALID_REFERENCE_TEMPLATES":
		WriteError(w, http.StatusUnprocessableEntity, serviceErr.Code, serviceErr.Message)
	default:
		return false
	}
	return true
}

type unavailableFaceTemplateStore struct{}

func (unavailableFaceTemplateStore) StartEnrollment(
	context.Context,
	string,
	employee.EnrollmentStarter,
) (string, error) {
	return "", errors.New("face template store unavailable")
}
func (unavailableFaceTemplateStore) CommitEnrollment(
	context.Context,
	string,
	employee.EnrollmentLoader,
) error {
	return errors.New("face template store unavailable")
}
func (unavailableFaceTemplateStore) DeleteFace(context.Context, string, func()) error {
	return errors.New("face template store unavailable")
}

type enrollmentInvalidator struct {
	store face.EnrollmentStore
}

func (i enrollmentInvalidator) InvalidateEmployee(id string) {
	employeeID, err := uuid.Parse(id)
	if err == nil {
		i.store.InvalidateEmployee(employeeID)
	}
}
