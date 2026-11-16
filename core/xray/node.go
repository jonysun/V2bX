package xray

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/InazumaV/V2bX/api/panel"
	"github.com/InazumaV/V2bX/conf"
	"github.com/InazumaV/V2bX/core/xray/app/dispatcher"
	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/features/inbound"
	"github.com/xtls/xray-core/features/outbound"
)

type DNSConfig struct {
	Servers []interface{} `json:"servers"`
	Tag     string        `json:"tag"`
}

func (c *Xray) AddNode(tag string, info *panel.NodeInfo, config *conf.Options) error {
	c.access.Lock()
	defer c.access.Unlock()
	if _, exists := c.nodeReportMinTrafficBytes[tag]; exists {
		return fmt.Errorf("duplicate node %q", tag)
	}
	err := updateDNSConfig(info)
	if err != nil {
		return fmt.Errorf("build dns error: %s", err)
	}
	inboundConfig, err := buildInbound(config, info, tag)
	if err != nil {
		return fmt.Errorf("build inbound error: %s", err)
	}
	outBoundConfig, err := buildOutbound(config, tag)
	if err != nil {
		return fmt.Errorf("build outbound error: %s", err)
	}
	if err := c.addInbound(inboundConfig); err != nil {
		return fmt.Errorf("add inbound: %w", err)
	}
	err = c.addOutbound(outBoundConfig)
	if err != nil {
		return errors.Join(fmt.Errorf("add outbound: %w", err), c.removeInbound(tag))
	}
	c.nodeReportMinTrafficBytes[tag] = config.ReportMinTraffic * 1024
	c.ivCheck.Store(tag, !config.XrayOptions.DisableIVCheck)
	return nil
}

func (c *Xray) addInbound(config *core.InboundHandlerConfig) error {
	rawHandler, err := core.CreateObject(c.Server, config)
	if err != nil {
		return err
	}
	handler, ok := rawHandler.(inbound.Handler)
	if !ok {
		return fmt.Errorf("not an InboundHandler: %s", err)
	}
	if err := c.ihm.AddHandler(context.Background(), handler); err != nil {
		_ = handler.Close()
		return err
	}
	return nil
}

func (c *Xray) addOutbound(config *core.OutboundHandlerConfig) error {
	rawHandler, err := core.CreateObject(c.Server, config)
	if err != nil {
		return err
	}
	handler, ok := rawHandler.(outbound.Handler)
	if !ok {
		return fmt.Errorf("not an InboundHandler: %s", err)
	}
	if err := c.ohm.AddHandler(context.Background(), handler); err != nil {
		_ = handler.Close()
		return err
	}
	return nil
}

func (c *Xray) DelNode(tag string) error {
	c.access.Lock()
	defer c.access.Unlock()
	var errs []error
	if _, err := c.ihm.GetHandler(context.Background(), tag); err == nil {
		errs = append(errs, c.removeInbound(tag))
	}
	if c.ohm.GetHandler(tag) != nil {
		errs = append(errs, c.removeOutbound(tag))
	}
	if err := errors.Join(errs...); err != nil {
		return err
	}
	c.ivCheck.Delete(tag)
	delete(c.nodeReportMinTrafficBytes, tag)
	c.users.mapLock.Lock()
	defer c.users.mapLock.Unlock()
	for user := range c.users.uidMap {
		if !strings.HasPrefix(user, tag+"|") {
			continue
		}
		delete(c.users.uidMap, user)
		if value, ok := c.dispatcher.LinkManagers.LoadAndDelete(user); ok {
			value.(*dispatcher.LinkManager).CloseAll()
		}
	}
	c.dispatcher.Counter.Delete(tag)
	return nil
}

func (c *Xray) removeInbound(tag string) error {
	return c.ihm.RemoveHandler(context.Background(), tag)
}

func (c *Xray) removeOutbound(tag string) error {
	err := c.ohm.RemoveHandler(context.Background(), tag)
	return err
}
