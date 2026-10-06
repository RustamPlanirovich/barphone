// Package pairing implements the one-time code that a phone exchanges for a device token.
package pairing

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"
)

const (
	TTL         = 5 * time.Minute
	MaxAttempts = 5
)

var (
	ErrNoSession = errors.New("no active pairing session")
	ErrBadCode   = errors.New("wrong pairing code")
)

// Sessions holds at most one active pairing session.
type Sessions struct {
	Now func() time.Time

	mu       sync.Mutex
	code     string
	expires  time.Time
	attempts int
}

func (s *Sessions) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Start replaces any active session with a fresh 6-digit code.
func (s *Sessions) Start() (code string, expires time.Time) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		panic(err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.code = fmt.Sprintf("%06d", n.Int64())
	s.expires = s.now().Add(TTL)
	s.attempts = 0
	return s.code, s.expires
}

func (s *Sessions) Cancel() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.code = ""
}

func (s *Sessions) Active() (code string, expires time.Time, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.code == "" || s.now().After(s.expires) {
		return "", time.Time{}, false
	}
	return s.code, s.expires, true
}

// Verify consumes the session on success. Too many wrong attempts burn the session,
// so the 10^6 code space cannot be brute-forced.
func (s *Sessions) Verify(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.code == "" || s.now().After(s.expires) {
		s.code = ""
		return ErrNoSession
	}
	if subtle.ConstantTimeCompare([]byte(code), []byte(s.code)) == 1 {
		s.code = ""
		return nil
	}
	s.attempts++
	if s.attempts >= MaxAttempts {
		s.code = ""
	}
	return ErrBadCode
}

// NewToken returns a device token and the hash that is persisted instead of it.
func NewToken() (token, hash string) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, HashToken(token)
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
