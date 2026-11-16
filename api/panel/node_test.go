package panel

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientGetNodeInfoUsesAppliedETag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/server/UniProxy/config" {
			t.Errorf("unexpected request path %q", r.URL.Path)
		}
		if r.Header.Get("If-None-Match") == `"node-v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"node-v1"`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"host":"node.example","server_port":443,"base_config":{"push_interval":20,"pull_interval":30}}`))
	}))
	defer server.Close()
	client := newHTTPTestClient(t, server)

	first, err := client.GetNodeInfo()
	if err != nil || first == nil {
		t.Fatalf("first GetNodeInfo() = (%v, %v), want node config", first, err)
	}
	client.CommitNodeInfo()
	second, err := client.GetNodeInfo()
	if err != nil || second != nil {
		t.Fatalf("second GetNodeInfo() = (%v, %v), want 304/no change", second, err)
	}
}

func TestClientReportUserTrafficPostsPanelPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/server/UniProxy/push" || r.Method != http.MethodPost {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var payload map[string][]int64
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode push payload: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if got := payload["10372"]; len(got) != 2 || got[0] != 1000 || got[1] != 2000 {
			t.Errorf("push payload for user 10372 = %v, want [1000 2000]", got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	client := newHTTPTestClient(t, server)

	if err := client.ReportUserTraffic([]UserTraffic{{UID: 10372, Upload: 1000, Download: 2000}}); err != nil {
		t.Fatalf("ReportUserTraffic() error: %v", err)
	}
}
