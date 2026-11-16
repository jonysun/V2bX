package adapter

import (
	"context"
	"testing"

	"github.com/sagernet/sing-box/log"
)

type scopeTestLifecycle struct {
	starts       int
	cleanupCalls int
}

func (c *scopeTestLifecycle) Start(_ StartStage, scope *Scope) error {
	c.starts++
	scope.Add(func() error {
		c.cleanupCalls++
		return nil
	})
	return nil
}

func TestScopeStopClosesComponentAndRemovesParentCleanup(t *testing.T) {
	logger := log.NewNOPFactory().NewLogger("scope-test")
	scope := NewScope(context.Background(), logger)
	component := &scopeTestLifecycle{}
	if err := scope.Start("test", component, StartStateInitialize); err != nil {
		t.Fatal(err)
	}
	if err := scope.Start("test", component, StartStateStart); err != nil {
		t.Fatal(err)
	}
	if err := scope.Stop(component); err != nil {
		t.Fatal(err)
	}
	if component.starts != 2 || component.cleanupCalls != 2 {
		t.Fatalf("starts=%d cleanup calls=%d, want 2 each", component.starts, component.cleanupCalls)
	}
	if err := scope.Close(); err != nil {
		t.Fatal(err)
	}
	if component.cleanupCalls != 2 {
		t.Fatalf("parent scope repeated stopped cleanup %d times", component.cleanupCalls-2)
	}
}
