package conf_test

import (
	"testing"

	. "github.com/xtls/xray-core/infra/conf"
	"github.com/xtls/xray-core/proxy/shadowsocks"
)

func TestShadowsocksIVCheckConfigCompatibility(t *testing.T) {
	built, err := (&ShadowsocksServerConfig{
		Users: []*ShadowsocksUserConfig{{
			Cipher:   "aes-128-gcm",
			Password: "test-password",
		}},
		NetworkList: &NetworkList{"tcp", "udp"},
		IVCheck:     true,
	}).Build()
	if err != nil {
		t.Fatal(err)
	}

	serverConfig, ok := built.(*shadowsocks.ServerConfig)
	if !ok || len(serverConfig.Users) != 1 {
		t.Fatalf("unexpected Shadowsocks config: %#v", built)
	}
	decoded, err := serverConfig.Users[0].Account.GetInstance()
	if err != nil {
		t.Fatal(err)
	}
	if !decoded.(*shadowsocks.Account).IvCheck {
		t.Fatal("IV check setting was not propagated to the user account")
	}
}
