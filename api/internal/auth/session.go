package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"sync"
	"time"
)

const tokenBytes = 32

var ErrSessionCapacity = errors.New("session store capacity reached")

type SessionStore interface {
	Create(username string, ttl time.Duration) (token string, expiresAt time.Time, err error)
	Get(token string) (username string, ok bool)
	Delete(token string)
}

type session struct {
	username  string
	expiresAt time.Time
}

type MemorySessionStore struct {
	mu       sync.Mutex
	sessions map[string]session
	max      int
}

func NewMemorySessionStore(max int) *MemorySessionStore {
	return &MemorySessionStore{
		sessions: make(map[string]session),
		max:      max,
	}
}

func (s *MemorySessionStore) Create(username string, ttl time.Duration) (string, time.Time, error) {
	now := time.Now()
	expiresAt := now.Add(ttl)

	s.mu.Lock()
	defer s.mu.Unlock()

	s.purgeExpired(now)
	if len(s.sessions) >= s.max {
		return "", time.Time{}, ErrSessionCapacity
	}

	for {
		random := make([]byte, tokenBytes)
		if _, err := rand.Read(random); err != nil {
			return "", time.Time{}, err
		}
		token := base64.RawURLEncoding.EncodeToString(random)
		if _, exists := s.sessions[token]; exists {
			continue
		}
		s.sessions[token] = session{username: username, expiresAt: expiresAt}
		return token, expiresAt, nil
	}
}

func (s *MemorySessionStore) Get(token string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.sessions[token]
	if !ok {
		return "", false
	}
	if !time.Now().Before(entry.expiresAt) {
		delete(s.sessions, token)
		return "", false
	}
	return entry.username, true
}

func (s *MemorySessionStore) Delete(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, token)
}

func (s *MemorySessionStore) purgeExpired(now time.Time) {
	for token, entry := range s.sessions {
		if !now.Before(entry.expiresAt) {
			delete(s.sessions, token)
		}
	}
}
