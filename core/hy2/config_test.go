package hy2

import (
	"testing"
)

func TestGetQUICConfigMapsInitialStreamWindowAndMTUSetting(t *testing.T) {
	got, err := (&Hysteria2node{}).getQUICConfig(&serverConfig{
		QUIC: serverConfigQUIC{
			InitStreamReceiveWindow:     32768,
			InitConnectionReceiveWindow: 65536,
			DisablePathMTUDiscovery:     true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.InitialStreamReceiveWindow != 32768 {
		t.Fatalf("initial stream window=%d, want 32768", got.InitialStreamReceiveWindow)
	}
	if got.InitialConnectionReceiveWindow != 65536 {
		t.Fatalf("initial connection window=%d, want 65536", got.InitialConnectionReceiveWindow)
	}
	if !got.DisablePathMTUDiscovery {
		t.Fatal("DisablePathMTUDiscovery option was ignored")
	}
}
