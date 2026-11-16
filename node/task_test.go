package node

import (
	"testing"
	"time"

	"github.com/InazumaV/V2bX/api/panel"
	"github.com/InazumaV/V2bX/common/format"
	"github.com/InazumaV/V2bX/conf"
	"github.com/InazumaV/V2bX/limiter"
)

func TestCompareUserListDetectsDeviceLimitChangesAndEmptyList(t *testing.T) {
	old := []panel.UserInfo{
		{Id: 1, Uuid: "unchanged", SpeedLimit: 10, DeviceLimit: 2},
		{Id: 2, Uuid: "updated", SpeedLimit: 10, DeviceLimit: 2},
	}
	current := []panel.UserInfo{
		{Id: 1, Uuid: "unchanged", SpeedLimit: 10, DeviceLimit: 2},
		{Id: 2, Uuid: "updated", SpeedLimit: 10, DeviceLimit: 4},
	}

	deleted, added := compareUserList(old, current)
	if len(deleted) != 1 || deleted[0].Uuid != "updated" || len(added) != 1 || added[0] != current[1] {
		t.Fatalf("compareUserList() = deleted %v, added %v", deleted, added)
	}

	deleted, added = compareUserList(old, nil)
	if len(deleted) != len(old) || len(added) != 0 {
		t.Fatalf("compareUserList(old, empty) = deleted %v, added %v", deleted, added)
	}
}

func TestDynamicSpeedLimitAccumulatesAndAppliesTraffic(t *testing.T) {
	limiter.Init()
	const tag = "dynamic-test"
	const uuid = "user-1"
	limitConfig := conf.LimitConfig{
		EnableDynamicSpeedLimit: true,
		DynamicSpeedLimitConfig: &conf.DynamicSpeedLimitConfig{
			Periodic:   60,
			Traffic:    150,
			SpeedLimit: 5,
			ExpireTime: 10,
		},
	}
	userLimiter := limiter.AddLimiter(tag, &limitConfig, []panel.UserInfo{{Id: 11, Uuid: uuid}}, nil)
	controller := &Controller{
		tag: tag,
		Options: &conf.Options{
			LimitConfig: limitConfig,
		},
		limiter: userLimiter,
		traffic: make(map[string]int64),
	}

	controller.recordDynamicTraffic([]panel.UserTraffic{{Uuid: uuid, Upload: 80, Download: 30}})
	controller.recordDynamicTraffic([]panel.UserTraffic{{Uuid: uuid, Upload: 20, Download: 25}})
	controller.SpeedChecker()

	userLimiter, err := limiter.GetLimiter(tag)
	if err != nil {
		t.Fatal(err)
	}
	value, ok := userLimiter.UserLimitInfo.Load(format.UserTag(tag, uuid))
	if !ok {
		t.Fatal("user limit info is missing")
	}
	info := value.(*limiter.UserLimitInfo)
	if info.DynamicSpeedLimit != limitConfig.DynamicSpeedLimitConfig.SpeedLimit {
		t.Fatalf("dynamic speed limit=%d, want %d", info.DynamicSpeedLimit, limitConfig.DynamicSpeedLimitConfig.SpeedLimit)
	}
	expiresIn := time.Until(time.Unix(info.ExpireTime, 0))
	wantExpiry := time.Duration(limitConfig.DynamicSpeedLimitConfig.ExpireTime) * time.Minute
	if expiresIn < wantExpiry-3*time.Second || expiresIn > wantExpiry {
		t.Fatalf("dynamic speed limit expiry is %s, want approximately %s from now", expiresIn, wantExpiry)
	}
	controller.trafficMu.Lock()
	got := controller.traffic[uuid]
	controller.trafficMu.Unlock()
	if got != 0 {
		t.Fatalf("traffic accumulator=%d after applying limit, want 0", got)
	}
}
