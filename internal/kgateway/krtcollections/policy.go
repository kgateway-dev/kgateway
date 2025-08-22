package krtcollections

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"istio.io/istio/pkg/config/labels"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/ptr"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	k8sptr "k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	gwxv1a1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"

	apiannotations "github.com/kgateway-dev/kgateway/v2/api/annotations"
	apilabels "github.com/kgateway-dev/kgateway/v2/api/labels"
	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/settings"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/backendref"
	tmetrics "github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/metrics"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/utils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/delegation"
	krtinternal "github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	pluginsdkir "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	krtpkg "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
)

var (
	ErrMissingReferenceGrant = errors.New("missing reference grant")
	ErrUnknownBackendKind    = errors.New("unknown backend kind")
	ErrPolicyNotFound        = errors.New("policy not found")
)

type NotFoundError struct {
	// I call this `NotFound` so its easy to find in krt dump.
	NotFoundObj ir.ObjectSource
}

func (n *NotFoundError) Error() string {
	return fmt.Sprintf("%s \"%s\" not found", n.NotFoundObj.Kind, n.NotFoundObj.Name)
}

// MARK: BackendIndex

type BackendIndex struct {
	// availableBackends are the backends as supplied by backend-contributed plugins.
	// Any policies here are attached directly at Backend generation and not attached via
	// policy index. Use availableBackendsWithPolicy when you need policy.
	availableBackends map[schema.GroupKind]krt.Collection[ir.BackendObjectIR]
	// aliasIndex indexes the availableBackends for a given GK by the BackendObjectIR's Alias
	aliasIndex map[schema.GroupKind]krt.Index[backendKey, ir.BackendObjectIR]

	// availableBackendsWithPolicy is built from availableBackends, attaching policy to the given backends.
	// BackendsWithPolicy is the public interface to access this.
	availableBackendsWithPolicy []krt.Collection[*ir.BackendObjectIR]

	gkAliases map[schema.GroupKind][]schema.GroupKind

	policies  *PolicyIndex
	refgrants *RefGrantIndex
	krtopts   krtinternal.KrtOptions
}

type backendKey struct {
	ir.ObjectSource
	port int32
}

func NewBackendIndex(
	krtopts krtinternal.KrtOptions,
	policies *PolicyIndex,
	refgrants *RefGrantIndex,
) *BackendIndex {
	return &BackendIndex{
		policies:          policies,
		refgrants:         refgrants,
		availableBackends: map[schema.GroupKind]krt.Collection[ir.BackendObjectIR]{},
		aliasIndex:        map[schema.GroupKind]krt.Index[backendKey, ir.BackendObjectIR]{},
		gkAliases:         map[schema.GroupKind][]schema.GroupKind{},
		krtopts:           krtopts,
	}
}

func (i *BackendIndex) HasSynced() bool {
	if !i.policies.HasSynced() {
		return false
	}
	if !i.refgrants.HasSynced() {
		return false
	}
	for _, col := range i.availableBackends {
		if !col.HasSynced() {
			return false
		}
	}
	for _, col := range i.availableBackendsWithPolicy {
		if !col.HasSynced() {
			return false
		}
	}
	return true
}

func (i *BackendIndex) BackendsWithPolicy() []krt.Collection[*ir.BackendObjectIR] {
	return i.availableBackendsWithPolicy
}

// AddBackends builds the backends stored in this BackendIndex by deriving a new BackendObjIR collection
// based on the provided `col` with all Backend-attached policies included on the new BackendObjIR.
// The BackendIndex will then store this collection of backendWithPolicies in its internal map, keyed by the
// provied gk. I.e. for the provided gk, it will carry the collection of backends derived from it, with all
// policies attached.
func (i *BackendIndex) AddBackends(gk schema.GroupKind, col krt.Collection[ir.BackendObjectIR], aliasKinds ...schema.GroupKind) {
	backendsWithPoliciesCol := krt.NewCollection(col, func(kctx krt.HandlerContext, backendObj ir.BackendObjectIR) **ir.BackendObjectIR {
		policies := i.policies.getTargetingPoliciesForBackends(kctx, backendObj.ObjectSource, "", backendObj.GetObjectLabels(), false)
		for _, aliasObjSrc := range backendObj.Aliases {
			if aliasObjSrc.Namespace == "" {
				// targeting policies must be namespace local
				// some aliases might be "global" but for policy purposes, give them the src namespace
				aliasObjSrc.Namespace = backendObj.GetNamespace()
			}
			aliasPolicies := i.policies.getTargetingPoliciesForBackends(kctx, aliasObjSrc, "", backendObj.GetObjectLabels(), true)
			policies = append(policies, aliasPolicies...)
		}
		backendObj.AttachedPolicies = toAttachedPolicies(policies)
		return ptr.Of(&backendObj)
	}, i.krtopts.ToOptions("")...)

	idx := krtpkg.UnnamedIndex(col, func(backendObj ir.BackendObjectIR) (aliasKeys []backendKey) {
		for _, alias := range backendObj.Aliases {
			aliasKeys = append(aliasKeys, backendKey{ObjectSource: alias, port: backendObj.Port})
		}
		return aliasKeys
	})
	i.availableBackends[gk] = col
	i.aliasIndex[gk] = idx
	i.availableBackendsWithPolicy = append(i.availableBackendsWithPolicy, backendsWithPoliciesCol)

	// when we query by the alias, also check our "actual" gk
	for _, aliasGK := range aliasKinds {
		i.gkAliases[aliasGK] = append(i.gkAliases[aliasGK], gk)
	}
}

// if we want to make this function public, make it do ref grants
func (i *BackendIndex) getBackend(kctx krt.HandlerContext, gk schema.GroupKind, n types.NamespacedName, gwport *gwv1.PortNumber) (*ir.BackendObjectIR, error) {
	key := ir.ObjectSource{
		Group:     emptyIfCore(gk.Group),
		Kind:      gk.Kind,
		Namespace: n.Namespace,
		Name:      n.Name,
	}

	var port int32
	if gwport != nil {
		port = int32(*gwport)
	}

	col := i.availableBackends[gk]
	if col == nil {
		return i.getBackendFromAlias(kctx, gk, n, port)
	}

	up := krt.FetchOne(kctx, col, krt.FilterKey(ir.BackendResourceName(key, port, "")))
	if up == nil {
		var err error
		if up, err = i.getBackendFromAlias(kctx, gk, n, port); err != nil {
			// getBackendFromAlias returns ErrUnknownBackendKind when there are no aliases
			// so return our own NotFoundError here
			return nil, &NotFoundError{NotFoundObj: key}
		}
	}

	return up, nil
}

func (i *BackendIndex) getBackendFromAlias(kctx krt.HandlerContext, gk schema.GroupKind, n types.NamespacedName, port int32) (*ir.BackendObjectIR, error) {
	actualGks := i.gkAliases[gk]

	key := backendKey{
		port: port,
		ObjectSource: ir.ObjectSource{
			Group:     gk.Group,
			Kind:      gk.Kind,
			Namespace: n.Namespace,
			Name:      n.Name,
		},
	}

	var didFetch bool
	var results []ir.BackendObjectIR
	for _, actualGk := range actualGks {
		col, ok := i.availableBackends[actualGk]
		if !ok {
			continue
		}

		results = append(results, krt.Fetch(kctx, col, krt.FilterIndex(i.aliasIndex[actualGk], key))...)

		didFetch = true
	}

	if !didFetch {
		return nil, ErrUnknownBackendKind
	}

	var out *ir.BackendObjectIR

	// must return only one
	for _, res := range results {
		if out == nil {
			out = &res // first result
		} else if res.Obj.GetCreationTimestamp().Time.Before(out.Obj.GetCreationTimestamp().Time) {
			out = &res // newer
		} else if res.Obj.GetCreationTimestamp().Time.Equal(out.Obj.GetCreationTimestamp().Time) &&
			res.ResourceName() < out.ResourceName() {
			out = &res // use name for tiebreaker
		}
	}

	if out == nil {
		return nil, &NotFoundError{NotFoundObj: key.ObjectSource}
	}

	return out, nil
}

func (i *BackendIndex) getBackendFromRef(kctx krt.HandlerContext, localns string, ref gwv1.BackendObjectReference) (*ir.BackendObjectIR, error) {
	resolved := toFromBackendRef(localns, ref)
	return i.getBackend(kctx, resolved.GetGroupKind(), types.NamespacedName{Namespace: resolved.Namespace, Name: resolved.Name}, ref.Port)
}

func (i *BackendIndex) GetBackendFromRef(kctx krt.HandlerContext, src ir.ObjectSource, ref gwv1.BackendObjectReference) (*ir.BackendObjectIR, error) {
	// Check if a ReferenceGrant allows the cross-namespace ref
	fromNs := src.Namespace
	fromGK := schema.GroupKind{Group: src.Group, Kind: src.Kind}
	to := toFromBackendRef(fromNs, ref)
	if !i.refgrants.ReferenceAllowed(kctx, fromGK, fromNs, to) {
		return nil, ErrMissingReferenceGrant
	}

	// Ignore user’s port and always use poolIR.Port for InferencePool backends.
	// TODO [danehans]: Add a warning message to HTTPRoute status the required change is made per
	// discussion in github.com/kubernetes-sigs/gateway-api-inference-extension/discussions/918
	if strOr(ref.Kind, string(wellknown.ServiceKind)) == wellknown.InferencePoolKind {
		if err := i.normalizeInfPoolBackendPort(kctx, src.Namespace, &ref); err != nil {
			return nil, err
		}
	}

	return i.getBackendFromRef(kctx, src.Namespace, ref)
}

// Intentionally long name, to make sure the user doesn't use this by mistake.
func (i *BackendIndex) GetBackendFromRefWithoutRefGrantValidation(kctx krt.HandlerContext, src ir.ObjectSource, ref gwv1.BackendObjectReference) (*ir.BackendObjectIR, error) {
	return i.getBackendFromRef(kctx, src.Namespace, ref)
}

// MARK: GatewayIndex

type GatewayIndex struct {
	policies *PolicyIndex
	Gateways krt.Collection[ir.Gateway]
}

func NewGatewayIndex(
	krtopts krtinternal.KrtOptions,
	controllerName string,
	policies *PolicyIndex,
	gws krt.Collection[*gwv1.Gateway],
	lss krt.Collection[*gwxv1a1.XListenerSet],
	gwClasses krt.Collection[*gwv1.GatewayClass],
	namespaces krt.Collection[NamespaceMetadata],
) *GatewayIndex {
	h := &GatewayIndex{policies: policies}

	byParentRefIndex := krtpkg.UnnamedIndex(lss, func(in *gwxv1a1.XListenerSet) []targetRefIndexKey {
		pRef := in.Spec.ParentRef
		ns := strOr(pRef.Namespace, "")
		if ns == "" {
			ns = in.GetNamespace()
		}
		// lookup by the root object
		return []targetRefIndexKey{{
			Group:     wellknown.GatewayGroup,
			Kind:      wellknown.GatewayKind,
			Name:      string(pRef.Name),
			Namespace: ns,
			// this index intentionally doesn't include sectionName
		}}
	})

	h.Gateways = krt.NewCollection(gws, func(kctx krt.HandlerContext, gw *gwv1.Gateway) *ir.Gateway {
		// only care about gateways use a class controlled by us
		gwClass := ptr.Flatten(krt.FetchOne(kctx, gwClasses, krt.FilterKey(string(gw.Spec.GatewayClassName))))
		if gwClass == nil || controllerName != string(gwClass.Spec.ControllerName) {
			return nil
		}

		gwIR := ir.Gateway{
			ObjectSource: ir.ObjectSource{
				Group:     gwv1.SchemeGroupVersion.Group,
				Kind:      wellknown.GatewayKind,
				Namespace: gw.Namespace,
				Name:      gw.Name,
			},
			Obj:       gw,
			Listeners: make([]ir.Listener, 0, len(gw.Spec.Listeners)),
		}

		if gw.Annotations[apiannotations.PerConnectionBufferLimit] != "" {
			limit, err := resource.ParseQuantity(gw.Annotations[apiannotations.PerConnectionBufferLimit])
			if err != nil {
				logger.Error("failed to parse per connection buffer limit", "error", err)
			} else {
				gwIR.PerConnectionBufferLimitBytes = k8sptr.To(uint32(limit.Value()))
			}
		}

		// TODO: http polic
		//		panic("TODO: implement http policies not just listener")
		gwIR.AttachedListenerPolicies = toAttachedPolicies(
			h.policies.getTargetingPolicies(kctx, gwIR.ObjectSource, "", gw.GetLabels()))
		gwIR.AttachedHttpPolicies = gwIR.AttachedListenerPolicies // see if i can find a better way to segment the listener level and http level policies
		for _, l := range gw.Spec.Listeners {
			gwIR.Listeners = append(gwIR.Listeners, ir.Listener{
				Listener:         l,
				Parent:           gw,
				AttachedPolicies: toAttachedPolicies(h.policies.getTargetingPolicies(kctx, gwIR.ObjectSource, string(l.Name), gw.GetLabels())),
				PolicyAncestorRef: gwv1.ParentReference{
					Group:     k8sptr.To(gwv1.Group(wellknown.GatewayGVK.Group)),
					Kind:      k8sptr.To(gwv1.Kind(wellknown.GatewayGVK.Kind)),
					Name:      gwv1.ObjectName(gw.Name),
					Namespace: k8sptr.To(gwv1.Namespace(gw.Namespace)),
				},
			})
		}

		listenerSets := krt.Fetch(kctx, lss, krt.FilterIndex(byParentRefIndex, targetRefIndexKey{
			Group:     wellknown.GatewayGroup,
			Kind:      wellknown.GatewayKind,
			Name:      gw.GetName(),
			Namespace: gw.GetNamespace(),
		}))

		// Sort by listener precedence
		// Ref: https://gateway-api.sigs.k8s.io/geps/gep-1713/#listener-precedence
		// - ListenerSet ordered by creation time (oldest first)
		// - ListenerSet ordered alphabetically by “{namespace}/{name}”
		slices.SortFunc(listenerSets, func(a, b *gwxv1a1.XListenerSet) int {
			// primary sort: creation timestamp (oldest first)
			if cmp := a.GetCreationTimestamp().Compare(b.GetCreationTimestamp().Time); cmp != 0 {
				return cmp
			}
			// secondary sort: alphabetically by "{namespace}/{name}"
			nnsString := func(ls *gwxv1a1.XListenerSet) string {
				return fmt.Sprintf("%s/%s", ls.Namespace, ls.Name)
			}
			return strings.Compare(nnsString(a), nnsString(b))
		})

		// Start the resource sync metrics for all XListenerSets before they are processed,
		// so they do not have staggered start times.
		for _, ls := range listenerSets {
			tmetrics.StartResourceSync(ls.Name,
				tmetrics.ResourceMetricLabels{
					Namespace: ls.Namespace,
					Gateway:   gw.GetName(),
					Resource:  wellknown.XListenerSetKind,
				})
		}

		for _, ls := range listenerSets {
			lsIR := ir.ListenerSet{
				ObjectSource: ir.ObjectSource{
					Group:     wellknown.XListenerSetGroup,
					Kind:      wellknown.XListenerSetKind,
					Namespace: ls.Namespace,
					Name:      ls.Name,
				},
				Obj:       ls,
				Listeners: make([]ir.Listener, 0),
			}
			listenerSetPolicies := h.policies.getTargetingPolicies(kctx, lsIR.ObjectSource, "", ls.GetLabels())

			for _, l := range ls.Spec.Listeners {
				listenerSpecificPolicies := h.policies.getTargetingPolicies(kctx, lsIR.ObjectSource, string(l.Name), ls.GetLabels())
				// The Gateway Polices applies to all listeners but we need to apply them to listeners within the LS.
				// Since there is no LS equivalent in Envoy, apply them on each listener in the LS.
				// Ensure the sectioned policies are first
				listenerPolicies := append(listenerSpecificPolicies, listenerSetPolicies...)

				lsIR.Listeners = append(lsIR.Listeners, ir.Listener{
					Listener:         utils.ToListener(l),
					Parent:           ls,
					AttachedPolicies: toAttachedPolicies(listenerPolicies),
					PolicyAncestorRef: gwv1.ParentReference{
						Group:     k8sptr.To(gwv1.Group(wellknown.XListenerSetGVK.Group)),
						Kind:      k8sptr.To(gwv1.Kind(wellknown.XListenerSetGVK.Kind)),
						Name:      gwv1.ObjectName(ls.Name),
						Namespace: k8sptr.To(gwv1.Namespace(ls.Namespace)),
					},
				})
			}

			if gw.Spec.AllowedListeners == nil {
				lsIR.Err = errors.New("Unable to attach to parent, gateway has not enabled allowedListeners")
				gwIR.DeniedListenerSets = append(gwIR.DeniedListenerSets, lsIR)
				continue
			}

			// TODO: this logic should be done once for the Gateway, not per ListenerSet
			// also means that we should report on the Gateway not any ListenerSet
			allowedNs, err := allowedListenerSet(gw, namespaces)
			if err != nil {
				lsIR.Err = errors.New("Unable to parse allowedListeners")
				gwIR.DeniedListenerSets = append(gwIR.DeniedListenerSets, lsIR)
				continue
			}

			// Check if the namespace of the listenerSet is allowed by the gateway
			// We return the denied list of ls to have their status set to rejected during validation
			if !allowedNs(kctx, ls.GetNamespace()) {
				lsIR.Err = errors.New("Attachment not allowed")
				gwIR.DeniedListenerSets = append(gwIR.DeniedListenerSets, lsIR)
				continue
			}

			gwIR.AllowedListenerSets = append(gwIR.AllowedListenerSets, lsIR)
			gwIR.Listeners = append(gwIR.Listeners, lsIR.Listeners...)
		}

		return &gwIR
	}, krtopts.ToOptions("gateways")...)

	return h
}

func allowedListenerSet(gw *gwv1.Gateway, namespaces krt.Collection[NamespaceMetadata]) (func(kctx krt.HandlerContext, namespace string) bool, error) {
	// Default to None. Ref: https://gateway-api.sigs.k8s.io/geps/gep-1713/#gateway-listenerset-handshake
	allowedNs := NoNamespace()

	if al := gw.Spec.AllowedListeners; al != nil {
		// Determine the allowed namespaces if specified
		if al.Namespaces != nil && al.Namespaces.From != nil {
			switch *al.Namespaces.From {
			case gwv1.NamespacesFromAll:
				allowedNs = AllNamespace()
			case gwv1.NamespacesFromSame:
				allowedNs = SameNamespace(gw.GetNamespace())
			case gwv1.NamespacesFromSelector:
				if al.Namespaces.Selector == nil {
					return nil, fmt.Errorf("selector must be set")
				}
				selector, err := metav1.LabelSelectorAsSelector(al.Namespaces.Selector)
				if err != nil {
					return nil, err
				}
				allowedNs = NamespaceSelector(namespaces, selector)
			}
		}
	}

	return allowedNs, nil
}

type targetRefIndexKey struct {
	Group       string
	Kind        string
	Name        string
	Namespace   string
	SectionName string
}

func (k targetRefIndexKey) String() string {
	return fmt.Sprintf("%s/%s/%s/%s/%s", k.Group, k.Kind, k.Name, k.Namespace, k.SectionName)
}

// HTTPRouteSelector is used to lookup HttpRouteIR using one of the following ways:
// - Only LabelValue
// - Only Namespace
// - LabelValue + Namespace
type HTTPRouteSelector struct {
	// LabelValue is the value of the HTTPRouteSelector label.
	// +optional
	LabelValue string
	// Namespace is to fetch routes from.
	// +optional
	Namespace string
}

func (k HTTPRouteSelector) String() string {
	return fmt.Sprintf("%s/%s", k.LabelValue, k.Namespace)
}

type globalPolicy struct {
	schema.GroupKind
	ir func(krt.HandlerContext) ir.PolicyIR
}

// MARK: PolicyIndex
type policyAndIndex struct {
	policies            krt.Collection[ir.PolicyWrapper]
	policiesByTargetRef krt.Collection[ir.PolicyWrapper]
	index               krt.Index[targetRefIndexKey, ir.PolicyWrapper]
	forBackends         bool
}
type PolicyIndex struct {
	globalPolicyNamespace string
	availablePolicies     map[schema.GroupKind]policyAndIndex

	policiesFetch  map[schema.GroupKind]func(n string, ns string) ir.PolicyIR
	globalPolicies []globalPolicy

	hasSyncedFuncs []func() bool
}
type policyFetcherMap = map[schema.GroupKind]func(n string, ns string) ir.PolicyIR

func (h *PolicyIndex) HasSynced() bool {
	for _, f := range h.hasSyncedFuncs {
		if !f() {
			return false
		}
	}
	for _, pi := range h.availablePolicies {
		if !pi.policies.HasSynced() {
			return false
		}
		if !pi.policiesByTargetRef.HasSynced() {
			return false
		}
	}
	return true
}

func NewPolicyIndex(
	krtopts krtinternal.KrtOptions,
	contributesPolicies extensionsplug.ContributesPolicies,
	globalSettings settings.Settings,
) *PolicyIndex {
	index := &PolicyIndex{
		globalPolicyNamespace: globalSettings.GlobalPolicyNamespace,
		policiesFetch:         policyFetcherMap{},
		availablePolicies:     map[schema.GroupKind]policyAndIndex{},
	}

	for gk, plugin := range contributesPolicies {
		if plugin.Policies != nil {
			policies := plugin.Policies
			forBackends := plugin.ProcessBackend != nil
			policiesByTargetRef := krt.NewCollection(policies, func(kctx krt.HandlerContext, a ir.PolicyWrapper) *ir.PolicyWrapper {
				if len(a.TargetRefs) == 0 {
					return nil
				}
				return &a
			}, krtopts.ToOptions(fmt.Sprintf("%s-policiesByTargetRef", gk.String()))...)

			targetRefIndex := krtpkg.UnnamedIndex(policiesByTargetRef, func(p ir.PolicyWrapper) []targetRefIndexKey {
				// Every policy is indexed by PolicyRef and PolicyRef without Name (by Group+Kind+Namespace)
				ret := make([]targetRefIndexKey, len(p.TargetRefs)*2)
				for i, tr := range p.TargetRefs {
					// Index using standard PolicyRef
					ret[i] = targetRefIndexKey{
						Group:       tr.Group,
						Kind:        tr.Kind,
						Name:        tr.Name,
						SectionName: tr.SectionName,
						Namespace:   p.Namespace,
					}
					// Also index by Namespace without Name
					ret[i+len(p.TargetRefs)] = targetRefIndexKey{
						Group:       tr.Group,
						Kind:        tr.Kind,
						SectionName: tr.SectionName,
						Namespace:   p.Namespace,
					}
				}
				return ret
			})

			index.availablePolicies[gk] = policyAndIndex{
				policies:            policies,
				policiesByTargetRef: policiesByTargetRef,
				index:               targetRefIndex,
				forBackends:         forBackends,
			}
			index.hasSyncedFuncs = append(index.hasSyncedFuncs, plugin.Policies.HasSynced)
		}
		if plugin.PoliciesFetch != nil {
			index.policiesFetch[gk] = plugin.PoliciesFetch
		}
		if plugin.GlobalPolicies != nil {
			index.globalPolicies = append(index.globalPolicies, globalPolicy{
				GroupKind: gk,
				ir:        plugin.GlobalPolicies,
			})
		}
	}

	return index
}

func (p *PolicyIndex) fetchByTargetRef(
	kctx krt.HandlerContext,
	targetRef targetRefIndexKey,
	onlyBackends bool,
) []ir.PolicyWrapper {
	var ret []ir.PolicyWrapper
	for _, policyCol := range p.availablePolicies {
		if onlyBackends && !policyCol.forBackends {
			continue
		}
		policies := krt.Fetch(kctx, policyCol.policiesByTargetRef, krt.FilterIndex(policyCol.index, targetRef))
		ret = append(ret, policies...)
	}
	return ret
}

func (p *PolicyIndex) fetchByTargetRefLabels(
	kctx krt.HandlerContext,
	targetRef targetRefIndexKey,
	onlyBackends bool,
	targetLabels map[string]string,
) []ir.PolicyWrapper {
	var ret []ir.PolicyWrapper
	for _, policyCol := range p.availablePolicies {
		if onlyBackends && !policyCol.forBackends {
			continue
		}
		policies := krt.Fetch(kctx, policyCol.policiesByTargetRef, krt.FilterIndex(policyCol.index, targetRef),
			krt.FilterGeneric(func(a any) bool {
				p := a.(ir.PolicyWrapper)
				for _, ref := range p.TargetRefs {
					targetRefKey := targetRefIndexKey{
						Group:       ref.Group,
						Kind:        ref.Kind,
						SectionName: ref.SectionName,
						Namespace:   p.Namespace,
					}
					if targetRef == targetRefKey && labels.Instance(ref.MatchLabels).Match(targetLabels) {
						return true
					}
				}
				return false
			}),
		)
		ret = append(ret, policies...)
	}
	return ret
}

// Attachment happens during collection creation (i.e. this file), and not translation. so these methods don't need to be public!
// note: we may want to change that for global policies maybe.

func (p *PolicyIndex) getTargetingPoliciesForBackends(
	kctx krt.HandlerContext,
	targetRef ir.ObjectSource,
	sectionName string,
	targetLabels map[string]string,
	excludeGlobal bool,
) []ir.PolicyAtt {
	return p.getTargetingPoliciesMaybeForBackends(kctx, targetRef, sectionName, true, excludeGlobal, targetLabels)
}

func (p *PolicyIndex) getTargetingPolicies(
	kctx krt.HandlerContext,
	targetRef ir.ObjectSource,
	sectionName string,
	targetLabels map[string]string,
) []ir.PolicyAtt {
	return p.getTargetingPoliciesMaybeForBackends(kctx, targetRef, sectionName, false, false, targetLabels)
}

func (p *PolicyIndex) getTargetingPoliciesMaybeForBackends(
	kctx krt.HandlerContext,
	targetRef ir.ObjectSource,
	sectionName string,
	onlyBackends bool,
	excludeGlobal bool,
	targetLabels map[string]string,
) []ir.PolicyAtt {
	var ret []ir.PolicyAtt
	if !excludeGlobal {
		for _, gp := range p.globalPolicies {
			if p := gp.ir(kctx); p != nil {
				gpAtt := ir.PolicyAtt{
					PolicyIr:  p,
					GroupKind: gp.GroupKind,
				}
				ret = append(ret, gpAtt)
			}
		}
	}

	// no need for ref grants here as target refs are namespace local
	refIndexKey := targetRefIndexKey{
		Group:       targetRef.Group,
		Kind:        targetRef.Kind,
		Name:        targetRef.Name,
		Namespace:   targetRef.Namespace,
		SectionName: sectionName,
	}

	policies := p.fetchByTargetRef(kctx, refIndexKey, onlyBackends)
	// Lookup policies that select targetLabels
	if len(targetLabels) > 0 {
		refIndexKeyByNamespace := targetRefIndexKey{
			Group:       targetRef.Group,
			Kind:        targetRef.Kind,
			Namespace:   targetRef.Namespace,
			SectionName: sectionName,
		}
		policiesByLabel := p.fetchByTargetRefLabels(kctx, refIndexKeyByNamespace, onlyBackends, targetLabels)
		policies = append(policies, policiesByLabel...)

		// Check if policies defined in the global policy namespace target this ref.
		// `targetRef.Namespace != p.globalPolicyNamespace` ensures we avoid a duplicate lookup as done
		// above when targetRef.Namespace is the same as globalPolicyNamespace
		if p.globalPolicyNamespace != "" && targetRef.Namespace != p.globalPolicyNamespace {
			refIndexKeyByNamespace.Namespace = p.globalPolicyNamespace
			globalPolicies := p.fetchByTargetRefLabels(kctx, refIndexKeyByNamespace, onlyBackends, targetLabels)
			policies = append(policies, globalPolicies...)
		}
	}

	for _, p := range policies {
		ret = append(ret, ir.PolicyAtt{
			Generation: p.Policy.GetGeneration(),
			GroupKind:  p.GetGroupKind(),
			PolicyIr:   p.PolicyIR,
			PolicyRef: &ir.AttachedPolicyRef{
				Group:       p.Group,
				Kind:        p.Kind,
				Name:        p.Name,
				Namespace:   p.Namespace,
				SectionName: sectionName,
			},
			Errors: p.Errors,
		})
	}

	slices.SortFunc(ret, func(a, b ir.PolicyAtt) int {
		return a.PolicyIr.CreationTime().Compare(b.PolicyIr.CreationTime())
	})
	return ret
}

func (p *PolicyIndex) fetchPolicy(kctx krt.HandlerContext, policyRef ir.ObjectSource) *ir.PolicyWrapper {
	gk := policyRef.GetGroupKind()
	if f, ok := p.policiesFetch[gk]; ok {
		if polIr := f(policyRef.Name, policyRef.Namespace); polIr != nil {
			return &ir.PolicyWrapper{PolicyIR: polIr}
		}
	}
	if pi, ok := p.availablePolicies[gk]; ok {
		return krt.FetchOne(kctx, pi.policies, krt.FilterKey(policyRef.ResourceName()))
	}
	return nil
}

// LookupTargetingPolicies returns the policies targeting the given object.
func (p *PolicyIndex) LookupTargetingPolicies(
	kctx krt.HandlerContext,
	targetRef ir.ObjectSource,
	sectionName string,
	targetLabels map[string]string,
) []ir.PolicyAtt {
	return p.getTargetingPolicies(kctx, targetRef, sectionName, targetLabels)
}

// PolicyIndex returns the underlying PolicyIndex reference.
func (i *BackendIndex) PolicyIndex() *PolicyIndex {
	return i.policies
}

type refGrantIndexKey struct {
	RefGrantNs string
	ToGK       schema.GroupKind
	ToName     string
	FromGK     schema.GroupKind
	FromNs     string
}

func (k refGrantIndexKey) String() string {
	return fmt.Sprintf("%s/%s/%s/%s/%s/%s/%s", k.RefGrantNs, k.FromNs, k.ToGK.Group, k.ToGK.Kind, k.ToName, k.FromGK.Group, k.FromGK.Kind)
}

// MARK: RefGrantIndex

type RefGrantIndex struct {
	refgrants     krt.Collection[*gwv1beta1.ReferenceGrant]
	refGrantIndex krt.Index[refGrantIndexKey, *gwv1beta1.ReferenceGrant]
}

func (h *RefGrantIndex) HasSynced() bool {
	return h.refgrants.HasSynced()
}

func NewRefGrantIndex(refgrants krt.Collection[*gwv1beta1.ReferenceGrant]) *RefGrantIndex {
	refGrantIndex := krtpkg.UnnamedIndex(refgrants, func(p *gwv1beta1.ReferenceGrant) []refGrantIndexKey {
		ret := make([]refGrantIndexKey, 0, len(p.Spec.To)*len(p.Spec.From))
		for _, from := range p.Spec.From {
			for _, to := range p.Spec.To {
				ret = append(ret, refGrantIndexKey{
					RefGrantNs: p.Namespace,
					ToGK:       schema.GroupKind{Group: emptyIfCore(string(to.Group)), Kind: string(to.Kind)},
					ToName:     strOr(to.Name, ""),
					FromGK:     schema.GroupKind{Group: emptyIfCore(string(from.Group)), Kind: string(from.Kind)},
					FromNs:     string(from.Namespace),
				})
			}
		}
		return ret
	})
	return &RefGrantIndex{refgrants: refgrants, refGrantIndex: refGrantIndex}
}

func (r *RefGrantIndex) ReferenceAllowed(kctx krt.HandlerContext, fromgk schema.GroupKind, fromns string, to ir.ObjectSource) bool {
	if fromns == to.Namespace {
		return true
	}
	to.Group = emptyIfCore(to.Group)
	fromgk.Group = emptyIfCore(fromgk.Group)

	// no good way to to refGrant here unless we do it _after_ resolving the ref
	if to.Namespace == "" && wellknown.GlobalRefGKs.Has(to.GetGroupKind()) {
		return true
	}

	key := refGrantIndexKey{
		RefGrantNs: to.Namespace,
		ToGK:       schema.GroupKind{Group: to.Group, Kind: to.Kind},
		FromGK:     fromgk,
		FromNs:     fromns,
	}
	matchingGrants := krt.Fetch(kctx, r.refgrants, krt.FilterIndex(r.refGrantIndex, key))
	if len(matchingGrants) != 0 {
		return true
	}
	// try with name:
	key.ToName = to.Name
	return len(krt.Fetch(kctx, r.refgrants, krt.FilterIndex(r.refGrantIndex, key))) != 0
}

type RouteWrapper struct {
	Route ir.Route
}

func (c RouteWrapper) ResourceName() string {
	os := ir.ObjectSource{
		Group:     c.Route.GetGroupKind().Group,
		Kind:      c.Route.GetGroupKind().Kind,
		Namespace: c.Route.GetNamespace(),
		Name:      c.Route.GetName(),
	}
	return os.ResourceName()
}

func (c RouteWrapper) Equals(in RouteWrapper) bool {
	switch a := c.Route.(type) {
	case *ir.HttpRouteIR:
		if bhttp, ok := in.Route.(*ir.HttpRouteIR); !ok {
			return false
		} else {
			return a.Equals(*bhttp)
		}
	case *ir.TcpRouteIR:
		if bhttp, ok := in.Route.(*ir.TcpRouteIR); !ok {
			return false
		} else {
			return a.Equals(*bhttp)
		}
	case *ir.TlsRouteIR:
		if bhttp, ok := in.Route.(*ir.TlsRouteIR); !ok {
			return false
		} else {
			return a.Equals(*bhttp)
		}
	}
	panic("unknown route type")
}

// MARK: RoutesIndex

type RoutesIndex struct {
	routes                  krt.Collection[RouteWrapper]
	httpRoutes              krt.Collection[ir.HttpRouteIR]
	httpBySelector          krt.Index[HTTPRouteSelector, ir.HttpRouteIR]
	byParentRef             krt.Index[targetRefIndexKey, RouteWrapper]
	weightedRoutePrecedence bool

	policies  *PolicyIndex
	refgrants *RefGrantIndex
	backends  *BackendIndex

	hasSyncedFuncs []func() bool
}

func (h *RoutesIndex) HasSynced() bool {
	for _, f := range h.hasSyncedFuncs {
		if !f() {
			return false
		}
	}
	return h.httpRoutes.HasSynced() && h.routes.HasSynced() && h.policies.HasSynced() && h.backends.HasSynced() && h.refgrants.HasSynced()
}

// HTTPRoutes returns the raw krt collection that contains only the HTTPRouteIR.
func (r *RoutesIndex) HTTPRoutes() krt.Collection[ir.HttpRouteIR] {
	return r.httpRoutes
}

func NewRoutesIndex(
	krtopts krtinternal.KrtOptions,
	httproutes krt.Collection[*gwv1.HTTPRoute],
	grpcroutes krt.Collection[*gwv1.GRPCRoute],
	tcproutes krt.Collection[*gwv1a2.TCPRoute],
	tlsroutes krt.Collection[*gwv1a2.TLSRoute],
	policies *PolicyIndex,
	backends *BackendIndex,
	refgrants *RefGrantIndex,
	globalSettings settings.Settings,
) *RoutesIndex {
	h := &RoutesIndex{
		policies:                policies,
		refgrants:               refgrants,
		backends:                backends,
		weightedRoutePrecedence: globalSettings.WeightedRoutePrecedence,
	}
	h.hasSyncedFuncs = append(h.hasSyncedFuncs, httproutes.HasSynced, grpcroutes.HasSynced, tcproutes.HasSynced, tlsroutes.HasSynced)

	h.httpRoutes = krt.NewCollection(httproutes, h.transformHttpRoute, krtopts.ToOptions("http-routes-with-policy")...)
	httpRouteCollection := krt.NewCollection(h.httpRoutes, func(kctx krt.HandlerContext, i ir.HttpRouteIR) *RouteWrapper {
		return &RouteWrapper{Route: &i}
	}, krtopts.ToOptions("routes-http-routes-with-policy")...)

	tcpRoutesCollection := krt.NewCollection(tcproutes, func(kctx krt.HandlerContext, i *gwv1a2.TCPRoute) *RouteWrapper {
		t := h.transformTcpRoute(kctx, i)
		return &RouteWrapper{Route: t}
	}, krtopts.ToOptions("routes-tcp-routes-with-policy")...)

	tlsRoutesCollection := krt.NewCollection(tlsroutes, func(kctx krt.HandlerContext, i *gwv1a2.TLSRoute) *RouteWrapper {
		t := h.transformTlsRoute(kctx, i)
		return &RouteWrapper{Route: t}
	}, krtopts.ToOptions("routes-tls-routes-with-policy")...)
	grpcRoutesCollection := krt.NewCollection(grpcroutes, func(kctx krt.HandlerContext, i *gwv1.GRPCRoute) *RouteWrapper {
		t := h.transformGRPCRoute(kctx, i)
		return &RouteWrapper{Route: t}
	}, krtopts.ToOptions("routes-grpc-routes-with-policy")...)
	h.routes = krt.JoinCollection([]krt.Collection[RouteWrapper]{httpRouteCollection, grpcRoutesCollection, tcpRoutesCollection, tlsRoutesCollection}, krtopts.ToOptions("all-routes-with-policy")...)

	httpBySelector := krtpkg.UnnamedIndex(h.httpRoutes, func(i ir.HttpRouteIR) []HTTPRouteSelector {
		value, ok := i.SourceObject.GetLabels()[apilabels.DelegationLabelSelector]
		if !ok {
			return []HTTPRouteSelector{
				// Key for wildcard namespace Fetch
				{Namespace: i.GetNamespace()},
			}
		}
		return []HTTPRouteSelector{
			// Key for namespace only Fetch
			{Namespace: i.GetNamespace()},
			// Key for label+namespace Fetch
			{LabelValue: value, Namespace: i.GetNamespace()},
			// Key for label only Fetch
			{LabelValue: value},
		}
	})
	h.httpBySelector = httpBySelector

	byParentRef := krtpkg.UnnamedIndex(h.routes, func(in RouteWrapper) []targetRefIndexKey {
		parentRefs := in.Route.GetParentRefs()
		ret := make([]targetRefIndexKey, len(parentRefs))
		for i, pRef := range parentRefs {
			ns := strOr(pRef.Namespace, "")
			if ns == "" {
				ns = in.Route.GetNamespace()
			}
			// HTTPRoute defaults GK to Gateway
			group := wellknown.GatewayGVK.Group
			kind := wellknown.GatewayGVK.Kind
			if pRef.Group != nil {
				group = string(*pRef.Group)
			}
			if pRef.Kind != nil {
				kind = string(*pRef.Kind)
			}
			// lookup by the root object
			ret[i] = targetRefIndexKey{
				Namespace: ns,
				Group:     group,
				Kind:      kind,
				Name:      string(pRef.Name),
				// this index intentionally doesn't include sectionName or port
			}
		}
		return ret
	})
	h.byParentRef = byParentRef

	return h
}

func (h *RoutesIndex) FetchHTTPRoutesBySelector(kctx krt.HandlerContext, selector HTTPRouteSelector) []ir.HttpRouteIR {
	return krt.Fetch(kctx, h.httpRoutes, krt.FilterIndex(h.httpBySelector, selector))
}

func (h *RoutesIndex) RoutesForGateway(kctx krt.HandlerContext, nns types.NamespacedName) []ir.Route {
	return h.RoutesFor(kctx, nns, wellknown.GatewayGVK.Group, wellknown.GatewayGVK.Kind)
}

func (h *RoutesIndex) RoutesForListenerSet(kctx krt.HandlerContext, nns types.NamespacedName) []ir.Route {
	return h.RoutesFor(kctx, nns, wellknown.XListenerSetGVK.Group, wellknown.XListenerSetGVK.Kind)
}

func (h *RoutesIndex) RoutesFor(kctx krt.HandlerContext, nns types.NamespacedName, group, kind string) []ir.Route {
	rts := krt.Fetch(kctx, h.routes, krt.FilterIndex(h.byParentRef, targetRefIndexKey{
		Name:      nns.Name,
		Group:     group,
		Kind:      kind,
		Namespace: nns.Namespace,
	}))
	ret := make([]ir.Route, len(rts))
	for i, r := range rts {
		ret[i] = r.Route
	}
	return ret
}

func (h *RoutesIndex) FetchHttp(kctx krt.HandlerContext, ns, n string) *ir.HttpRouteIR {
	src := ir.ObjectSource{
		Group:     gwv1.SchemeGroupVersion.Group,
		Kind:      "HTTPRoute",
		Namespace: ns,
		Name:      n,
	}
	route := krt.FetchOne(kctx, h.httpRoutes, krt.FilterKey(src.ResourceName()))
	return route
}

// ListHTTPRoutesInNamespace returns all HTTPRouteIRs in the given namespace.
func (h *RoutesIndex) ListHTTPRoutesInNamespace(ns string) []ir.HttpRouteIR {
	var out []ir.HttpRouteIR
	for _, rt := range h.httpRoutes.List() {
		if rt.GetNamespace() == ns {
			out = append(out, rt)
		}
	}
	return out
}

func (h *RoutesIndex) Fetch(kctx krt.HandlerContext, gk schema.GroupKind, ns, n string) *RouteWrapper {
	src := ir.ObjectSource{
		Group:     gk.Group,
		Kind:      gk.Kind,
		Namespace: ns,
		Name:      n,
	}
	return krt.FetchOne(kctx, h.routes, krt.FilterKey(src.ResourceName()))
}

func (h *RoutesIndex) transformTcpRoute(kctx krt.HandlerContext, i *gwv1a2.TCPRoute) *ir.TcpRouteIR {
	src := ir.ObjectSource{
		Group:     gwv1a2.SchemeGroupVersion.Group,
		Kind:      "TCPRoute",
		Namespace: i.Namespace,
		Name:      i.Name,
	}
	var backends []gwv1.BackendRef
	if len(i.Spec.Rules) > 0 {
		backends = i.Spec.Rules[0].BackendRefs
	}
	return &ir.TcpRouteIR{
		ObjectSource:     src,
		SourceObject:     i,
		ParentRefs:       i.Spec.ParentRefs,
		Backends:         h.getTcpBackends(kctx, src, backends),
		AttachedPolicies: toAttachedPolicies(h.policies.getTargetingPolicies(kctx, src, "", i.GetLabels())),
	}
}

func (h *RoutesIndex) transformTlsRoute(kctx krt.HandlerContext, i *gwv1a2.TLSRoute) *ir.TlsRouteIR {
	src := ir.ObjectSource{
		Group:     gwv1a2.SchemeGroupVersion.Group,
		Kind:      "TLSRoute",
		Namespace: i.Namespace,
		Name:      i.Name,
	}
	var backends []gwv1.BackendRef
	if len(i.Spec.Rules) > 0 {
		backends = i.Spec.Rules[0].BackendRefs
	}
	return &ir.TlsRouteIR{
		ObjectSource:     src,
		SourceObject:     i,
		ParentRefs:       i.Spec.ParentRefs,
		Backends:         h.getTcpBackends(kctx, src, backends),
		Hostnames:        tostr(i.Spec.Hostnames),
		AttachedPolicies: toAttachedPolicies(h.policies.getTargetingPolicies(kctx, src, "", i.GetLabels())),
	}
}

func (h *RoutesIndex) transformHttpRoute(kctx krt.HandlerContext, i *gwv1.HTTPRoute) *ir.HttpRouteIR {
	src := ir.ObjectSource{
		Group:     gwv1.SchemeGroupVersion.Group,
		Kind:      "HTTPRoute",
		Namespace: i.Namespace,
		Name:      i.Name,
	}

	inheritedPolicyPriority := getInheritedPolicyPriority(i.Annotations)

	var precedenceWeight int32
	var err error
	if h.weightedRoutePrecedence {
		precedenceWeight, err = parseRoutePrecedenceWeight(i.Annotations)
		if err != nil {
			logger.Error("error parsing route weight; defaulting to 0", "resource_ref", src, "error", err)
		}
	}

	return &ir.HttpRouteIR{
		ObjectSource: src,
		SourceObject: i,
		ParentRefs:   i.Spec.ParentRefs,
		Hostnames:    tostr(i.Spec.Hostnames),
		Rules: h.transformRules(
			kctx, src, i.Spec.Rules, i.GetLabels(), ir.WithInheritedPolicyPriority(inheritedPolicyPriority)),
		AttachedPolicies: toAttachedPolicies(
			h.policies.getTargetingPolicies(kctx, src, "", i.GetLabels()),
			ir.WithInheritedPolicyPriority(inheritedPolicyPriority),
		),
		PrecedenceWeight:               precedenceWeight,
		DelegationInheritParentMatcher: delegation.ShouldInheritParentMatcher(i.GetAnnotations()),
	}
}

func (h *RoutesIndex) transformRules(
	kctx krt.HandlerContext,
	src ir.ObjectSource,
	i []gwv1.HTTPRouteRule,
	srcLabels map[string]string,
	opts ...ir.PolicyAttachmentOpts,
) []ir.HttpRouteRuleIR {
	rules := make([]ir.HttpRouteRuleIR, 0, len(i))
	for _, r := range i {
		extensionRefs, err := h.getExtensionRefs(kctx, src.Namespace, r.Filters, opts...)

		var policies ir.AttachedPolicies
		if r.Name != nil {
			policies = toAttachedPolicies(h.policies.getTargetingPolicies(kctx, src, string(*r.Name), srcLabels), opts...)
		}

		rulePolicies := h.getBuiltInRulePolicies(r, opts...)
		policies.Append(rulePolicies)

		ruleOut := ir.HttpRouteRuleIR{
			ExtensionRefs:    extensionRefs,
			AttachedPolicies: policies,
			Backends:         h.getBackends(kctx, src, r.BackendRefs),
			Matches:          r.Matches,
			Name:             emptyIfNil(r.Name),
		}
		if err != nil {
			ruleOut.Err = err
		}
		rules = append(rules, ruleOut)
	}
	return rules
}

func (h *RoutesIndex) getExtensionRefs(
	kctx krt.HandlerContext,
	ns string,
	r []gwv1.HTTPRouteFilter,
	opts ...ir.PolicyAttachmentOpts,
) (ir.AttachedPolicies, error) {
	ret := ir.AttachedPolicies{
		Policies: map[schema.GroupKind][]ir.PolicyAtt{},
	}
	var errs []error
	for _, ext := range r {
		policyAtt, err := h.resolveExtension(kctx, ns, ext)
		if policyAtt != nil {
			for _, o := range opts {
				o(policyAtt)
			}
			ret.Policies[policyAtt.GroupKind] = append(ret.Policies[policyAtt.GroupKind], *policyAtt)
		} else if err != nil {
			errs = append(errs, err)
			logger.Error("unresolved HTTPRouteFilter", "error", err)
		}
	}
	return ret, errors.Join(errs...)
}

func (h *RoutesIndex) getBuiltInRulePolicies(
	rule gwv1.HTTPRouteRule,
	opts ...ir.PolicyAttachmentOpts,
) ir.AttachedPolicies {
	ret := ir.AttachedPolicies{
		Policies: map[schema.GroupKind][]ir.PolicyAtt{},
	}
	policy := NewBuiltInRuleIr(rule)
	if policy != nil {
		policyAtt := ir.PolicyAtt{PolicyIr: policy /*direct attachment - no target ref*/}
		for _, o := range opts {
			o(&policyAtt)
		}
		ret.Policies[pluginsdkir.VirtualBuiltInGK] = append(ret.Policies[pluginsdkir.VirtualBuiltInGK], policyAtt)
	}
	return ret
}

func (h *RoutesIndex) resolveExtension(kctx krt.HandlerContext, ns string, ext gwv1.HTTPRouteFilter) (*ir.PolicyAtt, error) {
	if ext.Type == gwv1.HTTPRouteFilterExtensionRef {
		if ext.ExtensionRef == nil {
			// TODO: report error!!
			return nil, nil
		}
		ref := *ext.ExtensionRef
		key := ir.ObjectSource{
			Group:     string(ref.Group),
			Kind:      string(ref.Kind),
			Namespace: ns,
			Name:      string(ref.Name),
		}
		policy := h.policies.fetchPolicy(kctx, key)
		if policy == nil {
			return nil, fmt.Errorf("%s: %w", key, ErrPolicyNotFound)
		}

		gk := schema.GroupKind{
			Group: string(ref.Group),
			Kind:  string(ref.Kind),
		}
		policyRef := &ir.AttachedPolicyRef{
			Group:     gk.Group,
			Kind:      gk.Kind,
			Name:      string(ref.Name),
			Namespace: ns,
		}
		policyAtt := &ir.PolicyAtt{
			Generation: policy.Policy.GetGeneration(),
			GroupKind:  gk,
			PolicyIr:   policy.PolicyIR,
			PolicyRef:  policyRef,
			Errors:     policy.Errors,
		}
		return policyAtt, nil
	}

	fromGK := schema.GroupKind{
		Group: gwv1.SchemeGroupVersion.Group,
		Kind:  "HTTPRoute",
	}

	builtinIR := NewBuiltInIr(kctx, ext, fromGK, ns, h.refgrants, h.backends)
	policyAtt := &ir.PolicyAtt{
		GroupKind: pluginsdkir.VirtualBuiltInGK,
		PolicyIr:  builtinIR,
	}
	return policyAtt, nil
}

func toFromBackendRef(fromns string, ref gwv1.BackendObjectReference) ir.ObjectSource {
	defNs := fromns
	gk := schema.GroupKind{
		Group: strOr(ref.Group, ""),
		Kind:  strOr(ref.Kind, "Service"),
	}
	if wellknown.GlobalRefGKs.Has(gk) {
		defNs = ""
	}
	return ir.ObjectSource{
		Group:     gk.Group,
		Kind:      gk.Kind,
		Namespace: strOr(ref.Namespace, defNs),
		Name:      string(ref.Name),
	}
}

func (h *RoutesIndex) getBackends(kctx krt.HandlerContext, src ir.ObjectSource, backendRefs []gwv1.HTTPBackendRef) []ir.HttpBackendOrDelegate {
	backends := make([]ir.HttpBackendOrDelegate, 0, len(backendRefs))
	for _, ref := range backendRefs {
		// ignore errs as invalid/missing policy for backendRef filters is undefined currently
		// see: https://github.com/kgateway-dev/kgateway/issues/11897
		extensionRefs, _ := h.getExtensionRefs(kctx, src.Namespace, ref.Filters)
		fromns := src.Namespace

		to := toFromBackendRef(fromns, ref.BackendObjectReference)
		if backendref.IsDelegatedHTTPRoute(ref.BackendRef.BackendObjectReference) {
			backends = append(backends, ir.HttpBackendOrDelegate{
				Delegate:         &to,
				AttachedPolicies: extensionRefs,
			})
			continue
		}

		backend, err := h.backends.GetBackendFromRef(kctx, src, ref.BackendRef.BackendObjectReference)

		// TODO: if we can't find the backend, should we
		// still use its cluster name in case it comes up later?
		// if so we need to think about the way create cluster names,
		// so it only depends on the backend-ref
		clusterName := "blackhole-cluster"
		if backend != nil {
			clusterName = backend.ClusterName()
		} else if err == nil {
			err = &NotFoundError{NotFoundObj: to}
		}
		backends = append(backends, ir.HttpBackendOrDelegate{
			Backend: &ir.BackendRefIR{
				BackendObject: backend,
				ClusterName:   clusterName,
				Weight:        weight(ref.Weight),
				Err:           err,
			},
			AttachedPolicies: extensionRefs,
		})
	}
	return backends
}

func (h *RoutesIndex) getTcpBackends(kctx krt.HandlerContext, src ir.ObjectSource, i []gwv1.BackendRef) []ir.BackendRefIR {
	backends := make([]ir.BackendRefIR, 0, len(i))
	for _, ref := range i {
		backend, err := h.backends.GetBackendFromRef(kctx, src, ref.BackendObjectReference)
		clusterName := "blackhole-cluster"
		if backend != nil {
			clusterName = backend.ClusterName()
		} else if err == nil {
			err = &NotFoundError{NotFoundObj: toFromBackendRef(src.Namespace, ref.BackendObjectReference)}
		}
		backends = append(backends, ir.BackendRefIR{
			BackendObject: backend,
			ClusterName:   clusterName,
			Weight:        weight(ref.Weight),
			Err:           err,
		})
	}
	return backends
}

func strOr[T ~string](s *T, def string) string {
	if s == nil {
		return def
	}
	return string(*s)
}

func weight(w *int32) uint32 {
	if w == nil {
		return 1
	}
	return uint32(*w)
}

func toAttachedPolicies(policies []ir.PolicyAtt, opts ...ir.PolicyAttachmentOpts) ir.AttachedPolicies {
	ret := ir.AttachedPolicies{
		Policies: map[schema.GroupKind][]ir.PolicyAtt{},
	}
	for _, p := range policies {
		gk := schema.GroupKind{
			Group: p.GroupKind.Group,
			Kind:  p.GroupKind.Kind,
		}
		// Create a new PolicyAtt instead of using `p` because the PolicyAttchmentOpts are per-route
		// and not encoded in `p`
		polAtt := ir.PolicyAtt{
			PolicyIr:   p.PolicyIr,
			PolicyRef:  p.PolicyRef,
			GroupKind:  gk,
			Errors:     p.Errors,
			Generation: p.Generation,
		}
		for _, o := range opts {
			o(&polAtt)
		}
		ret.Policies[gk] = append(ret.Policies[gk], polAtt)
	}
	return ret
}

func emptyIfNil(s *gwv1.SectionName) string {
	if s == nil {
		return ""
	}
	return string(*s)
}

func tostr(in []gwv1.Hostname) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	for i, h := range in {
		out[i] = string(h)
	}
	return out
}

func emptyIfCore(s string) string {
	if s == "core" {
		return ""
	}
	return s
}

// normalizeInfPoolBackendPort looks up the InferencePool IR for the given BackendObjectReference,
// logs a warning if the user-supplied port doesn’t match the pool’s targetPort, and then
// mutates ref.Port to the correct pool port.
func (i *BackendIndex) normalizeInfPoolBackendPort(
	kctx krt.HandlerContext,
	srcNamespace string,
	ref *gwv1.BackendObjectReference,
) error {
	// Build an ObjectSource for the pool (ignoring any port for lookup)
	poolSrc := toFromBackendRef(srcNamespace, *ref)
	poolGK := poolSrc.GetGroupKind()

	// Fetch the collection for that kind
	col, exists := i.availableBackends[poolGK]
	if !exists {
		return &NotFoundError{NotFoundObj: poolSrc}
	}

	// Find matching pool IR(s) by name/namespace
	matches := krt.Fetch(kctx, col, krt.FilterGeneric(func(obj any) bool {
		b, ok := obj.(ir.BackendObjectIR)
		return ok &&
			b.ObjectSource.Name == poolSrc.Name &&
			b.ObjectSource.Namespace == poolSrc.Namespace
	}))
	if len(matches) == 0 {
		return &NotFoundError{NotFoundObj: poolSrc}
	}
	poolIR := &matches[0]

	// If the user gave a port and it doesn’t match, warn
	resolvedPort := poolIR.Port
	if ref.Port != nil && int32(*ref.Port) != resolvedPort {
		logger.Warn(
			"backendRef.port does not match InferencePool targetPort; overriding",
			"provided_port", *ref.Port,
			"pool_port", resolvedPort,
			"inference_pool", types.NamespacedName{Namespace: poolSrc.Namespace, Name: poolSrc.Name},
		)
	}

	// Overwrite ref.Port so downstream lookup is correct
	correct := gwv1.PortNumber(resolvedPort)
	ref.Port = &correct
	return nil
}

func parseRoutePrecedenceWeight(annotations map[string]string) (int32, error) {
	val, ok := annotations[apiannotations.RoutePrecedenceWeight]
	if !ok {
		return 0, nil
	}
	weight, err := strconv.ParseInt(val, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid value for annotation %s: %s; must be a valid integer", apiannotations.RoutePrecedenceWeight, val)
	}
	return int32(weight), nil
}

func getInheritedPolicyPriority(annotations map[string]string) apiannotations.InheritedPolicyPriorityValue {
	def := apiannotations.ShallowMergePreferChild
	val, ok := annotations[apiannotations.InheritedPolicyPriority]
	if !ok {
		return def
	}
	switch v := apiannotations.InheritedPolicyPriorityValue(val); v {
	case apiannotations.ShallowMergePreferChild, apiannotations.ShallowMergePreferParent, apiannotations.DeepMergePreferChild, apiannotations.DeepMergePreferParent:
		return v
	default:
		logger.Error("invalid value for annotation", "annotation", apiannotations.InheritedPolicyPriority, "value", v)
		return def
	}
}
