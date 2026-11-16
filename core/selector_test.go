package core

import (
	"errors"
	"testing"

	"github.com/InazumaV/V2bX/api/panel"
	"github.com/InazumaV/V2bX/conf"
)

type selectorCore struct {
	Core
	kind     string
	added    int
	startErr error
	starts   int
	closes   int
}

func (c *selectorCore) Type() string                                         { return c.kind }
func (*selectorCore) Protocols() []string                                    { return []string{"vless"} }
func (c *selectorCore) Start() error                                         { c.starts++; return c.startErr }
func (c *selectorCore) Close() error                                         { c.closes++; return nil }
func (c *selectorCore) AddNode(string, *panel.NodeInfo, *conf.Options) error { c.added++; return nil }

func TestSelectorRejectsAmbiguousProtocol(t *testing.T) {
	a, b := &selectorCore{kind: "xray"}, &selectorCore{kind: "sing"}
	s := &Selector{cores: map[string]Core{"a": a, "b": b}}
	if err := s.AddNode("n", &panel.NodeInfo{Type: "vless"}, &conf.Options{}); err == nil {
		t.Fatal("ambiguous core accepted")
	}
	if a.added+b.added != 0 {
		t.Fatal("ambiguous selection mutated a core")
	}
	if err := s.AddNode("n", &panel.NodeInfo{Type: "vless"}, &conf.Options{CoreName: "b", Core: "sing"}); err != nil {
		t.Fatal(err)
	}
	if b.added != 1 {
		t.Fatal("explicit core was not selected")
	}
}

func TestSelectorRejectsDuplicateAliases(t *testing.T) {
	const kind = "selector-test"
	RegisterCore(kind, func(*conf.CoreConfig) (Core, error) { return &selectorCore{kind: kind}, nil })
	defer delete(cores, kind)
	if _, err := NewSelector([]conf.CoreConfig{{Type: kind, Name: "same"}, {Type: kind, Name: "same"}}); err == nil {
		t.Fatal("duplicate aliases accepted")
	}
}

func TestSelectorStartClosesFailedAndPreviouslyStartedCores(t *testing.T) {
	first := &selectorCore{kind: "first"}
	failing := &selectorCore{kind: "second", startErr: errors.New("start failed")}
	third := &selectorCore{kind: "third"}
	selector := &Selector{cores: map[string]Core{"a-first": first, "b-second": failing, "c-third": third}}

	if err := selector.Start(); err == nil {
		t.Fatal("Start() succeeded despite a core startup failure")
	}
	if first.starts != 1 || failing.starts != 1 || third.starts != 0 {
		t.Fatalf("start counts = (%d, %d, %d), want (1, 1, 0)", first.starts, failing.starts, third.starts)
	}
	if first.closes != 1 || failing.closes != 1 || third.closes != 1 {
		t.Fatalf("close counts = (%d, %d, %d), want (1, 1, 1)", first.closes, failing.closes, third.closes)
	}
}
