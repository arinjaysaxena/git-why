package forge

import (
	"context"
	"sync"
)

// Map runs fn over items with at most workers goroutines and returns results
// in input order. The first error cancels remaining work and is returned along
// with whatever results completed (zero values elsewhere).
func Map[T, R any](ctx context.Context, workers int, items []T, fn func(context.Context, T) (R, error)) ([]R, error) {
	if workers < 1 {
		workers = 1
	}
	parent := ctx
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	out := make([]R, len(items))
	var (
		wg       sync.WaitGroup
		once     sync.Once
		firstErr error
	)
	sem := make(chan struct{}, workers)
loop:
	for i := range items {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			break loop
		}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			r, err := fn(ctx, items[i])
			if err != nil {
				once.Do(func() { firstErr = err; cancel() })
				return
			}
			out[i] = r
		}(i)
	}
	wg.Wait()
	if firstErr == nil {
		firstErr = parent.Err()
	}
	return out, firstErr
}
