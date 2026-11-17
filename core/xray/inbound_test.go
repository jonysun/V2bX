package xray

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/InazumaV/V2bX/api/panel"
	"github.com/InazumaV/V2bX/conf"
	coreConf "github.com/xtls/xray-core/infra/conf"
	shadowsocks2022 "github.com/xtls/xray-core/proxy/shadowsocks_2022"
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

func TestBuildShadowsocks2022UsesValidPlaceholderUserKey(t *testing.T) {
	tests := []struct {
		cipher   string
		keyBytes int
	}{
		{cipher: "2022-blake3-aes-128-gcm", keyBytes: 16},
		{cipher: "2022-blake3-aes-256-gcm", keyBytes: 32},
	}

	for _, test := range tests {
		t.Run(test.cipher, func(t *testing.T) {
			serverKey := base64.StdEncoding.EncodeToString(make([]byte, test.keyBytes))
			info := &panel.NodeInfo{
				Type:   "shadowsocks",
				Common: &panel.CommonNode{ServerPort: 8388},
				Shadowsocks: &panel.ShadowsocksNode{
					Cipher:    test.cipher,
					ServerKey: serverKey,
				},
			}
			inbound := &coreConf.InboundDetourConfig{}
			options := &conf.Options{XrayOptions: conf.NewXrayOptions()}
			if err := buildShadowsocks(options, info, inbound); err != nil {
				t.Fatal(err)
			}

			var settings coreConf.ShadowsocksServerConfig
			if err := json.Unmarshal(*inbound.Settings, &settings); err != nil {
				t.Fatal(err)
			}
			built, err := settings.Build()
			if err != nil {
				t.Fatal(err)
			}
			multiUser, ok := built.(*shadowsocks2022.MultiUserServerConfig)
			if !ok {
				t.Fatalf("Build() returned %T, want *MultiUserServerConfig", built)
			}
			if _, err := shadowsocks2022.ParseKey(multiUser.Key, test.keyBytes); err != nil {
				t.Fatalf("server key rejected: %v", err)
			}
			if len(multiUser.Users) != 1 {
				t.Fatalf("placeholder user count = %d, want 1", len(multiUser.Users))
			}
			memoryUser, err := multiUser.Users[0].ToMemoryUser()
			if err != nil {
				t.Fatal(err)
			}
			account, ok := memoryUser.Account.(*shadowsocks2022.MemoryAccount)
			if !ok {
				t.Fatalf("placeholder account type = %T, want *MemoryAccount", memoryUser.Account)
			}
			if len(account.Key) != test.keyBytes {
				t.Fatalf("placeholder key length = %d, want %d", len(account.Key), test.keyBytes)
			}
		})
	}
}
