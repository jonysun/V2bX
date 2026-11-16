package inbound

import (
	"context"
	"os"
	"sync"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/log"
	E "github.com/sagernet/sing/common/exceptions"
)

var _ adapter.InboundManager = (*Manager)(nil)

type Manager struct {
	registry     adapter.InboundRegistry
	endpoint     adapter.EndpointManager
	access       sync.Mutex
	lifecycle    sync.Mutex
	scope        *adapter.Scope
	started      bool
	stage        adapter.StartStage
	inbounds     []adapter.Inbound
	inboundByTag map[string]adapter.Inbound
}

func NewManager(registry adapter.InboundRegistry, endpoint adapter.EndpointManager) *Manager {
	return &Manager{
		registry:     registry,
		endpoint:     endpoint,
		inboundByTag: make(map[string]adapter.Inbound),
	}
}

func (m *Manager) Start(stage adapter.StartStage, scope *adapter.Scope) error {
	m.lifecycle.Lock()
	defer m.lifecycle.Unlock()
	m.scope = scope
	m.started = true
	if stage > m.stage {
		m.stage = stage
	}
	m.access.Lock()
	inbounds := append([]adapter.Inbound(nil), m.inbounds...)
	m.access.Unlock()
	for _, inbound := range inbounds {
		name := "inbound/" + inbound.Type() + "[" + inbound.Tag() + "]"
		err := scope.Start(name, inbound, stage)
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) Inbounds() []adapter.Inbound {
	m.access.Lock()
	defer m.access.Unlock()
	return m.inbounds
}

func (m *Manager) Get(tag string) (adapter.Inbound, bool) {
	m.access.Lock()
	inbound, found := m.inboundByTag[tag]
	m.access.Unlock()
	if found {
		return inbound, true
	}
	return m.endpoint.Get(tag)
}

func (m *Manager) Remove(tag string) error {
	m.lifecycle.Lock()
	defer m.lifecycle.Unlock()

	m.access.Lock()
	inbound, found := m.inboundByTag[tag]
	if !found {
		m.access.Unlock()
		return os.ErrInvalid
	}
	delete(m.inboundByTag, tag)
	for i, current := range m.inbounds {
		if current == inbound {
			m.inbounds = append(m.inbounds[:i], m.inbounds[i+1:]...)
			break
		}
	}
	scope := m.scope
	m.access.Unlock()
	if scope != nil {
		return scope.Stop(inbound)
	}
	return nil
}

func (m *Manager) Create(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, outboundType string, options any) error {
	m.lifecycle.Lock()
	defer m.lifecycle.Unlock()

	inbound, err := m.registry.Create(ctx, router, logger, tag, outboundType, options)
	if err != nil {
		return err
	}
	m.access.Lock()
	_, loaded := m.inboundByTag[tag]
	scope := m.scope
	started := m.started
	stage := m.stage
	m.access.Unlock()
	if loaded {
		return E.New("duplicate inbound tag: ", tag)
	}
	if started {
		for currentStage := adapter.StartStateInitialize; currentStage <= stage; currentStage++ {
			if err = scope.Start("inbound/"+inbound.Type()+"["+inbound.Tag()+"]", inbound, currentStage); err != nil {
				_ = scope.Stop(inbound)
				return err
			}
		}
	}
	m.access.Lock()
	m.inbounds = append(m.inbounds, inbound)
	m.inboundByTag[tag] = inbound
	m.access.Unlock()
	return nil
}
