package krtcollections

import (
	"errors"

	"github.com/solo-io/gloo/projects/gateway2/ir"
	"github.com/solo-io/gloo/projects/gateway2/translator/backendref"
	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

var (
	ErrMissingReferenceGrant = errors.New("missing reference grant")
	ErrUnknownBackendKind    = errors.New("unknown backend kind")
	ErrNotFound              = errors.New("not found")
)

type UpstreamIndex struct {
	availableUpstreams map[schema.GroupKind]krt.Collection[ir.Upstream]
	policies           *PolicyIndex
}

func NewUpstreamIndex(policies *PolicyIndex) *UpstreamIndex {
	return &UpstreamIndex{policies: policies}
}

func (ui *UpstreamIndex) Upstreams() []krt.Collection[ir.Upstream] {
	ret := make([]krt.Collection[ir.Upstream], 0, len(ui.availableUpstreams))
	for _, u := range ui.availableUpstreams {
		ret = append(ret, u)
	}
	return ret
}

func (ui *UpstreamIndex) AddUpstreams(gk schema.GroupKind, col krt.Collection[ir.Upstream]) {
	ucol := krt.NewCollection(col, func(kctx krt.HandlerContext, u ir.Upstream) *ir.Upstream {
		u.AttachedPolicies = toAttachedPolicies(ui.policies.GetTargetingPolicies(kctx, u.ObjectSource, ""))
		return &u
	})
	ui.availableUpstreams[gk] = ucol
}

func AddUpstreamMany[T metav1.Object](ui *UpstreamIndex, gk schema.GroupKind, col krt.Collection[T], build func(kctx krt.HandlerContext, svc T) []ir.Upstream, opts ...krt.CollectionOption) krt.Collection[ir.Upstream] {
	ucol := krt.NewManyCollection(col, func(kctx krt.HandlerContext, svc T) []ir.Upstream {
		upstreams := build(kctx, svc)
		for i := range upstreams {
			u := &upstreams[i]
			u.AttachedPolicies = toAttachedPolicies(ui.policies.GetTargetingPolicies(kctx, u.ObjectSource, ""))
		}
		return upstreams
	}, opts...)
	ui.availableUpstreams[gk] = ucol
	return ucol
}

func AddUpstream[T metav1.Object](ui *UpstreamIndex, gk schema.GroupKind, col krt.Collection[T], build func(kctx krt.HandlerContext, svc T) *ir.Upstream) {
	ucol := krt.NewCollection(col, func(kctx krt.HandlerContext, svc T) *ir.Upstream {
		upstream := build(kctx, svc)
		if upstream == nil {
			return nil
		}
		upstream.AttachedPolicies = toAttachedPolicies(ui.policies.GetTargetingPolicies(kctx, upstream.ObjectSource, ""))

		return upstream
	})
	ui.availableUpstreams[gk] = ucol
}

// if we want to make this function public, make it do ref grants
func (i *UpstreamIndex) getUpstream(kctx krt.HandlerContext, gk schema.GroupKind, n types.NamespacedName) (*ir.Upstream, error) {
	key := ir.ObjectSource{
		Group:     gk.Group,
		Kind:      gk.Kind,
		Namespace: n.Namespace,
		Name:      n.Name,
	}
	col := i.availableUpstreams[gk]
	if col == nil {
		return nil, ErrUnknownBackendKind
	}

	up := krt.FetchOne(kctx, col, krt.FilterKey(key.ResourceName()))
	if up == nil {
		return nil, ErrNotFound
	}
	return up, nil
}

func (i *UpstreamIndex) getUpstreamFromRef(kctx krt.HandlerContext, localns string, ref gwv1.BackendObjectReference) (*ir.Upstream, error) {
	group := ""
	if ref.Group != nil {
		group = string(*ref.Group)
	}
	kind := "Service"
	if ref.Kind != nil {
		kind = string(*ref.Kind)
	}
	ns := localns
	if ref.Namespace != nil {
		ns = string(*ref.Namespace)
	}
	gk := schema.GroupKind{
		Group: group,
		Kind:  kind,
	}
	return i.getUpstream(kctx, gk, types.NamespacedName{Namespace: ns, Name: string(ref.Name)})
}

type GatweayIndex struct {
	policies *PolicyIndex
	Gateways krt.Collection[ir.Gateway]
}

func NewGatweayIndex(policies *PolicyIndex, gws krt.Collection[*gwv1.Gateway]) *GatweayIndex {
	h := &GatweayIndex{policies: policies}
	h.Gateways = krt.NewCollection(gws, func(kctx krt.HandlerContext, i *gwv1.Gateway) *ir.Gateway {
		out := ir.Gateway{
			ObjectSource: ir.ObjectSource{
				Group:     gwv1.SchemeGroupVersion.Group,
				Kind:      "Gateway",
				Namespace: i.Namespace,
				Name:      i.Name,
			},
			Obj:       i,
			Listeners: make([]ir.Listener, 0, len(i.Spec.Listeners)),
		}

		// TODO: http polic
		panic("TODO: implement http policies")
		out.AttachedListenerPolicies = toAttachedPolicies(h.policies.GetTargetingPolicies(kctx, out.ObjectSource, ""))

		for _, l := range i.Spec.Listeners {
			out.Listeners = append(out.Listeners, ir.Listener{
				Listener:         l,
				AttachedPolicies: toAttachedPolicies(h.policies.GetTargetingPolicies(kctx, out.ObjectSource, string(l.Name))),
			})
		}

		return &out
	})
	return h
}

type targetRefIndexKey struct {
	ir.PolicyTargetRef
	Namespace string
}

type PolicyIndex struct {
	policies       krt.Collection[ir.PolicyWrapper]
	targetRefIndex krt.Index[targetRefIndexKey, ir.PolicyWrapper]
}

func NewPolicyIndex(policies krt.Collection[ir.PolicyWrapper]) *PolicyIndex {
	targetRefIndex := krt.NewIndex(policies, func(p ir.PolicyWrapper) []targetRefIndexKey {
		ret := make([]targetRefIndexKey, len(p.TargetRefs))
		for i, tr := range p.TargetRefs {
			ret[i] = targetRefIndexKey{
				PolicyTargetRef: tr,
				Namespace:       p.Namespace,
			}
		}
		return ret
	})
	return &PolicyIndex{policies: policies, targetRefIndex: targetRefIndex}
}

func (p *PolicyIndex) GetTargetingPolicies(kctx krt.HandlerContext, ref ir.ObjectSource, sectionName string) []ir.PolicyWrapper {
	// no need for ref grants here as target refs are namespace local
	targetRefIndexKey := targetRefIndexKey{
		PolicyTargetRef: ir.PolicyTargetRef{
			Group: ref.Group,
			Kind:  ref.Kind,
			Name:  ref.Name,
		},
		Namespace: ref.Namespace,
	}
	return krt.Fetch(kctx, p.policies, krt.FilterIndex(p.targetRefIndex, targetRefIndexKey))
}

func (p *PolicyIndex) FetchPolicy(kctx krt.HandlerContext, ref ir.ObjectSource) *ir.PolicyWrapper {
	return krt.FetchOne(kctx, p.policies, krt.FilterKey(ref.ResourceName()))
}

type refGrantIndexKey struct {
	RefGrantNs string
	ToGK       schema.GroupKind
	ToName     string
	FromGK     schema.GroupKind
	FromNs     string
}
type RefGrantIndex struct {
	refgrants     krt.Collection[*gwv1beta1.ReferenceGrant]
	refGrantIndex krt.Index[refGrantIndexKey, *gwv1beta1.ReferenceGrant]
}

func NewRefGrantIndex(refgrants krt.Collection[*gwv1beta1.ReferenceGrant]) *RefGrantIndex {
	refGrantIndex := krt.NewIndex(refgrants, func(p *gwv1beta1.ReferenceGrant) []refGrantIndexKey {
		ret := make([]refGrantIndexKey, 0, len(p.Spec.To)*len(p.Spec.From))
		for _, from := range p.Spec.From {
			for _, to := range p.Spec.To {

				ret = append(ret, refGrantIndexKey{
					RefGrantNs: p.Namespace,
					ToGK:       schema.GroupKind{Group: string(to.Group), Kind: string(to.Kind)},
					ToName:     strOr(to.Name, ""),
					FromGK:     schema.GroupKind{Group: string(from.Group), Kind: string(from.Kind)},
					FromNs:     string(from.Namespace),
				})
			}
		}
		return ret
	})
	return &RefGrantIndex{refgrants: refgrants, refGrantIndex: refGrantIndex}
}

func (r *RefGrantIndex) ReferenceAllowed(kctx krt.HandlerContext, fromgk schema.GroupKind, fromns string, to ir.ObjectSource) bool {
	key := refGrantIndexKey{
		RefGrantNs: to.Namespace,
		ToGK:       schema.GroupKind{Group: to.Group, Kind: to.Kind},
		FromGK:     fromgk,
		FromNs:     fromns,
	}
	if krt.Fetch(kctx, r.refgrants, krt.FilterIndex(r.refGrantIndex, key)) != nil {
		return true
	}
	// try with name:
	key.ToName = to.Name
	if krt.Fetch(kctx, r.refgrants, krt.FilterIndex(r.refGrantIndex, key)) != nil {
		return true
	}
	return false
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
	return c.ResourceName() == in.ResourceName() && versionEquals(c.Route.GetSourceObject(), in.Route.GetSourceObject())
}
func versionEquals(a, b metav1.Object) bool {
	var versionEquals bool
	if a.GetGeneration() != 0 && b.GetGeneration() != 0 {
		versionEquals = a.GetGeneration() == b.GetGeneration()
	} else {
		versionEquals = a.GetResourceVersion() == b.GetResourceVersion()
	}
	return versionEquals && a.GetUID() == b.GetUID()
}

type RoutesIndex struct {
	routes          krt.Collection[RouteWrapper]
	httpRoutes      krt.Collection[ir.HttpRouteIR]
	httpByNamespace krt.Index[string, ir.HttpRouteIR]
	byTargetRef     krt.Index[types.NamespacedName, RouteWrapper]

	policies  *PolicyIndex
	refgrants *RefGrantIndex
	upstreams *UpstreamIndex
}

func NewRoutes(httproutes krt.Collection[*gwv1.HTTPRoute], tcproutes krt.Collection[*gwv1a2.TCPRoute], policies *PolicyIndex, upstreams *UpstreamIndex, refgrants *RefGrantIndex) *RoutesIndex {

	h := &RoutesIndex{policies: policies, refgrants: refgrants, upstreams: upstreams}
	h.httpRoutes = krt.NewCollection(httproutes, h.transformHttpRoute)
	hr := krt.NewCollection(h.httpRoutes, func(kctx krt.HandlerContext, i ir.HttpRouteIR) *RouteWrapper {
		return &RouteWrapper{Route: &i}
	})
	h.routes = krt.JoinCollection([]krt.Collection[RouteWrapper]{hr})

	httpByNamespace := krt.NewIndex(h.httpRoutes, func(i ir.HttpRouteIR) []string {
		return []string{i.GetNamespace()}
	})
	byTargetRef := krt.NewIndex(h.routes, func(in RouteWrapper) []types.NamespacedName {
		parentRefs := in.Route.GetParentRefs()
		ret := make([]types.NamespacedName, len(parentRefs))
		for i, pRef := range parentRefs {
			ns := strOr(pRef.Namespace, "")
			if ns == "" {
				ns = in.Route.GetNamespace()
			}
			ret[i] = types.NamespacedName{Namespace: ns, Name: string(pRef.Name)}
		}
		return ret
	})
	h.httpByNamespace = httpByNamespace
	h.byTargetRef = byTargetRef
	panic("TODO: implement tcp routes")
	return h
}

func (h *RoutesIndex) ListHttp(kctx krt.HandlerContext, ns string) []ir.HttpRouteIR {
	return krt.Fetch(kctx, h.httpRoutes, krt.FilterIndex(h.httpByNamespace, ns))
}

func (h *RoutesIndex) RoutesForGateway(kctx krt.HandlerContext, nns types.NamespacedName) []ir.Route {
	rts := krt.Fetch(kctx, h.routes, krt.FilterIndex(h.byTargetRef, nns))
	ret := make([]ir.Route, len(rts))
	for i, r := range rts {
		ret[i] = r.Route
	}
	return ret
}

func (h *RoutesIndex) FetchHttp(kctx krt.HandlerContext, n, ns string) *ir.HttpRouteIR {
	// TODO: maybe the key shouldnt include g and k?
	src := ir.ObjectSource{
		Group:     gwv1.SchemeGroupVersion.Group,
		Kind:      "HTTPRoute",
		Namespace: ns,
		Name:      n,
	}
	return krt.FetchOne(kctx, h.httpRoutes, krt.FilterKey(src.ResourceName()))
}

func (h *RoutesIndex) Fetch(kctx krt.HandlerContext, gk schema.GroupKind, n, ns string) *RouteWrapper {
	// TODO: maybe the key shouldnt include g and k?
	src := ir.ObjectSource{
		Group:     gk.Group,
		Kind:      gk.Kind,
		Namespace: ns,
		Name:      n,
	}
	return krt.FetchOne(kctx, h.routes, krt.FilterKey(src.ResourceName()))
}

func (h *RoutesIndex) transformHttpRoute(kctx krt.HandlerContext, i *gwv1.HTTPRoute) *ir.HttpRouteIR {
	src := ir.ObjectSource{
		Group:     gwv1.SchemeGroupVersion.Group,
		Kind:      "HTTPRoute",
		Namespace: i.Namespace,
		Name:      i.Name,
	}

	return &ir.HttpRouteIR{
		ObjectSource:     src,
		SourceObject:     i,
		ParentRefs:       i.Spec.ParentRefs,
		Hostnames:        tostr(i.Spec.Hostnames),
		Rules:            h.transformRules(kctx, src, i.Spec.Rules),
		AttachedPolicies: toAttachedPolicies(h.policies.GetTargetingPolicies(kctx, src, "")),
	}
}
func (h *RoutesIndex) transformRules(kctx krt.HandlerContext, src ir.ObjectSource, i []gwv1.HTTPRouteRule) []ir.HttpRouteRuleIR {
	rules := make([]ir.HttpRouteRuleIR, 0, len(i))
	for _, r := range i {

		extensionRefs := h.getExtensionRefs(kctx, src.Namespace, r)
		var policies ir.AttachedPolicies
		if r.Name != nil {
			policies = toAttachedPolicies(h.policies.GetTargetingPolicies(kctx, src, string(*r.Name)))
		}

		rules = append(rules, ir.HttpRouteRuleIR{
			HttpRouteRuleCommonIR: ir.HttpRouteRuleCommonIR{
				SourceRule:       &r,
				ExtensionRefs:    extensionRefs,
				AttachedPolicies: policies,
			},
			Backends: h.getBackends(kctx, src, r.BackendRefs),
			Matches:  r.Matches,
			Name:     emptyIfNil(r.Name),
		})
	}
	return rules

}
func (h *RoutesIndex) getExtensionRefs(kctx krt.HandlerContext, ns string, r gwv1.HTTPRouteRule) ir.AttachedPolicies {
	ret := ir.AttachedPolicies{
		Policies: map[schema.GroupKind][]ir.PolicyAtt{},
	}
	for _, ext := range r.Filters {
		if ext.ExtensionRef == nil {
			continue
		}
		ref := *ext.ExtensionRef
		gk := schema.GroupKind{
			Group: string(ref.Group),
			Kind:  string(ref.Kind),
		}
		key := ir.ObjectSource{
			Group:     string(ref.Group),
			Kind:      string(ref.Kind),
			Namespace: ns,
			Name:      string(ref.Name),
		}
		policy := h.policies.FetchPolicy(kctx, key)
		if policy != nil {
			ret.Policies[gk] = append(ret.Policies[gk], ir.PolicyAtt{PolicyIr: policy /*direct attachment - no target ref*/})
		}

	}
	return ret
}
func (h *RoutesIndex) getExtensionRefs2(kctx krt.HandlerContext, ns string, r []gwv1.HTTPRouteFilter) ir.AttachedPolicies {
	ret := ir.AttachedPolicies{
		Policies: map[schema.GroupKind][]ir.PolicyAtt{},
	}
	for _, ext := range r {
		if ext.ExtensionRef == nil {
			panic("TODO: handle built in extensions")
			continue
		}
		ref := *ext.ExtensionRef
		gk := schema.GroupKind{
			Group: string(ref.Group),
			Kind:  string(ref.Kind),
		}
		key := ir.ObjectSource{
			Group:     string(ref.Group),
			Kind:      string(ref.Kind),
			Namespace: ns,
			Name:      string(ref.Name),
		}
		policy := h.policies.FetchPolicy(kctx, key)
		if policy != nil {
			ret.Policies[gk] = append(ret.Policies[gk], ir.PolicyAtt{PolicyIr: policy /*direct attachment - no target ref*/})
		}

	}
	return ret
}

func (h *RoutesIndex) getBackends(kctx krt.HandlerContext, src ir.ObjectSource, i []gwv1.HTTPBackendRef) []ir.HttpBackendOrDelegate {
	backends := make([]ir.HttpBackendOrDelegate, 0, len(i))
	for _, ref := range i {
		extensionRefs := h.getExtensionRefs2(kctx, src.Namespace, ref.Filters)
		fromns := src.Namespace

		to := ir.ObjectSource{
			Group:     strOr(ref.BackendRef.Group, ""),
			Kind:      strOr(ref.BackendRef.Kind, "Service"),
			Namespace: strOr(ref.BackendRef.Namespace, fromns),
			Name:      string(ref.BackendRef.Name),
		}
		if backendref.RefIsHTTPRoute(ref.BackendRef.BackendObjectReference) {
			backends = append(backends, ir.HttpBackendOrDelegate{
				Delegate:         &to,
				AttachedPolicies: extensionRefs,
			})
			continue
		}

		var upstream *ir.Upstream
		fromgk := schema.GroupKind{
			Group: src.Group,
			Kind:  src.Kind,
		}
		var err error
		if h.refgrants.ReferenceAllowed(kctx, fromgk, fromns, to) {
			upstream, err = h.upstreams.getUpstreamFromRef(kctx, src.Namespace, ref.BackendRef.BackendObjectReference)
		} else {
			err = ErrMissingReferenceGrant
		}
		clusterName := "blackhole-cluster"
		if upstream != nil {
			panic("TODO: figure out cluster name")
			//			clusterName = ir.UpstreamToClusterName(upstream)
		}
		backends = append(backends, ir.HttpBackendOrDelegate{
			Backend: &ir.Backend{
				Upstream:    upstream,
				ClusterName: clusterName,
				Weight:      weight(ref.Weight),
				Err:         err,
			},
			AttachedPolicies: extensionRefs,
		})
	}
	return backends
}

type GwIndex struct {
	routes    krt.Collection[ir.GatewayIR]
	policies  *PolicyIndex
	refgrants *RefGrantIndex
}

//func NewGwIndex(gws krt.Collection[*gwv1.Gateway], policies *PolicyIndex, refgrants *RefGrantIndex) *HttpRoutesIndex {
//	h := &HttpRoutesIndex{policies: policies, refgrants: refgrants}
//	h.routes = krt.NewCollection(gws, h.transformGw)
//	return h
//}
//
//func (h *HttpRoutesIndex) transformGw(kctx krt.HandlerContext, i *gwv1.Gateway) *ir.GatewayWithPoliciesIR {
//	src := ir.ObjectSource{
//		Group:     gwv1.SchemeGroupVersion.Group,
//		Kind:      "Gateway",
//		Namespace: i.Namespace,
//		Name:      i.Name,
//	}
//
//	return &ir.HttpRouteIR{
//		ObjectSource:     src,
//		SourceObject:     i,
//		ParentRefs:       i.Spec.ParentRefs,
//		Hostnames:        tostr(i.Spec.Hostnames),
//		Rules:            h.transformRules(kctx, src, i.Spec.Rules),
//		AttachedPolicies: toAttachedPolicies(h.policies.GetTargetingPolicies(kctx, src, "")),
//	}
//}

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

func toAttachedPolicies(policies []ir.PolicyWrapper) ir.AttachedPolicies {
	ret := ir.AttachedPolicies{
		Policies: map[schema.GroupKind][]ir.PolicyAtt{},
	}
	for _, p := range policies {
		gk := schema.GroupKind{
			Group: p.Group,
			Kind:  p.Kind,
		}
		ret.Policies[gk] = append(ret.Policies[gk], ir.PolicyAtt{PolicyIr: p.PolicyIR})
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
	out := make([]string, len(in))
	for i, h := range in {
		out[i] = string(h)
	}
	return out
}
