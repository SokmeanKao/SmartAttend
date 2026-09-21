package face

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Pose string

const (
	PoseFront Pose = "FRONT"
	PoseLeft  Pose = "LEFT"
	PoseRight Pose = "RIGHT"
)

var (
	ErrEnrollmentExpired    = errors.New("enrollment expired")
	ErrEnrollmentIncomplete = errors.New("enrollment incomplete")
	ErrInvalidPose          = errors.New("invalid pose")
)

type StagedTemplate struct {
	Embedding    []byte
	EmbeddingDim int
	ModelName    string
	ModelVersion string
	QualityScore float32
}

type Enrollment struct {
	ID              string
	EmployeeID      uuid.UUID
	BaseTemplateIDs map[Pose]uuid.UUID
	Templates       map[Pose]StagedTemplate
	ExpiresAt       time.Time
}

func (e *Enrollment) ValidateComplete() error {
	for _, pose := range []Pose{PoseFront, PoseLeft, PoseRight} {
		if _, ok := e.Templates[pose]; !ok {
			return ErrEnrollmentIncomplete
		}
	}
	return nil
}

type EnrollmentStore interface {
	Start(employeeID uuid.UUID, baseTemplateIDs map[Pose]uuid.UUID) (enrollmentID string, err error)
	Capture(enrollmentID string, pose Pose, tmpl StagedTemplate) error
	Get(enrollmentID string) (*Enrollment, error)
	Abort(enrollmentID string)
	InvalidateEmployee(employeeID uuid.UUID)
}

type MemoryEnrollmentStore struct {
	mu          sync.Mutex
	enrollments map[string]*Enrollment
	ttl         time.Duration
	now         func() time.Time
}

func NewMemoryEnrollmentStore(ttl time.Duration, now func() time.Time) *MemoryEnrollmentStore {
	return &MemoryEnrollmentStore{
		enrollments: make(map[string]*Enrollment),
		ttl:         ttl,
		now:         now,
	}
}

func (s *MemoryEnrollmentStore) Start(
	employeeID uuid.UUID,
	baseTemplateIDs map[Pose]uuid.UUID,
) (string, error) {
	enrollmentID, err := randomEnrollmentID()
	if err != nil {
		return "", err
	}
	enrollment := &Enrollment{
		ID:              enrollmentID,
		EmployeeID:      employeeID,
		BaseTemplateIDs: cloneTemplateIDs(baseTemplateIDs),
		Templates:       make(map[Pose]StagedTemplate),
		ExpiresAt:       s.now().Add(s.ttl),
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.purgeExpiredLocked()
	s.enrollments[enrollmentID] = enrollment
	return enrollmentID, nil
}

func (s *MemoryEnrollmentStore) Capture(enrollmentID string, pose Pose, tmpl StagedTemplate) error {
	if !pose.Valid() {
		return ErrInvalidPose
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	enrollment, ok := s.activeLocked(enrollmentID)
	if !ok {
		return ErrEnrollmentExpired
	}
	enrollment.Templates[pose] = cloneTemplate(tmpl)
	return nil
}

func (s *MemoryEnrollmentStore) Get(enrollmentID string) (*Enrollment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	enrollment, ok := s.activeLocked(enrollmentID)
	if !ok {
		return nil, ErrEnrollmentExpired
	}
	return cloneEnrollment(enrollment), nil
}

func (s *MemoryEnrollmentStore) Abort(enrollmentID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.enrollments, enrollmentID)
}

func (s *MemoryEnrollmentStore) InvalidateEmployee(employeeID uuid.UUID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for enrollmentID, enrollment := range s.enrollments {
		if enrollment.EmployeeID == employeeID {
			delete(s.enrollments, enrollmentID)
		}
	}
}

func (s *MemoryEnrollmentStore) activeLocked(enrollmentID string) (*Enrollment, bool) {
	enrollment, ok := s.enrollments[enrollmentID]
	if !ok {
		return nil, false
	}
	if !s.now().Before(enrollment.ExpiresAt) {
		delete(s.enrollments, enrollmentID)
		return nil, false
	}
	return enrollment, true
}

func (s *MemoryEnrollmentStore) purgeExpiredLocked() {
	now := s.now()
	for enrollmentID, enrollment := range s.enrollments {
		if !now.Before(enrollment.ExpiresAt) {
			delete(s.enrollments, enrollmentID)
		}
	}
}

func (p Pose) Valid() bool {
	return p == PoseFront || p == PoseLeft || p == PoseRight
}

func randomEnrollmentID() (string, error) {
	var token [32]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(token[:]), nil
}

func cloneEnrollment(enrollment *Enrollment) *Enrollment {
	cloned := *enrollment
	cloned.BaseTemplateIDs = cloneTemplateIDs(enrollment.BaseTemplateIDs)
	cloned.Templates = make(map[Pose]StagedTemplate, len(enrollment.Templates))
	for pose, tmpl := range enrollment.Templates {
		cloned.Templates[pose] = cloneTemplate(tmpl)
	}
	return &cloned
}

func cloneTemplateIDs(ids map[Pose]uuid.UUID) map[Pose]uuid.UUID {
	cloned := make(map[Pose]uuid.UUID, len(ids))
	for pose, id := range ids {
		cloned[pose] = id
	}
	return cloned
}

func cloneTemplate(tmpl StagedTemplate) StagedTemplate {
	tmpl.Embedding = append([]byte(nil), tmpl.Embedding...)
	return tmpl
}
