package limiter

import (
	"sync"
	"testing"
	"time"

	"github.com/InazumaV/V2bX/api/panel"
	"github.com/InazumaV/V2bX/common/format"
	"github.com/InazumaV/V2bX/conf"
)

func TestRuleUpdateIsAtomicAndRejectsInvalidRegexp(t *testing.T) {
	l := NewLimiter("n", &conf.LimitConfig{}, nil, nil)
	if err := l.UpdateRule(&panel.Rules{Regexp: []string{"old"}}); err != nil {
		t.Fatal(err)
	}
	if err := l.UpdateRule(&panel.Rules{Regexp: []string{"new", "["}}); err == nil {
		t.Fatal("invalid regexp accepted")
	}
	if !l.CheckDomainRule("old") || l.CheckDomainRule("new") {
		t.Fatal("failed update replaced live rules")
	}
}

func TestActiveBucketTracksDynamicPolicyAndExpiry(t *testing.T) {
	l := NewLimiter("n", &conf.LimitConfig{}, []panel.UserInfo{{Id: 1, Uuid: "u"}}, nil)
	b, rejected := l.CheckLimit(format.UserTag("n", "u"), "127.0.0.1", true, true)
	if rejected || b == nil || b.Rate() != 0 {
		t.Fatal("unlimited user not wrapped")
	}
	if err := l.UpdateDynamicSpeedLimit("n", "u", 8, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if b.Rate() != 1000000 {
		t.Fatalf("existing bucket rate = %v", b.Rate())
	}
	if err := l.UpdateDynamicSpeedLimit("n", "u", 8, time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if b.Rate() != 0 {
		t.Fatal("expired dynamic policy did not restore unlimited base")
	}
	if _, reject := l.CheckLimit(format.UserTag("n", "u"), "127.0.0.1", true, true); reject {
		t.Fatal("expiry deleted the user's authorization")
	}
}

func TestDeletingUserReleasesPublishedSpeedBucket(t *testing.T) {
	l := NewLimiter("n", &conf.LimitConfig{}, []panel.UserInfo{{Id: 1, Uuid: "u"}}, nil)
	userTag := format.UserTag("n", "u")
	if _, rejected := l.CheckLimit(userTag, "127.0.0.1", true, true); rejected {
		t.Fatal("known user was rejected")
	}
	if _, ok := l.SpeedLimiter.Load(userTag); !ok {
		t.Fatal("CheckLimit did not publish the user's stable bucket")
	}
	l.UpdateUser("n", nil, []panel.UserInfo{{Id: 1, Uuid: "u"}})
	if _, ok := l.SpeedLimiter.Load(userTag); ok {
		t.Fatal("deleted user's speed bucket was retained")
	}
}

func TestConcurrentPolicyUpdates(t *testing.T) {
	l := NewLimiter("n", &conf.LimitConfig{}, []panel.UserInfo{{Id: 1, Uuid: "u"}}, nil)
	var wg sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				switch worker {
				case 0:
					b, _ := l.CheckLimit("n|u", "127.0.0.1", true, true)
					if b != nil {
						_ = b.Rate()
					}
					_ = l.CheckDomainRule("example.org")
				case 1:
					l.UpdateAlive(map[int]int{1: i % 2})
					_, _ = l.GetOnlineDevice()
				case 2:
					l.UpdateUser("n", []panel.UserInfo{{Id: 1, Uuid: "u", SpeedLimit: i % 3}}, nil)
				case 3:
					_ = l.UpdateRule(&panel.Rules{Regexp: []string{"blocked"}})
					_ = l.UpdateDynamicSpeedLimit("n", "u", 2, time.Now().Add(time.Hour))
				}
			}
		}(worker)
	}
	wg.Wait()
}
