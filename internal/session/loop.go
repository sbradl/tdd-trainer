// Package session drives one practice Session: it reruns the tests on
// every change and reports what happened as events.
package session

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/sbradl/tdd-trainer/internal/runner"
	"github.com/sbradl/tdd-trainer/internal/snapshot"
)

// Event is one of RunStarted, SlowRun, RunDone or RunFailed.
type Event interface{ isEvent() }

// RunStarted: a test run began; Changed is nil for the first run.
type RunStarted struct{ Changed []string }

// SlowRun: the current run has taken longer than the configured limit.
type SlowRun struct{ Limit time.Duration }

// RunDone: a run finished and was not superseded.
type RunDone struct {
	Result
	Duration time.Duration
}

// RunFailed: the build or test command could not be run at all.
type RunFailed struct{ Err error }

func (RunStarted) isEvent() {}
func (SlowRun) isEvent()    {}
func (RunDone) isEvent()    {}
func (RunFailed) isEvent()  {}

// Result is one test run and the snapshot of the code it tested.
type Result struct {
	Outcome  runner.Outcome
	Snapshot snapshot.ID
}

// RunFunc snapshots the code and runs build and tests once; it must stop
// when ctx is cancelled.
type RunFunc func(ctx context.Context) (Result, error)

// Loop runs the tests once, then again after every batch of changes. A
// batch arriving mid-run cancels that run and starts a new one; runs
// never overlap. Loop returns when ctx ends or changes is closed.
func Loop(ctx context.Context, changes <-chan []string, run RunFunc, slowAfter time.Duration, emit func(Event)) {
	type result struct {
		r   Result
		err error
		d   time.Duration
	}
	var (
		cancelRun context.CancelFunc
		done      chan result
		slow      <-chan time.Time
		next      []string // changes waiting for the cancelled run to exit
		queued    bool
	)
	startRun := func(changed []string) {
		var runCtx context.Context
		runCtx, cancelRun = context.WithCancel(ctx)
		done = make(chan result, 1)
		ch := done
		if slowAfter > 0 {
			slow = time.After(slowAfter)
		}
		emit(RunStarted{Changed: changed})
		go func() {
			t0 := time.Now()
			r, err := run(runCtx)
			ch <- result{r, err, time.Since(t0)}
		}()
	}
	defer func() {
		if cancelRun != nil {
			cancelRun()
			<-done
		}
	}()

	startRun(nil)
	for {
		select {
		case <-ctx.Done():
			return
		case c, ok := <-changes:
			if !ok {
				return
			}
			if done == nil {
				startRun(c)
				continue
			}
			next = merge(next, c)
			queued = true
			cancelRun()
		case <-slow:
			slow = nil
			emit(SlowRun{Limit: slowAfter})
		case r := <-done:
			cancelRun()
			cancelRun, done, slow = nil, nil, nil
			switch {
			case queued:
				// superseded: drop the result and rerun
			case r.err != nil && !errors.Is(r.err, context.Canceled):
				emit(RunFailed{Err: r.err})
			case r.err == nil:
				emit(RunDone{Result: r.r, Duration: r.d})
			}
			if queued {
				c := next
				next, queued = nil, false
				startRun(c)
			}
		}
	}
}

func merge(a, b []string) []string {
	out := append(append([]string{}, a...), b...)
	slices.Sort(out)
	return slices.Compact(out)
}
