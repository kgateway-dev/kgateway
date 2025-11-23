package agentjwksstore

import (
	"context"

	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/jwks"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
)

const JwksStoreConfigMapName = "jwks-store"

type JwksStoreController struct {
	mgr         manager.Manager
	agw         *plugins.AgwCollections
	apiClient   apiclient.Client
	jwks        krt.Singleton[jwks.JwksSources]
	jwksQueue   utils.AsyncQueue[jwks.JwksSources]
	waitForSync []cache.InformerSynced
}

var logger = logging.New("jwks_store")

func NewJWKSStoreController(mgr manager.Manager, apiClient apiclient.Client, agw *plugins.AgwCollections) *JwksStoreController {
	return &JwksStoreController{
		mgr:       mgr,
		agw:       agw,
		apiClient: apiClient,
		jwksQueue: utils.NewAsyncQueue[jwks.JwksSources](),
	}
}

func (j *JwksStoreController) Init(ctx context.Context) {
	policyCol := krt.WrapClient(kclient.NewFilteredDelayed[*v1alpha1.AgentgatewayPolicy](
		j.apiClient,
		wellknown.AgentgatewayPolicyGVR,
		kclient.Filter{ObjectFilter: j.agw.Client.ObjectFilter()},
	), j.agw.KrtOpts.ToOptions("AgentgatewayPolicy")...)
	j.jwks = krt.NewSingleton(func(kctx krt.HandlerContext) *jwks.JwksSources {
		pols := krt.Fetch(kctx, policyCol)
		toret := make(jwks.JwksSources, 0, len(pols))
		for _, p := range pols {
			if p.Spec.Traffic == nil || p.Spec.Traffic.JWTAuthentication == nil {
				continue
			}

			for _, provider := range p.Spec.Traffic.JWTAuthentication.Providers {
				if provider.JWKS.Remote == nil {
					continue
				}
				toret = append(toret, jwks.JwksSource{JwksURL: provider.JWKS.Remote.JwksUri, Ttl: provider.JWKS.Remote.CacheDuration.Duration})
			}
		}

		return &toret
	}, j.agw.KrtOpts.ToOptions("JwksSources")...)

	j.waitForSync = []cache.InformerSynced{
		policyCol.HasSynced,
	}
}

func (j *JwksStoreController) Start(ctx context.Context) error {
	logger.Info("waiting for cache to sync")
	j.apiClient.Core().WaitForCacheSync(
		"kube AgentgatewayPolicy syncer",
		ctx.Done(),
		j.waitForSync...,
	)

	j.jwks.Register(func(o krt.Event[jwks.JwksSources]) {
		j.jwksQueue.Enqueue(o.Latest())
	})

	<-ctx.Done()
	return nil
}

// runs on the leader only
func (j *JwksStoreController) NeedLeaderElection() bool {
	return true
}

func (j *JwksStoreController) JwksQueue() utils.AsyncQueue[jwks.JwksSources] {
	return j.jwksQueue
}
