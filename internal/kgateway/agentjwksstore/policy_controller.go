package agentjwksstore

import (
	"context"

	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/client-go/tools/cache"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/jwks"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
)

type JwksStorePolicyController struct {
	agw         *plugins.AgwCollections
	apiClient   apiclient.Client
	jwks        krt.Collection[jwks.JwksSource]
	jwksChanges chan jwks.JwksSource
	waitForSync []cache.InformerSynced
}

var polLogger = logging.New("jwks_store_policy_controller")

func NewJWKSStorePolicyController(apiClient apiclient.Client, agw *plugins.AgwCollections) *JwksStorePolicyController {
	polLogger.Info("creating jwks store policy controller")
	return &JwksStorePolicyController{
		agw:         agw,
		apiClient:   apiClient,
		jwksChanges: make(chan jwks.JwksSource),
	}
}

func (j *JwksStorePolicyController) Init(ctx context.Context) {
	policyCol := krt.WrapClient(kclient.NewFilteredDelayed[*v1alpha1.AgentgatewayPolicy](
		j.apiClient,
		wellknown.AgentgatewayPolicyGVR,
		kclient.Filter{ObjectFilter: j.agw.Client.ObjectFilter()},
	), j.agw.KrtOpts.ToOptions("AgentgatewayPolicy")...)
	j.jwks = krt.NewManyCollection(policyCol, func(krtctx krt.HandlerContext, p *v1alpha1.AgentgatewayPolicy) []jwks.JwksSource {
		if p.Spec.Traffic == nil || p.Spec.Traffic.JWTAuthentication == nil {
			return nil
		}

		toret := make([]jwks.JwksSource, 0)
		for _, provider := range p.Spec.Traffic.JWTAuthentication.Providers {
			if provider.JWKS.Remote == nil {
				continue
			}
			toret = append(toret, jwks.JwksSource{JwksURL: provider.JWKS.Remote.JwksUri, Ttl: provider.JWKS.Remote.CacheDuration.Duration})
		}

		return toret
	}, j.agw.KrtOpts.ToOptions("JwksSources")...)

	j.waitForSync = []cache.InformerSynced{
		policyCol.HasSynced,
	}
}

func (j *JwksStorePolicyController) Start(ctx context.Context) error {
	polLogger.Info("waiting for cache to sync")
	j.apiClient.Core().WaitForCacheSync(
		"kube AgentgatewayPolicy syncer",
		ctx.Done(),
		j.waitForSync...,
	)

	polLogger.Info("staring jwks store policy controller")
	j.jwks.Register(func(o krt.Event[jwks.JwksSource]) {
		switch o.Event {
		case controllers.EventAdd, controllers.EventUpdate:
			j.jwksChanges <- *o.New
		case controllers.EventDelete:
			deleted := *o.Old
			deleted.Deleted = true
			j.jwksChanges <- deleted
		}
	})

	<-ctx.Done()
	return nil
}

// runs on the leader only
func (j *JwksStorePolicyController) NeedLeaderElection() bool {
	return true
}

func (j *JwksStorePolicyController) JwksChanges() chan jwks.JwksSource {
	return j.jwksChanges
}
