package httpapi

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/smartattend/api/internal/employee"
	"github.com/smartattend/api/internal/face"
)

const maxVerificationImageBytes = 5 << 20

type verificationCandidate = employee.VerificationCandidate

type verifyCandidateStore interface {
	FindVerificationCandidate(context.Context, string) (verificationCandidate, error)
}

type unavailableVerifyCandidateStore struct{}

func (unavailableVerifyCandidateStore) FindVerificationCandidate(
	context.Context, string,
) (verificationCandidate, error) {
	return verificationCandidate{}, errors.New("verification store unavailable")
}

type faceVerifier interface {
	Verify(
		context.Context,
		[]byte,
		string,
		string,
		[]face.ReferenceTemplate,
	) (face.VerifyResult, error)
}

type receiptIssuer interface {
	Issue(face.Receipt) (string, error)
}

type verifyHandlers struct {
	store    verifyCandidateStore
	verifier faceVerifier
	receipts receiptIssuer
	limiter  *ipRateLimiter
}

func (h verifyHandlers) verify(w http.ResponseWriter, r *http.Request) {
	if !h.limiter.Allow(clientIP(r)) {
		WriteError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Too many verification attempts")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxVerificationImageBytes+(1<<20))
	if err := r.ParseMultipartForm(maxVerificationImageBytes); err != nil {
		WriteError(w, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE", "Image is too large")
		return
	}
	code := employee.NormalizeEmployeeCode(r.FormValue("employee_code"))
	if code == "" {
		writeValidationError(w, "employee_code is required")
		return
	}
	file, header, err := r.FormFile("image")
	if err != nil {
		writeValidationError(w, "image is required")
		return
	}
	defer file.Close()
	image, err := io.ReadAll(io.LimitReader(file, maxVerificationImageBytes+1))
	if err != nil || len(image) == 0 {
		writeValidationError(w, "image is required")
		return
	}
	if len(image) > maxVerificationImageBytes {
		WriteError(w, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE", "Image is too large")
		return
	}

	candidate, err := h.store.FindVerificationCandidate(r.Context(), code)
	if err != nil {
		if errors.Is(err, employee.ErrNotFound) {
			WriteError(w, http.StatusConflict, "VERIFICATION_UNAVAILABLE", "Verification is unavailable")
		} else {
			WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
		}
		return
	}
	if candidate.Employee.Status != employee.StatusActive ||
		candidate.Employee.EnrollmentStatus != employee.EnrollmentEnrolled ||
		len(candidate.Templates) == 0 {
		WriteError(w, http.StatusConflict, "VERIFICATION_UNAVAILABLE", "Verification is unavailable")
		return
	}
	result, err := h.verifier.Verify(
		r.Context(), image, header.Filename, header.Header.Get("Content-Type"), candidate.Templates,
	)
	if err != nil {
		switch {
		case writeFaceServiceError(w, err):
		case errors.Is(err, face.ErrFaceServiceTimeout):
			WriteError(w, http.StatusGatewayTimeout, "FACE_SERVICE_TIMEOUT", "Face service timed out")
		case errors.Is(err, face.ErrFaceServiceUnavailable):
			WriteError(w, http.StatusServiceUnavailable, "FACE_SERVICE_UNAVAILABLE", "Face service unavailable")
		default:
			WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
		}
		return
	}
	if !result.Matched {
		writeJSON(w, http.StatusOK, map[string]bool{"matched": false})
		return
	}
	token, err := h.receipts.Issue(face.Receipt{
		EmployeeID:      candidate.Employee.ID,
		SimilarityScore: result.BestScore,
		MatchedPose:     result.MatchedPose,
		ModelName:       result.ModelName,
		ModelVersion:    result.ModelVersion,
	})
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"matched": true,
		"employee": map[string]string{
			"id":            candidate.Employee.ID,
			"employee_code": candidate.Employee.EmployeeCode,
			"first_name":    candidate.Employee.FirstName,
			"last_name":     candidate.Employee.LastName,
		},
		"verification_token": token,
		"expires_in_seconds": 60,
	})
}

type rateWindow struct {
	start time.Time
	count int
}

type ipRateLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	now    func() time.Time
	ips    map[string]rateWindow
}

func newIPRateLimiter(limit int, window time.Duration, now func() time.Time) *ipRateLimiter {
	return &ipRateLimiter{
		limit: limit, window: window, now: now, ips: make(map[string]rateWindow),
	}
}

func (l *ipRateLimiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	for trackedIP, trackedWindow := range l.ips {
		if !now.Before(trackedWindow.start.Add(l.window)) {
			delete(l.ips, trackedIP)
		}
	}
	entry := l.ips[ip]
	if entry.start.IsZero() || !now.Before(entry.start.Add(l.window)) {
		l.ips[ip] = rateWindow{start: now, count: 1}
		return true
	}
	if entry.count >= l.limit {
		return false
	}
	entry.count++
	l.ips[ip] = entry
	return true
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}
