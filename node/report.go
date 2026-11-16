package node

import (
	"fmt"

	"github.com/InazumaV/V2bX/api/panel"
	vCore "github.com/InazumaV/V2bX/core"
	log "github.com/sirupsen/logrus"
)

// collectTraffic is called under reconcileMu. A reset transfers ownership of
// bytes from the core to this controller, until a successful panel response.
func (c *Controller) collectTraffic(force bool) error {
	var batch []panel.UserTraffic
	var err error
	if drain, ok := c.server.(vCore.TrafficDrainer); force && ok {
		batch, err = drain.DrainTraffic(c.tag)
	} else {
		batch, err = c.server.GetUserTrafficSlice(c.tag, true)
	}
	if err != nil {
		return fmt.Errorf("collect traffic: %w", err)
	}
	c.recordDynamicTraffic(batch)
	if c.pendingTraffic == nil {
		c.pendingTraffic = make(map[int]panel.UserTraffic)
	}
	for _, traffic := range batch {
		previous := c.pendingTraffic[traffic.UID]
		previous.UID, previous.Uuid = traffic.UID, traffic.Uuid
		previous.Upload += traffic.Upload
		previous.Download += traffic.Download
		c.pendingTraffic[traffic.UID] = previous
	}
	return nil
}

func (c *Controller) reportUserTrafficTask() error {
	c.reconcileMu.Lock()
	defer c.reconcileMu.Unlock()
	if c.needsRestore {
		return nil
	}
	if err := c.collectTraffic(false); err != nil {
		log.WithError(err).WithField("tag", c.tag).Error("Collect traffic failed")
		return nil
	}
	batch := make([]panel.UserTraffic, 0, len(c.pendingTraffic))
	for _, traffic := range c.pendingTraffic {
		batch = append(batch, traffic)
	}
	if len(batch) > 0 {
		if err := c.apiClient.ReportUserTraffic(batch); err != nil {
			log.WithError(err).WithField("tag", c.tag).Error("Report traffic failed; retaining pending bytes")
		} else {
			clear(c.pendingTraffic)
		}
	}
	// Retry the exact previous online snapshot before consuming a fresh one.
	if c.pendingOnline != nil {
		if err := c.apiClient.ReportNodeOnlineUsers(&c.pendingOnline); err != nil {
			log.WithError(err).Error("Retry online report failed")
			return nil
		}
		c.pendingOnline = nil
	}
	online, err := c.limiter.GetOnlineDevice()
	if err != nil {
		log.WithError(err).Error("Collect online users failed")
		return nil
	}
	trafficByUID := make(map[int]int64)
	for _, traffic := range batch {
		trafficByUID[traffic.UID] += traffic.Upload + traffic.Download
	}
	c.pendingOnline = make(map[int][]string)
	for _, user := range *online {
		if trafficByUID[user.UID] < c.Options.DeviceOnlineMinTraffic*1000 {
			continue
		}
		c.pendingOnline[user.UID] = append(c.pendingOnline[user.UID], user.IP)
	}
	// Even {} must be sent, so the panel can clear stale online IPs.
	if err := c.apiClient.ReportNodeOnlineUsers(&c.pendingOnline); err != nil {
		log.WithError(err).Error("Report online users failed; retaining pending snapshot")
	} else {
		c.pendingOnline = nil
	}
	return nil
}
