package forge

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// limiter tracks GitHub's rate-limit headers and delays requests once the
// remaining budget is exhausted until the advertised reset time.
type limiter struct {
	mu        sync.Mutex
	remaining int // -1 means unknown
	reset     time.Time
	now       func() time.Time
	sleep     func(ctx context.Context, d time.Duration) error
	maxWait   time.Duration
}

func newLimiter() *limiter {
	return &limiter{remaining: -1, now: time.Now, sleep: sleepCtx, maxWait: 30 * time.Second}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// wait blocks while the budget is spent. If the reset is further away than
// maxWait it returns ErrUnavailable instead of stalling the CLI.
func (l *limiter) wait(ctx context.Context) error {
	l.mu.Lock()
	if l.remaining != 0 {
		if l.remaining > 0 {
			l.remaining--
		}
		l.mu.Unlock()
		return nil
	}
	d := l.reset.Sub(l.now())
	l.mu.Unlock()
	if d <= 0 {
		return nil
	}
	if d > l.maxWait {
		return ErrUnavailable
	}
	return l.sleep(ctx, d)
}

// observe records the headers of a response.
func (l *limiter) observe(h http.Header) {
	rem, err1 := strconv.Atoi(h.Get("X-RateLimit-Remaining"))
	reset, err2 := strconv.ParseInt(h.Get("X-RateLimit-Reset"), 10, 64)
	l.mu.Lock()
	defer l.mu.Unlock()
	if err1 == nil {
		l.remaining = rem
	}
	if err2 == nil {
		l.reset = time.Unix(reset, 0)
	}
	if ra := h.Get("Retry-After"); ra != "" {
		if secs, err := strconv.Atoi(ra); err == nil {
			l.remaining = 0
			l.reset = l.now().Add(time.Duration(secs) * time.Second)
		}
	}
}
