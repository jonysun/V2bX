package sing

import (
	"encoding/json"
	"github.com/InazumaV/V2bX/api/panel"
	"github.com/InazumaV/V2bX/conf"
	"github.com/sagernet/sing-box/option"
	"testing"
)

func TestTransportValidationAndEmptyHTTPPath(t *testing.T) {
	for _, tc := range []struct {
		network, settings string
		invalid           bool
	}{
		{"tcp", `{"header":{"type":"http","request":{"path":[]}}}`, false},
		{"ws", `{"path":"/test?ed=-1"}`, true},
		{"ws", `{"path":"/test?ed=abc"}`, true},
		{"ws", `{"path":"/test?ed=4294967296"}`, true},
		{"xhttp", `{}`, true}, {"httpupgrade", `{"path":"/upgrade"}`, false},
	} {
		t.Run(tc.network+tc.settings, func(t *testing.T) {
			info := &panel.NodeInfo{Type: "vless", Common: &panel.CommonNode{ServerPort: 443}, VAllss: &panel.VAllssNode{Network: tc.network, NetworkSettings: json.RawMessage(tc.settings)}}
			_, err := getInboundOptions("n", info, &conf.Options{ListenIP: "127.0.0.1", SingOptions: conf.NewSingOptions()})
			if (err != nil) != tc.invalid {
				t.Fatalf("error=%v, invalid=%v", err, tc.invalid)
			}
		})
	}
}

func TestRealityDestIncludesPort(t *testing.T) {
	info := &panel.NodeInfo{Type: "vless", Security: panel.Reality, Common: &panel.CommonNode{ServerPort: 443}, VAllss: &panel.VAllssNode{Network: "tcp", TlsSettings: panel.TlsSettings{Dest: "example.org:8443"}}}
	in, err := getInboundOptions("n", info, &conf.Options{ListenIP: "127.0.0.1", SingOptions: conf.NewSingOptions()})
	if err != nil {
		t.Fatal(err)
	}
	tls := in.Options.(*option.VLESSInboundOptions).TLS
	if tls.Reality.Handshake.Server != "example.org" || tls.Reality.Handshake.ServerPort != 8443 {
		t.Fatalf("bad Reality handshake: %+v", tls.Reality.Handshake)
	}
}
