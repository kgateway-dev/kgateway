package irtranslator

import (
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"google.golang.org/protobuf/types/known/structpb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
)

const mergeMetadataKeyPrefix = "merge."

func reportPolicyAcceptanceStatus(
	rp reporter.Reporter,
	ancestorRef gwv1.ParentReference,
	policies ...ir.PolicyAtt,
) {
	for _, policy := range policies {
		if policy.PolicyRef == nil {
			// Not a policy associated with a CR, can't report status on it
			continue
		}

		key := reporter.PolicyKey{
			Group:     policy.PolicyRef.Group,
			Kind:      policy.PolicyRef.Kind,
			Namespace: policy.PolicyRef.Namespace,
			Name:      policy.PolicyRef.Name,
		}
		// Update the initial status
		r := rp.Policy(key, policy.Generation).AncestorRef(ancestorRef)

		if len(policy.Errors) > 0 {
			r.SetCondition(reporter.PolicyCondition{
				Type:               string(v1alpha1.PolicyConditionAccepted),
				Status:             metav1.ConditionFalse,
				Reason:             string(v1alpha1.PolicyReasonInvalid),
				Message:            policy.FormatErrors(),
				ObservedGeneration: policy.Generation,
			})
			continue
		}

		r.SetCondition(reporter.PolicyCondition{
			Type:               string(v1alpha1.PolicyConditionAccepted),
			Status:             metav1.ConditionTrue,
			Reason:             string(v1alpha1.PolicyReasonValid),
			Message:            reporter.PolicyAcceptedMsg,
			ObservedGeneration: policy.Generation,
		})
	}
}

func reportPolicyAttachmentStatus(
	rp reporter.Reporter,
	ancestorRef gwv1.ParentReference,
	mergeOrigins ir.MergeOrigins,
	policies ...ir.PolicyAtt,
) {
	for _, policy := range policies {
		if policy.PolicyRef == nil {
			// Not a policy associated with a CR, can't report status on it
			continue
		}

		key := reporter.PolicyKey{
			Group:     policy.PolicyRef.Group,
			Kind:      policy.PolicyRef.Kind,
			Namespace: policy.PolicyRef.Namespace,
			Name:      policy.PolicyRef.Name,
		}
		r := rp.Policy(key, policy.Generation).AncestorRef(ancestorRef)

		if !mergeOrigins.IsSet() {
			// Not a merged policy so this should be a direct attachment
			r.SetAttachmentState(reporter.PolicyAttachmentStateAttached)
			continue
		}

		switch mergeOrigins.GetRefCount(policy.PolicyRef) {
		case ir.MergeOriginsRefCountNone:
			r.SetAttachmentState(reporter.PolicyAttachmentStateOverridden)

		case ir.MergeOriginsRefCountPartial:
			r.SetAttachmentState(reporter.PolicyAttachmentStateMerged)

		case ir.MergeOriginsRefCountAll:
			r.SetAttachmentState(reporter.PolicyAttachmentStateAttached)
		}
	}
}

func addMergeOriginsToFilterMetadata(
	gk schema.GroupKind,
	mergeOrigins ir.MergeOrigins,
	metadata *envoycorev3.Metadata,
) *envoycorev3.Metadata {
	if !mergeOrigins.IsSet() {
		return metadata
	}
	pb := mergeOrigins.ToProtoStruct()
	if metadata == nil {
		metadata = &envoycorev3.Metadata{}
	}
	if metadata.FilterMetadata == nil {
		metadata.FilterMetadata = map[string]*structpb.Struct{}
	}
	metadata.FilterMetadata[mergeMetadataKeyPrefix+gk.String()] = pb
	return metadata
}

// reportRouteConfigPolicyErrors reports policy errors to the appropriate reporter based on attachment level.
// we can infer the attachment level of the policy based on a combination of PolicyRef.SectionName and the
// listener's PolicyAncestorRef kind: empty sectionName indicates a gateway-wide policy attachment, non-empty
// sectionName indicates a listener-level policy attachment. For XListenerSet reporting, we cannot rely on the
// sectionName value alone, so we check the ancestor reference kind to make sure we report on that resource
// instead of the Gateway.
//
// Note: this function has different reporting behavior for HTTP vs HTTPS listeners due to how policy attachment
// is handled at higher levels. For HTTPS listeners, this function is called from ComputeRouteConfiguration for
// each attached GK, which can result in condition message overwrites when both Gateway-wide and listener-level
// policies fail. The last SetCondition call wins, which is typically Gateway-wide due to processing order. For HTTP
// listeners, policy errors are handled in runVhostPlugins at the vhost scope and do not call this function,
// avoiding the overwrite issue.
func reportRouteConfigPolicyErrors(r reporter.Reporter, gw ir.GatewayIR, listener ir.ListenerIR, routeConfigName string, policies ...ir.PolicyAtt) {
	for _, policy := range policies {
		if policy.PolicyRef == nil {
			continue
		}
		if len(policy.Errors) == 0 {
			continue
		}
		if policy.PolicyRef.SectionName != "" || *listener.PolicyAncestorRef.Kind == wellknown.XListenerSetKind {
			listenerReporter := getReporterForFilterChain(gw, r, routeConfigName)
			listenerReporter.SetCondition(reporter.ListenerCondition{
				Type:    gwv1.ListenerConditionAccepted,
				Status:  metav1.ConditionFalse,
				Reason:  reporter.ListenerReplacedReason,
				Message: policy.FormatErrors(),
			})
			continue
		}
		r.Gateway(gw.SourceObject.Obj).SetCondition(reporter.GatewayCondition{
			Type:    gwv1.GatewayConditionAccepted,
			Status:  metav1.ConditionFalse,
			Reason:  reporter.GatewayReplacedReason,
			Message: policy.FormatErrors(),
		})
	}
}
