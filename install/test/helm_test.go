package test

import (
	. "github.com/onsi/ginkgo"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	. "github.com/solo-io/go-utils/manifesttestutils"
)

var _ = Describe("Helm Test", func() {

	Describe("gateway proxy extra annotations and crds", func() {
		labels := map[string]string{
			"gloo": "gateway-proxy",
			"app":  "gloo",
		}
		selector := map[string]string{
			"gloo": "gateway-proxy",
		}

		It("has a namespace", func() {
			rb := ResourceBuilder{
				Namespace: namespace,
				Name:      "gateway-proxy",
				Labels:    labels,
				Service: ServiceSpec{
					Ports: []PortSpec{
						{
							Name: "http",
							Port: 80,
						},
						{
							Name: "https",
							Port: 443,
						},
					},
				},
			}
			svc := rb.GetService()
			svc.Spec.Selector = selector
			svc.Spec.Type = v1.ServiceTypeLoadBalancer
			svc.Spec.Ports[0].TargetPort = intstr.FromInt(8080)
			svc.Spec.Ports[1].TargetPort = intstr.FromInt(8443)
			testManifest.ExpectService(svc)
		})
	})
})
