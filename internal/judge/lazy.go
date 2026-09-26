package judge

import (
	"context"
	"sync"
)

// Lazy is a Scorer whose engine loads in the background; Score waits for
// it. If loading fails, Score returns that error.
type Lazy struct {
	ready chan struct{}
	once  sync.Once
	e     *Engine
	err   error
}

// OpenLazy starts loading the engine and returns at once.
func OpenLazy(o EngineOptions) *Lazy {
	l := &Lazy{ready: make(chan struct{})}
	go func() {
		l.e, l.err = OpenEngine(o)
		close(l.ready)
	}()
	return l
}

// Wait blocks until loading finished and returns the engine or the error.
func (l *Lazy) Wait(ctx context.Context) (*Engine, error) {
	select {
	case <-l.ready:
		return l.e, l.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (l *Lazy) Score(ctx context.Context, state string, gates []Gate) ([][]float64, error) {
	e, err := l.Wait(ctx)
	if err != nil {
		return nil, err
	}
	return e.Score(ctx, state, gates)
}

// Close waits for loading to finish and frees the engine.
func (l *Lazy) Close() {
	<-l.ready
	if l.e != nil {
		l.e.Close()
	}
}
