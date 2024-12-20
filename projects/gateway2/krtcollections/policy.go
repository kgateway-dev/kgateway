package krtcollections

import (
	"errors"
	"fmt"
	"slices"

	extensionsplug "github.com/solo-io/gloo/projects/gateway2/extensions2/plugin"
	"github.com/solo-io/gloo/projects/gateway2/ir"
	"github.com/solo-io/gloo/projects/gateway2/translator/backendref"
	"github.com/solo-io/gloo/projects/gateway2/utils/krtutil"
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
)

type NotFoundError struct {
	// I call this `NotFound` so its easy to find in krt dump.
	NotFoundObj ir.ObjectSource
}

func (n *NotFoundError) Error() string {
	return fmt.Sprintf("%s \"%s\" not found", n.NotFoundObj.Kind, n.NotFoundObj.Name)
}

type UpstreamIndex struct {
	availableUpstreams  map[schema.GroupKind]krt.Collection[ir.Upstream]
	backendRefExtension []extensionsplug.GetBackendForRefPlugin
	policies            *PolicyIndex
	krtopts             krtutil.KrtOptions
}

func NewUpstreamIndex(krtopts krtutil.KrtOptions, backendRefExtension []extensionsplug.GetBackendForRefPlugin, policies *PolicyIndex) *UpstreamIndex {
	return &UpstreamIndex{
		policies:            policies,
		availableUpstreams:  map[schema.GroupKind]krt.Collection[ir.Upstream]{},
		krtopts:             krtopts,
		backendRefExtension: backendRefExtension,
	}
}

func (s *UpstreamIndex) HasSynced() bool {
	if !s.policies.HasSynced() {
		return false
	}
	for _, col := range s.availableUpstreams {
		if !col.Synced().HasSynced() {
			return false
		}
	}
	return true
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
		u.AttachedPolicies = toAttachedPolicies(ui.policies.getTargetingPolicies(kctx, extensionsplug.UpstreamAttachmentPoint, u.ObjectSource, ""))
		return &u
	}, ui.krtopts.ToOptions("")...)
	ui.availableUpstreams[gk] = ucol
}

func AddUpstreamMany[T metav1.Object](ui *UpstreamIndex, gk schema.GroupKind, col krt.Collection[T], build func(kctx krt.HandlerContext, svc T) []ir.Upstream) krt.Collection[ir.Upstream] {
	ucol := krt.NewManyCollection(col, func(kctx krt.HandlerContext, svc T) []ir.Upstream {
		upstreams := build(kctx, svc)
		for i := range upstreams {
			u := &upstreams[i]
			u.AttachedPolicies = toAttachedPolicies(ui.policies.getTargetingPolicies(kctx, extensionsplug.UpstreamAttachmentPoint, u.ObjectSource, ""))
		}
		return upstreams
	}, ui.krtopts.ToOptions("")...)
	ui.availableUpstreams[gk] = ucol
	return ucol
}

func AddUpstream[T metav1.Object](ui *UpstreamIndex, gk schema.GroupKind, col krt.Collection[T], build func(kctx krt.HandlerContext, svc T) *ir.Upstream) {
	ucol := krt.NewCollection(col, func(kctx krt.HandlerContext, svc T) *ir.Upstream {
		upstream := build(kctx, svc)
		if upstream == nil {
			return nil
		}
		upstream.AttachedPolicies = toAttachedPolicies(ui.policies.getTargetingPolicies(kctx, extensionsplug.UpstreamAttachmentPoint, upstream.ObjectSource, ""))

		return upstream
	}, ui.krtopts.ToOptions("")...)
	ui.availableUpstreams[gk] = ucol
}

// if we want to make this function public, make it do ref grants
func (i *UpstreamIndex) getUpstream(kctx krt.HandlerContext, gk schema.GroupKind, n types.NamespacedName, gwport *gwv1.PortNumber) (*ir.Upstream, error) {
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

	for _, getBackendRcol := range i.backendRefExtension {
		if up := getBackendRcol(kctx, key, port); up != nil {
			return up, nil
		}
	}

	col := i.availableUpstreams[gk]
	if col == nil {
		return nil, ErrUnknownBackendKind
	}

	up := krt.FetchOne(kctx, col, krt.FilterKey(ir.UpstreamResourceName(key, port)))
	if up == nil {
		return nil, &NotFoundError{NotFoundObj: key}
	}
	return up, nil
}

func (i *UpstreamIndex) getUpstreamFromRef(kctx krt.HandlerContext, localns string, ref gwv1.BackendObjectReference) (*ir.Upstream, error) {
	resolved := toFromBackendRef(localns, ref)
	return i.getUpstream(kctx, resolved.GetGroupKind(), types.NamespacedName{Namespace: resolved.Namespace, Name: resolved.Name}, ref.Port)
}

type GatewayIndex struct {
	policies *PolicyIndex
	gwClass  krt.Collection[gwv1.GatewayClass]
	Gateways krt.Collection[ir.Gateway]
}

func NewGatewayIndex(krtopts krtutil.KrtOptions, isOurGw func(gw *gwv1.Gateway) bool, policies *PolicyIndex, gws krt.Collection[*gwv1.Gateway]) *GatewayIndex {
	h := &GatewayIndex{policies: policies}
	h.Gateways = krt.NewCollection(gws, func(kctx krt.HandlerContext, i *gwv1.Gateway) *ir.Gateway {
		if !isOurGw(i) {
			return nil
		}
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
		//		panic("TODO: implement http policies not just listener")
		out.AttachedListenerPolicies = toAttachedPolicies(h.policies.getTargetingPolicies(kctx, extensionsplug.GatewayAttachmentPoint, out.ObjectSource, ""))
		out.AttachedHttpPolicies = out.AttachedListenerPolicies // see if i can find a better way to segment the listener level and http level policies
		for _, l := range i.Spec.Listeners {
			out.Listeners = append(out.Listeners, ir.Listener{
				Listener:         l,
				AttachedPolicies: toAttachedPolicies(h.policies.getTargetingPolicies(kctx, extensionsplug.RouteAttachmentPoint, out.ObjectSource, string(l.Name))),
			})
		}

		return &out
	}, krtopts.ToOptions("gateways")...)
	return h
}

type targetRefIndexKey struct {
	ir.PolicyTargetRef
	Namespace string
}

func (k targetRefIndexKey) String() string {
	return fmt.Sprintf("%s/%s/%s/%s", k.Group, k.Kind, k.Name, k.Namespace)
}

type globalPolicy struct {
	schema.GroupKind
	ir     func(krt.HandlerContext, extensionsplug.AttachmentPoints) ir.PolicyIR
	points extensionsplug.AttachmentPoints
}
type PolicyIndex struct {
	policies       krt.Collection[ir.PolicyWrapper]
	policiesFetch  map[schema.GroupKind]func(n string, ns string) ir.PolicyIR
	globalPolicies []globalPolicy
	targetRefIndex krt.Index[targetRefIndexKey, ir.PolicyWrapper]

	hasSyncedFuncs []func() bool
}

func (h *PolicyIndex) HasSynced() bool {
	for _, f := range h.hasSyncedFuncs {
		if !f() {
			return false
		}
	}
	return h.policies.Synced().HasSynced()
}

func NewPolicyIndex(krtopts krtutil.KrtOptions, contributesPolicies extensionsplug.ContributesPolicies) *PolicyIndex {

	h := &PolicyIndex{policiesFetch: map[schema.GroupKind]func(n string, ns string) ir.PolicyIR{}}

	var policycols []krt.Collection[ir.PolicyWrapper]
	for gk, ext := range contributesPolicies {
		if ext.Policies != nil {
			policycols = append(policycols, ext.Policies)
			h.hasSyncedFuncs = append(h.hasSyncedFuncs, ext.Policies.Synced().HasSynced)
		}
		if ext.PoliciesFetch != nil {
			h.policiesFetch[gk] = ext.PoliciesFetch
		}
		if ext.GlobalPolicies != nil {
			h.globalPolicies = append(h.globalPolicies, globalPolicy{GroupKind: gk, ir: ext.GlobalPolicies, points: ext.AttachmentPoints()})
		}
	}

	h.policies = krt.JoinCollection(policycols, krtopts.ToOptions("policies")...)

	h.targetRefIndex = krt.NewIndex(h.policies, func(p ir.PolicyWrapper) []targetRefIndexKey {
		ret := make([]targetRefIndexKey, len(p.TargetRefs))
		for i, tr := range p.TargetRefs {
			ret[i] = targetRefIndexKey{
				PolicyTargetRef: tr,
				Namespace:       p.Namespace,
			}
		}
		return ret
	})
	return h
}

// Attachment happens during collection creation (i.e. this file), and not translation. so these methods don't need to be public!
// note: we may want to change that for global policies maybe.
func (p *PolicyIndex) getTargetingPolicies(kctx krt.HandlerContext, pnt extensionsplug.AttachmentPoints, targetRef ir.ObjectSource, sectionName string) []ir.PolicyAtt {

	var ret []ir.PolicyAtt

	for _, gp := range p.globalPolicies {
		if gp.points.Has(pnt) {
			if p := gp.ir(kctx, pnt); p != nil {
				ret = append(ret, ir.PolicyAtt{PolicyIr: p, GroupKind: gp.GroupKind})
			}
		}
	}

	// no need for ref grants here as target refs are namespace local
	targetRefIndexKey := targetRefIndexKey{
		PolicyTargetRef: ir.PolicyTargetRef{
			Group: targetRef.Group,
			Kind:  targetRef.Kind,
			Name:  targetRef.Name,
		},
		Namespace: targetRef.Namespace,
	}
	policies := krt.Fetch(kctx, p.policies, krt.FilterIndex(p.targetRefIndex, targetRefIndexKey))
	targetRefIndexKey.SectionName = sectionName
	sectionNamePolicies := krt.Fetch(kctx, p.policies, krt.FilterIndex(p.targetRefIndex, targetRefIndexKey))

	for _, p := range policies {
		ret = append(ret, ir.PolicyAtt{PolicyIr: p.PolicyIR, GroupKind: p.GetGroupKind(), PolicyTargetRef: &ir.PolicyTargetRef{
			Group: p.Group,
			Kind:  p.Kind,
			Name:  p.Name,
		}})
	}
	for _, p := range sectionNamePolicies {
		ret = append(ret, ir.PolicyAtt{PolicyIr: p.PolicyIR, GroupKind: p.GetGroupKind(), PolicyTargetRef: &ir.PolicyTargetRef{
			Group:       p.Group,
			Kind:        p.Kind,
			Name:        p.Name,
			SectionName: sectionName,
		}})
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
	return krt.FetchOne(kctx, p.policies, krt.FilterKey(policyRef.ResourceName()))
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

type RefGrantIndex struct {
	refgrants     krt.Collection[*gwv1beta1.ReferenceGrant]
	refGrantIndex krt.Index[refGrantIndexKey, *gwv1beta1.ReferenceGrant]
}

func (h *RefGrantIndex) HasSynced() bool {
	return h.refgrants.Synced().HasSynced()
}

func NewRefGrantIndex(refgrants krt.Collection[*gwv1beta1.ReferenceGrant]) *RefGrantIndex {
	refGrantIndex := krt.NewIndex(refgrants, func(p *gwv1beta1.ReferenceGrant) []refGrantIndexKey {
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
	if len(krt.Fetch(kctx, r.refgrants, krt.FilterIndex(r.refGrantIndex, key))) != 0 {
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
	}
	panic("unknown route type")
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

	hasSyncedFuncs []func() bool
}

func (h *RoutesIndex) HasSynced() bool {
	for _, f := range h.hasSyncedFuncs {
		if !f() {
			return false
		}
	}
	return h.httpRoutes.Synced().HasSynced() && h.routes.Synced().HasSynced() && h.policies.HasSynced() && h.upstreams.HasSynced() && h.refgrants.HasSynced()
}

func NewRoutesIndex(krtopts krtutil.KrtOptions, httproutes krt.Collection[*gwv1.HTTPRoute], tcproutes krt.Collection[*gwv1a2.TCPRoute], policies *PolicyIndex, upstreams *UpstreamIndex, refgrants *RefGrantIndex) *RoutesIndex {

	h := &RoutesIndex{policies: policies, refgrants: refgrants, upstreams: upstreams}
	h.hasSyncedFuncs = append(h.hasSyncedFuncs, httproutes.Synced().HasSynced, tcproutes.Synced().HasSynced)
	h.httpRoutes = krt.NewCollection(httproutes, h.transformHttpRoute, krtopts.ToOptions("http-routes-with-policy")...)
	hr := krt.NewCollection(h.httpRoutes, func(kctx krt.HandlerContext, i ir.HttpRouteIR) *RouteWrapper {
		return &RouteWrapper{Route: &i}
	}, krtopts.ToOptions("routes-http-routes-with-policy")...)
	tr := krt.NewCollection(tcproutes, func(kctx krt.HandlerContext, i *gwv1a2.TCPRoute) *RouteWrapper {
		t := h.transformTcpRoute(kctx, i)
		return &RouteWrapper{Route: t}
	}, krtopts.ToOptions("routes-tcp-routes-with-policy")...)
	h.routes = krt.JoinCollection([]krt.Collection[RouteWrapper]{hr, tr}, krtopts.ToOptions("all-routes-with-policy")...)

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
		AttachedPolicies: toAttachedPolicies(h.policies.getTargetingPolicies(kctx, extensionsplug.RouteAttachmentPoint, src, "")),
	}
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
		AttachedPolicies: toAttachedPolicies(h.policies.getTargetingPolicies(kctx, extensionsplug.RouteAttachmentPoint, src, "")),
	}
}
func (h *RoutesIndex) transformRules(kctx krt.HandlerContext, src ir.ObjectSource, i []gwv1.HTTPRouteRule) []ir.HttpRouteRuleIR {
	rules := make([]ir.HttpRouteRuleIR, 0, len(i))
	for _, r := range i {

		extensionRefs := h.getExtensionRefs(kctx, src.Namespace, r.Filters)
		var policies ir.AttachedPolicies
		if r.Name != nil {
			policies = toAttachedPolicies(h.policies.getTargetingPolicies(kctx, extensionsplug.RouteAttachmentPoint, src, string(*r.Name)))
		}

		rules = append(rules, ir.HttpRouteRuleIR{
			ExtensionRefs:    extensionRefs,
			AttachedPolicies: policies,
			Backends:         h.getBackends(kctx, src, r.BackendRefs),
			Matches:          r.Matches,
			Name:             emptyIfNil(r.Name),
		})
	}
	return rules

}

func (h *RoutesIndex) getExtensionRefs(kctx krt.HandlerContext, ns string, r []gwv1.HTTPRouteFilter) ir.AttachedPolicies {
	ret := ir.AttachedPolicies{
		Policies: map[schema.GroupKind][]ir.PolicyAtt{},
	}
	for _, ext := range r {
		// TODO: propagate error if we can't find the extension
		gk, policy := h.resolveExtension(kctx, ns, ext)
		if policy != nil {
			ret.Policies[gk] = append(ret.Policies[gk], ir.PolicyAtt{PolicyIr: policy /*direct attachment - no target ref*/})
		}

	}
	return ret
}

func (h *RoutesIndex) resolveExtension(kctx krt.HandlerContext, ns string, ext gwv1.HTTPRouteFilter) (schema.GroupKind, ir.PolicyIR) {
	if ext.Type == gwv1.HTTPRouteFilterExtensionRef {
		if ext.ExtensionRef == nil {
			// TODO: report error!!
			return schema.GroupKind{}, nil
		}
		// panic("TODO: handle built in extensions")
		ref := *ext.ExtensionRef
		key := ir.ObjectSource{
			Group:     string(ref.Group),
			Kind:      string(ref.Kind),
			Namespace: ns,
			Name:      string(ref.Name),
		}
		policy := h.policies.fetchPolicy(kctx, key)
		if policy == nil {
			// TODO: report error!!
			return schema.GroupKind{}, nil
		}

		gk := schema.GroupKind{
			Group: string(ref.Group),
			Kind:  string(ref.Kind),
		}
		return gk, policy.PolicyIR
	}

	fromGK := schema.GroupKind{
		Group: gwv1.SchemeGroupVersion.Group,
		Kind:  "HTTPRoute",
	}

	return VirtualBuiltInGK, NewBuiltInIr(kctx, ext, fromGK, ns, h.refgrants, h.upstreams)
}

func toFromBackendRef(fromns string, ref gwv1.BackendObjectReference) ir.ObjectSource {
	return ir.ObjectSource{
		Group:     strOr(ref.Group, ""),
		Kind:      strOr(ref.Kind, "Service"),
		Namespace: strOr(ref.Namespace, fromns),
		Name:      string(ref.Name),
	}
}

func (h *RoutesIndex) getBackends(kctx krt.HandlerContext, src ir.ObjectSource, i []gwv1.HTTPBackendRef) []ir.HttpBackendOrDelegate {
	backends := make([]ir.HttpBackendOrDelegate, 0, len(i))
	for _, ref := range i {
		extensionRefs := h.getExtensionRefs(kctx, src.Namespace, ref.Filters)
		fromns := src.Namespace

		to := toFromBackendRef(fromns, ref.BackendObjectReference)
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
		// TODO: if we can't find the upstream, should we
		// still use its cluster name in case it comes up later?
		// if so we need to think about the way create cluster names,
		// so it only depends on the backend-ref
		clusterName := "blackhole-cluster"
		if upstream != nil {
			clusterName = upstream.ClusterName()
		} else if err == nil {
			err = &NotFoundError{NotFoundObj: to}
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

func (h *RoutesIndex) getTcpBackends(kctx krt.HandlerContext, src ir.ObjectSource, i []gwv1.BackendRef) []ir.Backend {
	backends := make([]ir.Backend, 0, len(i))
	for _, ref := range i {
		fromns := src.Namespace

		to := toFromBackendRef(fromns, ref.BackendObjectReference)
		var upstream *ir.Upstream
		fromgk := schema.GroupKind{
			Group: src.Group,
			Kind:  src.Kind,
		}
		var err error
		if h.refgrants.ReferenceAllowed(kctx, fromgk, fromns, to) {
			upstream, err = h.upstreams.getUpstreamFromRef(kctx, src.Namespace, ref.BackendObjectReference)
		} else {
			err = ErrMissingReferenceGrant
		}
		clusterName := "blackhole-cluster"
		if upstream != nil {
			clusterName = upstream.ClusterName()
		} else if err == nil {
			err = &NotFoundError{NotFoundObj: to}
		}
		backends = append(backends, ir.Backend{
			Upstream:    upstream,
			ClusterName: clusterName,
			Weight:      weight(ref.Weight),
			Err:         err,
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

func toAttachedPolicies(policies []ir.PolicyAtt) ir.AttachedPolicies {
	ret := ir.AttachedPolicies{
		Policies: map[schema.GroupKind][]ir.PolicyAtt{},
	}
	for _, p := range policies {
		gk := schema.GroupKind{
			Group: p.GroupKind.Group,
			Kind:  p.GroupKind.Kind,
		}
		ret.Policies[gk] = append(ret.Policies[gk], ir.PolicyAtt{PolicyIr: p.PolicyIr, PolicyTargetRef: p.PolicyTargetRef})
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
