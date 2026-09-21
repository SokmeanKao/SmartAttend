package httpapi

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"math"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/smartattend/api/internal/config"
	employeeapi "github.com/smartattend/api/internal/employee"
	faceapi "github.com/smartattend/api/internal/face"
)

func TestEnrollmentRoutesRequireAuthentication(t *testing.T) {
	cfg := authTestConfig()
	handler := NewRouter(cfg, &stubEmployeeStore{})
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/employees/00000000-0000-0000-0000-000000000001/face/enroll",
		nil,
	)
	req.Header.Set("Origin", cfg.WebOrigin)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestEnrollmentStartRejectsInactiveEmployee(t *testing.T) {
	inactive := sampleEmployee()
	inactive.Status = "INACTIVE"
	store := &stubEmployeeStore{employees: []employeeapi.Employee{inactive}}
	cfg := authTestConfig()
	handler := NewRouter(cfg, store)
	cookie := login(t, handler, cfg)

	rec := authenticatedRequest(t, handler, cfg, cookie, http.MethodPost,
		"/api/v1/employees/"+inactive.ID+"/face/enroll", "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
	assertErrorCode(t, rec, "EMPLOYEE_INACTIVE")
}

func TestEmployeePatchInactiveInvalidatesPendingEnrollment(t *testing.T) {
	store := &stubEmployeeStore{employees: []employeeapi.Employee{sampleEmployee()}}
	cfg := authTestConfig()
	handler := NewRouter(cfg, store)
	cookie := login(t, handler, cfg)
	employeeID := sampleEmployee().ID

	start := authenticatedRequest(t, handler, cfg, cookie, http.MethodPost,
		"/api/v1/employees/"+employeeID+"/face/enroll", "")
	var started struct {
		EnrollmentID string `json:"enrollment_id"`
	}
	if err := json.Unmarshal(start.Body.Bytes(), &started); err != nil {
		t.Fatal(err)
	}

	patched := authenticatedRequest(t, handler, cfg, cookie, http.MethodPatch,
		"/api/v1/employees/"+employeeID, `{"status":"INACTIVE"}`)
	if patched.Code != http.StatusOK {
		t.Fatalf("patch status = %d; body = %s", patched.Code, patched.Body.String())
	}

	capturePath := "/api/v1/employees/" + employeeID + "/face/enroll/" + started.EnrollmentID + "/FRONT"
	captured := enrollmentMultipartRequest(t, handler, cfg, cookie, capturePath)
	if captured.Code != http.StatusGone {
		t.Fatalf("capture status = %d, want 410; body = %s", captured.Code, captured.Body.String())
	}
	assertErrorCode(t, captured, "ENROLLMENT_EXPIRED")
}

func TestEnrollmentCaptureCommitAndDeleteFace(t *testing.T) {
	faceServer := newFaceStub(t)
	defer faceServer.Close()

	store := &stubEmployeeStore{employees: []employeeapi.Employee{sampleEmployee()}}
	cfg := authTestConfig()
	cfg.FaceServiceURL = faceServer.URL
	handler := NewRouter(cfg, store)
	cookie := login(t, handler, cfg)
	employeeID := sampleEmployee().ID

	start := authenticatedRequest(t, handler, cfg, cookie, http.MethodPost,
		"/api/v1/employees/"+employeeID+"/face/enroll", "")
	if start.Code != http.StatusCreated {
		t.Fatalf("start status = %d; body = %s", start.Code, start.Body.String())
	}
	var started struct {
		EnrollmentID string `json:"enrollment_id"`
		ExpiresIn    int    `json:"expires_in_seconds"`
	}
	if err := json.Unmarshal(start.Body.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	if started.EnrollmentID == "" || started.ExpiresIn != 600 {
		t.Fatalf("start response = %#v", started)
	}

	commitPath := "/api/v1/employees/" + employeeID + "/face/enroll/" + started.EnrollmentID + "/commit"
	incomplete := authenticatedRequest(t, handler, cfg, cookie, http.MethodPost, commitPath, "")
	if incomplete.Code != http.StatusConflict {
		t.Fatalf("incomplete status = %d; body = %s", incomplete.Code, incomplete.Body.String())
	}
	assertErrorCode(t, incomplete, "ENROLLMENT_INCOMPLETE")

	for _, pose := range []faceapi.Pose{faceapi.PoseFront, faceapi.PoseLeft, faceapi.PoseRight} {
		path := "/api/v1/employees/" + employeeID + "/face/enroll/" + started.EnrollmentID + "/" + string(pose)
		captured := enrollmentMultipartRequest(t, handler, cfg, cookie, path)
		if captured.Code != http.StatusOK {
			t.Fatalf("%s capture status = %d; body = %s", pose, captured.Code, captured.Body.String())
		}
		if strings.Contains(captured.Body.String(), "embedding") {
			t.Fatalf("capture response leaked embedding: %s", captured.Body.String())
		}
	}

	committed := authenticatedRequest(t, handler, cfg, cookie, http.MethodPost, commitPath, "")
	if committed.Code != http.StatusOK {
		t.Fatalf("commit status = %d; body = %s", committed.Code, committed.Body.String())
	}
	if len(store.committedTemplates) != 3 {
		t.Fatalf("committed templates = %d, want 3", len(store.committedTemplates))
	}

	deleted := authenticatedRequest(t, handler, cfg, cookie, http.MethodDelete,
		"/api/v1/employees/"+employeeID+"/face", "")
	if deleted.Code != http.StatusNoContent || store.deletedFaceID != employeeID {
		t.Fatalf("delete status/id = %d/%q", deleted.Code, store.deletedFaceID)
	}
}

func enrollmentMultipartRequest(
	t *testing.T,
	handler http.Handler,
	cfg config.Config,
	cookie *http.Cookie,
	path string,
) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("image", "face.jpg")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("jpeg"))
	_ = writer.Close()
	req := httptest.NewRequest(http.MethodPost, path, &body)
	req.AddCookie(cookie)
	req.Header.Set("Origin", cfg.WebOrigin)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func newFaceStub(t *testing.T) *httptest.Server {
	t.Helper()
	embedding := make([]byte, 128*4)
	binary.LittleEndian.PutUint32(embedding[:4], math.Float32bits(1))
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		pose := r.FormValue("expected_pose")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"quality_score":      0.91,
			"pose":               map[string]any{"label": pose, "yaw_score": 0},
			"embedding":          base64.StdEncoding.EncodeToString(embedding),
			"embedding_encoding": "float32-le-base64",
			"embedding_dim":      128,
			"model_name":         "sface",
			"model_version":      "2021dec",
		})
	}))
}
