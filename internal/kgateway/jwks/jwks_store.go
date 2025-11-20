package jwks

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
)

var JwksConfigMapNamespacedName = func(jwksUri string) *types.NamespacedName {
	return nil
}

// JwksStore handles initial fetching and periodic updates of jwks. Jwks are persisted
// in ConfigMaps, a jwks per ConfigMap. The ConfigMaps are used to re-create internal
// JwksStore state on startup and by traffic-plugins as source of remote jwks.
type JwksStore struct {
	jwksCache       *jwksCache
	jwksFetcher     *JwksFetcher
	configMapSyncer *configMapSyncer
	updates         <-chan map[string]string
	latestJwksQueue utils.AsyncQueue[JwksSources]
}

func BuildJwksStore(ctx context.Context, cli apiclient.Client, commonCols *collections.CommonCollections, jwksQueue utils.AsyncQueue[JwksSources], deploymentNamespace string) *JwksStore {
	log := log.Log.WithName("jwks store setup")
	log.Info("creating jwks store")

	jwksCache := NewJwksCache()
	jwksStore := &JwksStore{
		jwksCache:       jwksCache,
		latestJwksQueue: jwksQueue,
		jwksFetcher:     NewJwksFetcher(jwksCache),
		configMapSyncer: NewConfigMapSyncer(cli, deploymentNamespace, commonCols.KrtOpts),
	}
	jwksStore.updates = jwksStore.jwksFetcher.SubscribeToUpdates()
	BuildJwksConfigMapNamespacedNameFunc(deploymentNamespace)
	return jwksStore
}

func BuildJwksConfigMapNamespacedNameFunc(deploymentNamespace string) {
	JwksConfigMapNamespacedName = func(jwksUri string) *types.NamespacedName {
		return &types.NamespacedName{Namespace: deploymentNamespace, Name: JwksConfigMapName(jwksUri)}
	}
}

func (s *JwksStore) Start(ctx context.Context) error {
	log := log.FromContext(ctx)

	s.configMapSyncer.WaitForCacheSync(ctx)

	storedJwks, err := s.configMapSyncer.LoadJwksFromConfigMaps(ctx)
	if err != nil {
		log.Error(err, "error loading jwks store from a ConfigMap")
	}
	err = s.jwksCache.LoadJwksFromStores(storedJwks)
	if err != nil {
		log.Error(err, "error loading jwks store state")
	}

	go s.syncToConfigMaps(ctx)
	go s.jwksFetcher.Run(ctx)
	go s.updateJwksSources(ctx)

	<-ctx.Done()
	return nil
}

func (s *JwksStore) updateJwksSources(ctx context.Context) {
	log := log.FromContext(ctx)
	for {
		log.Info("dequeuing jwks update")
		latestJwks, err := s.latestJwksQueue.Dequeue(ctx)
		if err != nil {
			log.Error(err, "error dequeuing jwks update")
			return
		}
		s.jwksFetcher.UpdateJwksSources(ctx, latestJwks)
	}
}

func (s *JwksStore) syncToConfigMaps(ctx context.Context) {
	log := log.FromContext(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case update := <-s.updates:
			log.Info("received an update")
			err := s.configMapSyncer.WriteJwksToConfigMaps(ctx, update)
			if err != nil {
				log.Error(err, "error(s) syncing jwks cache to ConfigMaps")
			}
		}
	}
}

// JwksStore runs on the leader only
func (r *JwksStore) NeedLeaderElection() bool {
	return true
}
