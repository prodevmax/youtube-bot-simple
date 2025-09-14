package state

import (
	"context"
	"math/rand"
	"sync"
	"time"
)

// Payload — полезная нагрузка, которую храним по токену
// кратко и по делу

type Payload struct {
	URL string
}

type entry struct {
	p         Payload
	expiresAt time.Time
}

// Store — простое in-memory хранилище с TTL

type Store struct {
	mu   sync.Mutex
	data map[string]entry
}

func NewStore() *Store {
	return &Store{data: make(map[string]entry)}
}

func (s *Store) Put(token string, p Payload, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[token] = entry{p: p, expiresAt: time.Now().Add(ttl)}
}

func (s *Store) Get(token string) (Payload, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.data[token]; ok {
		if time.Now().Before(e.expiresAt) {
			return e.p, true
		}
		delete(s.data, token)
	}
	return Payload{}, false
}

func (s *Store) Delete(token string) {
	s.mu.Lock(); defer s.mu.Unlock(); delete(s.data, token)
}

func (s *Store) StartGC(ctx context.Context, interval time.Duration) {
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.gc()
			}
		}
	}()
}

func (s *Store) gc() {
	s.mu.Lock(); defer s.mu.Unlock()
	now := time.Now()
	for k, e := range s.data {
		if now.After(e.expiresAt) {
			delete(s.data, k)
		}
	}
}

// GenerateToken — короткий токен для callback_data
func GenerateToken(n int) string {
	letters := []rune("abcdefghijklmnopqrstuvwxyz0123456789")
	b := make([]rune, n)
	rand.Seed(time.Now().UnixNano())
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}
