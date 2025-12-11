package plugins

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"

	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/ptr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	v1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/agentgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
)

type JwksUrlFactory struct {
	cfgmaps                  krt.Collection[*corev1.ConfigMap]
	policiesByTargetRefIndex krt.Index[TargetRefIndexKey, *agentgateway.AgentgatewayPolicy]
	backends                 krt.Collection[*agentgateway.AgentgatewayBackend]
	agentgatewayPolicies     krt.Collection[*agentgateway.AgentgatewayPolicy]
}

func NewJwksUrlFactory(cfgmaps krt.Collection[*corev1.ConfigMap],
	backends krt.Collection[*agentgateway.AgentgatewayBackend],
	agentgatewayPolicies krt.Collection[*agentgateway.AgentgatewayPolicy],
	policiesByTargetRefIndex krt.Index[TargetRefIndexKey, *agentgateway.AgentgatewayPolicy]) *JwksUrlFactory {

	return &JwksUrlFactory{
		cfgmaps:                  cfgmaps,
		policiesByTargetRefIndex: policiesByTargetRefIndex,
		backends:                 backends,
		agentgatewayPolicies:     agentgatewayPolicies,
	}
}

func (j *JwksUrlFactory) BuildJwksUrl(krtctx krt.HandlerContext, policyName, defaultNS string, remoteProvider *agentgateway.RemoteJWKS) (string, *tls.Config, error) {
	ref := remoteProvider.BackendRef

	refName := string(ref.Name)
	refNamespace := string(ptr.OrDefault(ref.Namespace, v1.Namespace(defaultNS)))

	switch string(*ref.Kind) {
	case wellknown.AgentgatewayBackendGVK.Kind:
		backendRef := types.NamespacedName{
			Name:      refName,
			Namespace: refNamespace,
		}
		backend := ptr.Flatten(krt.FetchOne(krtctx, j.backends, krt.FilterObjectName(backendRef)))
		if backend == nil {
			return "", nil, fmt.Errorf("backend %s not found, policy %s", backendRef, types.NamespacedName{Namespace: defaultNS, Name: policyName})
		}
		if backend.Spec.Static == nil {
			return "", nil, fmt.Errorf("only static backends are supported; backend: %s, policy: %s", backendRef, types.NamespacedName{Namespace: defaultNS, Name: policyName})
		}

		// TODO (dmitri-d) only inline tls config is supported atm, do we want to support attching AgentgatewayPolicy too?
		var tlsConfig *tls.Config
		if backend.Spec.Policies != nil && backend.Spec.Policies.TLS != nil {
			tlsc, err := getTLSConfig(krtctx, j.cfgmaps, refNamespace, backend.Spec.Policies.TLS)
			if err != nil {
				return "", nil, fmt.Errorf("error setting tls options; backend: %s, policy: %s, %w",
					backendRef, types.NamespacedName{Namespace: refNamespace, Name: policyName}, err)
			}
			tlsConfig = tlsc
		}

		var url string
		if tlsConfig == nil {
			url = fmt.Sprintf("http://%s:%d/%s", backend.Spec.Static.Host, backend.Spec.Static.Port, remoteProvider.JwksPath)
		} else {
			url = fmt.Sprintf("https://%s:%d/%s", backend.Spec.Static.Host, backend.Spec.Static.Port, remoteProvider.JwksPath)
		}

		return url, tlsConfig, nil
	case wellknown.ServiceKind:
		agwPolicy := ptr.Flatten(krt.FetchOne(krtctx, j.agentgatewayPolicies, krt.FilterIndex(j.policiesByTargetRefIndex, TargetRefIndexKey{
			Name:      refName,
			Kind:      string(*ref.Kind),
			Group:     string(ptr.OrEmpty(ref.Group)),
			Namespace: refNamespace,
		})))

		var tlsConfig *tls.Config
		if agwPolicy.Spec.Backend != nil && agwPolicy.Spec.Backend.TLS != nil {
			tlsc, err := getTLSConfig(krtctx, j.cfgmaps, refNamespace, agwPolicy.Spec.Backend.TLS)
			if err != nil {
				return "", nil, fmt.Errorf("error setting tls options; service %s/%s, policy: %s %w",
					refName, refNamespace, types.NamespacedName{Namespace: refNamespace, Name: policyName}, err)
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
			url = fmt.Sprintf("http://%s/%s", fqdn, remoteProvider.JwksPath)
		} else {
			url = fmt.Sprintf("https://%s/%s", fqdn, remoteProvider.JwksPath)
		}

		return url, tlsConfig, nil
	}

	return "", nil, fmt.Errorf("unsupported target kind in remote jwks provider; kind: %s, policy: %s", string(*ref.Kind), types.NamespacedName{Namespace: refNamespace, Name: policyName})
}

func getTLSConfig(
	krtctx krt.HandlerContext,
	cfgmaps krt.Collection[*corev1.ConfigMap],
	namespace string,
	btls *agentgateway.BackendTLS,
) (*tls.Config, error) {
	toret := tls.Config{
		ServerName:         ptr.OrEmpty(btls.Sni),
		InsecureSkipVerify: insecureSkipVerify(btls.InsecureSkipVerify), //nolint:gosec
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
