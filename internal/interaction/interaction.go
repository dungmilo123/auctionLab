// Package interaction defines minimal private first-party observations. Actions
// are captured by the store in their parent transaction; no client action API exists.
package interaction

import (
	"context"
	"errors"
	"net"
	"regexp"
	"sync"
	"time"
)

var ErrNotFound = errors.New("published lot not found")
var ErrConflict = errors.New("observation already bound")
var UUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type Repository interface {
	Observe(context.Context, string, string, int64) error
}
type Service struct {
	Repo Repository
	Now  func() time.Time
}

func (s Service) Observe(ctx context.Context, lot, id string) error {
	return s.Repo.Observe(ctx, lot, id, s.Now().UnixMilli())
}

type attempt struct {
	count int
	until time.Time
}
type Limiter struct {
	mu    sync.Mutex
	peers map[string]attempt
	Now   func() time.Time
}

func NewLimiter() *Limiter { return &Limiter{peers: map[string]attempt{}, Now: time.Now} }

// Allow retains only normalized direct peer addresses in bounded volatile memory.
func (l *Limiter) Allow(remote string) bool {
	peer, _, err := net.SplitHostPort(remote)
	if err != nil {
		peer = remote
	}
	if ip := net.ParseIP(peer); ip != nil {
		peer = ip.String()
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.Now()
	for k, a := range l.peers {
		if !now.Before(a.until) {
			delete(l.peers, k)
		}
	}
	a, ok := l.peers[peer]
	if (!ok && len(l.peers) >= 4096) || a.count >= 30 {
		return false
	}
	if !ok {
		a.until = now.Add(time.Minute)
	}
	a.count++
	l.peers[peer] = a
	return true
}
