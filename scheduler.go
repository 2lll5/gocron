package main

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// ErrJobNotFound is returned when a removal is requested for a job that is not
// currently registered with the scheduler.
var ErrJobNotFound = errors.New("job not found")

// Scheduler runs interval-based jobs and owns all timers associated with those
// jobs. All mutations are protected by mu so removing and re-adding a job with
// the same ID cannot race with a running job's reschedule path.
type Scheduler struct {
	mu      sync.Mutex
	jobs    map[string]*Job
	stopped bool
}

// Job represents a scheduled task.
type Job struct {
	id       string
	interval time.Duration
	task     func(context.Context)

	scheduler *Scheduler

	mu      sync.Mutex
	timer   *time.Timer
	ctx     context.Context
	cancel  context.CancelFunc
	removed bool
	running atomic.Int32
}

// NewScheduler constructs an empty scheduler.
func NewScheduler() *Scheduler {
	return &Scheduler{jobs: make(map[string]*Job)}
}

// Every registers task to run repeatedly at interval. If another active job has
// the same ID, it is removed first so only the newly registered job can fire.
func (s *Scheduler) Every(id string, interval time.Duration, task func()) (*Job, error) {
	if task == nil {
		return nil, errors.New("task must not be nil")
	}
	return s.EveryWithContext(id, interval, func(context.Context) { task() })
}

// EveryWithContext registers a context-aware task to run repeatedly at interval.
// The context is cancelled when the job is removed or the scheduler is stopped.
func (s *Scheduler) EveryWithContext(id string, interval time.Duration, task func(context.Context)) (*Job, error) {
	if id == "" {
		return nil, errors.New("job id must not be empty")
	}
	if interval <= 0 {
		return nil, errors.New("interval must be positive")
	}
	if task == nil {
		return nil, errors.New("task must not be nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return nil, errors.New("scheduler is stopped")
	}

	if old := s.jobs[id]; old != nil {
		old.removeLocked()
		delete(s.jobs, id)
	}

	ctx, cancel := context.WithCancel(context.Background())
	job := &Job{
		id:        id,
		interval:  interval,
		task:      task,
		scheduler: s,
		ctx:       ctx,
		cancel:    cancel,
	}
	s.jobs[id] = job
	job.scheduleLocked()
	return job, nil
}

// Remove unregisters job and stops/cancels its timer state. A running task may
// finish naturally, but it receives a cancelled context and cannot reschedule
// itself after removal.
func (s *Scheduler) Remove(job *Job) error {
	if job == nil {
		return ErrJobNotFound
	}
	return s.RemoveByID(job.ID())
}

// RemoveByID unregisters the job with id and stops/cancels all scheduler-owned
// resources associated with it.
func (s *Scheduler) RemoveByID(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	job := s.jobs[id]
	if job == nil {
		return ErrJobNotFound
	}
	job.removeLocked()
	delete(s.jobs, id)
	return nil
}

// Stop removes all jobs and prevents new timer executions from being scheduled.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return
	}
	s.stopped = true
	for id, job := range s.jobs {
		job.removeLocked()
		delete(s.jobs, id)
	}
}

// ID returns the stable job identifier.
func (j *Job) ID() string { return j.id }

// Running reports whether the task function is currently executing.
func (j *Job) Running() bool { return j.running.Load() > 0 }

// scheduleLocked arms the next timer unless the job has been removed. The
// caller must hold either the scheduler lock during registration/reschedule or
// know no concurrent scheduler mutation is possible.
func (j *Job) scheduleLocked() {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.removed {
		return
	}
	j.timer = time.AfterFunc(j.interval, j.run)
}

// removeLocked marks the job removed, stops the timer, and cancels the task
// context. The caller must hold the scheduler mutex to serialize with re-adds.
func (j *Job) removeLocked() {
	j.mu.Lock()
	if j.removed {
		j.mu.Unlock()
		return
	}
	j.removed = true
	if j.timer != nil {
		j.timer.Stop()
		j.timer = nil
	}
	cancel := j.cancel
	j.mu.Unlock()

	if cancel != nil {
		cancel()
	}
}

func (j *Job) run() {
	j.mu.Lock()
	if j.removed {
		j.mu.Unlock()
		return
	}
	j.timer = nil
	ctx := j.ctx
	task := j.task
	j.running.Add(1)
	j.mu.Unlock()

	defer j.running.Add(-1)
	task(ctx)

	s := j.scheduler
	s.mu.Lock()
	defer s.mu.Unlock()

	// The job may have been removed while task was running, or a new job with the
	// same ID may have been added. Only the still-registered job may reschedule.
	j.mu.Lock()
	removed := j.removed
	j.mu.Unlock()
	if removed || s.stopped || s.jobs[j.id] != j {
		return
	}
	j.scheduleLocked()
}
