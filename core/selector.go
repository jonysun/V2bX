package core

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/InazumaV/V2bX/api/panel"
	"github.com/InazumaV/V2bX/conf"
)

type Selector struct {
	cores map[string]Core
	nodes sync.Map
}

func NewSelector(c []conf.CoreConfig) (Core, error) {
	cs := make(map[string]Core, len(c))
	cleanup := func() {
		for _, existing := range cs {
			_ = existing.Close()
		}
	}
	for _, t := range c {
		name := t.Name
		if name == "" {
			name = strings.ToLower(t.Type)
		}
		if _, exists := cs[name]; exists {
			cleanup()
			return nil, fmt.Errorf("duplicate core name %q", name)
		}
		f, ok := cores[strings.ToLower(t.Type)]
		if !ok {
			cleanup()
			return nil, errors.New("unknown core type: " + t.Type)
		}
		core1, err := f(&t)
		if err != nil {
			cleanup()
			return nil, err
		}
		cs[name] = core1
	}
	return &Selector{
		cores: cs,
	}, nil
}

func (s *Selector) Start() error {
	names := make([]string, 0, len(s.cores))
	for name := range s.cores {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := s.cores[name].Start(); err != nil {
			var errs []error
			errs = append(errs, fmt.Errorf("start core %q: %w", name, err))
			for i := len(names) - 1; i >= 0; i-- {
				nameToClose := names[i]
				if closeErr := s.cores[nameToClose].Close(); closeErr != nil {
					errs = append(errs, fmt.Errorf("close core %q after startup failure: %w", nameToClose, closeErr))
				}
			}
			return errors.Join(errs...)
		}
	}
	return nil
}

func (s *Selector) Close() error {
	var errs []error
	for i := range s.cores {
		if err := s.cores[i].Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func isSupported(protocol string, protocols []string) bool {
	for i := range protocols {
		if protocol == protocols[i] {
			return true
		}
	}
	return false
}

func (s *Selector) AddNode(tag string, info *panel.NodeInfo, option *conf.Options) error {
	if _, exists := s.nodes.Load(tag); exists {
		return fmt.Errorf("duplicate node tag %q", tag)
	}
	var core Core
	if len(option.CoreName) > 0 {
		// use name to select core
		if c, ok := s.cores[option.CoreName]; ok {
			core = c
			if core != nil && !isSupported(info.Type, core.Protocols()) {
				return fmt.Errorf("core %q does not support %q", option.CoreName, info.Type)
			}
		}
	} else {
		// use type to select core
		for _, c := range s.cores {
			if len(option.Core) == 0 {
				if !isSupported(info.Type, c.Protocols()) {
					continue
				}
			} else if option.Core != c.Type() {
				continue
			}
			if !isSupported(info.Type, c.Protocols()) {
				continue
			}
			if core != nil {
				return fmt.Errorf("multiple cores support %q; set CoreName explicitly", info.Type)
			}
			core = c
		}
	}
	if core == nil {
		return errors.New("the node type is not support")
	}
	if len(option.Core) == 0 {
		option.Core = core.Type()
		err := option.UnmarshalJSON(option.RawOptions)
		if err != nil {
			return fmt.Errorf("unmarshal option error: %s", err)
		}
		option.RawOptions = nil
	}
	err := core.AddNode(tag, info, option)
	if err != nil {
		return err
	}
	s.nodes.Store(tag, core)
	return nil
}

func (s *Selector) DelNode(tag string) error {
	if t, e := s.nodes.Load(tag); e {
		err := t.(Core).DelNode(tag)
		if err != nil {
			return err
		}
		s.nodes.Delete(tag)
		return nil
	}
	return nil
}

func (s *Selector) AddUsers(p *AddUsersParams) (added int, err error) {
	t, e := s.nodes.Load(p.Tag)
	if !e {
		return 0, errors.New("the node is not have")
	}
	return t.(Core).AddUsers(p)
}

func (s *Selector) GetUserTrafficSlice(tag string, reset bool) ([]panel.UserTraffic, error) {
	t, e := s.nodes.Load(tag)
	if !e {
		return nil, errors.New("the node is not have")
	}
	return t.(Core).GetUserTrafficSlice(tag, reset)
}

func (s *Selector) DrainTraffic(tag string) ([]panel.UserTraffic, error) {
	t, exists := s.nodes.Load(tag)
	if !exists {
		return nil, nil
	}
	if d, ok := t.(TrafficDrainer); ok {
		return d.DrainTraffic(tag)
	}
	return t.(Core).GetUserTrafficSlice(tag, true)
}

func (s *Selector) DelUsers(users []panel.UserInfo, tag string, info *panel.NodeInfo) error {
	t, e := s.nodes.Load(tag)
	if !e {
		return errors.New("the node is not have")
	}
	return t.(Core).DelUsers(users, tag, info)
}

func (s *Selector) Protocols() []string {
	protocols := make([]string, 0)
	for i := range s.cores {
		protocols = append(protocols, s.cores[i].Protocols()...)
	}
	return protocols
}

func (s *Selector) Type() string {
	t := "Selector("
	var flag bool
	for n, c := range s.cores {
		if flag {
			t += " "
		} else {
			flag = true
		}
		if len(n) == 0 {
			t += c.Type()
		} else {
			t += n
		}
	}
	t += ")"
	return t
}
