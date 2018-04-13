package controller

import (
	"fmt"
	"time"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/labels"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	kubelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/rest"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	kubeplugin "github.com/solo-io/gloo/pkg/plugins/kubernetes"
	"github.com/solo-io/gloo/pkg/storage"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/config"
	"github.com/solo-io/kubecontroller"
)

const (
	kubeSystemNamespace = "kube-system"

	ownerAnnotationKey = "generated_by"

	generatedBy =  "kubernetes-upstream-discovery"
)

type ServiceController struct {
	errors chan error

	serviceLister kubelisters.ServiceLister
	runFunc       func(stop <-chan struct{})

	generatedBy string

	syncer config.UpstreamSyncer
}

func NewServiceController(cfg *rest.Config,
	configStore storage.Interface,
	resyncDuration time.Duration) (*ServiceController, error) {
	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create kube clientset: %v", err)
	}

	// attempt to register upstreams if they don't exist
	if err := configStore.V1().Register(); err != nil && !storage.IsAlreadyExists(err) {
		return nil, errors.Wrap(err, "failed to register upstreams")
	}

	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeClient, resyncDuration)
	serviceInformer := kubeInformerFactory.Core().V1().Services()

	c := &ServiceController{
		errors: make(chan error),

		serviceLister: serviceInformer.Lister(),
		generatedBy :generatedBy,
		
		syncer : config.UpstreamSyncer {
			Owner :  generatedBy,
			GlooStorage : configStore,
		},
	}

	c.syncer.DesiredUpstreams = c.generateDesiredUpstreams

	kubeController := kubecontroller.NewController("gloo-service-discovery", kubeClient,
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

func (c *ServiceController) Run(stop <-chan struct{}) {
	c.runFunc(stop)
}

func (c *ServiceController) Error() <-chan error {
	return c.errors
}

func (c *ServiceController) syncGlooUpstreamsWithKubeServices() {
	if err := c.syncer.SyncDesiredState(); err != nil {
		c.errors <- err
	}
}

func (c *ServiceController) generateDesiredUpstreams() ([]*v1.Upstream, error) {
	serviceList, err := c.serviceLister.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("failed to list ingresses: %v", err)
	}
	var upstreams []*v1.Upstream
	for _, svc := range serviceList {
		// ignore services in the kube-system namespace
		if svc.Namespace == kubeSystemNamespace {
			continue
		}
		// ignore the kubernetes default service
		if svc.Name == "kubernetes" && svc.Namespace == "default" {
			continue
		}

		for _, port := range svc.Spec.Ports {
			// annotate
			annotations := map[string]string{
				ownerAnnotationKey: c.generatedBy,
			}
			for k, v := range svc.Annotations {
				annotations[k] = v
			}
			// copy annotations from the service = happy users
			upstream := &v1.Upstream{
				Name: upstreamName(svc.Namespace, svc.Name, port.Port),
				Type: kubeplugin.UpstreamTypeKube,
				Spec: kubeplugin.EncodeUpstreamSpec(kubeplugin.UpstreamSpec{
					ServiceNamespace: svc.Namespace,
					ServiceName:      svc.Name,
					ServicePort:      port.TargetPort.IntVal,
				}),
				// mark the upstream as ours
				Metadata: &v1.Metadata{
					Annotations: annotations,
				},
			}
			upstreams = append(upstreams, upstream)
		}
	}
	return upstreams, nil
}

func upstreamName(serviceNamespace, serviceName string, servicePort int32) string {
	return fmt.Sprintf("%s-%s-%v", serviceNamespace, serviceName, servicePort)
}
