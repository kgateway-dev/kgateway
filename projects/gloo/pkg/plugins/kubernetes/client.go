package kubernetes

import (
	"context"
	"sync"
	"time"

	"github.com/pkg/errors"
	"k8s.io/client-go/tools/cache"

	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/controller"
	kubeinformers "k8s.io/client-go/informers"
	kubelisters "k8s.io/client-go/listers/core/v1"

	"k8s.io/client-go/kubernetes"
)

type KubePluginSharedFactory interface {
	EndpointsLister() kubelisters.EndpointsLister
	ServicesLister() kubelisters.ServiceLister
	PodsLister() kubelisters.PodLister
	Subscribe() <-chan struct{}
	Unsubscribe(<-chan struct{})
}

type KubePluginListers struct {
	initError error

	endpointsLister kubelisters.EndpointsLister
	servicesLister  kubelisters.ServiceLister
	podsLister      kubelisters.PodLister

	cacheUpdatedWatchers      []chan struct{}
	cacheUpdatedWatchersMutex sync.Mutex
}

var kubePluginSharedFactory *KubePluginListers
var kubePluginSharedFactoryOnce sync.Once

// TODO(yuval-k): MUST MAKE SURE THAT THIS CLIENT DOESNT HAVE A CONTEXT THAT IS GOING TO EXPIRE!!
func getInformerFactory(client kubernetes.Interface) *KubePluginListers {
	kubePluginSharedFactoryOnce.Do(func() {
		kubePluginSharedFactory = startInformerFactory(context.TODO(), client)
	})
	if kubePluginSharedFactory.initError != nil {
		panic(kubePluginSharedFactory.initError)
	}
	return kubePluginSharedFactory
}

func startInformerFactory(ctx context.Context, client kubernetes.Interface) *KubePluginListers {
	resyncDuration := 12 * time.Hour
	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(client, resyncDuration)

	endpointInformer := kubeInformerFactory.Core().V1().Endpoints()
	podsInformer := kubeInformerFactory.Core().V1().Pods()
	servicesInformer := kubeInformerFactory.Core().V1().Services()

	k := &KubePluginListers{
		endpointsLister: endpointInformer.Lister(),
		servicesLister:  servicesInformer.Lister(),
		podsLister:      podsInformer.Lister(),
	}

	kubeController := controller.NewController("kube-plugin-controller",
		controller.NewLockingSyncHandler(k.updatedOccured),
		endpointInformer.Informer(), podsInformer.Informer(), servicesInformer.Informer())

	stop := ctx.Done()
	go kubeInformerFactory.Start(stop)
	go kubeController.Run(2, stop)

	ok := cache.WaitForCacheSync(stop,
		endpointInformer.Informer().HasSynced,
		podsInformer.Informer().HasSynced,
		servicesInformer.Informer().HasSynced)
	if !ok {
		// if initError is non-nil, the kube resource client will panic
		k.initError = errors.Errorf("waiting for kube pod, endpoints, services cache sync failed")
	}

	return k
}

func (k *KubePluginListers) EndpointsLister() kubelisters.EndpointsLister {
	return k.endpointsLister
}

func (k *KubePluginListers) ServicesLister() kubelisters.ServiceLister {
	return k.servicesLister
}

func (k *KubePluginListers) PodsLister() kubelisters.PodLister {
	return k.podsLister
}

func (k *KubePluginListers) Subscribe() <-chan struct{} {
	k.cacheUpdatedWatchersMutex.Lock()
	defer k.cacheUpdatedWatchersMutex.Unlock()
	c := make(chan struct{}, 1)
	k.cacheUpdatedWatchers = append(k.cacheUpdatedWatchers, c)
	return c
}

func (k *KubePluginListers) Unsubscribe(c <-chan struct{}) {

	k.cacheUpdatedWatchersMutex.Lock()
	defer k.cacheUpdatedWatchersMutex.Unlock()
	for i, cacheUpdated := range k.cacheUpdatedWatchers {
		if cacheUpdated == c {
			k.cacheUpdatedWatchers = append(k.cacheUpdatedWatchers[:i], k.cacheUpdatedWatchers[i+1:]...)
			return
		}
	}
}

func (k *KubePluginListers) updatedOccured() {
	k.cacheUpdatedWatchersMutex.Lock()
	defer k.cacheUpdatedWatchersMutex.Unlock()
	for _, cacheUpdated := range k.cacheUpdatedWatchers {
		select {
		case cacheUpdated <- struct{}{}:
		default:
		}
	}
}
