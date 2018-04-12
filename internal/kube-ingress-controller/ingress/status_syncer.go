package ingress

import (
	"fmt"
	"time"

	"reflect"
	"sync"

	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/kubecontroller"
	kubev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	corev1 "k8s.io/client-go/listers/core/v1"
	v1beta1listers "k8s.io/client-go/listers/extensions/v1beta1"
	"k8s.io/client-go/rest"
)

type IngressSyncer struct {
	errors chan error

	// name of the kubernetes service for the ingress (envoy)
	ingressService string

	// are we reading all ingress classes, or just gloo's?
	globalIngress bool

	client kubernetes.Interface

	ingressLister v1beta1listers.IngressLister
	serviceLister corev1.ServiceLister

	// cache ingress name -> versions so we don't bother updating ingresses in a loop
	cachedStatuses map[string]kubev1.LoadBalancerStatus

	// mutex to protect the map
	mu sync.RWMutex
}

func (c *IngressSyncer) Error() <-chan error {
	return c.errors
}

func NewIngressSyncer(cfg *rest.Config, resyncDuration time.Duration, stopCh <-chan struct{}, globalIngress bool, ingressService string) (*IngressSyncer, error) {
	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create kube clientset: %v", err)
	}
	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeClient, resyncDuration)
	ingressInformer := kubeInformerFactory.Extensions().V1beta1().Ingresses()
	serviceInformer := kubeInformerFactory.Core().V1().Services()

	c := &IngressSyncer{
		errors:         make(chan error),
		ingressService: ingressService,
		globalIngress:  globalIngress,
		client:         kubeClient,
		ingressLister:  ingressInformer.Lister(),
		serviceLister:  serviceInformer.Lister(),
		cachedStatuses: make(map[string]kubev1.LoadBalancerStatus),
	}

	kubeController := kubecontroller.NewController("gloo-ingress-syncer", kubeClient,
		kubecontroller.NewSyncHandler(c.syncIngressStatus),
		ingressInformer.Informer(),
		serviceInformer.Informer())

	go kubeInformerFactory.Start(stopCh)
	go func() {
		kubeController.Run(2, stopCh)
	}()

	return c, nil
}

func (c *IngressSyncer) syncIngressStatus() {
	if err := c.sync(); err != nil {
		c.errors <- err
	}
}

func (c *IngressSyncer) sync() error {
	ingresses, err := c.ingressLister.List(labels.Everything())
	if err != nil {
		return errors.Wrap(err, "failed to list ingresses")
	}
	services, err := c.serviceLister.List(labels.Everything())
	if err != nil {
		return errors.Wrap(err, "failed to list services")
	}
	var service *kubev1.Service
	for _, svc := range services {
		if svc.Name == c.ingressService {
			service = svc
			break
		}
	}
	if service == nil {
		return errors.Errorf("failed to find service %v", c.ingressService)
	}
	for _, ingress := range ingresses {
		if !isOurIngress(c.globalIngress, ingress) {
			continue
		}
		c.mu.RLock()
		// ignore this ingress if it has the same status from our last update
		if reflect.DeepEqual(c.cachedStatuses[ingress.Name], ingress.Status.LoadBalancer) {
			c.mu.RUnlock()
			continue
		}
		c.mu.RUnlock()
		ingress.Status.LoadBalancer = service.Status.LoadBalancer
		updated, err := c.client.ExtensionsV1beta1().Ingresses(ingress.Namespace).Update(ingress)
		if err != nil {
			log.Warnf("failed to update ingress with load status: %v", err)
		}
		c.mu.Lock()
		c.cachedStatuses[ingress.Name] = updated.Status.LoadBalancer
		c.mu.Unlock()
	}
	return nil
}
