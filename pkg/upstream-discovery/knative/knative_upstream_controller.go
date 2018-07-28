package knative

import (
	"fmt"
	"time"

	clientset "github.com/knative/serving/pkg/client/clientset/versioned"
	knativeinformers "github.com/knative/serving/pkg/client/informers/externalversions"
	knativeinformersv1alpha1 "github.com/knative/serving/pkg/client/informers/externalversions/serving/v1alpha1"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/kubernetes"

	"github.com/knative/serving/pkg/controller"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/config"
	"github.com/solo-io/gloo/pkg/log"
	knativeplugin "github.com/solo-io/gloo/pkg/plugins/knative"
	"github.com/solo-io/gloo/pkg/storage"

)

const (
	generatedBy = "knative-upstream-discovery"

	threadsPerController = 2
)

type UpstreamController struct {
	errors chan error

	serviceInformer knativeinformersv1alpha1.ServiceInformer
	runFunc         func(stop <-chan struct{})

	generatedBy string

	syncer config.UpstreamSyncer
}

func NewUpstreamController(cfg *rest.Config,
	configStore storage.Interface,
	resyncDuration time.Duration) (*UpstreamController, error) {

	// attempt to register upstreams if they don't exist
	if err := configStore.V1().Register(); err != nil && !storage.IsAlreadyExists(err) {
		return nil, errors.Wrap(err, "failed to register upstreams")
	}

	c := &UpstreamController{
		errors: make(chan error),

		generatedBy: generatedBy,

		syncer: config.UpstreamSyncer{
			Owner:       generatedBy,
			GlooStorage: configStore,
		},
	}

	c.syncer.DesiredUpstreams = c.generateDesiredUpstreams

	err := c.startControllers(cfg)
	if err != nil {
		return nil, err
	}

	return c, nil
}

func (c *UpstreamController) startControllers(cfg *rest.Config) error {
	notifications := make(chan struct{})

	servingClient, err := clientset.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to create serving clientset: %v", err)
	}
	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to create kube clientset: %v", err)
	}

	servingInformerFactory := knativeinformers.NewSharedInformerFactory(servingClient, time.Second*30)

	opt := controller.Options{
		KubeClientSet:    kubeClient,
		ServingClientSet: servingClient,
		BuildClientSet:   nil,
		ConfigMapWatcher: nil,
		Logger:           nil,
	}

	serviceInformer := servingInformerFactory.Serving().V1alpha1().Services()
	c.serviceInformer = serviceInformer
	// call informer method just because
	serviceInformer.Informer()

	// copied from here:
	// github.com/knative/serving/cmd/controller/main.go

	c.runFunc = func(stopCh <-chan struct{}) {
		// These are non-blocking.
		servingInformerFactory.Start(stopCh)

		// Wait for the caches to be synced before starting controllers.
		log.Printf("Waiting for informer caches to sync")

		if ok := cache.WaitForCacheSync(stopCh, serviceInformer.Informer().HasSynced); !ok {
			c.errors <- fmt.Errorf("failed to wait for cache to sync")
			return
		}

		ctrlr := NewController(
			opt,
			notifications,
			serviceInformer,
		)

		// Start all of the controllers.
		go func(ctrlr controller.Interface) {
			// We don't expect this to return until stop is called,
			// but if it does, propagate it back.
			if runErr := ctrlr.Run(threadsPerController, stopCh); runErr != nil {
				c.errors <- runErr
			}
		}(ctrlr)

		//		go kubeController.Run(2, stop)
		// refresh every minute
		tick := time.Tick(time.Minute)
		go func() {
			for {
				select {
				case <-notifications:
				case <-tick:
					c.syncGlooUpstreamsWithKubeServices()
				case <-stopCh:
					return
				}
			}
		}()
		<-stopCh
		log.Printf("kube upstream discovery stopped")
	}

	return nil
}

func (c *UpstreamController) Run(stop <-chan struct{}) {
	c.runFunc(stop)
}

func (c *UpstreamController) Error() <-chan error {
	return c.errors
}

func (c *UpstreamController) syncGlooUpstreamsWithKubeServices() {
	if err := c.syncer.SyncDesiredState(); err != nil {
		c.errors <- err
	}
}

func (c *UpstreamController) generateDesiredUpstreams() ([]*v1.Upstream, error) {
	serviceList, err := c.serviceInformer.Lister().List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %v", err)
	}
	var upstreams []*v1.Upstream
	for _, svc := range serviceList {
		domain := svc.Status.Domain
		upstream := &v1.Upstream{
			Name: upstreamName(domain),
			Type: knativeplugin.UpstreamTypeKnative,
			Spec: knativeplugin.EncodeUpstreamSpec(knativeplugin.UpstreamSpec{
				Hostname: domain,
			}),
		}
		upstreams = append(upstreams, upstream)
	}
	return upstreams, nil
}

func upstreamName(domain string) string {
	return fmt.Sprintf("%s", domain)
}
