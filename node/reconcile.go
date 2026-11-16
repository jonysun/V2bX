package node

import (
	"errors"
	"fmt"

	"github.com/InazumaV/V2bX/api/panel"
	vCore "github.com/InazumaV/V2bX/core"
	"github.com/InazumaV/V2bX/limiter"
	log "github.com/sirupsen/logrus"
)

func (c *Controller) nodeInfoMonitor() error {
	c.reconcileMu.Lock()
	defer c.reconcileMu.Unlock()
	if err := c.reconcile(); err != nil {
		log.WithField("tag", c.tag).WithError(err).Error("Reconcile failed; panel snapshot remains unacknowledged")
	}
	// A transient panel/core error must not stop the periodic retry task.
	return nil
}

func (c *Controller) reconcile() error {
	if c.needsRestore {
		if err := c.restoreNode(); err != nil {
			return err
		}
	}
	node, err := c.apiClient.GetNodeInfo()
	if err != nil {
		return fmt.Errorf("get node: %w", err)
	}
	users, err := c.apiClient.GetUserList()
	if err != nil {
		return fmt.Errorf("get users: %w", err)
	}
	alive, err := c.apiClient.GetUserAlive()
	if err != nil {
		return fmt.Errorf("get alive list: %w", err)
	}
	if node != nil {
		if users == nil {
			users = c.userList
		}
		if err := c.replaceNode(node, users, alive); err != nil {
			return err
		}
	} else if users != nil {
		deleted, added := compareUserList(c.userList, users)
		if len(deleted)+len(added) > 0 {
			if err := c.collectTraffic(true); err != nil {
				return err
			}
			if len(deleted) > 0 {
				if err := c.server.DelUsers(deleted, c.tag, c.info); err != nil {
					c.needsRestore = true
					return errors.Join(fmt.Errorf("delete users: %w", err), c.restoreNode())
				}
			}
			if len(added) > 0 {
				if _, err := c.server.AddUsers(&vCore.AddUsersParams{Tag: c.tag, NodeInfo: c.info, Users: added}); err != nil {
					c.needsRestore = true
					return errors.Join(fmt.Errorf("add users: %w", err), c.restoreNode())
				}
			}
			c.limiter.UpdateUser(c.tag, added, deleted)
		}
		c.userList = users
	}
	c.aliveMap = alive
	c.limiter.UpdateAlive(alive)
	c.apiClient.CommitNodeInfo()
	c.apiClient.CommitUserList()
	return nil
}

func (c *Controller) restoreNode() error {
	if err := c.server.DelNode(c.tag); err != nil {
		return fmt.Errorf("clean node before restoration: %w", err)
	}
	limiter.PublishLimiter(c.tag, c.limiter)
	if err := c.server.AddNode(c.tag, c.info, c.Options); err != nil {
		return fmt.Errorf("restore node: %w", err)
	}
	if _, err := c.server.AddUsers(&vCore.AddUsersParams{Tag: c.tag, NodeInfo: c.info, Users: c.userList}); err != nil {
		_ = c.server.DelNode(c.tag)
		return fmt.Errorf("restore users: %w", err)
	}
	c.needsRestore = false
	return nil
}

func (c *Controller) replaceNode(node *panel.NodeInfo, users []panel.UserInfo, alive map[int]int) error {
	if node.PullInterval <= 0 || node.PushInterval <= 0 {
		return errors.New("panel pull/push intervals must be positive")
	}
	tag := c.tag
	if c.Options.Name == "" {
		tag = c.buildNodeTag(node)
	}
	candidate := limiter.NewLimiter(tag, &c.LimitConfig, users, alive)
	if err := candidate.UpdateRule(&node.Rules); err != nil {
		return err
	}
	if node.Security == panel.Tls {
		if err := c.requestCert(); err != nil {
			return err
		}
	}
	if err := c.collectTraffic(true); err != nil {
		return err
	}
	if err := c.server.DelNode(c.tag); err != nil {
		c.needsRestore = true
		return fmt.Errorf("delete old node: %w", err)
	}
	c.needsRestore = true
	limiter.PublishLimiter(tag, candidate)
	err := c.server.AddNode(tag, node, c.Options)
	if err == nil {
		_, err = c.server.AddUsers(&vCore.AddUsersParams{Tag: tag, NodeInfo: node, Users: users})
	}
	if err != nil {
		cleanupErr := c.server.DelNode(tag)
		limiter.DeleteLimiter(tag)
		if cleanupErr != nil {
			return errors.Join(fmt.Errorf("activate replacement: %w", err), fmt.Errorf("clean replacement: %w", cleanupErr))
		}
		return errors.Join(fmt.Errorf("activate replacement: %w", err), c.restoreNode())
	}
	if tag != c.tag {
		limiter.DeleteLimiter(c.tag)
	}
	c.tag, c.info, c.userList, c.limiter = tag, node, users, candidate
	c.needsRestore = false
	if c.nodeInfoMonitorPeriodic != nil {
		_ = c.nodeInfoMonitorPeriodic.SetInterval(node.PullInterval)
	}
	if c.userReportPeriodic != nil {
		_ = c.userReportPeriodic.SetInterval(node.PushInterval)
	}
	return nil
}
