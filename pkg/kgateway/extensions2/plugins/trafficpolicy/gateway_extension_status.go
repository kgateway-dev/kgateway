package trafficpolicy

import (
	"errors"
	"slices"

	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/statussync"
)

// gatewayExtensionStatus is the status a GatewayExtension's translation earns: its Accepted
// condition, stamped with the generation it was translated from.
type gatewayExtensionStatus struct {
	types.NamespacedName
	Accepted metav1.Condition
}

func (s gatewayExtensionStatus) ResourceName() string {
	return s.NamespacedName.String()
}

func (s gatewayExtensionStatus) Equals(other gatewayExtensionStatus) bool {
	return s.NamespacedName == other.NamespacedName && s.Accepted == other.Accepted
}

// newGatewayExtensionStatus projects a translated extension onto its Accepted condition. The
// extension is rejected for the same errors that reject a TrafficPolicy referencing it: the
// translation error, or failing the proto validation the policy runs against its provider.
func newGatewayExtensionStatus(_ krt.HandlerContext, ext TrafficPolicyGatewayExtensionIR) *gatewayExtensionStatus {
	namespace, name, err := cache.SplitMetaNamespaceKey(ext.Name)
	if err != nil {
		logger.Error("unexpected GatewayExtension IR key", "key", ext.Name, "error", err)
		return nil
	}
	var errs []error
	// Validate reports nothing for an extension that already failed to translate.
	if err := errors.Join(ext.Err, ext.Validate()); err != nil {
		errs = []error{err}
	}
	accepted := pluginutils.BuildCondition("GatewayExtension", errs)
	accepted.ObservedGeneration = ext.Generation
	return &gatewayExtensionStatus{
		NamespacedName: types.NamespacedName{Namespace: namespace, Name: name},
		Accepted:       accepted,
	}
}

// registerGatewayExtensionStatus reports each GatewayExtension's Accepted condition, built from
// the translated extensions.
func registerGatewayExtensionStatus(
	commoncol *collections.CommonCollections,
	translated krt.Collection[TrafficPolicyGatewayExtensionIR],
) func(statussync.RegistrationInputs) {
	return func(in statussync.RegistrationInputs) {
		// Status is read from the shared raw collection and written through a client on the
		// same informer, as for Backend.
		raw := commoncol.RawGatewayExtensions
		cli := kclient.NewFilteredDelayed[*kgateway.GatewayExtension](
			commoncol.Client,
			wellknown.GatewayExtensionGVR,
			kclient.Filter{ObjectFilter: commoncol.Client.ObjectFilter()},
		)
		statuses := krt.NewCollection(translated, newGatewayExtensionStatus, in.KrtOpts.ToOptions("GatewayExtensionStatus")...)

		statussync.RegisterDerivedKind(in.Collections, wellknown.GatewayExtensionGVK, raw, statuses,
			func(s gatewayExtensionStatus) types.NamespacedName { return s.NamespacedName })
		in.RegisterWriter(wellknown.GatewayExtensionGVK, gatewayExtensionWriter(cli, raw, statuses))
	}
}

func gatewayExtensionWriter(
	cli kclient.Client[*kgateway.GatewayExtension],
	raw krt.Collection[*kgateway.GatewayExtension],
	statuses krt.Collection[gatewayExtensionStatus],
) statussync.Writer[*kgateway.GatewayExtension, kgateway.GatewayExtensionStatus] {
	return statussync.Writer[*kgateway.GatewayExtension, kgateway.GatewayExtensionStatus]{
		Name:    wellknown.GatewayExtensionGVK.Kind,
		Current: statussync.CollectionSource(raw),
		Desired: func(res statussync.Resource, ext *kgateway.GatewayExtension) (kgateway.GatewayExtensionStatus, bool) {
			status := statuses.GetKey(res.NamespacedName.String())
			if status == nil {
				return kgateway.GatewayExtensionStatus{}, false
			}
			// SetStatusCondition keeps the live LastTransitionTime while the condition's status
			// holds, and leaves conditions of other types untouched.
			conditions := slices.Clone(ext.Status.Conditions)
			meta.SetStatusCondition(&conditions, status.Accepted)
			return kgateway.GatewayExtensionStatus{Conditions: conditions}, true
		},
		UpdateStatus: statussync.ClientWriter(cli,
			func(om metav1.ObjectMeta, st kgateway.GatewayExtensionStatus) *kgateway.GatewayExtension {
				return &kgateway.GatewayExtension{ObjectMeta: om, Status: st}
			}),
		GetStatus: func(o *kgateway.GatewayExtension) kgateway.GatewayExtensionStatus { return o.Status },
	}
}
