package face

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestReceiptConcurrentClaimAllowsExactlyOneInFlight(t *testing.T) {
	now := time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)
	store := NewReceiptStore(time.Minute, func() time.Time { return now })
	token, err := store.Issue(Receipt{EmployeeID: "employee-1", SimilarityScore: 0.9})
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	const claimers = 32
	start := make(chan struct{})
	var successes atomic.Int32
	var wg sync.WaitGroup
	for range claimers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if _, claimErr := store.Claim(token); claimErr == nil {
				successes.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()

	if got := successes.Load(); got != 1 {
		t.Fatalf("successful claims = %d, want 1", got)
	}
}

func TestReceiptClaimChecksExpiryAtBoundary(t *testing.T) {
	now := time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)
	store := NewReceiptStore(time.Minute, func() time.Time { return now })
	token, err := store.Issue(Receipt{EmployeeID: "employee-1"})
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	now = now.Add(time.Minute)

	if _, err := store.Claim(token); err != ErrReceiptInvalid {
		t.Fatalf("Claim() error = %v, want ErrReceiptInvalid", err)
	}
}

func TestReceiptReleaseReturnsUnexpiredClaimToUnused(t *testing.T) {
	now := time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)
	store := NewReceiptStore(time.Minute, func() time.Time { return now })
	token, _ := store.Issue(Receipt{EmployeeID: "employee-1"})
	if _, err := store.Claim(token); err != nil {
		t.Fatalf("first Claim() error = %v", err)
	}

	store.Release(token)

	if _, err := store.Claim(token); err != nil {
		t.Fatalf("Claim() after Release error = %v", err)
	}
}

func TestReceiptConsumePreventsReuse(t *testing.T) {
	store := NewReceiptStore(time.Minute, time.Now)
	token, _ := store.Issue(Receipt{EmployeeID: "employee-1"})
	if _, err := store.Claim(token); err != nil {
		t.Fatalf("Claim() error = %v", err)
	}
	store.Consume(token)

	if _, err := store.Claim(token); err != ErrReceiptInvalid {
		t.Fatalf("Claim() after Consume error = %v, want ErrReceiptInvalid", err)
	}
}
