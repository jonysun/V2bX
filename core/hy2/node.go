package hy2

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/InazumaV/V2bX/api/panel"
	"github.com/InazumaV/V2bX/conf"
	vCore "github.com/InazumaV/V2bX/core"
	"github.com/apernet/hysteria/core/v2/server"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

type Hysteria2node struct {
	Auth          *V2bX
	conn          net.PacketConn
	masquerade    []*http.Server
	listeners     []net.Listener
	Hy2server     server.Server
	Tag           string
	Logger        *zap.Logger
	EventLogger   server.EventLogger
	TrafficLogger server.TrafficLogger
}

func (n *Hysteria2node) close() error {
	var errs []error
	if n.Hy2server != nil {
		errs = append(errs, n.Hy2server.Close())
	}
	for _, s := range n.masquerade {
		errs = append(errs, s.Close())
	}
	for _, l := range n.listeners {
		_ = l.Close()
	}
	if n.conn != nil {
		_ = n.conn.Close()
	}
	return errors.Join(errs...)
}

func (h *Hysteria2) AddNode(tag string, info *panel.NodeInfo, config *conf.Options) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, exists := h.Hy2nodes[tag]; exists {
		return fmt.Errorf("duplicate node %q", tag)
	}
	if err := vCore.ValidateNodeInfo(info); err != nil {
		return err
	}
	if len(info.Rules.Protocol) > 0 {
		return fmt.Errorf("standalone Hysteria2 cannot enforce panel protocol sniffing rules; use sing core or remove these rules")
	}
	if config.LimitConfig.ConnLimit > 0 || config.LimitConfig.IPLimit > 0 || config.LimitConfig.EnableRealtime || config.LimitConfig.EnableIpRecorder {
		return fmt.Errorf("standalone Hysteria2 does not support ConnLimit, node DeviceLimit, realtime or external IP recorder policies")
	}
	var err error
	hyconfig := &server.Config{}
	var c serverConfig
	v := viper.New()
	if len(config.Hysteria2ConfigPath) != 0 {
		v.SetConfigFile(config.Hysteria2ConfigPath)
		if err := v.ReadInConfig(); err != nil {
			return fmt.Errorf("failed to read server config: %w", err)
		}
		if err := v.Unmarshal(&c); err != nil {
			return fmt.Errorf("failed to parse server config: %w", err)
		}
	}
	n := Hysteria2node{
		Auth:   &V2bX{Tag: tag, usersMap: make(map[string]int)},
		Tag:    tag,
		Logger: h.Logger,
		EventLogger: &serverLogger{
			Tag:    tag,
			logger: h.Logger,
		},
		TrafficLogger: &HookServer{
			Tag:                   tag,
			logger:                h.Logger,
			ReportMinTrafficBytes: config.ReportMinTraffic * 1024,
		},
	}

	hyconfig, err = n.getHyConfig(info, config, &c)
	if err != nil {
		_ = n.close()
		return err
	}
	hyconfig.Authenticator = n.Auth
	s, err := server.NewServer(hyconfig)
	if err != nil {
		_ = n.close()
		return err
	}
	n.Hy2server = s
	h.Hy2nodes[tag] = n
	go func() {
		if err := s.Serve(); err != nil {
			if !strings.Contains(err.Error(), "quic: server closed") {
				h.Logger.Error("Server Error", zap.Error(err))
			}
		}
	}()
	return nil
}

func (h *Hysteria2) DelNode(tag string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	n, ok := h.Hy2nodes[tag]
	if !ok {
		return nil
	}
	err := n.close()
	if err != nil {
		return err
	}
	delete(h.Hy2nodes, tag)
	return nil
}
