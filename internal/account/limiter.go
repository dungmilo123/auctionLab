package account

import (
	"net"
	"sync"
	"time"
)

type attempt struct {
	count int
	until time.Time
}
type Limiter struct {
	mu    sync.Mutex
	peers map[string]attempt
	work  chan struct{}
	Now   func() time.Time
}

func NewLimiter() *Limiter {
	return &Limiter{peers: map[string]attempt{}, work: make(chan struct{}, 4), Now: time.Now}
}

// Enter uses only the connection's direct peer, never forwarded request headers.
func (l *Limiter) Enter(remote string) (func(), bool) {
	peer, _, e := net.SplitHostPort(remote)
	if e != nil {
		peer = remote
	}
	if ip := net.ParseIP(peer); ip != nil {
		peer = ip.String()
	}
	l.mu.Lock()
	now := l.Now()
	for k, a := range l.peers {
		if !now.Before(a.until) {
			delete(l.peers, k)
		}
	}
	a, found := l.peers[peer]
	if (!found && len(l.peers) >= 4096) || a.count >= 10 {
		l.mu.Unlock()
		return nil, false
	}
	if !found {
		a.until = now.Add(time.Minute)
	}
	a.count++
	l.peers[peer] = a
	l.mu.Unlock()
	select {
	case l.work <- struct{}{}:
		return func() { <-l.work }, true
	default:
		return nil, false
	}
}
