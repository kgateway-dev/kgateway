package agentjwksstore

import (
	"context"
	"time"

	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/agentgateway"
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
	backendCol := krt.WrapClient(kclient.NewFilteredDelayed[*agentgateway.AgentgatewayBackend](
		j.apiClient,
		wellknown.AgentgatewayBackendGVR,
		kclient.Filter{ObjectFilter: j.agw.Client.ObjectFilter()},
	), j.agw.KrtOpts.ToOptions("AgentgatewayBackend")...)
	policyCol := krt.WrapClient(kclient.NewFilteredDelayed[*agentgateway.AgentgatewayPolicy](
		j.apiClient,
		wellknown.AgentgatewayPolicyGVR,
		kclient.Filter{ObjectFilter: j.agw.Client.ObjectFilter()},
	), j.agw.KrtOpts.ToOptions("AgentgatewayPolicy")...)
	j.jwks = krt.NewSingleton(func(kctx krt.HandlerContext) *jwks.JwksSources {
		pols := krt.Fetch(kctx, policyCol)
		toret := make(jwks.JwksSources, 0, len(pols))
		for _, p := range pols {
			// enqueue Traffic JWT providers (if present)
			if p.Spec.Traffic != nil && p.Spec.Traffic.JWTAuthentication != nil {
				for _, provider := range p.Spec.Traffic.JWTAuthentication.Providers {
					if provider.JWKS.Remote == nil {
						continue
					}
					toret = append(toret, jwks.JwksSource{
						JwksURL: provider.JWKS.Remote.JwksUri,
						Ttl:     provider.JWKS.Remote.CacheDuration.Duration,
					})
				}
			}

			// enqueue Backend MCP authentication JWKS (if present)
			if p.Spec.Backend != nil && p.Spec.Backend.MCP != nil && p.Spec.Backend.MCP.Authentication != nil {
				ttl := time.Duration(0)
				if p.Spec.Backend.MCP.Authentication.JWKS.CacheDuration != nil {
					ttl = p.Spec.Backend.MCP.Authentication.JWKS.CacheDuration.Duration
				}
				if p.Spec.Backend.MCP.Authentication.JWKS.JwksUri != "" {
					toret = append(toret, jwks.JwksSource{
						JwksURL: p.Spec.Backend.MCP.Authentication.JWKS.JwksUri,
						Ttl:     ttl,
					})
				}
			}
		}

		backends := krt.Fetch(kctx, backendCol)
		for _, b := range backends {
			if b.Spec.MCP == nil {
				// ignore non-mcp backend types
				continue
			}
			if b.Spec.Policies != nil && b.Spec.Policies.MCP != nil && b.Spec.Policies.MCP.Authentication != nil {
				ttl := time.Duration(0)
				if b.Spec.Policies.MCP.Authentication.JWKS.CacheDuration != nil {
					ttl = b.Spec.Policies.MCP.Authentication.JWKS.CacheDuration.Duration
				}
				if b.Spec.Policies.MCP.Authentication.JWKS.JwksUri != "" {
					toret = append(toret, jwks.JwksSource{
						JwksURL: b.Spec.Policies.MCP.Authentication.JWKS.JwksUri,
						Ttl:     ttl,
					})
				}
			}
		}

		return &toret
	}, j.agw.KrtOpts.ToOptions("JwksSources")...)

	j.waitForSync = []cache.InformerSynced{
		policyCol.HasSynced,
		backendCol.HasSynced,
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
