package trafficpolicy

import (
	"context"
	"fmt"

	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
)

// FetchGatewayExtensionFunc defines the signature for fetching gateway extensions
type FetchGatewayExtensionFunc func(krtctx krt.HandlerContext, extensionRef *corev1.LocalObjectReference, ns string) (*TrafficPolicyGatewayExtensionIR, error)

type TrafficPolicyBuilder struct {
	commoncol         *common.CommonCollections
	gatewayExtensions krt.Collection[TrafficPolicyGatewayExtensionIR]
	extBuilder        func(krtctx krt.HandlerContext, gExt ir.GatewayExtension) *TrafficPolicyGatewayExtensionIR
}

func NewTrafficPolicyBuilder(
	ctx context.Context,
	commoncol *common.CommonCollections,
) *TrafficPolicyBuilder {
	extBuilder := TranslateGatewayExtensionBuilder(commoncol)
	defaultExtBuilder := func(krtctx krt.HandlerContext, gExt ir.GatewayExtension) *TrafficPolicyGatewayExtensionIR {
		return extBuilder(krtctx, gExt)
	}
	gatewayExtensions := krt.NewCollection(commoncol.GatewayExtensions, defaultExtBuilder)
	return &TrafficPolicyBuilder{
		commoncol:         commoncol,
		gatewayExtensions: gatewayExtensions,
		extBuilder:        extBuilder,
	}
}

func (b *TrafficPolicyBuilder) Translate(
	krtctx krt.HandlerContext,
	policyCR *v1alpha1.TrafficPolicy,
) (*TrafficPolicy, []error) {
	policyIr := TrafficPolicy{
		ct: policyCR.CreationTimestamp.Time,
	}
	outSpec := trafficPolicySpecIr{}

	var errors []error
	// Apply AI specific translation
	if err := aiForSpec(krtctx, policyCR, &outSpec, b.commoncol.Secrets); err != nil {
		errors = append(errors, err)
	}
	// Apply transformation specific translation
	if err := transformationForSpec(policyCR, &outSpec); err != nil {
		errors = append(errors, err)
	}
	// Apply rustformation specific translation
	if err := rustformationForSpec(policyCR, &outSpec); err != nil {
		errors = append(errors, err)
	}
	// Apply extproc specific translation
	if err := extProcForSpec(krtctx, policyCR, &outSpec, b.FetchGatewayExtension); err != nil {
		errors = append(errors, err)
	}
	// Apply extauth specific translation
	if err := extAuthForSpec(krtctx, policyCR, &outSpec, b.FetchGatewayExtension); err != nil {
		errors = append(errors, err)
	}
	// Apply local rate limit specific translation
	if err := localRateLimitForSpec(policyCR, &outSpec); err != nil {
		errors = append(errors, err)
	}
	// Apply global rate limit specific translation
	if err := globalRateLimitForSpec(krtctx, policyCR, &outSpec, b.FetchGatewayExtension); err != nil {
		errors = append(errors, err)
	}
	// Apply cors specific translation
	if err := corsForSpec(policyCR, &outSpec); err != nil {
		errors = append(errors, err)
	}

	// Apply csrf specific translation
	err := csrfForSpec(policyCR.Spec, &outSpec)
	if err != nil {
		errors = append(errors, err)
	}

	hashPolicyForSpec(policyCR.Spec, &outSpec)

	// Apply auto host rewrite specific translation
	autoHostRewriteForSpec(policyCR.Spec, &outSpec)

	bufferForSpec(policyCR.Spec, &outSpec)

	for _, err := range errors {
		logger.Error("error translating gateway extension", "namespace", policyCR.GetNamespace(), "name", policyCR.GetName(), "error", err)
	}
	policyIr.spec = outSpec

	return &policyIr, errors
}

func (b *TrafficPolicyBuilder) FetchGatewayExtension(krtctx krt.HandlerContext, extensionRef *corev1.LocalObjectReference, ns string) (*TrafficPolicyGatewayExtensionIR, error) {
	var gatewayExtension *TrafficPolicyGatewayExtensionIR
	if extensionRef != nil {
		gwExtName := types.NamespacedName{Name: extensionRef.Name, Namespace: ns}
		gatewayExtension = krt.FetchOne(krtctx, b.gatewayExtensions, krt.FilterObjectName(gwExtName))
	}
	if gatewayExtension == nil {
		return nil, fmt.Errorf("extension not found")
	}
	if gatewayExtension.Err != nil {
		return gatewayExtension, gatewayExtension.Err
	}
	return gatewayExtension, nil
}

func (b *TrafficPolicyBuilder) HasSynced() bool {
	return b.gatewayExtensions.HasSynced()
}
