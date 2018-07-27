/*
Copyright 2018 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package knative

// copied from : github.com/knative/serving/pkg/controller/service/service.go

import (
	"k8s.io/client-go/tools/cache"

	servinginformers "github.com/knative/serving/pkg/client/informers/externalversions/serving/v1alpha1"
	"github.com/knative/serving/pkg/controller"
	"go.uber.org/zap"
)

const controllerAgentName = "service-controller"

// Controller implements the controller for Service resources.
type Controller struct {
	*controller.Base

	notifications chan struct{}
}

// NewController initializes the controller and is called by the generated code
// Registers eventhandlers to enqueue events
func NewController(
	opt controller.Options,
	notifications chan struct{},
	serviceInformer servinginformers.ServiceInformer,
) *Controller {
	logger, _ := zap.NewProduction()
	opt.Logger = logger.Sugar()
	c := &Controller{
		Base: controller.NewBase(opt, controllerAgentName, "Services"),
	}

	c.Logger.Info("Setting up event handlers")
	serviceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.Enqueue,
		UpdateFunc: controller.PassNew(c.Enqueue),
		DeleteFunc: c.Enqueue,
	})

	return c
}

// Run starts the controller's worker threads, the number of which is threadiness. It then blocks until stopCh
// is closed, at which point it shuts down its internal work queue and waits for workers to finish processing their
// current work items.
func (c *Controller) Run(threadiness int, stopCh <-chan struct{}) error {
	return c.RunController(threadiness, stopCh, c.Reconcile, "Service")
}

// Reconcile compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the Service resource
// with the current status of the resource.
func (c *Controller) Reconcile(key string) error {
	select {
	case c.notifications <- struct{}{}:
	default:
	}
	return nil
}
