package face

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestEnrollmentStartReturnsOpaqueIDAndBaseTemplates(t *testing.T) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	store := NewMemoryEnrollmentStore(10*time.Minute, func() time.Time { return now })
	employeeID := uuid.New()
	baseID := uuid.New()

	enrollmentID, err := store.Start(employeeID, map[Pose]uuid.UUID{PoseFront: baseID})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if enrollmentID == "" || enrollmentID == employeeID.String() {
		t.Fatalf("enrollment ID = %q, want opaque non-empty value", enrollmentID)
	}

	enrollment, err := store.Get(enrollmentID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if enrollment.EmployeeID != employeeID || enrollment.BaseTemplateIDs[PoseFront] != baseID {
		t.Fatalf("enrollment = %#v, want employee and base template", enrollment)
	}
	if want := now.Add(10 * time.Minute); !enrollment.ExpiresAt.Equal(want) {
		t.Fatalf("ExpiresAt = %v, want %v", enrollment.ExpiresAt, want)
	}
}

func TestEnrollmentCaptureReplacesSamePose(t *testing.T) {
	store := NewMemoryEnrollmentStore(10*time.Minute, time.Now)
	enrollmentID, err := store.Start(uuid.New(), nil)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	first := stagedTemplate(0.80)
	second := stagedTemplate(0.95)
	if err := store.Capture(enrollmentID, PoseFront, first); err != nil {
		t.Fatalf("first Capture() error = %v", err)
	}
	if err := store.Capture(enrollmentID, PoseFront, second); err != nil {
		t.Fatalf("second Capture() error = %v", err)
	}

	enrollment, err := store.Get(enrollmentID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if len(enrollment.Templates) != 1 || enrollment.Templates[PoseFront].QualityScore != second.QualityScore {
		t.Fatalf("templates = %#v, want only replacement FRONT", enrollment.Templates)
	}
}

func TestEnrollmentIncompleteUntilAllRequiredPosesCaptured(t *testing.T) {
	store := NewMemoryEnrollmentStore(10*time.Minute, time.Now)
	enrollmentID, _ := store.Start(uuid.New(), nil)
	_ = store.Capture(enrollmentID, PoseFront, stagedTemplate(0.9))
	_ = store.Capture(enrollmentID, PoseLeft, stagedTemplate(0.9))

	enrollment, err := store.Get(enrollmentID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if err := enrollment.ValidateComplete(); !errors.Is(err, ErrEnrollmentIncomplete) {
		t.Fatalf("ValidateComplete() error = %v, want ErrEnrollmentIncomplete", err)
	}

	_ = store.Capture(enrollmentID, PoseRight, stagedTemplate(0.9))
	enrollment, _ = store.Get(enrollmentID)
	if err := enrollment.ValidateComplete(); err != nil {
		t.Fatalf("complete ValidateComplete() error = %v", err)
	}
}

func TestEnrollmentValidateModelSet(t *testing.T) {
	templates := map[Pose]StagedTemplate{
		PoseFront: stagedTemplate(0.9),
		PoseLeft:  stagedTemplate(0.9),
		PoseRight: stagedTemplate(0.9),
	}
	enrollment := &Enrollment{Templates: templates}
	if err := enrollment.ValidateModelSet(); err != nil {
		t.Fatalf("ValidateModelSet() error = %v", err)
	}

	right := templates[PoseRight]
	right.ModelVersion = "different"
	templates[PoseRight] = right
	if err := enrollment.ValidateModelSet(); !errors.Is(err, ErrModelVersionMismatch) {
		t.Fatalf("mixed model error = %v, want ErrModelVersionMismatch", err)
	}

	templates[PoseRight] = stagedTemplate(0.9)
	right = templates[PoseRight]
	right.EmbeddingDim++
	templates[PoseRight] = right
	if err := enrollment.ValidateModelSet(); !errors.Is(err, ErrValidation) {
		t.Fatalf("mixed dimension error = %v, want ErrValidation", err)
	}
}

func TestEnrollmentExpiresAfterTTL(t *testing.T) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	store := NewMemoryEnrollmentStore(10*time.Minute, func() time.Time { return now })
	enrollmentID, _ := store.Start(uuid.New(), nil)

	now = now.Add(10*time.Minute + time.Nanosecond)
	if _, err := store.Get(enrollmentID); !errors.Is(err, ErrEnrollmentExpired) {
		t.Fatalf("Get() error = %v, want ErrEnrollmentExpired", err)
	}
	if err := store.Capture(enrollmentID, PoseFront, stagedTemplate(0.9)); !errors.Is(err, ErrEnrollmentExpired) {
		t.Fatalf("Capture() error = %v, want ErrEnrollmentExpired", err)
	}
}

func TestInvalidateEmployeeDropsAllTheirSessions(t *testing.T) {
	store := NewMemoryEnrollmentStore(10*time.Minute, time.Now)
	employeeID := uuid.New()
	first, _ := store.Start(employeeID, nil)
	second, _ := store.Start(employeeID, nil)
	other, _ := store.Start(uuid.New(), nil)

	store.InvalidateEmployee(employeeID)

	for _, enrollmentID := range []string{first, second} {
		if _, err := store.Get(enrollmentID); !errors.Is(err, ErrEnrollmentExpired) {
			t.Errorf("Get(%q) error = %v, want ErrEnrollmentExpired", enrollmentID, err)
		}
	}
	if _, err := store.Get(other); err != nil {
		t.Fatalf("other employee Get() error = %v", err)
	}
}

func TestAbortDropsSessionAndInvalidPoseIsRejected(t *testing.T) {
	store := NewMemoryEnrollmentStore(10*time.Minute, time.Now)
	enrollmentID, _ := store.Start(uuid.New(), nil)

	if err := store.Capture(enrollmentID, Pose("UP"), stagedTemplate(0.9)); !errors.Is(err, ErrInvalidPose) {
		t.Fatalf("Capture() error = %v, want ErrInvalidPose", err)
	}
	store.Abort(enrollmentID)
	if _, err := store.Get(enrollmentID); !errors.Is(err, ErrEnrollmentExpired) {
		t.Fatalf("Get() after Abort error = %v, want ErrEnrollmentExpired", err)
	}
}

func stagedTemplate(quality float32) StagedTemplate {
	return StagedTemplate{
		Embedding:    make([]byte, 128*4),
		EmbeddingDim: 128,
		ModelName:    "sface",
		ModelVersion: "2021dec",
		QualityScore: quality,
	}
}
