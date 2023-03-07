package kubernetes

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	skkube "github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var _ = Describe("Conversions", func() {

	It("correctly builds service-derived upstream name", func() {
		name := fakeUpstreamName("my-service", "ns", 8080)
		Expect(name).To(Equal(upstreamNamePrefix + "ns-my-service-8080"))
	})

	It("correctly detects upstreams derived from Kubernetes services", func() {
		Expect(IsKubeUpstream(upstreamNamePrefix + "my-service-8080")).To(BeTrue())
		Expect(IsKubeUpstream("my-" + upstreamNamePrefix + "service-8080")).To(BeFalse())
		Expect(IsKubeUpstream("svc:my-service-8080")).To(BeFalse())
	})

	It("correctly converts a list of services to upstreams", func() {
		svc := skkube.NewService("ns-1", "svc-1")
		svc.Spec = corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "port-1",
					Port:       8080,
					TargetPort: intstr.FromInt(80),
				},
				{
					Name:       "port-2",
					Port:       8081,
					TargetPort: intstr.FromInt(8081),
				},
			},
		}
		usList := KubeServicesToUpstreams(context.TODO(), skkube.ServiceList{svc})
		usList.Sort()
		Expect(usList).To(HaveLen(2))
		Expect(usList[0].Metadata.Name).To(Equal(upstreamNamePrefix + "ns-1-svc-1-8080"))
		Expect(usList[0].Metadata.Namespace).To(Equal("ns-1"))
		Expect(usList[0].GetKube()).NotTo(BeNil())
		Expect(usList[0].GetKube().ServiceName).To(Equal("svc-1"))
		Expect(usList[0].GetKube().ServiceNamespace).To(Equal("ns-1"))
		Expect(usList[0].GetKube().ServicePort).To(BeEquivalentTo(8080))

		Expect(usList[1].Metadata.Name).To(Equal(upstreamNamePrefix + "ns-1-svc-1-8081"))
		Expect(usList[1].Metadata.Namespace).To(Equal("ns-1"))
		Expect(usList[1].GetKube()).NotTo(BeNil())
		Expect(usList[1].GetKube().ServiceName).To(Equal("svc-1"))
		Expect(usList[1].GetKube().ServiceNamespace).To(Equal("ns-1"))
		Expect(usList[1].GetKube().ServicePort).To(BeEquivalentTo(8081))
	})
})
