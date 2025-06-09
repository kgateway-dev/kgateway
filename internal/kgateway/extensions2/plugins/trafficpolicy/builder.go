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
	if policyCR.Spec.AI != nil {
		outSpec.AI = &AIPolicyIR{}

		// Augment with AI secrets as needed
		var err error
		outSpec.AI.AISecret, err = aiSecretForSpec(krtctx, b.commoncol.Secrets, policyCR)
		if err != nil {
			errors = append(errors, err)
		}

		// Preprocess the AI backend
		err = preProcessAITrafficPolicy(policyCR.Spec.AI, outSpec.AI)
		if err != nil {
			errors = append(errors, err)
		}
	}
	// Apply transformation specific translation
	err := transformationForSpec(policyCR.Spec, &outSpec)
	if err != nil {
		errors = append(errors, err)
	}

	if policyCR.Spec.ExtProc != nil {
		extproc, err := b.toEnvoyExtProc(krtctx, policyCR)
		if err != nil {
			errors = append(errors, err)
		} else {
			outSpec.ExtProc = extproc
		}
	}

	// Apply ExtAuthz specific translation
	err = b.extAuthForSpec(krtctx, policyCR, &outSpec)
	if err != nil {
		errors = append(errors, err)
	}

	// Apply rate limit specific translation
	err = localRateLimitForSpec(policyCR.Spec, &outSpec)
	if err != nil {
		errors = append(errors, err)
	}

	// Apply global rate limit specific translation
	errs := b.globalRateLimitForSpec(krtctx, policyCR, &outSpec)
	errors = append(errors, errs...)

	// Apply cors specific translation
	err = corsForSpec(policyCR.Spec, &outSpec)
	if err != nil {
		errors = append(errors, err)
	}

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
