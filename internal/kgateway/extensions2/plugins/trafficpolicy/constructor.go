package trafficpolicy

import (
	"context"
	"fmt"

	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
)

// FetchGatewayExtensionFunc defines the signature for fetching gateway extensions
type FetchGatewayExtensionFunc func(krtctx krt.HandlerContext, extensionRef v1alpha1.NamespacedObjectReference, ns string) (*TrafficPolicyGatewayExtensionIR, error)

type TrafficPolicyConstructor struct {
	commoncol         *common.CommonCollections
	gatewayExtensions krt.Collection[TrafficPolicyGatewayExtensionIR]
	extBuilder        func(krtctx krt.HandlerContext, gExt ir.GatewayExtension) *TrafficPolicyGatewayExtensionIR
}

func NewTrafficPolicyConstructor(
	ctx context.Context,
	commoncol *common.CommonCollections,
) *TrafficPolicyConstructor {
	extBuilder := TranslateGatewayExtensionBuilder(commoncol)
	defaultExtBuilder := func(krtctx krt.HandlerContext, gExt ir.GatewayExtension) *TrafficPolicyGatewayExtensionIR {
		return extBuilder(krtctx, gExt)
	}
	gatewayExtensions := krt.NewCollection(commoncol.GatewayExtensions, defaultExtBuilder)
	return &TrafficPolicyConstructor{
		commoncol:         commoncol,
		gatewayExtensions: gatewayExtensions,
		extBuilder:        extBuilder,
	}
}

func (c *TrafficPolicyConstructor) ConstructIR(
	krtctx krt.HandlerContext,
	policyCR *v1alpha1.TrafficPolicy,
) (*TrafficPolicy, []error) {
	policyIr := TrafficPolicy{
		ct: policyCR.CreationTimestamp.Time,
	}
	outSpec := trafficPolicySpecIr{}

	var errors []error
	// Construct AI specific IR
	if err := constructAI(krtctx, policyCR, c.commoncol.Secrets, &outSpec); err != nil {
		errors = append(errors, err)
	}
	// Construct transformation specific IR
	if err := constructTransformation(policyCR, &outSpec); err != nil {
		errors = append(errors, err)
	}
	// Construct rustformation specific IR
	if err := constructRustformation(policyCR, &outSpec); err != nil {
		errors = append(errors, err)
	}
	// Construct extproc specific IR
	if err := constructExtProc(krtctx, policyCR, c.FetchGatewayExtension, &outSpec); err != nil {
		errors = append(errors, err)
	}
	// Construct extauth specific IR
	if err := constructExtAuth(krtctx, policyCR, c.FetchGatewayExtension, &outSpec); err != nil {
		errors = append(errors, err)
	}
	// Construct local rate limit specific IR
	if err := constructLocalRateLimit(policyCR, &outSpec); err != nil {
		errors = append(errors, err)
	}
	// Construct global rate limit specific IR
	if err := constructGlobalRateLimit(krtctx, policyCR, c.FetchGatewayExtension, &outSpec); err != nil {
		errors = append(errors, err)
	}
	// Construct cors specific IR
	if err := constructCORS(policyCR, &outSpec); err != nil {
		errors = append(errors, err)
	}
	// Construct csrf specific IR
	if err := constructCSRF(policyCR.Spec, &outSpec); err != nil {
		errors = append(errors, err)
	}

	// Construct header modifiers specific IR
	if err := constructHeaderModifiers(policyCR.Spec, &outSpec); err != nil {
		errors = append(errors, err)
	}

	// Construct header modifiers specific IR
	if err := constructHeaderModifiers(policyCR.Spec, &outSpec); err != nil {
		errors = append(errors, err)
	}

	// Construct auto host rewrite specific IR
	constructAutoHostRewrite(policyCR.Spec, &outSpec)
	// Construct buffer specific IR
	constructBuffer(policyCR.Spec, &outSpec)
	// Construct timeout and retry specific IR
	constructTimeoutRetry(policyCR.Spec, &outSpec)

	// Construct rbac specific IR
	if err := constructRBAC(policyCR, &outSpec); err != nil {
		errors = append(errors, err)
	}

	for _, err := range errors {
		logger.Error("error translating traffic policy", "namespace", policyCR.GetNamespace(), "name", policyCR.GetName(), "error", err)
	}
	policyIr.spec = outSpec

	return &policyIr, errors
}

func (c *TrafficPolicyConstructor) FetchGatewayExtension(krtctx krt.HandlerContext, extensionRef v1alpha1.NamespacedObjectReference, ns string) (*TrafficPolicyGatewayExtensionIR, error) {
	namespace := ptr.Deref(extensionRef.Namespace, "")
	if namespace == "" {
		namespace = gwv1.Namespace(ns)
	}

	gwExtNN := types.NamespacedName{Name: string(extensionRef.Name), Namespace: string(namespace)}
	gatewayExtension := krt.FetchOne(krtctx, c.gatewayExtensions, krt.FilterObjectName(gwExtNN))
	if gatewayExtension == nil {
		return nil, fmt.Errorf("gateway extension %s not found", gwExtNN.String())
	}
	if gatewayExtension.Err != nil {
		return gatewayExtension, gatewayExtension.Err
	}
	return gatewayExtension, nil
}

func (c *TrafficPolicyConstructor) HasSynced() bool {
	return c.gatewayExtensions.HasSynced()
}
