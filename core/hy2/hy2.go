package hy2

import (
	"errors"
	"github.com/InazumaV/V2bX/conf"
	vCore "github.com/InazumaV/V2bX/core"
	"go.uber.org/zap"
	"sync"
)

var _ vCore.Core = (*Hysteria2)(nil)

type Hysteria2 struct {
	mu       sync.RWMutex
	Hy2nodes map[string]Hysteria2node
	Logger   *zap.Logger
}

func init() {
	vCore.RegisterCore("hysteria2", New)
}

func New(c *conf.CoreConfig) (vCore.Core, error) {
	loglever := "error"
	if c.Hysteria2Config.LogConfig.Level != "" {
		loglever = c.Hysteria2Config.LogConfig.Level
	}
	log, err := initLogger(loglever, "console")
	if err != nil {
		return nil, err
	}
	return &Hysteria2{
		Hy2nodes: make(map[string]Hysteria2node),
		Logger:   log,
	}, nil
}

func (h *Hysteria2) Protocols() []string {
	return []string{
		"hysteria2",
	}
}

func (h *Hysteria2) Start() error {
	return nil
}

func (h *Hysteria2) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	var errs []error
	for tag, n := range h.Hy2nodes {
		errs = append(errs, n.close())
		delete(h.Hy2nodes, tag)
	}
	return errors.Join(errs...)
}

func (h *Hysteria2) Type() string {
	return "hysteria2"
}
