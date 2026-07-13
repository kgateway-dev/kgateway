package backend

import (
	"testing"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"github.com/onsi/gomega"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
)

func staticBackend(name string, hosts ...kgateway.Host) *kgateway.Backend {
	return &kgateway.Backend{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: kgateway.BackendSpec{
			Static: &kgateway.StaticBackend{Hosts: hosts},
		},
	}
}

func priorityGroupsBackend(groups ...kgateway.PriorityGroup) *kgateway.Backend {
	return &kgateway.Backend{
		ObjectMeta: metav1.ObjectMeta{Name: "pg", Namespace: "default"},
		Spec:       kgateway.BackendSpec{PriorityGroups: groups},
	}
}

func group(names ...string) kgateway.PriorityGroup {
	g := kgateway.PriorityGroup{}
	for _, name := range names {
		g.BackendRefs = append(g.BackendRefs, corev1.LocalObjectReference{Name: name})
	}
	return g
}

func TestBuildPriorityGroupsIr(t *testing.T) {
	g := gomega.NewWithT(t)

	col := krt.NewStaticCollection(nil, []*kgateway.Backend{
		staticBackend("primary", kgateway.Host{Host: "1.2.3.4", Port: gwv1.PortNumber(8080)}),
		staticBackend("failover-a", kgateway.Host{Host: "5.6.7.8", Port: gwv1.PortNumber(3000)}),
		staticBackend("failover-b",
			kgateway.Host{Host: "9.10.11.12", Port: gwv1.PortNumber(8000)},
			kgateway.Host{Host: "9.10.11.13", Port: gwv1.PortNumber(8000)}),
	})

	pgIr, errs := buildPriorityGroupsIr(krt.TestingDummyContext{}, col,
		priorityGroupsBackend(group("primary"), group("failover-a", "failover-b")))

	g.Expect(errs).To(gomega.BeEmpty(), "all refs resolve to static backends")
	g.Expect(pgIr.clusterTypeConfig).To(gomega.BeNil(), "IP-only hosts need no DNS cluster")

	localities := pgIr.loadAssignment.GetEndpoints()
	g.Expect(localities).To(gomega.HaveLen(2), "one locality per priority group")

	g.Expect(localities[0].GetPriority()).To(gomega.Equal(uint32(0)))
	g.Expect(localities[0].GetLbEndpoints()).To(gomega.HaveLen(1))
	g.Expect(localities[0].GetLbEndpoints()[0].GetEndpoint().GetAddress().GetSocketAddress().GetAddress()).
		To(gomega.Equal("1.2.3.4"))

	g.Expect(localities[1].GetPriority()).To(gomega.Equal(uint32(1)), "priority matches group order")
	g.Expect(localities[1].GetLbEndpoints()).To(gomega.HaveLen(3), "group members' endpoints are merged")
	var addrs []string
	for _, ep := range localities[1].GetLbEndpoints() {
		addrs = append(addrs, ep.GetEndpoint().GetAddress().GetSocketAddress().GetAddress())
	}
	g.Expect(addrs).To(gomega.ConsistOf("5.6.7.8", "9.10.11.12", "9.10.11.13"))
}

func TestBuildPriorityGroupsIrDnsHostname(t *testing.T) {
	g := gomega.NewWithT(t)

	col := krt.NewStaticCollection(nil, []*kgateway.Backend{
		staticBackend("primary", kgateway.Host{Host: "1.2.3.4", Port: gwv1.PortNumber(8080)}),
		staticBackend("failover", kgateway.Host{Host: "example.com", Port: gwv1.PortNumber(80)}),
	})

	pgIr, errs := buildPriorityGroupsIr(krt.TestingDummyContext{}, col,
		priorityGroupsBackend(group("primary"), group("failover")))

	g.Expect(errs).To(gomega.BeEmpty())
	g.Expect(pgIr.clusterTypeConfig).NotTo(gomega.BeNil(), "DNS hostname requires the DNS cluster type")
}

func TestBuildPriorityGroupsIrErrors(t *testing.T) {
	g := gomega.NewWithT(t)

	awsBackend := &kgateway.Backend{
		ObjectMeta: metav1.ObjectMeta{Name: "lambda", Namespace: "default"},
		Spec: kgateway.BackendSpec{
			Aws: &kgateway.AwsBackend{},
		},
	}
	col := krt.NewStaticCollection(nil, []*kgateway.Backend{
		staticBackend("primary", kgateway.Host{Host: "1.2.3.4", Port: gwv1.PortNumber(8080)}),
		awsBackend,
	})

	_, errs := buildPriorityGroupsIr(krt.TestingDummyContext{}, col,
		priorityGroupsBackend(group("primary"), group("lambda"), group("missing")))

	g.Expect(errs).To(gomega.HaveLen(2))
	g.Expect(errs[0]).To(gomega.MatchError(
		`priority group 1: backend "lambda" is not a static backend; only static backends are supported in priority groups`))
	g.Expect(errs[1]).To(gomega.MatchError(
		`priority group 2: backend "missing" not found in namespace "default"`))
}

func TestProcessPriorityGroups(t *testing.T) {
	g := gomega.NewWithT(t)

	col := krt.NewStaticCollection(nil, []*kgateway.Backend{
		staticBackend("primary", kgateway.Host{Host: "1.2.3.4", Port: gwv1.PortNumber(8080)}),
	})
	pgIr, errs := buildPriorityGroupsIr(krt.TestingDummyContext{}, col,
		priorityGroupsBackend(group("primary")))
	g.Expect(errs).To(gomega.BeEmpty())

	out := &envoyclusterv3.Cluster{Name: "pg-cluster"}
	processPriorityGroups(pgIr, out)

	g.Expect(out.GetType()).To(gomega.Equal(envoyclusterv3.Cluster_STATIC))
	g.Expect(out.GetLoadAssignment().GetClusterName()).To(gomega.Equal("pg-cluster"))
	g.Expect(pgIr.loadAssignment.GetClusterName()).To(gomega.BeEmpty(), "IR must not be mutated")
}
