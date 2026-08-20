package trafficpolicy

import (
	"context"
	"fmt"

	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// FetchGatewayExtensionFunc defines the signature for fetching gateway extensions
type FetchGatewayExtensionFunc func(krtctx krt.HandlerContext, extensionRef shared.NamespacedObjectReference, ns string) (*TrafficPolicyGatewayExtensionIR, error)

type TrafficPolicyConstructor struct {
	commoncol         *collections.CommonCollections
	gatewayExtensions krt.Collection[TrafficPolicyGatewayExtensionIR]
	extBuilder        func(krtctx krt.HandlerContext, gExt ir.GatewayExtension) *TrafficPolicyGatewayExtensionIR

	// sourceGroupKind is the identity a ReferenceGrant has to name to permit the
	// cross-namespace references in TrafficPolicySpec. Empty means TrafficPolicy;
	// see WithSourceGroupKind.
	sourceGroupKind schema.GroupKind
}

// TrafficPolicyConstructorOption configures a TrafficPolicyConstructor.
type TrafficPolicyConstructorOption func(*TrafficPolicyConstructor)

// WithSourceGroupKind sets the identity that ReferenceGrants are evaluated against
// for the cross-namespace references TrafficPolicySpec holds: API key and basic auth
// secrets, secret-backed header values, and, in Strict mode, GatewayExtension
// references.
//
// It defaults to gateway.kgateway.dev/TrafficPolicy, the kind that declares the spec.
// Set it when a different kind carries the spec being translated, so a ReferenceGrant
// names the resource that actually holds the reference. It replaces the default
// rather than adding to it: from.kind identifies one referencing kind, and a
// namespace granting access to one kind has not granted it to another, which may well
// be creatable by a different set of users.
func WithSourceGroupKind(gk schema.GroupKind) TrafficPolicyConstructorOption {
	return func(c *TrafficPolicyConstructor) {
		c.sourceGroupKind = gk
	}
}

func NewTrafficPolicyConstructor(
	ctx context.Context,
	commoncol *collections.CommonCollections,
	opts ...TrafficPolicyConstructorOption,
) *TrafficPolicyConstructor {
	extBuilder := TranslateGatewayExtensionBuilder(ctx, commoncol)
	defaultExtBuilder := func(krtctx krt.HandlerContext, gExt ir.GatewayExtension) *TrafficPolicyGatewayExtensionIR {
		return extBuilder(krtctx, gExt)
	}
	gatewayExtensions := krt.NewCollection(commoncol.GatewayExtensions, defaultExtBuilder)
	c := &TrafficPolicyConstructor{
		commoncol:         commoncol,
		gatewayExtensions: gatewayExtensions,
		extBuilder:        extBuilder,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// refGrantSource returns the source identity that ReferenceGrants are evaluated
// against for references held by a TrafficPolicySpec in ns.
func (c *TrafficPolicyConstructor) refGrantSource(ns string) krtcollections.From {
	gk := c.sourceGroupKind
	if gk.Empty() {
		gk = wellknown.TrafficPolicyGVK.GroupKind()
	}
	return krtcollections.From{GroupKind: gk, Namespace: ns}
}

func (c *TrafficPolicyConstructor) ConstructIR(
	krtctx krt.HandlerContext,
	policyCR *kgateway.TrafficPolicy,
) (*TrafficPolicy, []error) {
	policyIr := TrafficPolicy{
		ct: policyCR.CreationTimestamp.Time,
	}
	outSpec := trafficPolicySpecIr{}

	var errors []error

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
	constructLocalRateLimit(policyCR, &outSpec)
	// Construct global rate limit specific IR
	if err := constructGlobalRateLimit(krtctx, policyCR, c.FetchGatewayExtension, &outSpec); err != nil {
		errors = append(errors, err)
	}
	// Construct cors specific IR
	constructCORS(policyCR, &outSpec)
	// Construct csrf specific IR
	constructCSRF(policyCR.Spec, &outSpec)
	// Construct compression/decompression specific IR
	constructCompression(policyCR.Spec, &outSpec)

	// Construct header modifiers specific IR
	if err := constructHeaderModifiers(krtctx, policyCR, c.refGrantSource(policyCR.Namespace), c.commoncol.Secrets, &outSpec); err != nil {
		errors = append(errors, err)
	}
	// Construct request mirror specific IR
	constructRequestMirror(policyCR.Spec, &outSpec)
	// Construct auto host rewrite specific IR
	constructAutoHostRewrite(policyCR.Spec, &outSpec)
	// Construct buffer specific IR
	constructBuffer(policyCR.Spec, &outSpec)
	// Construct fault injection specific IR
	constructFaultInjection(policyCR.Spec, &outSpec)
	// Construct HTTP ACL specific IR
	if err := constructHttpACL(policyCR, &outSpec); err != nil {
		errors = append(errors, err)
	}
	// Construct timeout and retry specific IR
	constructTimeoutRetry(policyCR.Spec, &outSpec)
	// Construct internal redirect specific IR
	constructInternalRedirect(policyCR.Spec, &outSpec)

	// Construct rbac specific IR
	if err := constructRBAC(policyCR, &outSpec); err != nil {
		errors = append(errors, err)
	}

	// Construct API key auth specific IR
	if err := constructAPIKeyAuth(krtctx, policyCR, c.refGrantSource(policyCR.Namespace), c.commoncol, &outSpec); err != nil {
		errors = append(errors, err)
	}

	// Construct jwt specific IR
	if err := constructJwt(krtctx, policyCR, &outSpec, c.FetchGatewayExtension); err != nil {
		errors = append(errors, err)
	}
	// Construct OIDC specific IR
	if err := constructOAuth2(krtctx, policyCR, c.FetchGatewayExtension, &outSpec); err != nil {
		errors = append(errors, err)
	}

	// Construct route tracing specific IR
	constructRouteTracing(policyCR.Spec, &outSpec)
	// Construct url rewrite specific IR
	constructURLRewrite(policyCR.Spec, &outSpec)
	// Construct stat prefix specific IR
	constructStatPrefix(policyCR.Spec, &outSpec)
	// Construct basic auth specific IR
	if err := constructBasicAuth(krtctx, policyCR, c.refGrantSource(policyCR.Namespace), &outSpec, c.commoncol.Secrets); err != nil {
		errors = append(errors, err)
	}

	for _, err := range errors {
		logger.Error("error translating traffic policy", "namespace", policyCR.GetNamespace(), "name", policyCR.GetName(), "error", err)
	}
	policyIr.spec = outSpec

	return &policyIr, errors
}

func (c *TrafficPolicyConstructor) FetchGatewayExtension(krtctx krt.HandlerContext, extensionRef shared.NamespacedObjectReference, ns string) (*TrafficPolicyGatewayExtensionIR, error) {
	namespace := ptr.Deref(extensionRef.Namespace, "")
	if namespace == "" {
		namespace = gwv1.Namespace(ns)
	}

	// In Strict mode, cross-namespace ExtensionRef requires a ReferenceGrant.
	if c.commoncol.Settings.ReferenceGrantMode == apisettings.ReferenceGrantStrict {
		from := c.refGrantSource(ns)
		to := ir.ObjectSource{
			Group:     wellknown.GatewayExtensionGVK.Group,
			Kind:      wellknown.GatewayExtensionGVK.Kind,
			Namespace: string(namespace),
			Name:      string(extensionRef.Name),
		}
		if !c.commoncol.RefGrants.ReferenceAllowed(krtctx, from.GroupKind, from.Namespace, to) {
			return nil, &krtcollections.MissingReferenceGrantError{From: from, To: to}
		}
	}

	gwExtNN := types.NamespacedName{Name: string(extensionRef.Name), Namespace: string(namespace)}
	gatewayExtension := krt.FetchOne(krtctx, c.gatewayExtensions, krt.FilterObjectName(gwExtNN))
	if gatewayExtension == nil {
		return nil, fmt.Errorf("%s: %w", gwExtNN.String(), pluginutils.ErrGatewayExtensionNotFound)
	}
	if gatewayExtension.Err != nil {
		return gatewayExtension, gatewayExtension.Err
	}
	return gatewayExtension, nil
}

func (c *TrafficPolicyConstructor) HasSynced() bool {
	return c.gatewayExtensions.HasSynced()
}
