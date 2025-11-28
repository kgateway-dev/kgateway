package agentjwksstore

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"time"

	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/ptr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/jwks"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	krtpkg "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
)

type JwksStorePolicyController struct {
	agw                       *plugins.AgwCollections
	commonCol                 *collections.CommonCollections
	apiClient                 apiclient.Client
	jwks                      krt.Collection[jwks.JwksSource]
	jwksChanges               chan jwks.JwksSource
	waitForSync               []cache.InformerSynced
	tlsPolicyByTragetRefIndex krt.Index[targetRefIndexKey, *gwv1.BackendTLSPolicy]
}

type targetRefIndexKey struct {
	Group     string
	Kind      string
	Name      string
	Namespace string
}

func (k targetRefIndexKey) String() string {
	return fmt.Sprintf("%s:%s:%s:%s", k.Group, k.Kind, k.Namespace, k.Name)
}

var polLogger = logging.New("jwks_store_policy_controller")

func NewJWKSStorePolicyController(apiClient apiclient.Client, commonCol *collections.CommonCollections, agw *plugins.AgwCollections) *JwksStorePolicyController {
	polLogger.Info("creating jwks store policy controller")
	return &JwksStorePolicyController{
		agw:         agw,
		commonCol:   commonCol,
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

	j.tlsPolicyByTragetRefIndex = krtpkg.UnnamedIndex(j.agw.BackendTLSPolicies, func(in *gwv1.BackendTLSPolicy) []targetRefIndexKey {
		keys := make([]targetRefIndexKey, 0)
		for _, ref := range in.Spec.TargetRefs {
			keys = append(keys, targetRefIndexKey{
				Name:      string(ref.Name),
				Kind:      string(ref.Kind),
				Group:     string(ref.Group),
				Namespace: in.Namespace,
			})
		}
		return keys
	})

	// TODO JwksSource should be per-policy, i.e. the same jwks url for multiple policies should result in multiple JwksSources
	// Otherwise changes to one policy (removal for example) could result in disruption of traffic for other policies (while ConfigMaps are re-synced)
	j.jwks = krt.NewManyCollection(policyCol, func(krtctx krt.HandlerContext, p *v1alpha1.AgentgatewayPolicy) []jwks.JwksSource {
		if p.Spec.Traffic == nil || p.Spec.Traffic.JWTAuthentication == nil {
			return nil
		}

		toret := make([]jwks.JwksSource, 0)
		for _, provider := range p.Spec.Traffic.JWTAuthentication.Providers {
			if provider.JWKS.Remote == nil {
				continue
			}

			if provider.JWKS.Remote.BackendRef == nil {
				toret = append(toret, jwksSourceWithDefaultHttpClient(provider.JWKS.Remote.JwksUri, provider.JWKS.Remote.CacheDuration.Duration))
				continue
			}

			if s := j.jwksSourceWithCustomTLSConfig(krtctx, p.Name, p.Namespace, provider.JWKS.Remote); s != nil {
				toret = append(toret, *s)
			}
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
			polLogger.Info("add/update jwks source", "jwks", o.New.JwksURL)
			j.jwksChanges <- *o.New
		case controllers.EventDelete:
			deleted := *o.Old
			deleted.Deleted = true
			polLogger.Info("deleting jwks source", "jwks", deleted.JwksURL)
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

func jwksSourceWithDefaultHttpClient(jwksURL string, ttl time.Duration) jwks.JwksSource {
	return jwks.JwksSource{JwksURL: jwksURL, Ttl: ttl}
}

func (j *JwksStorePolicyController) jwksSourceWithCustomTLSConfig(krtctx krt.HandlerContext, policyName, defaultNS string, remoteProvider *v1alpha1.AgentRemoteJWKS) *jwks.JwksSource {
	ref := *remoteProvider.BackendRef
	refName := string(ref.Name)
	refNamespace := string(ptr.OrDefault(ref.Namespace, gwv1.Namespace(defaultNS)))

	if string(*ref.Kind) != wellknown.AgentgatewayBackendGVK.Kind && string(*ref.Kind) != wellknown.ServiceKind {
		// backendRef := types.NamespacedName{
		// 	Name:      refName,
		// 	Namespace: refNamespace,
		// }
		// backend := ptr.Flatten(krt.FetchOne(krtctx, j.agw.Backends, krt.FilterObjectName(backendRef)))
		// if backend == nil {
		// 	logger.Error("backend not found; skipping policy", "backend", backendRef, "policy", kubeutils.NamespacedNameFrom(btls))
		// 	return nil
		// }
		// if backend.Spec.Static == nil {
		// 	logger.Error("static only")
		// 	return nil
		// }

		// jwksUrlPrefix = fmt.Sprintf("https://%s:%s/", backend.Spec.Static.Host, backend.Spec.Static.Port)
		// case wellknown.ServiceKind:
		// clusterDomain := kubeutils.GetClusterDomainName()
		// host := fmt.Sprintf("%s.%s.svc.%s", refName, refNamespace, clusterDomain)
		// // If SectionName is specified to select the port, use service/<namespace>/<hostname>:<port>
		// if port := string(ptr.OrEmpty(ref.Port)); port != "" {
		// 	jwksUrlPrefix = fmt.Sprintf("https://%s:%s/", host, port)
		// } else {
		// 	jwksUrlPrefix = fmt.Sprintf("https://%s/", host)
		// }
		polLogger.Error("unsupported target kind in remote jwks provider", "kind", ref.Kind, "policy", policyName)
		return nil
	}

	var tlsConfig *tls.Config
	tlsPolicy := ptr.Flatten(krt.FetchOne(krtctx, j.agw.BackendTLSPolicies,
		krt.FilterIndex(j.tlsPolicyByTragetRefIndex, targetRefIndexKey{
			Name:      refName,
			Kind:      string(*ref.Kind),
			Group:     string(ptr.OrEmpty(ref.Group)),
			Namespace: refNamespace,
		})))
	if tlsPolicy != nil {
		t, err := getTLSConfig(krtctx, j.commonCol.ConfigMaps, tlsPolicy)
		if err != nil {
			polLogger.Error("error creating tls config", "error", err)
			return nil
		}
		tlsConfig = t
	}

	return &jwks.JwksSource{
		JwksURL:   remoteProvider.JwksUri,
		TlsConfig: tlsConfig,
		Ttl:       remoteProvider.CacheDuration.Duration,
	}
}

func getTLSConfig(
	krtctx krt.HandlerContext,
	cfgmaps krt.Collection[*corev1.ConfigMap],
	btls *gwv1.BackendTLSPolicy,
) (*tls.Config, error) {
	// handle InsecureSkipVerify via Options?
	validation := btls.Spec.Validation

	toret := tls.Config{
		ServerName: string(validation.Hostname),
	}

	if wk := validation.WellKnownCACertificates; wk != nil && *wk == gwv1.WellKnownCACertificatesSystem {
		return &toret, nil
	}

	if len(validation.CACertificateRefs) > 0 {
		certPool := x509.NewCertPool()
		for _, ref := range validation.CACertificateRefs {
			if ref.Group != gwv1.Group(wellknown.ConfigMapGVK.Group) || ref.Kind != gwv1.Kind(wellknown.ConfigMapGVK.Kind) {
				return nil, fmt.Errorf("BackendTLSPolicy's validation.caCertificateRefs must be a ConfigMap reference; got %s", ref)
			}
			nn := types.NamespacedName{
				Name:      string(ref.Name),
				Namespace: btls.Namespace,
			}
			cfgmap := krt.FetchOne(krtctx, cfgmaps, krt.FilterObjectName(nn))
			if cfgmap == nil {
				return nil, fmt.Errorf("ConfigMap %s not found", nn)
			}
			success := appendPoolWithCertsFromConfigMap(certPool, ptr.Flatten(cfgmap))
			if !success {
				return nil, fmt.Errorf("error extracting CA cert from ConfigMap %s: %w", nn)
			}
		}
		toret.RootCAs = certPool
		return &toret, nil
	}

	// should never happen as this is CEL validated.
	return nil, errors.New("BackendTLSPolicy must specify either wellKnownCACertificates or caCertificateRefs")
}

func appendPoolWithCertsFromConfigMap(pool *x509.CertPool, cm *corev1.ConfigMap) bool {
	caCrts, ok := cm.Data["ca.crt"]
	if !ok {
		return false
	}
	return pool.AppendCertsFromPEM([]byte(caCrts))
}
