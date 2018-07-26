package knative

import (
	"fmt"
	"time"

	knativeinformers "github.com/knative/serving/pkg/client/informers/externalversions"
	knativelisters "github.com/knative/serving/pkg/client/listers/serving/v1alpha1"
	clientset "github.com/knative/serving/pkg/client/clientset/versioned"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/rest"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/config"
	"github.com/solo-io/gloo/pkg/log"
	knativeplugin "github.com/solo-io/gloo/pkg/plugins/knative"
	"github.com/solo-io/gloo/pkg/storage"
	"github.com/solo-io/kubecontroller"
)

const (
	generatedBy = "knative-upstream-discovery"
)

type UpstreamController struct {
	errors chan error

	serviceLister knativelisters.ServiceLister
	runFunc       func(stop <-chan struct{})

	generatedBy string

	syncer config.UpstreamSyncer
}

func NewUpstreamController(cfg *rest.Config,
	configStore storage.Interface,
	resyncDuration time.Duration) (*UpstreamController, error) {

	servingClient, err := clientset.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create kube clientset: %v", err)
	}


	// attempt to register upstreams if they don't exist
	if err := configStore.V1().Register(); err != nil && !storage.IsAlreadyExists(err) {
		return nil, errors.Wrap(err, "failed to register upstreams")
	}

	knativeInformerFactory := knativeinformers.NewSharedInformerFactory(servingClient, resyncDuration)
	serviceInformer := knativeInformerFactory.Serving().V1alpha1().Services()

	c := &UpstreamController{
		errors: make(chan error),

		serviceLister: serviceInformer.Lister(),
		generatedBy:   generatedBy,

		syncer: config.UpstreamSyncer{
			Owner:       generatedBy,
			GlooStorage: configStore,
		},
	}

	c.syncer.DesiredUpstreams = c.generateDesiredUpstreams

	kubeController := kubecontroller.NewController("kube-upstream-discovery", kubeClient,
		kubecontroller.NewLockingSyncHandler(c.syncGlooUpstreamsWithKubeServices),
		serviceInformer.Informer())

	c.runFunc = func(stop <-chan struct{}) {
		go kubeInformerFactory.Start(stop)
		go kubeController.Run(2, stop)
		// refresh every minute
		tick := time.Tick(time.Minute)
		go func() {
			for {
				select {
				case <-tick:
					c.syncGlooUpstreamsWithKubeServices()
				case <-stop:
					return
				}
			}
		}()
		<-stop
		log.Printf("kube upstream discovery stopped")
	}

	return c, nil
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
	serviceList, err := c.serviceLister.List(labels.Everything())
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
