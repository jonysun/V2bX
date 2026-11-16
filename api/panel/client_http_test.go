package panel

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/InazumaV/V2bX/conf"
)

func newHTTPTestClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()
	client, err := New(&conf.ApiConfig{
		APIHost:  server.URL,
		Key:      "test-token",
		NodeType: "vless",
		NodeID:   7,
		Timeout:  2,
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestGetNodeInfoChecksResponseBeforeCachingBody(t *testing.T) {
	status := http.StatusServiceUnavailable
	body := `{"host":"node.example","server_port":443,"base_config":{"push_interval":20,"pull_interval":30}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()
	client := newHTTPTestClient(t, server)

	if node, err := client.GetNodeInfo(); err == nil || node != nil {
		t.Fatalf("GetNodeInfo() = (%v, %v), want response error", node, err)
	}
	if client.responseBodyHash != "" {
		t.Fatal("failed response body was cached as a valid node configuration")
	}

	status = http.StatusOK
	node, err := client.GetNodeInfo()
	if err != nil {
		t.Fatalf("GetNodeInfo() after recovery: %v", err)
	}
	if node == nil || node.PushInterval.Seconds() != 20 || node.PullInterval.Seconds() != 30 {
		t.Fatalf("unexpected node or intervals: %+v", node)
	}
}

func TestGetNodeInfoETagIsAcknowledgedOnlyAfterCommit(t *testing.T) {
	var seenETags []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenETags = append(seenETags, r.Header.Get("If-None-Match"))
		if r.Header.Get("If-None-Match") == `"node-v2"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"node-v2"`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"host":"node.example","server_port":443,"base_config":{"push_interval":20,"pull_interval":30}}`))
	}))
	defer server.Close()
	client := newHTTPTestClient(t, server)

	if node, err := client.GetNodeInfo(); err != nil || node == nil {
		t.Fatalf("first GetNodeInfo() = (%v, %v), want a candidate snapshot", node, err)
	}
	if client.nodeEtag != "" {
		t.Fatalf("node ETag was acknowledged before apply: %q", client.nodeEtag)
	}
	if node, err := client.GetNodeInfo(); err != nil || node == nil {
		t.Fatalf("unacknowledged GetNodeInfo() = (%v, %v), want candidate to be retried", node, err)
	}
	client.CommitNodeInfo()
	if node, err := client.GetNodeInfo(); err != nil || node != nil {
		t.Fatalf("acknowledged GetNodeInfo() = (%v, %v), want 304/no change", node, err)
	}

	want := []string{"", "", `"node-v2"`}
	if len(seenETags) != len(want) {
		t.Fatalf("seen ETags = %#v, want %#v", seenETags, want)
	}
	for i := range want {
		if seenETags[i] != want[i] {
			t.Fatalf("request %d If-None-Match = %q, want %q", i+1, seenETags[i], want[i])
		}
	}
}

func TestGetNodeInfoUnchangedBodyWithNewETagWaitsForCommit(t *testing.T) {
	var received []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = append(received, r.Header.Get("If-None-Match"))
		switch r.Header.Get("If-None-Match") {
		case `"node-v1"`, `"node-v2"`:
			etag := `"node-v2"`
			if r.Header.Get("If-None-Match") == `"node-v2"` {
				etag = `"node-v3"`
			}
			w.Header().Set("ETag", etag)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"host":"node.example","server_port":443,"base_config":{"push_interval":20,"pull_interval":30}}`))
		case `"node-v3"`:
			w.WriteHeader(http.StatusNotModified)
		default:
			t.Errorf("unexpected If-None-Match: %q", r.Header.Get("If-None-Match"))
		}
	}))
	defer server.Close()
	client := newHTTPTestClient(t, server)
	client.nodeEtag = `"node-v1"`
	node, err := client.GetNodeInfo()
	if err != nil || node == nil {
		t.Fatalf("initial GetNodeInfo() = (%v, %v), want candidate node", node, err)
	}
	client.CommitNodeInfo()

	// Return the same body under a new ETag. Until committed, repeat the fetch
	// with the old ETag; once committed, the new ETag can receive 304.
	if node, err = client.GetNodeInfo(); err != nil || node != nil {
		t.Fatalf("unchanged GetNodeInfo() = (%v, %v), want no apply candidate", node, err)
	}
	if client.nodeEtag != `"node-v2"` {
		t.Fatalf("node ETag advanced before commit: %q", client.nodeEtag)
	}
	if node, err = client.GetNodeInfo(); err != nil || node != nil {
		t.Fatalf("retried unchanged GetNodeInfo() = (%v, %v), want no apply candidate", node, err)
	}
	client.CommitNodeInfo()
	if node, err = client.GetNodeInfo(); err != nil || node != nil {
		t.Fatalf("committed GetNodeInfo() = (%v, %v), want 304/no change", node, err)
	}
	if len(received) != 4 || received[0] != `"node-v1"` || received[1] != `"node-v2"` || received[2] != `"node-v2"` || received[3] != `"node-v3"` {
		t.Fatalf("If-None-Match sequence = %#v, want node-v1, node-v2 twice, then node-v3", received)
	}
}

func TestGetUserListDistinguishesEmptyListFromNotModified(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == `"users-v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"users-v1"`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"users":[]}`))
	}))
	defer server.Close()
	client := newHTTPTestClient(t, server)

	users, err := client.GetUserList()
	if err != nil {
		t.Fatalf("GetUserList() empty response: %v", err)
	}
	if users == nil || len(users) != 0 {
		t.Fatalf("GetUserList() = %#v, want a non-nil empty slice", users)
	}
	client.CommitUserList()

	users, err = client.GetUserList()
	if err != nil {
		t.Fatalf("GetUserList() not-modified response: %v", err)
	}
	if users != nil {
		t.Fatalf("GetUserList() for 304 = %#v, want nil", users)
	}
}

func TestGetUserListETagIsAcknowledgedOnlyAfterCommit(t *testing.T) {
	var seenETags []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenETags = append(seenETags, r.Header.Get("If-None-Match"))
		if r.Header.Get("If-None-Match") == `"users-v2"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"users-v2"`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"users":[{"id":12,"uuid":"user-12"}]}`))
	}))
	defer server.Close()
	client := newHTTPTestClient(t, server)

	if users, err := client.GetUserList(); err != nil || len(users) != 1 {
		t.Fatalf("first GetUserList() = (%v, %v), want one candidate user", users, err)
	}
	if client.userEtag != "" {
		t.Fatalf("user ETag was acknowledged before apply: %q", client.userEtag)
	}
	if users, err := client.GetUserList(); err != nil || len(users) != 1 {
		t.Fatalf("unacknowledged GetUserList() = (%v, %v), want candidate to be retried", users, err)
	}
	client.CommitUserList()
	if users, err := client.GetUserList(); err != nil || users != nil {
		t.Fatalf("acknowledged GetUserList() = (%v, %v), want 304/no change", users, err)
	}

	want := []string{"", "", `"users-v2"`}
	if len(seenETags) != len(want) {
		t.Fatalf("seen ETags = %#v, want %#v", seenETags, want)
	}
	for i := range want {
		if seenETags[i] != want[i] {
			t.Fatalf("request %d If-None-Match = %q, want %q", i+1, seenETags[i], want[i])
		}
	}
}

func TestGetUserAliveReturnsHTTPAndDecodeErrors(t *testing.T) {
	status := http.StatusInternalServerError
	body := `temporarily unavailable`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()
	client := newHTTPTestClient(t, server)

	if alive, err := client.GetUserAlive(); err == nil || alive != nil {
		t.Fatalf("GetUserAlive() = (%v, %v), want HTTP error", alive, err)
	}

	status = http.StatusOK
	body = `not-json`
	if alive, err := client.GetUserAlive(); err == nil || alive != nil {
		t.Fatalf("GetUserAlive() = (%v, %v), want decode error", alive, err)
	}
}

func TestCheckResponseRejectsNilResponse(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	client := newHTTPTestClient(t, server)
	if err := client.checkResponse(nil, "/test", nil); err == nil {
		t.Fatal("checkResponse accepted a nil response")
	}
}
