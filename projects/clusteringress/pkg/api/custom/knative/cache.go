package knative

import (
	"context"
	"sync"
	"time"

	knativeclient "github.com/knative/serving/pkg/client/clientset/versioned"
	knativeinformers "github.com/knative/serving/pkg/client/informers/externalversions"
	knativelisters "github.com/knative/serving/pkg/client/listers/networking/v1alpha1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/controller"
)

type Cache interface {
	ClusterIngressLister() knativelisters.ClusterIngressLister
	Subscribe() <-chan struct{}
	Unsubscribe(<-chan struct{})
}

type knativeCache struct {
	clusterIngress knativelisters.ClusterIngressLister

	cacheUpdatedWatchers      []chan struct{}
	cacheUpdatedWatchersMutex sync.Mutex
}

// This context should live as long as the cache is desired. i.e. if the cache is shared
// across clients, it should get a context that has a longer lifetime than the clients themselves
func NewClusterIngreessCache(ctx context.Context, knativeClient knativeclient.Interface) (*knativeCache, error) {
	resyncDuration := 12 * time.Hour
	sharedInformerFactory := knativeinformers.NewSharedInformerFactory(knativeClient, resyncDuration)

	clusterIngress := sharedInformerFactory.Networking().V1alpha1().ClusterIngresses()

	k := &knativeCache{
		clusterIngress: clusterIngress.Lister(),
	}

	kubeController := controller.NewController("knative-resources-cache",
		controller.NewLockingSyncHandler(k.updatedOccured),
		clusterIngress.Informer())

	stop := ctx.Done()
	err := kubeController.Run(2, stop)
	if err != nil {
		return nil, err
	}

	return k, nil
}

func (k *knativeCache) ClusterIngressLister() knativelisters.ClusterIngressLister {
	return k.clusterIngress
}

func (k *knativeCache) Subscribe() <-chan struct{} {
	k.cacheUpdatedWatchersMutex.Lock()
	defer k.cacheUpdatedWatchersMutex.Unlock()
	c := make(chan struct{}, 10)
	k.cacheUpdatedWatchers = append(k.cacheUpdatedWatchers, c)
	return c
}

func (k *knativeCache) Unsubscribe(c <-chan struct{}) {
	k.cacheUpdatedWatchersMutex.Lock()
	defer k.cacheUpdatedWatchersMutex.Unlock()
	for i, cacheUpdated := range k.cacheUpdatedWatchers {
		if cacheUpdated == c {
			k.cacheUpdatedWatchers = append(k.cacheUpdatedWatchers[:i], k.cacheUpdatedWatchers[i+1:]...)
			return
		}
	}
}

func (k *knativeCache) updatedOccured() {
	k.cacheUpdatedWatchersMutex.Lock()
	defer k.cacheUpdatedWatchersMutex.Unlock()
	for _, cacheUpdated := range k.cacheUpdatedWatchers {
		select {
		case cacheUpdated <- struct{}{}:
		default:
		}
	}
}
