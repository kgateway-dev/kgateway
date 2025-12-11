package agentjwksstore

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"time"

	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/ptr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	v1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/agentgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/jwks"
	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	krtpkg "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
)

type JwksStorePolicyController struct {
	agw                      *plugins.AgwCollections
	cfgmaps                  krt.Collection[*corev1.ConfigMap]
	policiesByTargetRefIndex krt.Index[targetRefIndexKey, *agentgateway.AgentgatewayPolicy]
	apiClient                apiclient.Client
	jwks                     krt.Collection[jwks.JwksSource]
	jwksChanges              chan jwks.JwksSource
	waitForSync              []cache.InformerSynced
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

func NewJWKSStorePolicyController(apiClient apiclient.Client, agw *plugins.AgwCollections) *JwksStorePolicyController {
	polLogger.Info("creating jwks store policy controller")
	return &JwksStorePolicyController{
		agw:         agw,
		apiClient:   apiClient,
		jwksChanges: make(chan jwks.JwksSource),
	}
}

func (j *JwksStorePolicyController) Init(ctx context.Context) {
	backendCol := krt.WrapClient(kclient.NewFilteredDelayed[*agentgateway.AgentgatewayBackend](
		j.apiClient,
		wellknown.AgentgatewayBackendGVR,
		kclient.Filter{ObjectFilter: j.agw.Client.ObjectFilter()},
	), j.agw.KrtOpts.ToOptions("AgentgatewayBackend")...)

	cmClient := kclient.NewFiltered[*corev1.ConfigMap](
		j.apiClient,
		kclient.Filter{ObjectFilter: j.agw.Client.ObjectFilter()},
	)
	j.cfgmaps = krt.WrapClient(cmClient, j.agw.KrtOpts.ToOptions("ConfigMaps")...)

	j.policiesByTargetRefIndex = krtpkg.UnnamedIndex(j.agw.AgentgatewayPolicies, func(in *agentgateway.AgentgatewayPolicy) []targetRefIndexKey {
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
	j.jwks = krt.NewManyCollection(j.agw.AgentgatewayPolicies, func(kctx krt.HandlerContext, p *agentgateway.AgentgatewayPolicy) []jwks.JwksSource {
		toret := make([]jwks.JwksSource, 0)

		// enqueue Traffic JWT providers (if present)
		if p.Spec.Traffic != nil && p.Spec.Traffic.JWTAuthentication != nil {
			for _, provider := range p.Spec.Traffic.JWTAuthentication.Providers {
				if provider.JWKS.Remote == nil {
					continue
				}

				if s := j.jwksSourceWithCustomTLSConfig(kctx, p.Name, p.Namespace, provider.JWKS.Remote); s != nil {
					toret = append(toret, *s)
				}
			}
		}

		// enqueue Backend MCP authentication JWKS (if present)
		if p.Spec.Backend != nil && p.Spec.Backend.MCP != nil && p.Spec.Backend.MCP.Authentication != nil {
			ttl := 5 * time.Minute
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

		backends := krt.Fetch(kctx, backendCol)
		for _, b := range backends {
			if b.Spec.MCP == nil {
				// ignore non-mcp backend types
				continue
			}
			if b.Spec.Policies != nil && b.Spec.Policies.MCP != nil && b.Spec.Policies.MCP.Authentication != nil {
				ttl := 5 * time.Minute
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

		return toret
	}, j.agw.KrtOpts.ToOptions("JwksSources")...)

	j.waitForSync = []cache.InformerSynced{
		backendCol.HasSynced,
	}
}

func (j *JwksStorePolicyController) Start(ctx context.Context) error {
	polLogger.Info("waiting for cache to sync")
	j.apiClient.Core().WaitForCacheSync(
		"kube AgentgatewayPolicy syncer",
		ctx.Done(),
		j.waitForSync...,
	)

	polLogger.Info("starting jwks store policy controller")
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

func (j *JwksStorePolicyController) jwksSourceWithCustomTLSConfig(krtctx krt.HandlerContext, policyName, defaultNS string, remoteProvider *agentgateway.RemoteJWKS) *jwks.JwksSource {
	ref := *remoteProvider.BackendRef

	refName := string(ref.Name)
	refNamespace := string(ptr.OrDefault(ref.Namespace, v1.Namespace(defaultNS)))

	switch string(*ref.Kind) {
	case wellknown.AgentgatewayBackendGVK.Kind:
		backendRef := types.NamespacedName{
			Name:      refName,
			Namespace: refNamespace,
		}
		backend := ptr.Flatten(krt.FetchOne(krtctx, j.agw.Backends, krt.FilterObjectName(backendRef)))
		if backend == nil {
			polLogger.Error("backend not found", "backend", backendRef, "policy", types.NamespacedName{Namespace: defaultNS, Name: policyName})
			return nil
		}
		if backend.Spec.Static == nil {
			polLogger.Error("only static backends are supported", "backend", backendRef, "policy", types.NamespacedName{Namespace: defaultNS, Name: policyName})
			return nil
		}

		var tlsConfig *tls.Config
		if backend.Spec.Policies != nil && backend.Spec.Policies.TLS != nil {
			tlsc, err := getTLSConfig(krtctx, j.cfgmaps, refNamespace, backend.Spec.Policies.TLS)
			if err != nil {
				polLogger.Error(
					"error setting tls options",
					"backend", backendRef, "policy", types.NamespacedName{Namespace: refNamespace, Name: policyName}, "error", err)
			}
			tlsConfig = tlsc
		}

		var url string
		if tlsConfig == nil {
			url = fmt.Sprintf("http://%s:%d/%s", backend.Spec.Static.Host, backend.Spec.Static.Port, remoteProvider.JwksUri)
		} else {
			url = fmt.Sprintf("https://%s:%d/%s", backend.Spec.Static.Host, backend.Spec.Static.Port, remoteProvider.JwksUri)
		}

		return &jwks.JwksSource{
			JwksURL:   url,
			TlsConfig: tlsConfig,
			Ttl:       remoteProvider.CacheDuration.Duration,
		}
	case wellknown.ServiceKind:
		agwPolicy := ptr.Flatten(krt.FetchOne(krtctx, j.agw.AgentgatewayPolicies, krt.FilterIndex(j.policiesByTargetRefIndex, targetRefIndexKey{
			Name:      refName,
			Kind:      string(*ref.Kind),
			Group:     string(ptr.OrEmpty(ref.Group)),
			Namespace: refNamespace,
		})))

		var tlsConfig *tls.Config
		if agwPolicy.Spec.Backend != nil && agwPolicy.Spec.Backend.TLS != nil {
			tlsc, err := getTLSConfig(krtctx, j.cfgmaps, refNamespace, agwPolicy.Spec.Backend.TLS)
			if err != nil {
				polLogger.Error(
					"error setting tls options",
					"service", refNamespace+"/"+refName, "policy", types.NamespacedName{Namespace: refNamespace, Name: policyName}, "error", err)
			}
			tlsConfig = tlsc
		}

		clusterDomain := kubeutils.GetClusterDomainName()
		host := fmt.Sprintf("%s.%s.svc.%s", refName, refNamespace, clusterDomain)
		var fqdn string
		if port := ptr.OrEmpty(ref.Port); port != 0 {
			fqdn = fmt.Sprintf("%s:%d", host, port)
		} else {
			fqdn = host
		}

		var url string
		if tlsConfig == nil {
			url = fmt.Sprintf("http://%s/%s", fqdn, remoteProvider.JwksUri)
		} else {
			url = fmt.Sprintf("https://%s/%s", fqdn, remoteProvider.JwksUri)
		}

		return &jwks.JwksSource{
			JwksURL:   url,
			TlsConfig: tlsConfig,
			Ttl:       remoteProvider.CacheDuration.Duration,
		}
	}

	polLogger.Error("unsupported target kind in remote jwks provider", "kind", ref.Kind, "policy", policyName)
	return nil
}

func getTLSConfig(
	krtctx krt.HandlerContext,
	cfgmaps krt.Collection[*corev1.ConfigMap],
	namespace string,
	btls *agentgateway.BackendTLS,
) (*tls.Config, error) {
	toret := tls.Config{
		ServerName:         ptr.OrEmpty(btls.Sni),
		InsecureSkipVerify: insecureSkipVerify(btls.InsecureSkipVerify),
		NextProtos:         ptr.OrEmpty(btls.AlpnProtocols),
	}

	if len(btls.CACertificateRefs) > 0 {
		certPool := x509.NewCertPool()
		for _, ref := range btls.CACertificateRefs {
			nn := types.NamespacedName{
				Name:      string(ref.Name),
				Namespace: namespace,
			}
			cfgmap := krt.FetchOne(krtctx, cfgmaps, krt.FilterObjectName(nn))
			if cfgmap == nil {
				return nil, fmt.Errorf("ConfigMap %s not found", nn)
			}
			success := appendPoolWithCertsFromConfigMap(certPool, ptr.Flatten(cfgmap))
			if !success {
				return nil, fmt.Errorf("error extracting CA cert from ConfigMap %s", nn)
			}
		}
		toret.RootCAs = certPool
	}

	return &toret, nil
}

func appendPoolWithCertsFromConfigMap(pool *x509.CertPool, cm *corev1.ConfigMap) bool {
	caCrts, ok := cm.Data["ca.crt"]
	if !ok {
		return false
	}
	return pool.AppendCertsFromPEM([]byte(caCrts))
}

func insecureSkipVerify(mode *agentgateway.InsecureTLSMode) bool {
	return mode != nil
}
