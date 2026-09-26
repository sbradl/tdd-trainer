package session

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/sbradl/tdd-trainer/internal/results"
	"github.com/sbradl/tdd-trainer/internal/runner"
)

type recorder struct {
	mu     sync.Mutex
	events []Event
	ch     chan Event
}

func newRecorder() *recorder { return &recorder{ch: make(chan Event, 100)} }

func (r *recorder) emit(e Event) { r.ch <- e }

func (r *recorder) next(t *testing.T) Event {
	t.Helper()
	select {
	case e := <-r.ch:
		return e
	case <-time.After(2 * time.Second):
		t.Fatal("no event")
		return nil
	}
}

// fakeRun blocks until released or cancelled and counts concurrent runs.
type fakeRun struct {
	mu      sync.Mutex
	active  int
	overlap bool
	release chan Result
	started chan struct{}
}

func newFakeRun() *fakeRun {
	return &fakeRun{release: make(chan Result), started: make(chan struct{}, 100)}
}

func (f *fakeRun) run(ctx context.Context) (Result, error) {
	f.mu.Lock()
	f.active++
	if f.active > 1 {
		f.overlap = true
	}
	f.mu.Unlock()
	defer func() { f.mu.Lock(); f.active--; f.mu.Unlock() }()
	f.started <- struct{}{}
	select {
	case o := <-f.release:
		return o, nil
	case <-ctx.Done():
		return Result{}, ctx.Err()
	}
}

func red(id string) Result {
	return Result{Outcome: runner.Outcome{State: results.TestState{Failing: []results.Failure{{ID: id}}}}}
}

func TestLoopRunsOnStartAndOnChanges(t *testing.T) {
	f, rec := newFakeRun(), newRecorder()
	changes := make(chan []string)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go Loop(ctx, changes, f.run, 0, rec.emit)

	if e := rec.next(t).(RunStarted); e.Changed != nil {
		t.Fatalf("first run has changes %v", e.Changed)
	}
	f.release <- red("a")
	if e := rec.next(t).(RunDone); e.Outcome.State.Failing[0].ID != "a" {
		t.Fatalf("got %+v", e)
	}
	changes <- []string{"x.go"}
	if e := rec.next(t).(RunStarted); e.Changed[0] != "x.go" {
		t.Fatalf("got %+v", e)
	}
}

func TestChangeDuringRunCancelsIt(t *testing.T) {
	f, rec := newFakeRun(), newRecorder()
	changes := make(chan []string)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go Loop(ctx, changes, f.run, 0, rec.emit)
	rec.next(t) // started
	<-f.started

	changes <- []string{"b.go"}
	changes <- []string{"a.go", "b.go"}
	e := rec.next(t).(RunStarted)
	if len(e.Changed) != 2 || e.Changed[0] != "a.go" || e.Changed[1] != "b.go" {
		t.Fatalf("want merged sorted changes, got %v", e.Changed)
	}
	f.release <- red("new")
	if e := rec.next(t).(RunDone); e.Outcome.State.Failing[0].ID != "new" {
		t.Fatalf("got %+v", e)
	}
	if f.overlap {
		t.Fatal("runs overlapped")
	}
}

func TestSlowRunWarning(t *testing.T) {
	f, rec := newFakeRun(), newRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go Loop(ctx, make(chan []string), f.run, 50*time.Millisecond, rec.emit)
	rec.next(t)
	if _, ok := rec.next(t).(SlowRun); !ok {
		t.Fatal("want SlowRun")
	}
	f.release <- red("a")
	rec.next(t)
}

func TestRunErrorsAreReported(t *testing.T) {
	rec := newRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	boom := errors.New("sh: not found")
	go Loop(ctx, make(chan []string), func(context.Context) (Result, error) { return Result{}, boom }, 0, rec.emit)
	rec.next(t)
	if e, ok := rec.next(t).(RunFailed); !ok || e.Err != boom {
		t.Fatalf("got %+v", e)
	}
}

func TestLoopStopsRunOnExit(t *testing.T) {
	f, rec := newFakeRun(), newRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	exited := make(chan struct{})
	go func() { Loop(ctx, make(chan []string), f.run, 0, rec.emit); close(exited) }()
	<-f.started
	cancel()
	select {
	case <-exited:
	case <-time.After(2 * time.Second):
		t.Fatal("loop did not exit")
	}
	if f.active != 0 {
		t.Fatal("run still active")
	}
}
