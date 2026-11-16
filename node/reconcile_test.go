package node

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/InazumaV/V2bX/api/panel"
	"github.com/InazumaV/V2bX/conf"
	vCore "github.com/InazumaV/V2bX/core"
	"github.com/InazumaV/V2bX/limiter"
)

type faultCore struct {
	node                                *panel.NodeInfo
	users                               []panel.UserInfo
	failAdd, failUsers, failDeleteUsers bool
	traffic                             []panel.UserTraffic
	onDelete                            func()
}

func (*faultCore) Start() error        { return nil }
func (*faultCore) Close() error        { return nil }
func (*faultCore) Protocols() []string { return []string{"vless"} }
func (*faultCore) Type() string        { return "fake" }
func (f *faultCore) AddNode(_ string, n *panel.NodeInfo, _ *conf.Options) error {
	if f.failAdd {
		f.failAdd = false
		return errors.New("injected add failure")
	}
	f.node = n
	return nil
}
func (f *faultCore) DelNode(string) error {
	if f.onDelete != nil {
		f.onDelete()
	}
	f.node = nil
	f.users = nil
	return nil
}
func (f *faultCore) AddUsers(p *vCore.AddUsersParams) (int, error) {
	if f.failUsers {
		f.failUsers = false
		return 0, errors.New("injected user failure")
	}
	f.users = append(f.users, p.Users...)
	return len(p.Users), nil
}
func (f *faultCore) DelUsers(_ []panel.UserInfo, _ string, _ *panel.NodeInfo) error {
	f.users = nil
	if f.failDeleteUsers {
		f.failDeleteUsers = false
		return errors.New("injected delete failure")
	}
	return nil
}
func (f *faultCore) GetUserTrafficSlice(string, bool) ([]panel.UserTraffic, error) {
	v := f.traffic
	f.traffic = nil
	return v, nil
}

type contractPanel struct {
	mu                              sync.Mutex
	version                         int
	users                           []panel.UserInfo
	failAlive, failPush, failOnline bool
	configETags                     []string
	pushes                          []map[int][]int64
	online                          []map[int][]string
}

func (p *contractPanel) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	etag := fmt.Sprintf(`"v%d"`, p.version)
	switch strings.TrimPrefix(r.URL.Path, "/api/v1/server/UniProxy/") {
	case "config":
		p.configETags = append(p.configETags, r.Header.Get("If-None-Match"))
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(304)
			return
		}
		w.Header().Set("ETag", etag)
		fmt.Fprintf(w, `{"host":"version-%d","server_port":443,"network":"tcp","base_config":{"push_interval":3600,"pull_interval":3600}}`, p.version)
	case "user":
		w.Header().Set("ETag", etag)
		_ = json.NewEncoder(w).Encode(panel.UserListBody{Users: p.users})
	case "alivelist":
		if p.failAlive {
			w.WriteHeader(503)
			return
		}
		fmt.Fprint(w, `{"alive":{}}`)
	case "push":
		var v map[int][]int64
		_ = json.NewDecoder(r.Body).Decode(&v)
		p.pushes = append(p.pushes, v)
		if p.failPush {
			w.WriteHeader(503)
		}
	case "alive":
		var v map[int][]string
		_ = json.NewDecoder(r.Body).Decode(&v)
		p.online = append(p.online, v)
		if p.failOnline {
			w.WriteHeader(503)
		}
	default:
		w.WriteHeader(404)
	}
}

func newContractController(t *testing.T) (*Controller, *faultCore, *contractPanel) {
	t.Helper()
	limiter.Init()
	p := &contractPanel{version: 1, users: []panel.UserInfo{{Id: 1, Uuid: "user"}}}
	httpServer := httptest.NewServer(p)
	t.Cleanup(httpServer.Close)
	api, err := panel.New(&conf.ApiConfig{APIHost: httpServer.URL, NodeType: "vless", NodeID: 1})
	if err != nil {
		t.Fatal(err)
	}
	f := &faultCore{}
	c := NewController(f, api, &conf.Options{ListenIP: "127.0.0.1", CertConfig: conf.NewCertConfig()})
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c, f, p
}

func TestNodeChangeRollbackAndRetryDoesNotAcknowledgeCandidate(t *testing.T) {
	for _, failure := range []string{"alive", "node", "users", "delete-users"} {
		t.Run(failure, func(t *testing.T) {
			c, f, p := newContractController(t)
			old := c.info
			p.mu.Lock()
			if failure != "delete-users" {
				p.version = 2
			}
			p.users = []panel.UserInfo{{Id: 2, Uuid: "replacement"}}
			p.failAlive = failure == "alive"
			p.mu.Unlock()
			f.failAdd = failure == "node"
			f.failUsers = failure == "users"
			f.failDeleteUsers = failure == "delete-users"
			if err := c.reconcile(); err == nil {
				t.Fatal("injected failure was swallowed")
			}
			if c.info != old || len(c.userList) != 1 || c.userList[0].Id != 1 {
				t.Fatal("failed activation published candidate state")
			}
			if failure != "alive" && (f.node != old || len(f.users) != 1 || f.users[0].Id != 1) {
				t.Fatal("old node/users were not restored")
			}
			p.mu.Lock()
			p.failAlive = false
			p.mu.Unlock()
			if err := c.reconcile(); err != nil {
				t.Fatal(err)
			}
			if c.userList[0].Id != 2 {
				t.Fatal("retry did not apply users")
			}
			p.mu.Lock()
			defer p.mu.Unlock()
			if got := p.configETags[len(p.configETags)-1]; got != `"v1"` {
				t.Fatalf("failed snapshot acknowledged prematurely: %q", got)
			}
		})
	}
}

func TestReportRetriesTrafficOnlineAndSendsEmptySnapshot(t *testing.T) {
	c, f, p := newContractController(t)
	f.traffic = []panel.UserTraffic{{UID: 1, Uuid: "user", Upload: 10, Download: 20}}
	_, _ = c.limiter.CheckLimit(c.tag+"|user", "127.0.0.1", true, true)
	p.mu.Lock()
	p.failPush = true
	p.failOnline = true
	p.mu.Unlock()
	_ = c.reportUserTrafficTask()
	if c.pendingTraffic[1].Upload != 10 || len(c.pendingOnline[1]) != 1 {
		t.Fatal("failed report lost its pending state")
	}
	f.traffic = []panel.UserTraffic{{UID: 1, Uuid: "user", Upload: 3, Download: 4}}
	p.mu.Lock()
	p.failPush = false
	p.failOnline = false
	p.mu.Unlock()
	_ = c.reportUserTrafficTask()
	p.mu.Lock()
	defer p.mu.Unlock()
	last := p.pushes[len(p.pushes)-1][1]
	if len(last) != 2 || last[0] != 13 || last[1] != 24 {
		t.Fatalf("retry lost or duplicated traffic: %v", last)
	}
	if len(p.online) < 3 || len(p.online[len(p.online)-2][1]) != 1 || len(p.online[len(p.online)-1]) != 0 {
		t.Fatalf("expected retained online snapshot followed by empty clear: %v", p.online)
	}
}

func TestEmptyUserSnapshotRemovesAllUsers(t *testing.T) {
	c, f, p := newContractController(t)
	p.mu.Lock()
	p.users = []panel.UserInfo{}
	p.mu.Unlock()
	if err := c.reconcile(); err != nil {
		t.Fatal(err)
	}
	if len(c.userList) != 0 || len(f.users) != 0 {
		t.Fatal("empty user snapshot was treated as no change")
	}
}

func TestCloseKeepsLimiterUntilCoreNodeIsDeleted(t *testing.T) {
	c, f, _ := newContractController(t)
	checked := false
	f.onDelete = func() {
		if checked {
			return
		}
		if _, err := limiter.GetLimiter(c.tag); err != nil {
			t.Error("limiter was removed before the core node", err)
		}
		checked = true
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if !checked {
		t.Fatal("core node deletion callback was not invoked")
	}
	if _, err := limiter.GetLimiter(c.tag); err == nil {
		t.Fatal("limiter remained after core node deletion")
	}
}
