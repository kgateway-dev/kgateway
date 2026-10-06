package proxy_syncer

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/test"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/plugins/backendtlspolicy"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/statussync"
	pluginsdkutils "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
	utilkrt "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
)

type policyTargetTestIR struct{}

func (policyTargetTestIR) CreationTime() time.Time { return time.Time{} }
func (policyTargetTestIR) Equals(any) bool         { return true }

const policyTargetTestNS = "default"

func trafficPolicyWrapper(name string, generation int64, refs ...ir.PolicyRef) ir.PolicyWrapper {
	return ir.PolicyWrapper{
		ObjectSource: ir.ObjectSource{
			Group:     wellknown.TrafficPolicyGVK.Group,
			Kind:      wellknown.TrafficPolicyGVK.Kind,
			Namespace: policyTargetTestNS,
			Name:      name,
		},
		Policy: &kgateway.TrafficPolicy{ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: policyTargetTestNS, Generation: generation,
		}},
		PolicyIR:   policyTargetTestIR{},
		TargetRefs: refs,
	}
}

func gatewayRef(name, section string) ir.PolicyRef {
	return ir.PolicyRef{Group: wellknown.GatewayGVK.Group, Kind: wellknown.GatewayGVK.Kind, Name: name, SectionName: section}
}

func httpRouteRef(name, section string) ir.PolicyRef {
	return ir.PolicyRef{Group: wellknown.HTTPRouteGVK.Group, Kind: wellknown.HTTPRouteGVK.Kind, Name: name, SectionName: section}
}

func tcpRouteRef(name, section string) ir.PolicyRef {
	return ir.PolicyRef{Group: wellknown.TCPRouteGVK.Group, Kind: wellknown.TCPRouteGVK.Kind, Name: name, SectionName: section}
}

func tlsRouteRef(name string) ir.PolicyRef {
	return ir.PolicyRef{Group: wellknown.TLSRouteGVK.Group, Kind: wellknown.TLSRouteGVK.Kind, Name: name}
}

func serviceRef(name string) ir.PolicyRef {
	return ir.PolicyRef{Group: "", Kind: wellknown.ServiceGVK.Kind, Name: name}
}

func listenerSetRef(gvk schema.GroupVersionKind, name, section string) ir.PolicyRef {
	return ir.PolicyRef{Group: gvk.Group, Kind: gvk.Kind, Name: name, SectionName: section}
}

// serviceEntryBackend mirrors how the serviceentry plugin aliases its backends.
func serviceEntryBackend(name, namespace, hostname string) ir.BackendObjectIR {
	objSrc := ir.ObjectSource{
		Group: wellknown.ServiceEntryGVK.Group, Kind: wellknown.ServiceEntryGVK.Kind, Namespace: namespace, Name: name,
	}
	backend := ir.NewBackendObjectIR(objSrc, 80, hostname, "istio-se")
	backend.Aliases = []ir.ObjectSource{
		objSrc,
		{Group: wellknown.HostnameGVK.Group, Kind: wellknown.HostnameGVK.Kind, Name: hostname},
	}
	return backend
}

func hostnameRef(hostname string) ir.PolicyRef {
	return ir.PolicyRef{Group: wellknown.HostnameGVK.Group, Kind: wellknown.HostnameGVK.Kind, Name: hostname}
}

func serviceEntryRef(name string) ir.PolicyRef {
	return ir.PolicyRef{Group: wellknown.ServiceEntryGVK.Group, Kind: wellknown.ServiceEntryGVK.Kind, Name: name}
}

// policyTargetGateway returns a Gateway in the test namespace with one HTTP listener per name.
func policyTargetGateway(name string, listeners ...gwv1.SectionName) *gwv1.Gateway {
	gw := &gwv1.Gateway{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: policyTargetTestNS}}
	port := gwv1.PortNumber(80)
	for _, l := range listeners {
		gw.Spec.Listeners = append(gw.Spec.Listeners, gwv1.Listener{Name: l, Port: port, Protocol: gwv1.HTTPProtocolType})
		port++
	}
	return gw
}

type policyTargetFixture struct {
	gateways      krt.StaticCollection[*gwv1.Gateway]
	policies      krt.StaticCollection[ir.PolicyWrapper]
	contributions krt.Collection[reports.StatusContribution]
}

func newPolicyTargetFixture(t *testing.T, policies ...ir.PolicyWrapper) policyTargetFixture {
	t.Helper()
	return newPolicyTargetFixtureWithOptions(t, nil, policies...)
}

// newPolicyTargetFixtureWithOptions builds the fixture with the resolvers the given status
// syncer options register, the way NewProxySyncer receives them.
func newPolicyTargetFixtureWithOptions(t *testing.T, opts []StatusSyncerOption, policies ...ir.PolicyWrapper) policyTargetFixture {
	t.Helper()
	krtopts := krtutil.NewKrtOptions(t.Context().Done(), nil)

	gateways := krt.NewStaticCollection(nil, []*gwv1.Gateway{policyTargetGateway("gw", "http")},
		krtopts.ToOptions("Gateways")...)
	ruleName := gwv1.SectionName("rule-a")
	routes := krt.NewStaticCollection(nil, []*gwv1.HTTPRoute{{
		ObjectMeta: metav1.ObjectMeta{Name: "route-a", Namespace: policyTargetTestNS},
		Spec:       gwv1.HTTPRouteSpec{Rules: []gwv1.HTTPRouteRule{{Name: &ruleName}}},
	}}, krtopts.ToOptions("HTTPRoutes")...)
	tcpRoutes := krt.NewStaticCollection(nil, []*gwv1a2.TCPRoute{{
		ObjectMeta: metav1.ObjectMeta{Name: "tcp-a", Namespace: policyTargetTestNS},
	}}, krtopts.ToOptions("TCPRoutes")...)
	tlsRoutes := krt.NewStaticCollection(nil, []*gwv1a2.TLSRoute{{
		ObjectMeta: metav1.ObjectMeta{Name: "tls-a", Namespace: policyTargetTestNS},
	}}, krtopts.ToOptions("TLSRoutes")...)
	services := krt.NewStaticCollection(nil, []*corev1.Service{{
		ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: policyTargetTestNS},
	}}, krtopts.ToOptions("Services")...)
	// The normalized ListenerSet collection: a promoted object carries no GVK in TypeMeta,
	// a legacy one is stamped XListenerSet by the converter.
	legacy := &gwv1.ListenerSet{
		ObjectMeta: metav1.ObjectMeta{Name: "ls-legacy", Namespace: policyTargetTestNS},
		Spec:       gwv1.ListenerSetSpec{Listeners: []gwv1.ListenerEntry{{Name: "http", Port: 80, Protocol: gwv1.HTTPProtocolType}}},
	}
	legacy.SetGroupVersionKind(wellknown.XListenerSetGVK)
	listenerSets := krt.NewStaticCollection(nil, []*gwv1.ListenerSet{
		{ObjectMeta: metav1.ObjectMeta{Name: "ls-promoted", Namespace: policyTargetTestNS}},
		legacy,
	}, krtopts.ToOptions("ListenerSets")...)
	// A backend plugin exposing alias kinds, shaped like the serviceentry plugin: one backend per
	// host carrying the ServiceEntry itself and a namespace-less Hostname alias.
	serviceEntryBackends := krt.NewStaticCollection(nil, []ir.BackendObjectIR{
		serviceEntryBackend("se", policyTargetTestNS, "se.example.com"),
		serviceEntryBackend("se-other-ns", "other", "other.example.com"),
	}, krtopts.ToOptions("ServiceEntryBackends")...)

	resolvers := newPolicyTargetResolvers(&collections.CommonCollections{
		RawGateways:     gateways,
		RawListenerSets: listenerSets,
		RawHTTPRoutes:   routes,
		RawTCPRoutes:    tcpRoutes,
		RawTLSRoutes:    tlsRoutes,
		Services:        services,
	}, map[schema.GroupKind]sdk.BackendPlugin{
		wellknown.ServiceEntryGVK.GroupKind(): {
			Backends:   serviceEntryBackends,
			AliasKinds: []schema.GroupKind{wellknown.HostnameGVK.GroupKind(), wellknown.ServiceEntryGVK.GroupKind()},
		},
	}, processStatusSyncerOptions(opts...).policyTargetResolvers)

	policyCol := krt.NewStaticCollection(nil, policies, krtopts.ToOptions("Policies")...)
	contributions := policyTargetStatusContributions(policyCol, resolvers, krtopts)
	require.True(t, contributions.WaitUntilSynced(t.Context().Done()), "contributions should sync")
	return policyTargetFixture{gateways: gateways, policies: policyCol, contributions: contributions}
}

// acceptedCondition returns the Accepted condition of the single summary ancestor in a policy
// target contribution, failing the test if the contribution has any other shape.
func acceptedCondition(t *testing.T, c reports.StatusContribution, policy ir.PolicyWrapper) metav1.Condition {
	t.Helper()
	require.Equal(t, reports.PolicyTargetStatusSource, c.Source.Kind)
	require.Equal(t, policy.ResourceName(), c.Source.Name)
	require.Equal(t, policy.Kind, c.Target.Kind)
	require.Equal(t, policy.Name, c.Target.Name)
	require.NotNil(t, c.Policy, "contribution should carry a policy report")
	require.Len(t, c.Policy.Ancestors, 1, "unresolved targets should share one ancestor")

	status := reports.BuildPolicyStatus(c.Policy, reporter.PolicyKey{
		Group: policy.Group, Kind: policy.Kind, Namespace: policy.Namespace, Name: policy.Name,
	}, "test-controller", gwv1.PolicyStatus{})
	require.NotNil(t, status)
	require.Len(t, status.Ancestors, 1)
	ancestor := status.Ancestors[0]
	require.True(t, reports.ParentRefEqual(reporter.PolicyStatusSummaryAncestorRef(), ancestor.AncestorRef),
		"the ancestor should be the summary entry, with explicit group and kind: %+v", ancestor.AncestorRef)

	attached := meta.FindStatusCondition(ancestor.Conditions, string(shared.PolicyConditionAttached))
	require.NotNil(t, attached)
	require.Equal(t, metav1.ConditionFalse, attached.Status)
	require.Equal(t, string(shared.PolicyReasonTargetNotFound), attached.Reason)

	accepted := meta.FindStatusCondition(ancestor.Conditions, string(shared.PolicyConditionAccepted))
	require.NotNil(t, accepted)
	require.Equal(t, metav1.ConditionFalse, accepted.Status)
	require.Equal(t, string(shared.PolicyReasonTargetNotFound), accepted.Reason)
	require.Equal(t, policy.Policy.GetGeneration(), accepted.ObservedGeneration)
	return *accepted
}

func TestPolicyTargetStatusContributions(t *testing.T) {
	resolved := trafficPolicyWrapper("resolved", 1, gatewayRef("gw", ""), gatewayRef("gw", "http"), httpRouteRef("route-a", "rule-a"), serviceRef("svc"))
	missingGateway := trafficPolicyWrapper("missing-gateway", 3, gatewayRef("missing", ""))
	partial := trafficPolicyWrapper("partial", 1, httpRouteRef("route-a", ""), httpRouteRef("route-b-typo", ""))
	badSection := trafficPolicyWrapper("bad-section", 1, gatewayRef("gw", "https"), httpRouteRef("route-a", "rule-z"))
	unknownKind := trafficPolicyWrapper("unknown-kind", 1, ir.PolicyRef{Group: "example.com", Kind: "Widget", Name: "missing"})
	selectorOnly := trafficPolicyWrapper("selector-only", 1, ir.PolicyRef{
		Group: wellknown.HTTPRouteGVK.Group, Kind: wellknown.HTTPRouteGVK.Kind, MatchLabels: map[string]string{"app": "none"},
	})

	f := newPolicyTargetFixture(t, resolved, missingGateway, partial, badSection, unknownKind, selectorOnly)

	byPolicy := map[string]reports.StatusContribution{}
	for _, c := range f.contributions.List() {
		byPolicy[c.Target.Name] = c
	}

	require.NotContains(t, byPolicy, "resolved", "every target resolves, so nothing to report")
	require.NotContains(t, byPolicy, "unknown-kind", "kinds without a resolver are not claimed")
	require.NotContains(t, byPolicy, "selector-only", "a selector matching nothing is not a missing target")

	accepted := acceptedCondition(t, byPolicy["missing-gateway"], missingGateway)
	require.Equal(t, "Gateway default/missing not found", accepted.Message)

	accepted = acceptedCondition(t, byPolicy["partial"], partial)
	require.Equal(t, "HTTPRoute default/route-b-typo not found", accepted.Message,
		"only the unresolved ref is reported; the valid one gets its Gateway ancestor from translation")

	accepted = acceptedCondition(t, byPolicy["bad-section"], badSection)
	require.Equal(t, `sectionName "https" not found in Gateway default/gw; sectionName "rule-z" not found in HTTPRoute default/route-a`,
		accepted.Message)
}

// TestPolicyTargetStatusContributionsListenerSetFlavors pins that a ListenerSet target is
// matched on group and kind, not only on name: attachment keys policies on the object's own
// GVK, so a same-named object of the other flavor is not the target.
func TestPolicyTargetStatusContributionsListenerSetFlavors(t *testing.T) {
	promoted, legacy := wellknown.ListenerSetGVK, wellknown.XListenerSetGVK
	resolved := trafficPolicyWrapper("resolved", 1,
		listenerSetRef(promoted, "ls-promoted", ""), listenerSetRef(legacy, "ls-legacy", ""), listenerSetRef(legacy, "ls-legacy", "http"))
	legacyRefToPromoted := trafficPolicyWrapper("legacy-ref-to-promoted", 1, listenerSetRef(legacy, "ls-promoted", ""))
	promotedRefToLegacy := trafficPolicyWrapper("promoted-ref-to-legacy", 1, listenerSetRef(promoted, "ls-legacy", ""))
	badSection := trafficPolicyWrapper("bad-section", 1, listenerSetRef(legacy, "ls-legacy", "https"))

	f := newPolicyTargetFixture(t, resolved, legacyRefToPromoted, promotedRefToLegacy, badSection)

	byPolicy := map[string]reports.StatusContribution{}
	for _, c := range f.contributions.List() {
		byPolicy[c.Target.Name] = c
	}
	require.NotContains(t, byPolicy, "resolved")
	require.Equal(t, "XListenerSet default/ls-promoted not found",
		acceptedCondition(t, byPolicy["legacy-ref-to-promoted"], legacyRefToPromoted).Message)
	require.Equal(t, "ListenerSet default/ls-legacy not found",
		acceptedCondition(t, byPolicy["promoted-ref-to-legacy"], promotedRefToLegacy).Message)
	require.Equal(t, `sectionName "https" not found in XListenerSet default/ls-legacy`,
		acceptedCondition(t, byPolicy["bad-section"], badSection).Message)
}

// TestPolicyTargetStatusContributionsAliasKinds pins that alias kinds declared by backend
// plugins resolve through the backends carrying the alias, scoped to the policy's namespace the
// way attachment scopes a namespace-less alias to its backend's namespace.
func TestPolicyTargetStatusContributionsAliasKinds(t *testing.T) {
	resolved := trafficPolicyWrapper("resolved", 1, hostnameRef("se.example.com"), serviceEntryRef("se"))
	missingHost := trafficPolicyWrapper("missing-host", 1, hostnameRef("se.exmaple.com"))
	otherNamespaceHost := trafficPolicyWrapper("other-ns-host", 1, hostnameRef("other.example.com"))
	missingServiceEntry := trafficPolicyWrapper("missing-se", 1, serviceEntryRef("se-typo"))

	f := newPolicyTargetFixture(t, resolved, missingHost, otherNamespaceHost, missingServiceEntry)

	byPolicy := map[string]reports.StatusContribution{}
	for _, c := range f.contributions.List() {
		byPolicy[c.Target.Name] = c
	}
	require.NotContains(t, byPolicy, "resolved", "a host and a ServiceEntry the plugin exposes both resolve")
	require.Equal(t, "Hostname default/se.exmaple.com not found",
		acceptedCondition(t, byPolicy["missing-host"], missingHost).Message)
	require.Equal(t, "Hostname default/other.example.com not found",
		acceptedCondition(t, byPolicy["other-ns-host"], otherNamespaceHost).Message,
		"a host declared only by a ServiceEntry in another namespace is not a target of this policy")
	require.Equal(t, "ServiceEntry default/se-typo not found",
		acceptedCondition(t, byPolicy["missing-se"], missingServiceEntry).Message)
}

// TestPolicyTargetStatusContributionsTCPAndTLSRoutes pins the TCPRoute and TLSRoute resolvers.
// Both kinds attach policies at the route level only, so a sectionName is not checked.
func TestPolicyTargetStatusContributionsTCPAndTLSRoutes(t *testing.T) {
	resolved := trafficPolicyWrapper("resolved", 1, tcpRouteRef("tcp-a", ""), tcpRouteRef("tcp-a", "any-rule"), tlsRouteRef("tls-a"))
	missingTCP := trafficPolicyWrapper("missing-tcp", 1, tcpRouteRef("tcp-typo", ""))
	missingTLS := trafficPolicyWrapper("missing-tls", 1, tlsRouteRef("tls-typo"))

	f := newPolicyTargetFixture(t, resolved, missingTCP, missingTLS)

	byPolicy := map[string]reports.StatusContribution{}
	for _, c := range f.contributions.List() {
		byPolicy[c.Target.Name] = c
	}
	require.NotContains(t, byPolicy, "resolved", "existing TCP and TLS routes resolve, with or without a sectionName")
	require.Equal(t, "TCPRoute default/tcp-typo not found",
		acceptedCondition(t, byPolicy["missing-tcp"], missingTCP).Message)
	require.Equal(t, "TLSRoute default/tls-typo not found",
		acceptedCondition(t, byPolicy["missing-tls"], missingTLS).Message)
}

// TestPolicyTargetStatusContributionsExtensionResolver pins WithPolicyTargetResolver: an
// extension kind kgateway does not know is checked once registered, section names included,
// and a registration for a built-in kind replaces the built-in resolver.
func TestPolicyTargetStatusContributionsExtensionResolver(t *testing.T) {
	krtopts := krtutil.NewKrtOptions(t.Context().Done(), nil)
	extensionGK := schema.GroupKind{Group: "example.com", Kind: "ExtensionListenerSet"}
	extensionListenerSets := krt.NewStaticCollection(nil, []*gwv1.ListenerSet{{
		ObjectMeta: metav1.ObjectMeta{Name: "els", Namespace: policyTargetTestNS},
		Spec:       gwv1.ListenerSetSpec{Listeners: []gwv1.ListenerEntry{{Name: "http", Port: 80, Protocol: gwv1.HTTPProtocolType}}},
	}}, krtopts.ToOptions("ExtensionListenerSets")...)
	extensionRef := func(name, section string) ir.PolicyRef {
		return ir.PolicyRef{Group: extensionGK.Group, Kind: extensionGK.Kind, Name: name, SectionName: section}
	}
	opts := []StatusSyncerOption{
		WithPolicyTargetResolver(extensionGK, NewObjectPolicyTargetResolver(extensionListenerSets, extensionGK.Kind,
			func(ls *gwv1.ListenerSet) []string {
				return namesOf(ls.Spec.Listeners, func(l gwv1.ListenerEntry) string { return string(l.Name) })
			})),
		// Replaces the built-in Service resolver: every Service ref now resolves.
		WithPolicyTargetResolver(wellknown.ServiceGVK.GroupKind(),
			func(krt.HandlerContext, string, string, string) error { return nil }),
		// A nil resolver is ignored rather than registered.
		WithPolicyTargetResolver(wellknown.GatewayGVK.GroupKind(), nil),
	}

	resolved := trafficPolicyWrapper("resolved", 1, extensionRef("els", ""), extensionRef("els", "http"), serviceRef("svc-missing"))
	missing := trafficPolicyWrapper("missing", 1, extensionRef("els-typo", ""))
	badSection := trafficPolicyWrapper("bad-section", 1, extensionRef("els", "https"))
	missingGateway := trafficPolicyWrapper("missing-gateway", 1, gatewayRef("missing", ""))

	f := newPolicyTargetFixtureWithOptions(t, opts, resolved, missing, badSection, missingGateway)

	byPolicy := map[string]reports.StatusContribution{}
	for _, c := range f.contributions.List() {
		byPolicy[c.Target.Name] = c
	}
	require.NotContains(t, byPolicy, "resolved", "the extension target resolves, and the Service override accepts any name")
	require.Equal(t, "ExtensionListenerSet default/els-typo not found",
		acceptedCondition(t, byPolicy["missing"], missing).Message)
	require.Equal(t, `sectionName "https" not found in ExtensionListenerSet default/els`,
		acceptedCondition(t, byPolicy["bad-section"], badSection).Message)
	require.Equal(t, "Gateway default/missing not found",
		acceptedCondition(t, byPolicy["missing-gateway"], missingGateway).Message,
		"a nil registration leaves the built-in Gateway resolver in place")
}

// TestGeneratePolicyTargetReportsExtensionResolver pins that the golden-test helper applies the
// resolvers registered through the options, as the proxy syncer does: without them an extension
// kind is unchecked and a missing target of it reports nothing.
func TestGeneratePolicyTargetReportsExtensionResolver(t *testing.T) {
	krtopts := krtutil.NewKrtOptions(t.Context().Done(), nil)
	extensionGK := schema.GroupKind{Group: "example.com", Kind: "ExtensionListenerSet"}
	extensionListenerSets := krt.NewStaticCollection(nil, []*gwv1.ListenerSet{{
		ObjectMeta: metav1.ObjectMeta{Name: "els", Namespace: policyTargetTestNS},
	}}, krtopts.ToOptions("ExtensionListenerSets")...)
	missing := trafficPolicyWrapper("missing", 1,
		ir.PolicyRef{Group: extensionGK.Group, Kind: extensionGK.Kind, Name: "els-typo"})
	resolved := trafficPolicyWrapper("resolved", 1,
		ir.PolicyRef{Group: extensionGK.Group, Kind: extensionGK.Kind, Name: "els"})
	plugins := sdk.Plugin{ContributesPolicies: map[schema.GroupKind]sdk.PolicyPlugin{
		wellknown.TrafficPolicyGVK.GroupKind(): {
			Policies: krt.NewStaticCollection(nil, []ir.PolicyWrapper{missing, resolved}, krtopts.ToOptions("Policies")...),
		},
	}}

	require.Empty(t, GeneratePolicyTargetReports(nil, plugins).Policies,
		"an extension kind without a registered resolver is not checked")

	out := GeneratePolicyTargetReports(nil, plugins,
		WithPolicyTargetResolver(extensionGK, NewObjectPolicyTargetResolver(extensionListenerSets, extensionGK.Kind, nil)))
	require.Len(t, out.Policies, 1)
	key := reporter.PolicyKey{Group: missing.Group, Kind: missing.Kind, Namespace: missing.Namespace, Name: missing.Name}
	status := reports.BuildPolicyStatus(out.Policies[key], key, "test-controller", gwv1.PolicyStatus{})
	require.NotNil(t, status)
	require.Len(t, status.Ancestors, 1)
	accepted := meta.FindStatusCondition(status.Ancestors[0].Conditions, string(shared.PolicyConditionAccepted))
	require.NotNil(t, accepted)
	require.Equal(t, string(shared.PolicyReasonTargetNotFound), accepted.Reason)
	require.Equal(t, "ExtensionListenerSet default/els-typo not found", accepted.Message)
}

// TestPolicyTargetReportThroughBackendTLSPolicyBuilder pins ObservedGeneration on the
// conditions themselves: BackendTLSPolicy's builder copies report conditions verbatim rather
// than stamping the report's generation the way the standard builder does.
func TestPolicyTargetReportThroughBackendTLSPolicyBuilder(t *testing.T) {
	const generation int64 = 7
	btp := &gwv1.BackendTLSPolicy{ObjectMeta: metav1.ObjectMeta{Name: "btp", Namespace: policyTargetTestNS, Generation: generation}}
	policy := ir.PolicyWrapper{
		ObjectSource: ir.ObjectSource{
			Group: wellknown.BackendTLSPolicyGVK.Group, Kind: wellknown.BackendTLSPolicyGVK.Kind,
			Namespace: btp.Namespace, Name: btp.Name,
		},
		Policy:     btp,
		PolicyIR:   policyTargetTestIR{},
		TargetRefs: []ir.PolicyRef{serviceRef("missing")},
	}

	report := buildPolicyTargetReport(policy, []string{"Service default/missing not found"})
	status := backendtlspolicy.BuildDesiredPolicyStatus(report, btp, "test-controller")
	require.NotNil(t, status)
	require.Len(t, status.Ancestors, 1)
	require.Len(t, status.Ancestors[0].Conditions, 2)
	for _, condition := range status.Ancestors[0].Conditions {
		require.Equal(t, generation, condition.ObservedGeneration, "condition %s", condition.Type)
		require.Equal(t, string(shared.PolicyReasonTargetNotFound), condition.Reason)
	}
}

// TestPolicyTargetStatusContributionsFollowTarget pins the krt dependencies: the contribution
// is recomputed when the target object changes and when the policy's own targetRefs change, and
// it is retracted as soon as every ref resolves.
func TestPolicyTargetStatusContributionsFollowTarget(t *testing.T) {
	// contributionFor returns the policy's contribution and the Accepted message and
	// generation it carries, without asserting on its shape: mid-update it may still describe
	// the previous generation.
	contributionFor := func(f policyTargetFixture, policy ir.PolicyWrapper) (*reports.StatusContribution, string, int64) {
		for _, c := range f.contributions.List() {
			if c.Target.Name != policy.Name || c.Policy == nil {
				continue
			}
			for _, ancestor := range c.Policy.Ancestors {
				if accepted := meta.FindStatusCondition(ancestor.Conditions, string(shared.PolicyConditionAccepted)); accepted != nil {
					return &c, accepted.Message, accepted.ObservedGeneration
				}
			}
		}
		return nil, "", 0
	}
	// waitFor waits until the policy's contribution reports want for the policy's current
	// generation ("" meaning no contribution), then checks the contribution's full shape.
	waitFor := func(t *testing.T, f policyTargetFixture, policy ir.PolicyWrapper, want, why string) {
		t.Helper()
		gen := policy.Policy.GetGeneration()
		require.Eventually(t, func() bool {
			c, msg, got := contributionFor(f, policy)
			if want == "" {
				return c == nil
			}
			return msg == want && got == gen
		}, 5*time.Second, 10*time.Millisecond, why)
		if c, _, _ := contributionFor(f, policy); c != nil {
			acceptedCondition(t, *c, policy)
		}
	}

	t.Run("the target is created and deleted", func(t *testing.T) {
		policy := trafficPolicyWrapper("policy", 1, gatewayRef("late", ""))
		f := newPolicyTargetFixture(t, policy)
		waitFor(t, f, policy, "Gateway default/late not found", "the Gateway does not exist yet")

		late := policyTargetGateway("late")
		f.gateways.UpdateObject(late)
		waitFor(t, f, policy, "", "creating the target should retract the contribution")

		f.gateways.DeleteObject(krt.GetKey(late))
		waitFor(t, f, policy, "Gateway default/late not found", "deleting the target should report it missing again")
	})

	t.Run("a section is added and then renamed", func(t *testing.T) {
		policy := trafficPolicyWrapper("policy", 1, gatewayRef("gw", "https"))
		f := newPolicyTargetFixture(t, policy)
		const missingSection = `sectionName "https" not found in Gateway default/gw`
		waitFor(t, f, policy, missingSection, "the Gateway has no https listener yet")

		f.gateways.UpdateObject(policyTargetGateway("gw", "http", "https"))
		waitFor(t, f, policy, "", "adding the listener should retract the contribution")

		f.gateways.UpdateObject(policyTargetGateway("gw", "http", "https-renamed"))
		waitFor(t, f, policy, missingSection, "renaming the listener should report the section missing again")
	})

	t.Run("the policy's targetRefs are fixed", func(t *testing.T) {
		policy := trafficPolicyWrapper("policy", 1, gatewayRef("gw-typo", ""), httpRouteRef("route-a", "rule-z"))
		f := newPolicyTargetFixture(t, policy)
		waitFor(t, f, policy,
			`Gateway default/gw-typo not found; sectionName "rule-z" not found in HTTPRoute default/route-a`,
			"both refs are wrong")

		partlyFixed := trafficPolicyWrapper("policy", 2, gatewayRef("gw", ""), httpRouteRef("route-a", "rule-z"))
		f.policies.UpdateObject(partlyFixed)
		waitFor(t, f, partlyFixed, `sectionName "rule-z" not found in HTTPRoute default/route-a`,
			"fixing one ref should leave only the other reported")

		fixed := trafficPolicyWrapper("policy", 3, gatewayRef("gw", ""), httpRouteRef("route-a", "rule-a"))
		f.policies.UpdateObject(fixed)
		waitFor(t, f, fixed, "", "fixing every ref should retract the contribution")
	})
}

// TestPolicyTargetContributionMergesWithGatewayAncestors covers the one-valid-one-typo case
// end to end through the reducer: the Gateway ancestor from translation and the TargetNotFound
// ancestor from targetRef resolution land on the same policy side by side.
func TestPolicyTargetContributionMergesWithGatewayAncestors(t *testing.T) {
	policy := trafficPolicyWrapper("partial", 2, httpRouteRef("route-a", ""), httpRouteRef("route-b-typo", ""))
	key := reporter.PolicyKey{Group: policy.Group, Kind: policy.Kind, Namespace: policy.Namespace, Name: policy.Name}

	gatewayReports := reports.NewPolicyReportMap()
	gatewayAncestor := gwv1.ParentReference{
		Group:     new(gwv1.Group(wellknown.GatewayGVK.Group)),
		Kind:      new(gwv1.Kind(wellknown.GatewayGVK.Kind)),
		Namespace: new(gwv1.Namespace(policyTargetTestNS)),
		Name:      "gw",
	}
	r := reports.NewReporter(&gatewayReports).Policy(key, 2).AncestorRef(gatewayAncestor)
	r.SetCondition(reporter.PolicyCondition{
		Type: string(shared.PolicyConditionAccepted), Status: metav1.ConditionTrue, Reason: string(shared.PolicyReasonValid),
	})
	r.SetAttachmentState(reporter.PolicyAttachmentStateAttached)

	contributions := reports.StatusContributionsFromReportMap(
		reports.StatusSource{Kind: reports.GatewayStatusSource, Name: "default/gw"}, gatewayReports)
	contributions = append(contributions,
		policyTargetStatusContribution(policy, []string{"HTTPRoute default/route-b-typo not found"}))

	reduced := reports.ReduceStatusContributions(contributions)
	status := reports.BuildPolicyStatus(reduced.Policy, key, "test-controller", gwv1.PolicyStatus{})
	require.NotNil(t, status)
	require.Len(t, status.Ancestors, 2)

	var sawGateway, sawSelf bool
	for _, ancestor := range status.Ancestors {
		accepted := meta.FindStatusCondition(ancestor.Conditions, string(shared.PolicyConditionAccepted))
		require.NotNil(t, accepted)
		switch {
		case reports.ParentRefEqual(ancestor.AncestorRef, gatewayAncestor):
			sawGateway = true
			require.Equal(t, metav1.ConditionTrue, accepted.Status, "the valid target keeps its healthy Gateway ancestor")
		case reports.ParentRefEqual(ancestor.AncestorRef, reporter.PolicyStatusSummaryAncestorRef()):
			sawSelf = true
			require.Equal(t, string(shared.PolicyReasonTargetNotFound), accepted.Reason)
		default:
			t.Fatalf("unexpected ancestor %+v", ancestor.AncestorRef)
		}
	}
	require.True(t, sawGateway && sawSelf, "both ancestors should be present")
}

// TestPolicyTargetStatusSummaryIsRemovedFromWrittenStatus runs the policy target producer
// through the status collections and a registered policy writer against a fake API server: the
// StatusSummary entry is written while a target is missing, and a later write removes it once
// the target exists. Each case runs with the standard policy builder and with
// BackendTLSPolicy's builder, which suppresses non-Gateway ancestors once a Gateway ancestor
// exists and must still keep the summary.
func TestPolicyTargetStatusSummaryIsRemovedFromWrittenStatus(t *testing.T) {
	builders := []struct {
		name  string
		build pluginutils.BuildDesiredPolicyStatusFn[*gwv1.BackendTLSPolicy]
	}{
		{name: "standard builder", build: nil},
		{name: "BackendTLSPolicy builder", build: backendtlspolicy.BuildDesiredPolicyStatus},
	}
	for _, b := range builders {
		t.Run(b.name, func(t *testing.T) {
			t.Run("the summary is the only ancestor", func(t *testing.T) {
				runStatusSummaryRemoval(t, b.build, false)
			})
			t.Run("the summary sits beside a Gateway ancestor", func(t *testing.T) {
				runStatusSummaryRemoval(t, b.build, true)
			})
		})
	}
}

func runStatusSummaryRemoval(
	t *testing.T,
	buildDesired pluginutils.BuildDesiredPolicyStatusFn[*gwv1.BackendTLSPolicy],
	withGatewayAncestor bool,
) {
	const controller = "kgateway.dev/kgateway"
	gvk := wellknown.BackendTLSPolicyGVK
	stop := test.NewStop(t)
	krtopts := krtutil.NewKrtOptions(stop, nil)

	c := kube.NewFakeClient()
	cl := kclient.NewFiltered[*gwv1.BackendTLSPolicy](c, kclient.Filter{})
	btps := krt.WrapClient(cl, krtopts.ToOptions("BackendTLSPolicies")...)
	btp := &gwv1.BackendTLSPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "btp", Namespace: policyTargetTestNS, Generation: 1},
		Spec: gwv1.BackendTLSPolicySpec{TargetRefs: []gwv1.LocalPolicyTargetReferenceWithSectionName{{
			LocalPolicyTargetReference: gwv1.LocalPolicyTargetReference{Group: "", Kind: "Service", Name: "svc-late"},
		}}},
	}
	_, err := c.GatewayAPI().GatewayV1().BackendTLSPolicies(policyTargetTestNS).Create(context.Background(), btp, metav1.CreateOptions{})
	require.NoError(t, err)
	c.RunAndWait(stop)

	// The producer, fed the way the proxy syncer feeds it: policy wrappers derived from the
	// informer, resolved against a Service collection that starts without the target.
	services := krt.NewStaticCollection[*corev1.Service](nil, nil, krtopts.ToOptions("Services")...)
	policies := krt.NewCollection(btps, func(_ krt.HandlerContext, p *gwv1.BackendTLSPolicy) *ir.PolicyWrapper {
		return &ir.PolicyWrapper{
			ObjectSource: ir.ObjectSource{Group: gvk.Group, Kind: gvk.Kind, Namespace: p.Namespace, Name: p.Name},
			Policy:       p,
			PolicyIR:     policyTargetTestIR{},
			TargetRefs:   pluginsdkutils.TargetRefsToPolicyRefsWithSectionNameV1(p.Spec.TargetRefs),
		}
	}, krtopts.ToOptions("BackendTLSPolicyWrappers")...)
	targetContributions := policyTargetStatusContributions(policies,
		newPolicyTargetResolvers(&collections.CommonCollections{Services: services}, nil, nil), krtopts)

	// Gateway translation's contribution, standing in for a second targetRef that is routed.
	gatewayAncestor := gwv1.ParentReference{
		Group:     new(gwv1.Group(wellknown.GatewayGVK.Group)),
		Kind:      new(gwv1.Kind(wellknown.GatewayGVK.Kind)),
		Namespace: new(gwv1.Namespace(policyTargetTestNS)),
		Name:      "gw",
	}
	var gatewayContributions []reports.StatusContribution
	if withGatewayAncestor {
		reportMap := reports.NewPolicyReportMap()
		key := reporter.PolicyKey{Group: gvk.Group, Kind: gvk.Kind, Namespace: btp.Namespace, Name: btp.Name}
		r := reports.NewReporter(&reportMap).Policy(key, 1).AncestorRef(gatewayAncestor)
		r.SetCondition(reporter.PolicyCondition{
			Type: string(shared.PolicyConditionAccepted), Status: metav1.ConditionTrue,
			Reason: string(shared.PolicyReasonValid), ObservedGeneration: 1,
		})
		r.SetAttachmentState(reporter.PolicyAttachmentStateAttached)
		gatewayContributions = reports.StatusContributionsFromReportMap(
			reports.StatusSource{Kind: reports.GatewayStatusSource, Name: policyTargetTestNS + "/gw"}, reportMap)
	}
	contributions := krt.JoinCollection([]krt.Collection[reports.StatusContribution]{
		krt.NewStaticCollection(nil, gatewayContributions, krtopts.ToOptions("GatewayContributions")...),
		targetContributions,
	}, krtopts.ToOptions("StatusContributions")...)
	byTarget := utilkrt.UnnamedIndex(contributions, func(c reports.StatusContribution) []reports.StatusKey {
		return []reports.StatusKey{c.Target}
	})

	statusCollections := statussync.NewStatusCollections()
	var writer statussync.ResourceStatusSyncer
	pluginutils.RegisterPolicyStatusWithBuilder(gvk, btps, cl, controller,
		func(p *gwv1.BackendTLSPolicy) gwv1.PolicyStatus { return p.Status },
		func(om metav1.ObjectMeta, st gwv1.PolicyStatus) *gwv1.BackendTLSPolicy {
			return &gwv1.BackendTLSPolicy{ObjectMeta: om, Status: st}
		},
		buildDesired, pluginutils.NoConditionErrorMetric,
	)(sdk.PolicyStatusInputs{
		Collections:           statusCollections,
		StatusContributions:   contributions,
		ContributionsByTarget: byTarget,
		KrtOpts:               krtopts,
		RegisterWriter: func(_ schema.GroupVersionKind, w statussync.ResourceStatusSyncer) {
			writer = w
		},
	})
	require.NotNil(t, writer, "the hook must register a writer")
	require.Eventually(t, statusCollections.HasSynced, 5*time.Second, 10*time.Millisecond,
		"the report reducer must sync")

	res := statussync.Resource{
		GroupVersionKind: gvk,
		NamespacedName:   types.NamespacedName{Namespace: btp.Namespace, Name: btp.Name},
	}
	live := func() []gwv1.PolicyAncestorStatus {
		got, err := c.GatewayAPI().GatewayV1().BackendTLSPolicies(policyTargetTestNS).
			Get(context.Background(), btp.Name, metav1.GetOptions{})
		require.NoError(t, err)
		return got.Status.Ancestors
	}
	hasAncestor := func(ancestors []gwv1.PolicyAncestorStatus, match func(gwv1.ParentReference) bool) bool {
		for _, a := range ancestors {
			if string(a.ControllerName) == controller && match(a.AncestorRef) {
				return true
			}
		}
		return false
	}
	isSummary := reporter.IsPolicyStatusSummaryAncestorRef
	isGateway := func(ref gwv1.ParentReference) bool { return reports.ParentRefEqual(ref, gatewayAncestor) }

	// Each write builds from whatever the reducer holds at that moment, so write until the
	// expected status lands, as the queue would on every report change.
	require.Eventually(t, func() bool {
		writer.ApplyStatus(context.Background(), res)
		return hasAncestor(live(), isSummary)
	}, 5*time.Second, 10*time.Millisecond, "the missing Service should be written on the StatusSummary ancestor")
	if withGatewayAncestor {
		require.True(t, hasAncestor(live(), isGateway), "the Gateway ancestor should be written beside the summary")
	}

	services.UpdateObject(&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "svc-late", Namespace: policyTargetTestNS}})
	require.Eventually(t, func() bool {
		writer.ApplyStatus(context.Background(), res)
		return !hasAncestor(live(), isSummary)
	}, 5*time.Second, 10*time.Millisecond, "creating the Service should remove the written StatusSummary ancestor")

	ancestors := live()
	if withGatewayAncestor {
		require.Len(t, ancestors, 1, "only the Gateway ancestor should remain")
		require.True(t, hasAncestor(ancestors, isGateway), "removing the summary must not touch the Gateway ancestor")
	} else {
		require.Empty(t, ancestors, "the summary was the policy's only ancestor, so nothing of ours should remain")
	}
}
