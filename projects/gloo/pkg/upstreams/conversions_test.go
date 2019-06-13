package upstreams

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	skkube "github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var _ = Describe("Conversions", func() {

	It("correctly builds service-derived upstream name", func() {
		name := buildFakeUpstreamName("my-service", "ns", 8080)
		Expect(name).To(Equal(ServiceUpstreamNamePrefix + "ns-my-service-8080"))
	})

	It("correctly detects service-derived upstreams", func() {
		Expect(isRealUpstream(ServiceUpstreamNamePrefix + "my-service-8080")).To(BeFalse())
		Expect(isRealUpstream("my-" + ServiceUpstreamNamePrefix + "service-8080")).To(BeTrue())
		Expect(isRealUpstream("my-service-8080")).To(BeTrue())
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
		usList := servicesToUpstreams(skkube.ServiceList{svc})
		usList.Sort()
		Expect(usList).To(HaveLen(2))
		Expect(usList[0].Metadata.Name).To(Equal("svc:ns-1-svc-1-8080"))
		Expect(usList[0].Metadata.Namespace).To(Equal("ns-1"))
		Expect(usList[0].UpstreamSpec.GetKube()).NotTo(BeNil())
		Expect(usList[0].UpstreamSpec.GetKube().ServiceName).To(Equal("svc-1"))
		Expect(usList[0].UpstreamSpec.GetKube().ServiceNamespace).To(Equal("ns-1"))
		Expect(usList[0].UpstreamSpec.GetKube().ServicePort).To(BeEquivalentTo(8080))

		Expect(usList[1].Metadata.Name).To(Equal("svc:ns-1-svc-1-8081"))
		Expect(usList[1].Metadata.Namespace).To(Equal("ns-1"))
		Expect(usList[1].UpstreamSpec.GetKube()).NotTo(BeNil())
		Expect(usList[1].UpstreamSpec.GetKube().ServiceName).To(Equal("svc-1"))
		Expect(usList[1].UpstreamSpec.GetKube().ServiceNamespace).To(Equal("ns-1"))
		Expect(usList[1].UpstreamSpec.GetKube().ServicePort).To(BeEquivalentTo(8081))
	})
})
