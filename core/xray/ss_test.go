package xray

import (
	"testing"

	"github.com/InazumaV/V2bX/api/panel"
	"github.com/xtls/xray-core/proxy/shadowsocks"
)

func TestGetCipherFromString(t *testing.T) {
	tests := []struct {
		method string
		want   shadowsocks.CipherType
	}{
		{method: "aes-128-gcm", want: shadowsocks.CipherType_AES_128_GCM},
		{method: "aes-256-gcm", want: shadowsocks.CipherType_AES_256_GCM},
		{method: "chacha20-poly1305", want: shadowsocks.CipherType_CHACHA20_POLY1305},
		{method: "xchacha20-poly1305", want: shadowsocks.CipherType_XCHACHA20_POLY1305},
		{method: "none", want: shadowsocks.CipherType_UNKNOWN},
		{method: "plain", want: shadowsocks.CipherType_UNKNOWN},
	}

	for _, test := range tests {
		t.Run(test.method, func(t *testing.T) {
			if got := getCipherFromString(test.method); got != test.want {
				t.Fatalf("getCipherFromString(%q) = %v, want %v", test.method, got, test.want)
			}
		})
	}
}

func TestBuildSSUserCarriesIVCheck(t *testing.T) {
	user := buildSSUser("tag", &panel.UserInfo{Uuid: "password"}, "aes-128-gcm", "", true)
	message, err := user.Account.GetInstance()
	if err != nil {
		t.Fatal(err)
	}
	if !message.(*shadowsocks.Account).IvCheck {
		t.Fatal("dynamic Shadowsocks user did not carry the IV check setting")
	}
}
