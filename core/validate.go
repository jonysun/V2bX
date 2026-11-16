package core

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/InazumaV/V2bX/api/panel"
)

func ValidateNodeInfo(info *panel.NodeInfo) error {
	if info == nil || info.Common == nil {
		return fmt.Errorf("node common configuration is required")
	}
	if info.Common.ServerPort < 1 || info.Common.ServerPort > 65535 {
		return fmt.Errorf("server_port must be between 1 and 65535")
	}
	valid := false
	switch info.Type {
	case "vmess", "vless":
		valid = info.VAllss != nil
	case "trojan":
		valid = info.Trojan != nil
	case "shadowsocks":
		valid = info.Shadowsocks != nil
	case "tuic":
		valid = info.Tuic != nil
	case "anytls":
		valid = info.AnyTls != nil
	case "hysteria":
		valid = info.Hysteria != nil
	case "hysteria2":
		valid = info.Hysteria2 != nil
	}
	if !valid {
		return fmt.Errorf("missing or unsupported protocol configuration %q", info.Type)
	}
	if info.Security == panel.Reality && info.Type != "vless" && info.Type != "vmess" {
		return fmt.Errorf("Reality is only mapped for VMess/VLESS")
	}
	return nil
}

// RealityHandshake accepts either dest=host:port or dest=host plus server_port.
// It rejects conflicting ports instead of truncating an integer to uint16.
func RealityHandshake(v *panel.VAllssNode) (host string, port uint16, maxDiff time.Duration, err error) {
	if v == nil {
		err = fmt.Errorf("Reality requires VLESS/VMess configuration")
		return
	}
	host = v.TlsSettings.Dest
	if host == "" {
		host = v.TlsSettings.ServerName
	}
	portText := v.TlsSettings.ServerPort
	if h, p, splitErr := net.SplitHostPort(host); splitErr == nil {
		if portText != "" && portText != p {
			err = fmt.Errorf("Reality dest port conflicts with server_port")
			return
		}
		host, portText = h, p
	}
	host = strings.Trim(host, "[]")
	if host == "" {
		err = fmt.Errorf("Reality handshake host is required")
		return
	}
	if portText == "" {
		portText = "443"
	}
	var parsed int
	parsed, err = strconv.Atoi(portText)
	if err != nil || parsed < 1 || parsed > 65535 {
		err = fmt.Errorf("Reality server_port %q must be between 1 and 65535", portText)
		return
	}
	port = uint16(parsed)
	if v.RealityConfig.MaxTimeDiff != "" {
		maxDiff, err = time.ParseDuration(v.RealityConfig.MaxTimeDiff)
		if err != nil || maxDiff < 0 {
			err = fmt.Errorf("invalid Reality max time difference %q", v.RealityConfig.MaxTimeDiff)
			return
		}
	}
	err = nil
	return
}
