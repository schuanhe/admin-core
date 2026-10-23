package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"regexp"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
	"workstation/internal/store"
)

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{3,32}$`)
var ErrCredentials = errors.New("用户名或密码错误")
var dummyHash = []byte("$2a$10$7EqJtq98hPqEX7fNZaFWoO5Kx6Z17jMhzWAmhO7YSFJxZ8Vm3Wi8q")

type Manager struct {
	Store    *store.Store
	Hours    int
	mu       sync.Mutex
	attempts map[string]bucket
}
type bucket struct {
	count int
	until time.Time
}

func New(s *store.Store, hours int) *Manager {
	return &Manager{Store: s, Hours: hours, attempts: map[string]bucket{}}
}
func Equal(a, b string) bool {
	ah := sha256.Sum256([]byte(a))
	bh := sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(ah[:], bh[:]) == 1
}
func Hash(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
func token() (string, error) {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	return hex.EncodeToString(b), nil
}
func (m *Manager) Allow(ip string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := time.Now()
	for k, v := range m.attempts {
		if t.After(v.until) {
			delete(m.attempts, k)
		}
	}
	v := m.attempts[ip]
	if v.until.IsZero() {
		if len(m.attempts) >= 4096 {
			return false
		}
		v.until = t.Add(time.Minute)
	}
	v.count++
	m.attempts[ip] = v
	return v.count <= 10
}
func (m *Manager) Setup(ctx context.Context, username, password string) error {
	if !usernamePattern.MatchString(username) {
		return errors.New("用户名须为3到32位字母、数字、点、横线或下划线")
	}
	if len(password) < 12 || len(password) > 72 {
		return errors.New("密码长度须为12到72字节")
	}
	h, e := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if e != nil {
		return e
	}
	return m.Store.Setup(ctx, username, string(h))
}
func (m *Manager) Login(ctx context.Context, username, password string) (session, csrf string, expires time.Time, err error) {
	h, e := m.Store.Password(ctx, username)
	if e != nil && !errors.Is(e, store.ErrNotFound) {
		return "", "", time.Time{}, e
	}
	if e != nil {
		h = string(dummyHash)
	}
	compare := bcrypt.CompareHashAndPassword([]byte(h), []byte(password))
	if e != nil || compare != nil || len(password) > 72 {
		return "", "", time.Time{}, ErrCredentials
	}
	session, err = token()
	if err != nil {
		return
	}
	csrf, err = token()
	if err != nil {
		return
	}
	expires = time.Now().Add(time.Duration(m.Hours) * time.Hour)
	err = m.Store.CreateSession(ctx, Hash(session), csrf, username, expires)
	return
}
