package node

import (
	"errors"
	"fmt"
	"sync"

	"github.com/InazumaV/V2bX/api/panel"
	"github.com/InazumaV/V2bX/common/task"
	"github.com/InazumaV/V2bX/conf"
	vCore "github.com/InazumaV/V2bX/core"
	"github.com/InazumaV/V2bX/limiter"
	log "github.com/sirupsen/logrus"
)

type Controller struct {
	server                    vCore.Core
	apiClient                 *panel.Client
	tag                       string
	limiter                   *limiter.Limiter
	traffic                   map[string]int64
	trafficMu                 sync.Mutex
	reconcileMu               sync.Mutex
	pendingTraffic            map[int]panel.UserTraffic
	pendingOnline             map[int][]string
	needsRestore              bool
	userList                  []panel.UserInfo
	aliveMap                  map[int]int
	info                      *panel.NodeInfo
	nodeInfoMonitorPeriodic   *task.Task
	userReportPeriodic        *task.Task
	renewCertPeriodic         *task.Task
	dynamicSpeedLimitPeriodic *task.Task
	onlineIpReportPeriodic    *task.Task
	*conf.Options
}

// NewController return a Node controller with default parameters.
func NewController(server vCore.Core, api *panel.Client, config *conf.Options) *Controller {
	controller := &Controller{
		server:    server,
		Options:   config,
		apiClient: api,
	}
	return controller
}

// Start implement the Start() function of the service interface
func (c *Controller) Start() error {
	if c.LimitConfig.EnableDynamicSpeedLimit {
		cfg := c.LimitConfig.DynamicSpeedLimitConfig
		if cfg == nil || cfg.Periodic <= 0 || cfg.Traffic <= 0 || cfg.SpeedLimit <= 0 || cfg.ExpireTime <= 0 {
			return errors.New("dynamic speed limit requires positive periodic, traffic, speed limit, and expiry settings")
		}
	}
	// First fetch Node Info
	var err error
	node, err := c.apiClient.GetNodeInfo()
	if err != nil {
		return fmt.Errorf("get node info error: %s", err)
	}
	// Fetch user state into a candidate snapshot. The panel ETag is acknowledged
	// only after the core accepts both the node and its users.
	users, err := c.apiClient.GetUserList()
	if err != nil {
		return fmt.Errorf("get user list error: %s", err)
	}
	aliveMap, err := c.apiClient.GetUserAlive()
	if err != nil {
		return fmt.Errorf("failed to get user alive list: %s", err)
	}
	if node == nil {
		return errors.New("get node info returned no snapshot during controller start")
	}
	if node.PullInterval <= 0 || node.PushInterval <= 0 {
		return fmt.Errorf("panel intervals must be positive (pull=%s, push=%s)", node.PullInterval, node.PushInterval)
	}
	c.userList = users
	c.aliveMap = aliveMap
	if len(c.Options.Name) == 0 {
		c.tag = c.buildNodeTag(node)
	} else {
		c.tag = c.Options.Name
	}

	// add limiter
	l := limiter.AddLimiter(c.tag, &c.LimitConfig, c.userList, c.aliveMap)
	// add rule limiter
	if err = l.UpdateRule(&node.Rules); err != nil {
		limiter.DeleteLimiter(c.tag)
		return fmt.Errorf("update rule error: %s", err)
	}
	c.limiter = l
	if node.Security == panel.Tls {
		err = c.requestCert()
		if err != nil {
			limiter.DeleteLimiter(c.tag)
			return fmt.Errorf("request cert error: %s", err)
		}
	}
	// Add new tag
	err = c.server.AddNode(c.tag, node, c.Options)
	if err != nil {
		limiter.DeleteLimiter(c.tag)
		return fmt.Errorf("add new node error: %s", err)
	}
	added, err := c.server.AddUsers(&vCore.AddUsersParams{
		Tag:      c.tag,
		Users:    c.userList,
		NodeInfo: node,
	})
	if err != nil {
		_ = c.server.DelNode(c.tag)
		limiter.DeleteLimiter(c.tag)
		return fmt.Errorf("add users error: %s", err)
	}
	log.WithField("tag", c.tag).Infof("Added %d new users", added)
	c.info = node
	if err := c.startTasks(node); err != nil {
		_ = c.server.DelNode(c.tag)
		limiter.DeleteLimiter(c.tag)
		return err
	}
	c.apiClient.CommitNodeInfo()
	c.apiClient.CommitUserList()
	return nil
}

// Close implement the Close() function of the service interface
func (c *Controller) Close() error {
	if c.nodeInfoMonitorPeriodic != nil {
		c.nodeInfoMonitorPeriodic.Close()
	}
	if c.userReportPeriodic != nil {
		c.userReportPeriodic.Close()
	}
	if c.renewCertPeriodic != nil {
		c.renewCertPeriodic.Close()
	}
	if c.dynamicSpeedLimitPeriodic != nil {
		c.dynamicSpeedLimitPeriodic.Close()
	}
	if c.onlineIpReportPeriodic != nil {
		c.onlineIpReportPeriodic.Close()
	}
	c.reconcileMu.Lock()
	defer c.reconcileMu.Unlock()
	err := c.server.DelNode(c.tag)
	if err != nil {
		return fmt.Errorf("del node error: %s", err)
	}
	limiter.DeleteLimiter(c.tag)
	return nil
}

func (c *Controller) buildNodeTag(node *panel.NodeInfo) string {
	return fmt.Sprintf("[%s]-%s:%d", c.apiClient.APIHost, node.Type, node.Id)
}
