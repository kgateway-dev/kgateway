package gatewaytls

import (
	"context"
	"fmt"

	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/query"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/sslutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

type SecretResolver func(secretRef gwv1.SecretObjectReference) (*ir.Secret, error)

func ResolveBackendClientCertificate(
	gateway *ir.Gateway,
	resolveSecret SecretResolver,
) (*ir.GatewayBackendClientCertificateIR, error) {
	if gateway == nil || !gateway.HasBackendClientCertificateRef() {
		return nil, nil
	}

	ref := gateway.BackendTLSConfig.ClientCertificateRef
	if err := validateClientCertificateRef(gateway.GetNamespace(), ref); err != nil {
		return nil, err
	}

	secret, err := resolveSecret(*ref)
	if err != nil {
		return nil, err
	}

	return buildBackendClientCertificate(secret)
}

// ResolveForGateway resolves the Gateway's backend client certificate via the queries-backed
// secret index.
func ResolveForGateway(
	kctx krt.HandlerContext,
	ctx context.Context,
	queries query.GatewayQueries,
	gateway *ir.Gateway,
) (*ir.GatewayBackendClientCertificateIR, error) {
	return ResolveBackendClientCertificate(gateway, func(secretRef gwv1.SecretObjectReference) (*ir.Secret, error) {
		return queries.GetSecretForRef(kctx, ctx, gateway.GetGroupKind(), gateway.GetNamespace(), secretRef)
	})
}

func validateClientCertificateRef(defaultNamespace string, ref *gwv1.SecretObjectReference) error {
	if ref == nil {
		return nil
	}

	group := ""
	if ref.Group != nil {
		group = string(*ref.Group)
	}

	kind := wellknown.SecretKind
	if ref.Kind != nil {
		kind = string(*ref.Kind)
	}

	if group == "" && kind == wellknown.SecretKind {
		return nil
	}

	namespace := defaultNamespace
	if ref.Namespace != nil {
		namespace = string(*ref.Namespace)
	}

	if group == "" {
		return fmt.Errorf(
			"unsupported backend client certificate ref kind %q for %s/%s: only core Secret references are supported",
			kind,
			namespace,
			ref.Name,
		)
	}

	return fmt.Errorf(
		"unsupported backend client certificate ref %q/%q for %s/%s: only core Secret references are supported",
		group,
		kind,
		namespace,
		ref.Name,
	)
}

func buildBackendClientCertificate(secret *ir.Secret) (*ir.GatewayBackendClientCertificateIR, error) {
	if secret == nil {
		return nil, fmt.Errorf("backend client certificate secret is nil")
	}

	certChainBytes, ok := secret.Data[corev1.TLSCertKey]
	if !ok || len(certChainBytes) == 0 {
		return nil, sslutils.InvalidTlsSecretError(
			secret.Name,
			secret.Namespace,
			fmt.Errorf("%s is required", corev1.TLSCertKey),
		)
	}

	privateKeyBytes, ok := secret.Data[corev1.TLSPrivateKeyKey]
	if !ok || len(privateKeyBytes) == 0 {
		return nil, sslutils.InvalidTlsSecretError(
			secret.Name,
			secret.Namespace,
			fmt.Errorf("%s is required", corev1.TLSPrivateKeyKey),
		)
	}

	cleanedCertChain, err := pluginutils.CleanedSslKeyPair(string(certChainBytes), string(privateKeyBytes))
	if err != nil {
		return nil, sslutils.InvalidTlsSecretError(secret.Name, secret.Namespace, err)
	}

	return &ir.GatewayBackendClientCertificateIR{
		Certificate: ir.TLSCertificate{
			CertChain:  []byte(cleanedCertChain),
			PrivateKey: append([]byte(nil), privateKeyBytes...),
		},
	}, nil
}
