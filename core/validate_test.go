package core

import (
	"github.com/InazumaV/V2bX/api/panel"
	"testing"
	"time"
)

func TestRealityHandshakeValidation(t *testing.T) {
	for _, tc := range []struct {
		dest, port, diff string
		invalid          bool
	}{
		{"example.org", "443", "1s", false}, {"[::1]:8443", "", "", false},
		{"example.org", "65536", "", true}, {"example.org", "-1", "", true},
		{"example.org", "443", "garbage", true}, {"example.org", "443", "-1s", true},
		{"example.org:8443", "443", "", true},
	} {
		t.Run(tc.dest+tc.port+tc.diff, func(t *testing.T) {
			v := &panel.VAllssNode{TlsSettings: panel.TlsSettings{Dest: tc.dest, ServerPort: tc.port}, RealityConfig: panel.RealityConfig{MaxTimeDiff: tc.diff}}
			_, port, diff, err := RealityHandshake(v)
			if (err != nil) != tc.invalid {
				t.Fatalf("port=%d diff=%s error=%v", port, diff, err)
			}
			if tc.diff == "1s" && diff != time.Second {
				t.Fatalf("duration = %s", diff)
			}
		})
	}
}
