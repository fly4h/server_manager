package auth

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// SessionStore keeps sessions in memory. Restarting the service logs
// everyone out — acceptable for a single-admin V1 deployment.
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	ttl      time.Duration
}

// Session is one logged-in user.
type Session struct {
	Token      string
	UserID     int64
	Username   string
	Role       string
	CreatedAt  time.Time
	ExpiresAt  time.Time
}

func NewSessionStore() *SessionStore {
	s := &SessionStore{
		sessions: make(map[string]*Session),
		ttl:      24 * time.Hour,
	}
	go s.gcLoop()
	return s
}

// Create makes a new session and returns its token.
func (s *SessionStore) Create(userID int64, username, role string) string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	token := hex.EncodeToString(b)
	s.mu.Lock()
	s.sessions[token] = &Session{
		Token:     token,
		UserID:    userID,
		Username:  username,
		Role:      role,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(s.ttl),
	}
	s.mu.Unlock()
	return token
}

// Get returns a live session for the token, or nil.
func (s *SessionStore) Get(token string) *Session {
	s.mu.RLock()
	sess := s.sessions[token]
	s.mu.RUnlock()
	if sess == nil {
		return nil
	}
	if time.Now().After(sess.ExpiresAt) {
		s.Delete(token)
		return nil
	}
	return sess
}

// Delete removes a session (logout).
func (s *SessionStore) Delete(token string) {
	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()
}

func (s *SessionStore) gcLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		s.mu.Lock()
		for k, v := range s.sessions {
			if now.After(v.ExpiresAt) {
				delete(s.sessions, k)
			}
		}
		s.mu.Unlock()
	}
}
