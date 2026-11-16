package xray

import (
	"github.com/InazumaV/V2bX/api/panel"
	"github.com/InazumaV/V2bX/conf"
	"testing"
)

func TestBuildInboundWithoutTransportSettings(t *testing.T) {
	for _, protocol := range []string{"vless", "vmess", "trojan"} {
		t.Run(protocol, func(t *testing.T) {
			info := &panel.NodeInfo{Type: protocol, Common: &panel.CommonNode{ServerPort: 12345}, VAllss: &panel.VAllssNode{Network: "tcp"}, Trojan: &panel.TrojanNode{}}
			_, err := buildInbound(&conf.Options{ListenIP: "127.0.0.1", XrayOptions: conf.NewXrayOptions()}, info, "n")
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestBuildInboundRejectsInvalidPort(t *testing.T) {
	info := &panel.NodeInfo{Type: "vless", Common: &panel.CommonNode{ServerPort: 65536}, VAllss: &panel.VAllssNode{Network: "tcp"}}
	if _, err := buildInbound(&conf.Options{ListenIP: "127.0.0.1", XrayOptions: conf.NewXrayOptions()}, info, "n"); err == nil {
		t.Fatal("accepted overflowing port")
	}
}
