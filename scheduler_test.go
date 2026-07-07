package main

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestRemoveRunningJobDoesNotDuplicateAfterReAdd(t *testing.T) {
	s := NewScheduler()
	defer s.Stop()

	started := make(chan struct{}, 1)
	allowFinish := make(chan struct{})
	var oldRuns atomic.Int32

	oldJob, err := s.EveryWithContext("job", 10*time.Millisecond, func(ctx context.Context) {
		if oldRuns.Add(1) == 1 {
			started <- struct{}{}
		}
		select {
		case <-allowFinish:
		case <-ctx.Done():
		}
	})
	if err != nil {
		t.Fatalf("EveryWithContext() error = %v", err)
	}

	select {
	case <-started:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("job did not start")
	}

	if err := s.Remove(oldJob); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}

	// Give any orphaned timer enough time to fire. The removed job must not run a
	// second time while its original invocation is still unwinding.
	time.Sleep(75 * time.Millisecond)
	if got := oldRuns.Load(); got != 1 {
		t.Fatalf("removed job ran %d times, want exactly 1", got)
	}
	close(allowFinish)

	var newRuns atomic.Int32
	newStarted := make(chan struct{}, 3)
	if _, err := s.Every("job", 20*time.Millisecond, func() {
		newRuns.Add(1)
		select {
		case newStarted <- struct{}{}:
		default:
		}
	}); err != nil {
		t.Fatalf("re-add Every() error = %v", err)
	}

	for i := 0; i < 2; i++ {
		select {
		case <-newStarted:
		case <-time.After(500 * time.Millisecond):
			t.Fatalf("new job run %d did not occur", i+1)
		}
	}

	if got := oldRuns.Load(); got != 1 {
		t.Fatalf("old removed job was rescheduled after re-add: runs=%d", got)
	}
	if got := newRuns.Load(); got < 2 || got > 3 {
		t.Fatalf("new job executions = %d, want 2 or 3 without duplicate burst", got)
	}
}

func TestRemoveByIDStopsPendingTimer(t *testing.T) {
	s := NewScheduler()
	defer s.Stop()

	var runs atomic.Int32
	if _, err := s.Every("pending", 40*time.Millisecond, func() { runs.Add(1) }); err != nil {
		t.Fatalf("Every() error = %v", err)
	}
	if err := s.RemoveByID("pending"); err != nil {
		t.Fatalf("RemoveByID() error = %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	if got := runs.Load(); got != 0 {
		t.Fatalf("removed pending job ran %d times, want 0", got)
	}
	if err := s.RemoveByID("pending"); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("second RemoveByID() error = %v, want ErrJobNotFound", err)
	}
}

func TestReAddReplacesExistingTimer(t *testing.T) {
	s := NewScheduler()
	defer s.Stop()

	var oldRuns atomic.Int32
	if _, err := s.Every("same", 20*time.Millisecond, func() { oldRuns.Add(1) }); err != nil {
		t.Fatalf("Every(old) error = %v", err)
	}

	var newRuns atomic.Int32
	newStarted := make(chan struct{}, 1)
	if _, err := s.Every("same", 20*time.Millisecond, func() {
		newRuns.Add(1)
		select {
		case newStarted <- struct{}{}:
		default:
		}
	}); err != nil {
		t.Fatalf("Every(new) error = %v", err)
	}

	select {
	case <-newStarted:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("replacement job did not run")
	}
	time.Sleep(50 * time.Millisecond)
	if got := oldRuns.Load(); got != 0 {
		t.Fatalf("replaced job ran %d times, want 0", got)
	}
	if got := newRuns.Load(); got == 0 || got > 4 {
		t.Fatalf("replacement job executions = %d, want normal interval executions", got)
	}
}
