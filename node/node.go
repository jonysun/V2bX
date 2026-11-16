package node

import (
	"errors"
	"fmt"

	"github.com/InazumaV/V2bX/api/panel"
	"github.com/InazumaV/V2bX/conf"
	vCore "github.com/InazumaV/V2bX/core"
	log "github.com/sirupsen/logrus"
)

type Node struct {
	controllers []*Controller
}

func New() *Node {
	return &Node{}
}

func (n *Node) Start(nodes []conf.NodeConfig, core vCore.Core) error {
	if len(n.controllers) != 0 {
		return errors.New("nodes are already started")
	}
	controllers := make([]*Controller, len(nodes))
	for i := range nodes {
		p, err := panel.New(&nodes[i].ApiConfig)
		if err != nil {
			return err
		}
		// Register controller service
		controllers[i] = NewController(core, p, &nodes[i].Options)
	}
	lifecycle := make([]controllerLifecycle, len(controllers))
	for i, controller := range controllers {
		lifecycle[i] = controller
	}
	if err := startControllerSet(lifecycle, func(i int) string {
		api := nodes[i].ApiConfig
		return fmt.Sprintf("[%s-%s-%d]", api.APIHost, api.NodeType, api.NodeID)
	}); err != nil {
		return err
	}
	n.controllers = controllers
	return nil
}

type controllerLifecycle interface {
	Start() error
	Close() error
}

func startControllerSet(controllers []controllerLifecycle, label func(int) string) error {
	started := make([]controllerLifecycle, 0, len(controllers))
	for i, controller := range controllers {
		err := controller.Start()
		if err != nil {
			started = append(started, controller)
			return errors.Join(fmt.Errorf("start node controller %s: %w", label(i), err), closeControllerSet(started))
		}
		started = append(started, controller)
	}
	return nil
}

func closeControllerSet(controllers []controllerLifecycle) error {
	var errs []error
	for i := len(controllers) - 1; i >= 0; i-- {
		if err := controllers[i].Close(); err != nil {
			errs = append(errs, fmt.Errorf("close node controller %d: %w", i, err))
		}
	}
	return errors.Join(errs...)
}

func (n *Node) Close() {
	var errs []error
	for i := len(n.controllers) - 1; i >= 0; i-- {
		if err := n.controllers[i].Close(); err != nil {
			errs = append(errs, fmt.Errorf("close node controller %d: %w", i, err))
		}
	}
	n.controllers = nil
	if err := errors.Join(errs...); err != nil {
		log.WithError(err).Error("Close nodes failed")
	}
}
