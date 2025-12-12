package jwks

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	"github.com/kgateway-dev/kgateway/v2/pkg/common"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
)

const DefaultJwksStorePrefix = "jwks-store"
const RunnableName = "jwks-store"

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
	latestJwks      <-chan JwksSources
}

func BuildJwksStore(ctx context.Context, cli apiclient.Client, commonCols *collections.CommonCollections, jwksQueue <-chan JwksSources, storePrefix, deploymentNamespace string) *JwksStore {
	log := log.Log.WithName("jwks store setup")
	log.Info("creating jwks store", "prefix", storePrefix)

	jwksCache := NewJwksCache()
	jwksStore := &JwksStore{
		jwksCache:       jwksCache,
		latestJwks:      jwksQueue,
		jwksFetcher:     NewJwksFetcher(jwksCache),
		configMapSyncer: NewConfigMapSyncer(cli, storePrefix, deploymentNamespace, commonCols.KrtOpts),
	}
	jwksStore.updates = jwksStore.jwksFetcher.SubscribeToUpdates()
	BuildJwksConfigMapNamespacedNameFunc(storePrefix, deploymentNamespace)
	return jwksStore
}

func BuildJwksConfigMapNamespacedNameFunc(storePrefix, deploymentNamespace string) {
	JwksConfigMapNamespacedName = func(jwksUri string) *types.NamespacedName {
		return &types.NamespacedName{Namespace: deploymentNamespace, Name: JwksConfigMapName(storePrefix, jwksUri)}
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
	for {
		select {
		case jwks := <-s.latestJwks:
			s.jwksFetcher.UpdateJwksSources(ctx, jwks)
		case <-ctx.Done():
			return
		}
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

var _ common.NamedRunnable = &JwksStore{}

func (r *JwksStore) RunnableName() string {
	return RunnableName
}
