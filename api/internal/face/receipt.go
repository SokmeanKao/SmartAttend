package face

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"sync"
	"time"
)

type ReceiptState string

const (
	ReceiptUnused   ReceiptState = "UNUSED"
	ReceiptInFlight ReceiptState = "IN_FLIGHT"
	ReceiptConsumed ReceiptState = "CONSUMED"
)

var ErrReceiptInvalid = errors.New("verification receipt invalid")

type Receipt struct {
	EmployeeID      string
	SimilarityScore float32
	MatchedPose     Pose
	ModelName       string
	ModelVersion    string
	State           ReceiptState
	ExpiresAt       time.Time
}

type ReceiptStore struct {
	mu       sync.Mutex
	receipts map[string]*Receipt
	ttl      time.Duration
	now      func() time.Time
}

func NewReceiptStore(ttl time.Duration, now func() time.Time) *ReceiptStore {
	return &ReceiptStore{
		receipts: make(map[string]*Receipt),
		ttl:      ttl,
		now:      now,
	}
}

func (s *ReceiptStore) Issue(receipt Receipt) (string, error) {
	var random [32]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(random[:])
	now := s.now()
	receipt.State = ReceiptUnused
	receipt.ExpiresAt = now.Add(s.ttl)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.purgeExpiredLocked(now)
	s.receipts[token] = &receipt
	return token, nil
}

func (s *ReceiptStore) Claim(token string) (*Receipt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	s.purgeExpiredLocked(now)
	receipt, ok := s.receipts[token]
	if !ok || !now.Before(receipt.ExpiresAt) {
		delete(s.receipts, token)
		return nil, ErrReceiptInvalid
	}
	if receipt.State != ReceiptUnused {
		return nil, ErrReceiptInvalid
	}
	receipt.State = ReceiptInFlight
	claimed := *receipt
	return &claimed, nil
}

func (s *ReceiptStore) Release(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	receipt, ok := s.receipts[token]
	if !ok || receipt.State != ReceiptInFlight {
		return
	}
	if !s.now().Before(receipt.ExpiresAt) {
		delete(s.receipts, token)
		return
	}
	receipt.State = ReceiptUnused
}

func (s *ReceiptStore) Consume(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if receipt, ok := s.receipts[token]; ok && receipt.State == ReceiptInFlight {
		receipt.State = ReceiptConsumed
	}
}

func (s *ReceiptStore) purgeExpiredLocked(now time.Time) {
	for token, receipt := range s.receipts {
		if (receipt.State == ReceiptUnused || receipt.State == ReceiptConsumed) &&
			!now.Before(receipt.ExpiresAt) {
			delete(s.receipts, token)
		}
	}
}
