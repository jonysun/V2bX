package hy2

import (
	"fmt"
	"net"

	"github.com/InazumaV/V2bX/common/format"
	"github.com/InazumaV/V2bX/limiter"
	"github.com/apernet/hysteria/core/v2/server"
)

type policyOutbound struct {
	server.Outbound
	tag string
}

func (p *policyOutbound) check(info *server.RequestInfo, address string, udp bool) error {
	l, err := limiter.GetLimiter(p.tag)
	if err != nil {
		return fmt.Errorf("node policy unavailable: %w", err)
	}
	if _, rejected := l.CheckLimit(format.UserTag(p.tag, info.AuthID), extractIPFromAddr(info.ClientAddr), !udp, true); rejected {
		return fmt.Errorf("user policy rejected request")
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}
	if l.CheckDomainRule(host) {
		return fmt.Errorf("destination rejected by panel domain rule")
	}
	return nil
}

func (p *policyOutbound) TCP(info *server.RequestInfo, address string) (net.Conn, error) {
	if err := p.check(info, address, false); err != nil {
		return nil, err
	}
	return p.Outbound.TCP(info, address)
}

func (p *policyOutbound) UDP(info *server.RequestInfo, address string) (server.UDPConn, error) {
	if err := p.check(info, address, true); err != nil {
		return nil, err
	}
	return p.Outbound.UDP(info, address)
}

func (p *policyOutbound) CheckUDP(info *server.RequestInfo, address string) error {
	if err := p.check(info, address, true); err != nil {
		return err
	}
	return p.Outbound.CheckUDP(info, address)
}
