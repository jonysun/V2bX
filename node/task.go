package node

import (
	"errors"
	"fmt"
	"time"

	"github.com/InazumaV/V2bX/api/panel"
	"github.com/InazumaV/V2bX/common/task"
	log "github.com/sirupsen/logrus"
)

func (c *Controller) startTasks(node *panel.NodeInfo) error {
	if node.PullInterval <= 0 || node.PushInterval <= 0 {
		return fmt.Errorf("panel intervals must be positive (pull=%s, push=%s)", node.PullInterval, node.PushInterval)
	}
	var started []*task.Task
	start := func(t *task.Task, first bool) error {
		if err := t.Start(first); err != nil {
			for _, running := range started {
				running.Close()
			}
			return err
		}
		started = append(started, t)
		return nil
	}
	// fetch node info task
	c.nodeInfoMonitorPeriodic = &task.Task{
		Interval: node.PullInterval,
		Execute:  c.nodeInfoMonitor,
	}
	// fetch user list task
	c.userReportPeriodic = &task.Task{
		Interval: node.PushInterval,
		Execute:  c.reportUserTrafficTask,
	}
	log.WithField("tag", c.tag).Info("Start monitor node status")
	// delay to start nodeInfoMonitor
	if err := start(c.nodeInfoMonitorPeriodic, false); err != nil {
		return err
	}
	log.WithField("tag", c.tag).Info("Start report node status")
	if err := start(c.userReportPeriodic, false); err != nil {
		return err
	}
	if node.Security == panel.Tls {
		switch c.CertConfig.CertMode {
		case "none", "", "file", "self":
		default:
			c.renewCertPeriodic = &task.Task{
				Interval: time.Hour * 24,
				Execute:  c.renewCertTask,
			}
			log.WithField("tag", c.tag).Info("Start renew cert")
			// delay to start renewCert
			if err := start(c.renewCertPeriodic, true); err != nil {
				return err
			}
		}
	}
	if c.LimitConfig.EnableDynamicSpeedLimit {
		if c.LimitConfig.DynamicSpeedLimitConfig == nil || c.LimitConfig.DynamicSpeedLimitConfig.Periodic <= 0 {
			for _, running := range started {
				running.Close()
			}
			return errors.New("dynamic speed limit requires a positive periodic interval")
		}
		c.trafficMu.Lock()
		c.traffic = make(map[string]int64)
		c.trafficMu.Unlock()
		c.dynamicSpeedLimitPeriodic = &task.Task{
			Interval: time.Duration(c.LimitConfig.DynamicSpeedLimitConfig.Periodic) * time.Second,
			Execute:  c.SpeedChecker,
		}
		if err := start(c.dynamicSpeedLimitPeriodic, false); err != nil {
			return err
		}
		log.Printf("[%s: %d] Start dynamic speed limit", c.apiClient.NodeType, c.apiClient.NodeId)
	}
	return nil
}

func (c *Controller) SpeedChecker() error {
	c.reconcileMu.Lock()
	defer c.reconcileMu.Unlock()
	if !c.LimitConfig.EnableDynamicSpeedLimit || c.LimitConfig.DynamicSpeedLimitConfig == nil || c.limiter == nil {
		return nil
	}
	c.trafficMu.Lock()
	defer c.trafficMu.Unlock()
	for u, t := range c.traffic {
		if t >= c.LimitConfig.DynamicSpeedLimitConfig.Traffic {
			err := c.limiter.UpdateDynamicSpeedLimit(c.tag, u,
				c.LimitConfig.DynamicSpeedLimitConfig.SpeedLimit,
				time.Now().Add(time.Duration(c.LimitConfig.DynamicSpeedLimitConfig.ExpireTime)*time.Minute))
			if err != nil {
				log.WithField("err", err).Error("Update dynamic speed limit failed")
			} else {
				delete(c.traffic, u)
			}
		}
	}
	return nil
}

func (c *Controller) recordDynamicTraffic(traffic []panel.UserTraffic) {
	if !c.LimitConfig.EnableDynamicSpeedLimit {
		return
	}
	c.trafficMu.Lock()
	defer c.trafficMu.Unlock()
	if c.traffic == nil {
		c.traffic = make(map[string]int64)
	}
	for _, userTraffic := range traffic {
		if userTraffic.Uuid != "" {
			c.traffic[userTraffic.Uuid] += userTraffic.Upload + userTraffic.Download
		}
	}
}
