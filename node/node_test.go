package node

import (
	"errors"
	"testing"
)

type fakeControllerLifecycle struct {
	startErr error
	closeErr error
	name     string
	closeLog *[]string
	starts   int
	closes   int
}

func (c *fakeControllerLifecycle) Start() error {
	c.starts++
	return c.startErr
}

func (c *fakeControllerLifecycle) Close() error {
	c.closes++
	if c.closeLog != nil {
		*c.closeLog = append(*c.closeLog, c.name)
	}
	return c.closeErr
}

func TestStartControllerSetClosesPartialStartupInReverseOrder(t *testing.T) {
	var closeLog []string
	first := &fakeControllerLifecycle{name: "first", closeLog: &closeLog}
	failing := &fakeControllerLifecycle{startErr: errors.New("start failed"), name: "failing", closeLog: &closeLog}
	third := &fakeControllerLifecycle{}

	if err := startControllerSet([]controllerLifecycle{first, failing, third}, func(int) string { return "controller" }); err == nil {
		t.Fatal("startControllerSet() succeeded despite startup failure")
	}
	if first.starts != 1 || failing.starts != 1 || third.starts != 0 {
		t.Fatalf("start counts = (%d, %d, %d), want (1, 1, 0)", first.starts, failing.starts, third.starts)
	}
	if first.closes != 1 || failing.closes != 1 || third.closes != 0 {
		t.Fatalf("close counts = (%d, %d, %d), want (1, 1, 0)", first.closes, failing.closes, third.closes)
	}
	if len(closeLog) != 2 || closeLog[0] != "failing" || closeLog[1] != "first" {
		t.Fatalf("close order = %v, want [failing first]", closeLog)
	}
}

func TestCloseControllerSetContinuesAfterCloseError(t *testing.T) {
	first := &fakeControllerLifecycle{}
	second := &fakeControllerLifecycle{closeErr: errors.New("close failed")}
	if err := closeControllerSet([]controllerLifecycle{first, second}); err == nil {
		t.Fatal("closeControllerSet() did not return close error")
	}
	if first.closes != 1 || second.closes != 1 {
		t.Fatalf("close counts = (%d, %d), want (1, 1)", first.closes, second.closes)
	}
}
