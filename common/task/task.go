package task

import (
	"errors"
	"sync"
	"time"
)

var ErrInvalidInterval = errors.New("task interval must be greater than zero")
var ErrNilExecute = errors.New("task execute function is nil")

type generation struct {
	stop     chan struct{}
	done     chan struct{}
	stopOnce sync.Once
}

type Task struct {
	// Interval is the initial interval. Change a running task with SetInterval.
	Interval time.Duration
	Execute  func() error

	access  sync.Mutex
	current *generation
}

func (t *Task) Start(first bool) error {
	t.access.Lock()
	if t.current != nil {
		t.access.Unlock()
		return nil
	}
	if t.Interval <= 0 {
		t.access.Unlock()
		return ErrInvalidInterval
	}
	if t.Execute == nil {
		t.access.Unlock()
		return ErrNilExecute
	}
	run := &generation{stop: make(chan struct{}), done: make(chan struct{})}
	t.current = run
	interval := t.Interval
	t.access.Unlock()

	go t.run(run, interval, first)
	return nil
}

func (t *Task) run(run *generation, interval time.Duration, first bool) {
	defer func() {
		t.access.Lock()
		if t.current == run {
			t.current = nil
		}
		t.access.Unlock()
		close(run.done)
	}()

	if first {
		if err := t.Execute(); err != nil {
			return
		}
	}
	for {
		timer := time.NewTimer(interval)
		select {
		case <-timer.C:
		case <-run.stop:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return
		}
		select {
		case <-run.stop:
			return
		default:
		}
		if err := t.Execute(); err != nil {
			return
		}
		t.access.Lock()
		interval = t.Interval
		t.access.Unlock()
		if interval <= 0 {
			return
		}
	}
}

// SetInterval changes the delay used after the current task execution finishes.
func (t *Task) SetInterval(interval time.Duration) error {
	if interval <= 0 {
		return ErrInvalidInterval
	}
	t.access.Lock()
	t.Interval = interval
	t.access.Unlock()
	return nil
}

func (t *Task) GetInterval() time.Duration {
	t.access.Lock()
	defer t.access.Unlock()
	return t.Interval
}

// Close cancels the current generation and waits for an active Execute call to
// return. A subsequent Start cannot overlap with the closed generation.
func (t *Task) Close() {
	t.access.Lock()
	run := t.current
	if run != nil {
		run.stopOnce.Do(func() { close(run.stop) })
	}
	t.access.Unlock()
	if run != nil {
		<-run.done
	}
}
