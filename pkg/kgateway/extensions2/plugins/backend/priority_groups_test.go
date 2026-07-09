package backend

import (
	"testing"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoytcp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	envoywellknown "github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/gomega"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
)

func staticBackend(namespace, name, host string) *kgateway.Backend {
	return &kgateway.Backend{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		Spec: kgateway.BackendSpec{
			Static: &kgateway.StaticBackend{
				Hosts: []kgateway.Host{{Host: host, Port: 8080}},
			},
		},
	}
}

func priorityGroupsBackend(namespace, name string, groups ...[]string) *kgateway.Backend {
	be := &kgateway.Backend{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
	}
	for _, refs := range groups {
		group := kgateway.PriorityGroups{}
		for _, ref := range refs {
			group.BackendRefs = append(group.BackendRefs, corev1.LocalObjectReference{Name: ref})
		}
		be.Spec.PriorityGroups = append(be.Spec.PriorityGroups, group)
	}
	return be
}

// internalAddressNames returns the internal listener names the locality's
// endpoints point to.
func internalAddressNames(t *testing.T, locality *envoyendpointv3.LocalityLbEndpoints) []string {
	t.Helper()
	var names []string
	for _, ep := range locality.GetLbEndpoints() {
		name := ep.GetEndpoint().GetAddress().GetEnvoyInternalAddress().GetServerListenerName()
		if name == "" {
			t.Fatalf("endpoint has no internal address: %v", ep)
		}
		names = append(names, name)
	}
	return names
}

// bridgeFilter returns the single filter of the internal listener's filter chain.
func bridgeFilter(t *testing.T, l *envoylistenerv3.Listener) *envoylistenerv3.Filter {
	t.Helper()
	g := NewWithT(t)
	g.Expect(l.GetFilterChains()).To(HaveLen(1))
	g.Expect(l.GetFilterChains()[0].GetFilters()).To(HaveLen(1))
	return l.GetFilterChains()[0].GetFilters()[0]
}

func TestBuildPriorityGroupsIrGroupPriorities(t *testing.T) {
	g := NewWithT(t)

	col := krt.NewStaticCollection(nil, []*kgateway.Backend{
		staticBackend("default", "primary-a", "1.2.3.4"),
		staticBackend("default", "primary-b", "5.6.7.8"),
		staticBackend("default", "failover", "9.9.9.9"),
	})
	be := priorityGroupsBackend("default", "pg",
		[]string{"primary-a", "primary-b"},
		[]string{"failover"},
	)

	pgIr, errs := buildPriorityGroupsIr(krt.TestingDummyContext{}, col, be)

	g.Expect(errs).To(BeEmpty(), "all refs resolve, no errors expected")
	localities := pgIr.loadAssignment.GetEndpoints()
	g.Expect(localities).To(HaveLen(2), "one locality per priority group")

	g.Expect(localities[0].GetPriority()).To(Equal(uint32(0)))
	g.Expect(internalAddressNames(t, localities[0])).To(Equal([]string{
		internalListenerName(backendClusterName("default", "pg"), "primary-a"),
		internalListenerName(backendClusterName("default", "pg"), "primary-b"),
	}), "backends of the same group share the group's priority")

	g.Expect(localities[1].GetPriority()).To(Equal(uint32(1)), "second group is the first failover level")
	g.Expect(internalAddressNames(t, localities[1])).To(HaveLen(1))

	g.Expect(pgIr.internalListeners).To(HaveLen(3), "one internal listener per referenced backend")
	g.Expect(pgIr.needsGcpAuthn).To(BeFalse())
}

func TestBuildPriorityGroupsIrStaticRefUsesTCPBridge(t *testing.T) {
	g := NewWithT(t)

	col := krt.NewStaticCollection(nil, []*kgateway.Backend{
		staticBackend("default", "primary", "1.2.3.4"),
	})
	be := priorityGroupsBackend("default", "pg", []string{"primary"})

	pgIr, errs := buildPriorityGroupsIr(krt.TestingDummyContext{}, col, be)

	g.Expect(errs).To(BeEmpty())
	g.Expect(pgIr.internalListeners).To(HaveLen(1))
	listener := pgIr.internalListeners[0]
	g.Expect(listener.GetInternalListener()).NotTo(BeNil(), "bridge must be an internal listener")

	filter := bridgeFilter(t, listener)
	g.Expect(filter.GetName()).To(Equal(envoywellknown.TCPProxy), "plain backends get a protocol-agnostic tcp_proxy bridge")
	tcpProxy := &envoytcp.TcpProxy{}
	g.Expect(filter.GetTypedConfig().UnmarshalTo(tcpProxy)).To(Succeed())
	g.Expect(tcpProxy.GetCluster()).To(Equal(backendClusterName("default", "primary")))
}

func TestBuildPriorityGroupsIrGcpRefUsesHTTPBridge(t *testing.T) {
	g := NewWithT(t)

	gcp := &kgateway.Backend{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "gcp"},
		Spec: kgateway.BackendSpec{
			Gcp: &kgateway.GcpBackend{Host: "example.googleapis.com"},
		},
	}
	col := krt.NewStaticCollection(nil, []*kgateway.Backend{gcp})
	be := priorityGroupsBackend("default", "pg", []string{"gcp"})

	pgIr, errs := buildPriorityGroupsIr(krt.TestingDummyContext{}, col, be)

	g.Expect(errs).To(BeEmpty())
	g.Expect(pgIr.needsGcpAuthn).To(BeTrue(), "gcp ref needs the shared gcp metadata cluster")
	g.Expect(pgIr.internalListeners).To(HaveLen(1))

	filter := bridgeFilter(t, pgIr.internalListeners[0])
	g.Expect(filter.GetName()).To(Equal(envoywellknown.HTTPConnectionManager))
	hcm := &envoy_hcm.HttpConnectionManager{}
	g.Expect(filter.GetTypedConfig().UnmarshalTo(hcm)).To(Succeed())

	g.Expect(hcm.GetHttpFilters()).To(HaveLen(2), "gcp_authn followed by router")
	g.Expect(hcm.GetHttpFilters()[0].GetName()).To(Equal(gcpAuthnFilterName))
	g.Expect(hcm.GetHttpFilters()[1].GetName()).To(Equal(envoywellknown.Router))

	routes := hcm.GetRouteConfig().GetVirtualHosts()[0].GetRoutes()
	g.Expect(routes).To(HaveLen(1))
	g.Expect(routes[0].GetRoute().GetCluster()).To(Equal(backendClusterName("default", "gcp")))
	g.Expect(routes[0].GetRoute().GetAutoHostRewrite().GetValue()).To(BeTrue(),
		"gcp backends require host rewrite to the gcp service")
}

func TestBuildPriorityGroupsIrDfpRefUsesHTTPBridge(t *testing.T) {
	g := NewWithT(t)

	dfp := &kgateway.Backend{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "dfp"},
		Spec: kgateway.BackendSpec{
			DynamicForwardProxy: &kgateway.DynamicForwardProxyBackend{},
		},
	}
	col := krt.NewStaticCollection(nil, []*kgateway.Backend{dfp})
	be := priorityGroupsBackend("default", "pg", []string{"dfp"})

	pgIr, errs := buildPriorityGroupsIr(krt.TestingDummyContext{}, col, be)

	g.Expect(errs).To(BeEmpty())
	g.Expect(pgIr.needsGcpAuthn).To(BeFalse())

	filter := bridgeFilter(t, pgIr.internalListeners[0])
	g.Expect(filter.GetName()).To(Equal(envoywellknown.HTTPConnectionManager))
	hcm := &envoy_hcm.HttpConnectionManager{}
	g.Expect(filter.GetTypedConfig().UnmarshalTo(hcm)).To(Succeed())
	g.Expect(hcm.GetHttpFilters()).To(HaveLen(2), "dynamic_forward_proxy followed by router")
	g.Expect(hcm.GetHttpFilters()[0].GetName()).To(Equal("envoy.filters.http.dynamic_forward_proxy"))
}

func TestBuildPriorityGroupsIrSharedRefBuildsOneListener(t *testing.T) {
	g := NewWithT(t)

	col := krt.NewStaticCollection(nil, []*kgateway.Backend{
		staticBackend("default", "shared", "1.2.3.4"),
	})
	be := priorityGroupsBackend("default", "pg", []string{"shared"}, []string{"shared"})

	pgIr, errs := buildPriorityGroupsIr(krt.TestingDummyContext{}, col, be)

	g.Expect(errs).To(BeEmpty())
	g.Expect(pgIr.internalListeners).To(HaveLen(1), "same ref in two groups shares one internal listener")
	g.Expect(pgIr.loadAssignment.GetEndpoints()).To(HaveLen(2), "but still appears at both priorities")
}

func TestBuildPriorityGroupsIrErrors(t *testing.T) {
	g := NewWithT(t)

	nested := priorityGroupsBackend("default", "nested", []string{"whatever"})
	col := krt.NewStaticCollection(nil, []*kgateway.Backend{nested})

	// missing ref
	_, errs := buildPriorityGroupsIr(krt.TestingDummyContext{}, col,
		priorityGroupsBackend("default", "pg", []string{"does-not-exist"}))
	g.Expect(errs).To(HaveLen(1))
	g.Expect(errs[0]).To(MatchError(`priority group 0: backend "does-not-exist" not found`))

	// nested priority groups ref
	_, errs = buildPriorityGroupsIr(krt.TestingDummyContext{}, col,
		priorityGroupsBackend("default", "pg", []string{"nested"}))
	g.Expect(errs).To(HaveLen(1))
	g.Expect(errs[0]).To(MatchError(`priority group 0: backend "nested" is itself a priority groups backend; nested priority groups are not supported`))

	// empty group
	_, errs = buildPriorityGroupsIr(krt.TestingDummyContext{}, col,
		priorityGroupsBackend("default", "pg", []string{}))
	g.Expect(errs).To(HaveLen(1))
	g.Expect(errs[0]).To(MatchError("priority group 0: backendRefs must not be empty"))
}

func TestProcessPriorityGroups(t *testing.T) {
	g := NewWithT(t)

	col := krt.NewStaticCollection(nil, []*kgateway.Backend{
		staticBackend("default", "primary", "1.2.3.4"),
		staticBackend("default", "failover", "5.6.7.8"),
	})
	be := priorityGroupsBackend("default", "pg", []string{"primary"}, []string{"failover"})
	pgIr, errs := buildPriorityGroupsIr(krt.TestingDummyContext{}, col, be)
	g.Expect(errs).To(BeEmpty())

	out := &envoyclusterv3.Cluster{Name: "pg-cluster"}
	processPriorityGroups(pgIr, out)

	g.Expect(out.GetType()).To(Equal(envoyclusterv3.Cluster_STATIC))
	g.Expect(out.GetLoadAssignment().GetClusterName()).To(Equal("pg-cluster"))
	g.Expect(out.GetLoadAssignment().GetEndpoints()).To(HaveLen(2))
	g.Expect(out.GetLoadAssignment().GetEndpoints()[1].GetPriority()).To(Equal(uint32(1)))
	g.Expect(pgIr.loadAssignment.GetClusterName()).To(BeEmpty(), "IR must not be mutated by translation")
}

func TestPriorityGroupsIrEquals(t *testing.T) {
	g := NewWithT(t)

	col := krt.NewStaticCollection(nil, []*kgateway.Backend{
		staticBackend("default", "primary", "1.2.3.4"),
		staticBackend("default", "failover", "5.6.7.8"),
	})
	build := func(groups ...[]string) *PriorityGroupsIr {
		pgIr, errs := buildPriorityGroupsIr(krt.TestingDummyContext{}, col,
			priorityGroupsBackend("default", "pg", groups...))
		g.Expect(errs).To(BeEmpty())
		return pgIr
	}

	base := build([]string{"primary"}, []string{"failover"})

	g.Expect(base.Equals(build([]string{"primary"}, []string{"failover"}))).To(BeTrue())
	g.Expect(base.Equals(build([]string{"failover"}, []string{"primary"}))).To(BeFalse(),
		"group order is failover priority, reorder must not compare equal")
	g.Expect(base.Equals(build([]string{"primary", "failover"}))).To(BeFalse())
	g.Expect(base.Equals(nil)).To(BeFalse())

	var nilIr *PriorityGroupsIr
	g.Expect(nilIr.Equals(nil)).To(BeTrue())
	g.Expect(nilIr.Equals(base)).To(BeFalse())

	withGcp := build([]string{"primary"}, []string{"failover"})
	withGcp.needsGcpAuthn = true
	g.Expect(base.Equals(withGcp)).To(BeFalse())
}

func TestTranslateFuncPriorityGroups(t *testing.T) {
	g := NewWithT(t)

	col := krt.NewStaticCollection(nil, []*kgateway.Backend{
		staticBackend("default", "primary", "1.2.3.4"),
		staticBackend("default", "failover", "5.6.7.8"),
	})
	be := priorityGroupsBackend("default", "pg", []string{"primary"}, []string{"failover"})

	beIr := buildTranslateFunc(col, nil, false)(krt.TestingDummyContext{}, be)

	g.Expect(beIr.errors).To(BeEmpty())
	g.Expect(beIr.priorityGroupsIr).NotTo(BeNil())
	g.Expect(beIr.priorityGroupsIr.loadAssignment.GetEndpoints()).To(HaveLen(2))
	g.Expect(beIr.staticIr).To(BeNil(), "priority groups backend must not translate as static")
}
