package jwks

import (
	"context"
	"sync"

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
	jwksCache        *jwksCache
	jwksFetcher      *JwksFetcher
	configMapSyncer  *configMapSyncer
	updates          chan string
	jwksUpdatesQueue utils.AsyncQueue[JwksSource] // TODO (dmitri-d) this can result in lost events
	cmNameToJwks     map[string]string
	l                sync.Mutex
}

func BuildJwksStore(ctx context.Context, cli apiclient.Client, commonCols *collections.CommonCollections, jwksQueue utils.AsyncQueue[JwksSource], deploymentNamespace string) *JwksStore {
	log := log.Log.WithName("jwks store setup")
	log.Info("creating jwks store")

	jwksCache := NewJwksCache()
	jwksStore := &JwksStore{
		jwksCache:        jwksCache,
		jwksUpdatesQueue: jwksQueue,
		jwksFetcher:      NewJwksFetcher(jwksCache),
		configMapSyncer:  NewConfigMapSyncer(cli, deploymentNamespace, commonCols.KrtOpts),
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
	log := log.FromContext(ctx)

	storedJwks, err := s.configMapSyncer.LoadJwksFromConfigMaps(ctx)
	if err != nil {
		log.Error(err, "error loading jwks store from a ConfigMap")
	}
	err = s.jwksCache.LoadJwksFromStores(storedJwks)
	if err != nil {
		log.Error(err, "error loading jwks store state")
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
	log := log.FromContext(ctx)
	for {
		log.Info("dequeuing jwks update")
		jwksUpdate, err := s.jwksUpdatesQueue.Dequeue(ctx)
		if err != nil {
			log.Error(err, "error dequeuing jwks update")
			return
		}

		if jwksUpdate.Deleted {
			s.jwksFetcher.RemoveKeyset(jwksUpdate)
			s.l.Lock()
			delete(s.cmNameToJwks, JwksConfigMapName(jwksUpdate.JwksURL))
			s.l.Unlock()
		} else {
			err := s.jwksFetcher.AddOrUpdateKeyset(jwksUpdate)
			if err != nil {
				log.Error(err, "error adding/updating a jwks keyset", "uri", jwksUpdate.JwksURL)
			}
			s.l.Lock()
			s.cmNameToJwks[JwksConfigMapName(jwksUpdate.JwksURL)] = jwksUpdate.JwksURL
			s.l.Unlock()
		}
	}
}

// JwksStore runs on the leader only
func (r *JwksStore) NeedLeaderElection() bool {
	return true
}
