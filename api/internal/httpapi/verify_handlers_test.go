package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/smartattend/api/internal/employee"
	"github.com/smartattend/api/internal/face"
)

type verifyStoreStub struct {
	candidate verificationCandidate
	err       error
}

func (s verifyStoreStub) FindVerificationCandidate(
	context.Context, string,
) (verificationCandidate, error) {
	return s.candidate, s.err
}

type faceVerifierStub struct {
	result face.VerifyResult
	err    error
}

func (s faceVerifierStub) Verify(
	context.Context, []byte, string, string, []face.ReferenceTemplate,
) (face.VerifyResult, error) {
	return s.result, s.err
}

func verifyRequest(t *testing.T, employeeCode string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("employee_code", employeeCode); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("image", "face.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("image")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/face/verify", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.RemoteAddr = "203.0.113.4:1234"
	return request
}

func newVerifyHandlerForTest(
	store verifyCandidateStore,
	verifier faceVerifier,
	receipts *face.ReceiptStore,
) verifyHandlers {
	return verifyHandlers{
		store:    store,
		verifier: verifier,
		receipts: receipts,
		limiter:  newIPRateLimiter(20, time.Minute, time.Now),
	}
}

func TestPublicVerifyNoMatchReturnsNoToken(t *testing.T) {
	receipts := face.NewReceiptStore(time.Minute, time.Now)
	handler := newVerifyHandlerForTest(
		verifyStoreStub{candidate: verificationCandidate{
			Employee:  employee.Employee{ID: "employee-1", Status: employee.StatusActive, EnrollmentStatus: employee.EnrollmentEnrolled},
			Templates: []face.ReferenceTemplate{{Pose: face.PoseFront}},
		}},
		faceVerifierStub{result: face.VerifyResult{Matched: false, BestScore: 0.2}},
		receipts,
	)
	response := httptest.NewRecorder()

	handler.verify(response, verifyRequest(t, " EMP001 "))

	if response.Code != http.StatusOK || response.Body.String() != "{\"matched\":false}\n" {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if bytes.Contains(response.Body.Bytes(), []byte("token")) {
		t.Fatal("no-match response exposed a token")
	}
}

func TestPublicVerifyUnavailableIsGeneric(t *testing.T) {
	handler := newVerifyHandlerForTest(
		verifyStoreStub{err: employee.ErrNotFound},
		faceVerifierStub{},
		face.NewReceiptStore(time.Minute, time.Now),
	)
	response := httptest.NewRecorder()

	handler.verify(response, verifyRequest(t, "UNKNOWN"))

	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d", response.Code)
	}
	var payload errorEnvelope
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Error.Code != "VERIFICATION_UNAVAILABLE" {
		t.Fatalf("error = %#v", payload.Error)
	}
}

func TestPublicVerifyStoreFailureIsInternalError(t *testing.T) {
	handler := newVerifyHandlerForTest(
		verifyStoreStub{err: errors.New("database unavailable")},
		faceVerifierStub{},
		face.NewReceiptStore(time.Minute, time.Now),
	)
	response := httptest.NewRecorder()

	handler.verify(response, verifyRequest(t, "EMP001"))

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", response.Code)
	}
}

func TestPublicVerifyPropagatesFaceServiceError(t *testing.T) {
	handler := newVerifyHandlerForTest(
		verifyStoreStub{candidate: verificationCandidate{
			Employee: employee.Employee{
				ID: "employee-1", Status: employee.StatusActive,
				EnrollmentStatus: employee.EnrollmentEnrolled,
			},
			Templates: []face.ReferenceTemplate{{Pose: face.PoseFront}},
		}},
		faceVerifierStub{err: &face.ServiceError{
			Code: "FACE_NOT_FOUND", Message: "no qualifying face was detected",
			StatusCode: http.StatusUnprocessableEntity,
		}},
		face.NewReceiptStore(time.Minute, time.Now),
	)
	response := httptest.NewRecorder()

	handler.verify(response, verifyRequest(t, "EMP001"))

	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body = %s", response.Code, response.Body.String())
	}
	var payload errorEnvelope
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Error.Code != "FACE_NOT_FOUND" ||
		payload.Error.Message != "no qualifying face was detected" {
		t.Fatalf("error = %#v", payload.Error)
	}
}

func TestPublicVerifyMatchIssuesReceiptWithServerMetadata(t *testing.T) {
	receipts := face.NewReceiptStore(time.Minute, time.Now)
	found := employee.Employee{
		ID: "employee-1", EmployeeCode: "EMP001", FirstName: "Sokmean", LastName: "Test",
		Status: employee.StatusActive, EnrollmentStatus: employee.EnrollmentEnrolled,
	}
	handler := newVerifyHandlerForTest(
		verifyStoreStub{candidate: verificationCandidate{
			Employee: found, Templates: []face.ReferenceTemplate{{Pose: face.PoseFront}},
		}},
		faceVerifierStub{result: face.VerifyResult{
			Matched: true, BestScore: 0.91, MatchedPose: face.PoseFront,
			ModelName: "sface", ModelVersion: "2021dec",
		}},
		receipts,
	)
	response := httptest.NewRecorder()

	handler.verify(response, verifyRequest(t, "EMP001"))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Matched           bool   `json:"matched"`
		VerificationToken string `json:"verification_token"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.Matched || payload.VerificationToken == "" {
		t.Fatalf("payload = %#v", payload)
	}
	receipt, err := receipts.Claim(payload.VerificationToken)
	if err != nil {
		t.Fatalf("Claim() error = %v", err)
	}
	if receipt.EmployeeID != found.ID || receipt.SimilarityScore != 0.91 {
		t.Fatalf("receipt = %#v", receipt)
	}
	if bytes.Contains(response.Body.Bytes(), []byte("embedding")) {
		t.Fatal("response exposed an embedding")
	}
}

func TestPublicVerifyRateLimitsByIP(t *testing.T) {
	handler := newVerifyHandlerForTest(
		verifyStoreStub{err: employee.ErrNotFound},
		faceVerifierStub{err: errors.New("must not matter")},
		face.NewReceiptStore(time.Minute, time.Now),
	)
	for range 20 {
		handler.verify(httptest.NewRecorder(), verifyRequest(t, "UNKNOWN"))
	}
	response := httptest.NewRecorder()

	handler.verify(response, verifyRequest(t, "UNKNOWN"))

	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", response.Code)
	}
}

func TestIPRateLimiterEvictsStaleWindows(t *testing.T) {
	now := time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)
	limiter := newIPRateLimiter(20, time.Minute, func() time.Time { return now })
	limiter.Allow("203.0.113.1")
	limiter.Allow("203.0.113.2")
	now = now.Add(time.Minute)

	limiter.Allow("203.0.113.3")

	if len(limiter.ips) != 1 {
		t.Fatalf("tracked IP windows = %d, want 1", len(limiter.ips))
	}
	if _, ok := limiter.ips["203.0.113.3"]; !ok {
		t.Fatal("current IP window was not retained")
	}
}
