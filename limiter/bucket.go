package limiter

import (
	"sync"
	"time"

	"github.com/juju/ratelimit"
)

// Bucket keeps a stable identity for active connections, while reading the
// current user policy on every wait, including dynamic-limit expiry.
type Bucket struct {
	owner   *Limiter
	userTag string
	mu      sync.Mutex
	rate    int64
	bucket  *ratelimit.Bucket
}

func (l *Limiter) effectiveRate(userTag string) int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	v, ok := l.UserLimitInfo.Load(userTag)
	if !ok {
		return 0
	}
	u := v.(*UserLimitInfo)
	dynamic := u.DynamicSpeedLimit
	if u.ExpireTime != 0 && u.ExpireTime <= time.Now().Unix() {
		copy := *u
		copy.DynamicSpeedLimit, copy.ExpireTime = 0, 0
		l.UserLimitInfo.Store(userTag, &copy)
		dynamic = 0
	}
	return int64(determineSpeedLimit(l.SpeedLimit, determineSpeedLimit(u.SpeedLimit, dynamic))) * 1000000 / 8
}

func (b *Bucket) Wait(count int64) {
	for count > 0 {
		rate := b.owner.effectiveRate(b.userTag)
		if rate <= 0 {
			return
		}
		b.mu.Lock()
		if b.bucket == nil || b.rate != rate {
			b.rate = rate
			b.bucket = ratelimit.NewBucketWithRate(float64(rate), rate)
		}
		taken := b.bucket.TakeAvailable(count)
		b.mu.Unlock()
		count -= taken
		if count > 0 {
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func (b *Bucket) Rate() float64 { return float64(b.owner.effectiveRate(b.userTag)) }
