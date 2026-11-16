package task

import (
	"errors"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

func TestStartRejectsInvalidConfiguration(t *testing.T) {
	for _, interval := range []time.Duration{0, -time.Second} {
		task := &Task{Interval: interval, Execute: func() error { return nil }}
		if err := task.Start(false); !errors.Is(err, ErrInvalidInterval) {
			t.Fatalf("Start() interval %s error = %v, want ErrInvalidInterval", interval, err)
		}
	}
	if err := (&Task{Interval: time.Second}).Start(false); !errors.Is(err, ErrNilExecute) {
		t.Fatalf("Start() without Execute error = %v, want ErrNilExecute", err)
	}
}

func TestCloseWaitsForExecuteAndPreventsOverlappingRestart(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var active atomic.Int32
	var maxActive atomic.Int32
	task := &Task{Interval: time.Hour}
	task.Execute = func() error {
		current := active.Add(1)
		for old := maxActive.Load(); current > old && !maxActive.CompareAndSwap(old, current); old = maxActive.Load() {
		}
		defer active.Add(-1)
		select {
		case entered <- struct{}{}:
		default:
		}
		<-release
		return nil
	}

	if err := task.Start(true); err != nil {
		t.Fatal(err)
	}
	<-entered
	closed := make(chan struct{})
	go func() {
		task.Close()
		close(closed)
	}()

	// Wait until Close has cancelled this generation while Execute remains held.
	deadline := time.After(2 * time.Second)
	for {
		task.access.Lock()
		run := task.current
		cancelled := false
		if run != nil {
			select {
			case <-run.stop:
				cancelled = true
			default:
			}
		}
		task.access.Unlock()
		if cancelled {
			break
		}
		select {
		case <-deadline:
			t.Fatal("Close did not cancel the active generation")
		default:
			runtimeYield()
		}
	}

	if err := task.Start(true); err != nil {
		t.Fatal(err)
	}
	select {
	case <-closed:
		t.Fatal("Close returned before Execute finished")
	default:
	}
	if got := active.Load(); got != 1 {
		t.Fatalf("active Execute calls before release = %d, want 1", got)
	}

	close(release)
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not wait for the active generation to finish")
	}
	task.Close()
	if got := active.Load(); got != 0 {
		t.Fatalf("active Execute calls after Close = %d, want 0", got)
	}
	if got := maxActive.Load(); got != 1 {
		t.Fatalf("maximum overlapping Execute calls = %d, want 1", got)
	}
}

func TestCloseIsIdempotentAndIntervalCanBeUpdated(t *testing.T) {
	task := &Task{Interval: time.Hour, Execute: func() error { return nil }}
	if err := task.SetInterval(0); !errors.Is(err, ErrInvalidInterval) {
		t.Fatalf("SetInterval(0) error = %v, want ErrInvalidInterval", err)
	}
	if err := task.SetInterval(2 * time.Hour); err != nil {
		t.Fatalf("SetInterval() error: %v", err)
	}
	task.Close()
	task.Close()
	if task.Interval != 2*time.Hour {
		t.Fatalf("Interval = %s, want 2h", task.Interval)
	}
}

func runtimeYield() {
	runtime.Gosched()
}
