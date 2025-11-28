package jwks

import (
	"context"
	"sync"

	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
)

var logger = logging.New("jwks_store")

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
	updates         chan string
	jwksChanges     <-chan JwksSource
	cmNameToJwks    map[string]string
	l               sync.Mutex
}

func BuildJwksStore(ctx context.Context, cli apiclient.Client, commonCols *collections.CommonCollections, jwksChanges <-chan JwksSource, deploymentNamespace string) *JwksStore {
	logger.Info("creating jwks store")

	jwksCache := NewJwksCache()
	jwksStore := &JwksStore{
		jwksCache:       jwksCache,
		jwksChanges:     jwksChanges,
		jwksFetcher:     NewJwksFetcher(jwksCache),
		configMapSyncer: NewConfigMapSyncer(cli, deploymentNamespace, commonCols.KrtOpts),
		cmNameToJwks:    make(map[string]string),
	}

	BuildJwksConfigMapNamespacedNameFunc(deploymentNamespace)
	return jwksStore
}

func BuildJwksConfigMapNamespacedNameFunc(deploymentNamespace string) {
	JwksConfigMapNamespacedName = func(jwksUri string) *types.NamespacedName {
		return &types.NamespacedName{Namespace: deploymentNamespace, Name: JwksConfigMapName(jwksUri)}
	}
}

func (s *JwksStore) Start(ctx context.Context) error {
	logger.Info("staring jwks store")

	storedJwks, err := s.configMapSyncer.LoadJwksFromConfigMaps(ctx)
	if err != nil {
		logger.Error("error loading jwks store from a ConfigMap", "error", err)
	}
	err = s.jwksCache.LoadJwksFromStores(storedJwks)
	if err != nil {
		logger.Error("error loading jwks store state", "error", err)
	}

	go s.jwksFetcher.Run(ctx)
	go s.updateJwksSources(ctx)

	<-ctx.Done()
	return nil
}

func (s *JwksStore) SubscribeToUpdates() chan map[string]string {
	return s.jwksFetcher.SubscribeToUpdates()
}

func (s *JwksStore) JwksByConfigMapName(cmName string) (string, string, bool) {
	s.l.Lock()
	defer s.l.Unlock()

	uri, ok := s.cmNameToJwks[cmName]
	if !ok {
		return "", "", false
	}

	jwks, ok := s.jwksCache.GetJwks(uri)
	if !ok {
		return "", "", false
	}

	return uri, jwks, true
}

func (s *JwksStore) updateJwksSources(ctx context.Context) {
	for {
		select {
		case jwksUpdate := <-s.jwksChanges:
			if jwksUpdate.Deleted {
				logger.Info("deleting keyset")
				s.jwksFetcher.RemoveKeyset(jwksUpdate)
				s.l.Lock()
				delete(s.cmNameToJwks, JwksConfigMapName(jwksUpdate.JwksURL))
				s.l.Unlock()
			} else {
				logger.Info("updating keyset")
				err := s.jwksFetcher.AddOrUpdateKeyset(jwksUpdate)
				if err != nil {
					logger.Error("error adding/updating a jwks keyset", "error", err, "uri", jwksUpdate.JwksURL)
				} else {
					s.l.Lock()
					s.cmNameToJwks[JwksConfigMapName(jwksUpdate.JwksURL)] = jwksUpdate.JwksURL
					s.l.Unlock()
				}
			}
		case <-ctx.Done():
			return
		}
	}
}

// JwksStore runs on the leader only
func (r *JwksStore) NeedLeaderElection() bool {
	return true
}
