package inbound

import (
	"context"
	"net"
	"testing"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
)

type managerTestInbound struct {
	tag                string
	started            []adapter.StartStage
	cleanupCalls       int
	listenAddress      string
	listener           net.Listener
	listenerCloseCalls int
}

func (i *managerTestInbound) Type() string { return "test" }
func (i *managerTestInbound) Tag() string  { return i.tag }

func (i *managerTestInbound) Start(stage adapter.StartStage, scope *adapter.Scope) error {
	i.started = append(i.started, stage)
	if i.listenAddress != "" && stage == adapter.StartStateStarted {
		listener, err := net.Listen("tcp", i.listenAddress)
		if err != nil {
			return err
		}
		i.listener = listener
		scope.Add(func() error {
			i.listenerCloseCalls++
			return listener.Close()
		})
	}
	scope.Add(func() error {
		i.cleanupCalls++
		return nil
	})
	return nil
}

type managerTestRegistry struct {
	option.InboundOptionsRegistry
	inbound adapter.Inbound
}

func (r *managerTestRegistry) Create(_ context.Context, _ adapter.Router, _ log.ContextLogger, tag string, _ string, _ any) (adapter.Inbound, error) {
	r.inbound.(*managerTestInbound).tag = tag
	return r.inbound, nil
}

func TestStartedManagerCreatesAndRemovesInbound(t *testing.T) {
	inbound := &managerTestInbound{}
	manager := NewManager(&managerTestRegistry{inbound: inbound}, nil)
	parentScope := adapter.NewScope(context.Background(), log.NewNOPFactory().NewLogger("inbound-manager-test"))
	if err := manager.Start(adapter.StartStateStarted, parentScope); err != nil {
		t.Fatal(err)
	}

	if err := manager.Create(context.Background(), nil, nil, "test-tag", "", nil); err != nil {
		t.Fatal(err)
	}
	if len(inbound.started) != len(adapter.ListStartStages) {
		t.Fatalf("inbound started at %d stages, want %d", len(inbound.started), len(adapter.ListStartStages))
	}
	if got, found := manager.Get("test-tag"); !found || got != inbound {
		t.Fatal("created inbound is not registered by tag")
	}

	if err := manager.Remove("test-tag"); err != nil {
		t.Fatal(err)
	}
	if len(manager.Inbounds()) != 0 {
		t.Fatal("removed inbound remains in the manager")
	}
	if inbound.cleanupCalls != len(adapter.ListStartStages) {
		t.Fatalf("cleanup calls=%d, want %d", inbound.cleanupCalls, len(adapter.ListStartStages))
	}
	if err := parentScope.Close(); err != nil {
		t.Fatal(err)
	}
	if inbound.cleanupCalls != len(adapter.ListStartStages) {
		t.Fatal("parent scope repeated cleanup after inbound removal")
	}
}

func TestStartedManagerClosesInboundListenerOnRemove(t *testing.T) {
	reservation, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	listenAddress := reservation.Addr().String()
	if err := reservation.Close(); err != nil {
		t.Fatal(err)
	}

	inbound := &managerTestInbound{listenAddress: listenAddress}
	manager := NewManager(&managerTestRegistry{inbound: inbound}, nil)
	parentScope := adapter.NewScope(context.Background(), log.NewNOPFactory().NewLogger("inbound-listener-test"))
	if err := manager.Start(adapter.StartStateStarted, parentScope); err != nil {
		t.Fatal(err)
	}
	if err := manager.Create(context.Background(), nil, nil, "listening-tag", "", nil); err != nil {
		t.Fatal(err)
	}
	if inbound.listener == nil {
		t.Fatal("dynamic inbound did not create its listener")
	}
	if duplicate, err := net.Listen("tcp", listenAddress); err == nil {
		_ = duplicate.Close()
		t.Fatal("dynamic inbound listener is not bound")
	}

	if err := manager.Remove("listening-tag"); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", listenAddress)
	if err != nil {
		t.Fatalf("listener remained bound after inbound removal: %v", err)
	}
	_ = listener.Close()
	if inbound.listenerCloseCalls != 1 {
		t.Fatalf("listener close calls=%d, want 1", inbound.listenerCloseCalls)
	}
	if err := parentScope.Close(); err != nil {
		t.Fatal(err)
	}
	if inbound.listenerCloseCalls != 1 {
		t.Fatalf("parent scope closed removed listener again: calls=%d", inbound.listenerCloseCalls)
	}
}
