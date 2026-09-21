package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/smartattend/api/internal/attendance"
	"github.com/smartattend/api/internal/auth"
	"github.com/smartattend/api/internal/config"
	"github.com/smartattend/api/internal/employee"
	"github.com/smartattend/api/internal/face"
)

func NewRouter(cfg config.Config, employeeStores ...employeeStore) http.Handler {
	var store employeeStore = unavailableEmployeeStore{}
	if len(employeeStores) > 0 && employeeStores[0] != nil {
		store = employeeStores[0]
	}
	return newRouter(cfg, store, nil)
}

func NewRouterWithDatabase(
	cfg config.Config,
	store *employee.Store,
	pool *pgxpool.Pool,
) http.Handler {
	return newRouter(cfg, store, pool)
}

func newRouter(
	cfg config.Config,
	configuredEmployeeStore employeeStore,
	pool *pgxpool.Pool,
) http.Handler {
	sessionStore := auth.NewMemorySessionStore(10_000)
	authAPI := authHandlers{cfg: cfg, store: sessionStore}
	employeeStore := configuredEmployeeStore
	enrollmentStore := face.NewMemoryEnrollmentStore(10*time.Minute, time.Now)
	employeeAPI := employeeHandlers{
		store:       employeeStore,
		invalidator: enrollmentInvalidator{store: enrollmentStore},
	}
	var templateStore faceTemplateStore = unavailableFaceTemplateStore{}
	if configured, ok := employeeStore.(faceTemplateStore); ok {
		templateStore = configured
	}
	enrollAPI := enrollHandlers{
		employees:   employeeStore,
		templates:   templateStore,
		enrollments: enrollmentStore,
		faceClient:  face.NewClient(cfg.FaceServiceURL, 5*time.Second),
	}
	var verifyStore verifyCandidateStore = unavailableVerifyCandidateStore{}
	if configured, ok := employeeStore.(verifyCandidateStore); ok {
		verifyStore = configured
	}
	receipts := face.NewReceiptStore(time.Minute, time.Now)
	verifyAPI := verifyHandlers{
		store:    verifyStore,
		verifier: face.NewClient(cfg.FaceServiceURL, 5*time.Second),
		receipts: receipts,
		limiter:  newIPRateLimiter(20, time.Minute, time.Now),
	}
	var attendanceService attendanceService = unavailableAttendanceService{}
	if pool != nil {
		attendanceService = attendance.NewEngine(pool, receipts, time.Minute, time.Now)
	}
	location := cfg.BusinessTimezone
	if location == nil {
		location = time.UTC
	}
	attendanceAPI := attendanceHandlers{service: attendanceService, location: location}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthHandler)
	mux.HandleFunc("POST /api/v1/auth/login", authAPI.login)
	mux.Handle("GET /api/v1/auth/me", requireSession(sessionStore, http.HandlerFunc(authAPI.me)))
	mux.Handle("POST /api/v1/auth/logout", requireSession(sessionStore, http.HandlerFunc(authAPI.logout)))
	mux.Handle("GET /api/v1/employees", requireSession(sessionStore, http.HandlerFunc(employeeAPI.list)))
	mux.Handle("POST /api/v1/employees", requireSession(sessionStore, http.HandlerFunc(employeeAPI.create)))
	mux.Handle("GET /api/v1/employees/{id}", requireSession(sessionStore, http.HandlerFunc(employeeAPI.get)))
	mux.Handle("PATCH /api/v1/employees/{id}", requireSession(sessionStore, http.HandlerFunc(employeeAPI.patch)))
	mux.Handle("DELETE /api/v1/employees/{id}", requireSession(sessionStore, http.HandlerFunc(employeeAPI.deactivate)))
	mux.Handle("POST /api/v1/employees/{id}/face/enroll", requireSession(sessionStore, http.HandlerFunc(enrollAPI.start)))
	mux.Handle("POST /api/v1/employees/{id}/face/enroll/{enrollment_id}/{pose}", requireSession(sessionStore, http.HandlerFunc(enrollAPI.capture)))
	mux.Handle("POST /api/v1/employees/{id}/face/enroll/{enrollment_id}/commit", requireSession(sessionStore, http.HandlerFunc(enrollAPI.commit)))
	mux.Handle("POST /api/v1/employees/{id}/face/enroll/{enrollment_id}/abort", requireSession(sessionStore, http.HandlerFunc(enrollAPI.abort)))
	mux.Handle("DELETE /api/v1/employees/{id}/face", requireSession(sessionStore, http.HandlerFunc(enrollAPI.deleteFace)))
	mux.HandleFunc("POST /api/v1/face/verify", verifyAPI.verify)
	mux.HandleFunc("POST /api/v1/attendance/check-in", attendanceAPI.checkIn)
	mux.HandleFunc("POST /api/v1/attendance/check-out", attendanceAPI.checkOut)
	mux.Handle("GET /api/v1/attendance/today", requireSession(sessionStore, http.HandlerFunc(attendanceAPI.today)))
	return CORSMiddleware(cfg)(mux)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
