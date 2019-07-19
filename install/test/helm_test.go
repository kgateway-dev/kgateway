package test

import (
	. "github.com/onsi/ginkgo"
	"github.com/solo-io/gloo/projects/gateway/pkg/translator"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	. "github.com/solo-io/go-utils/manifesttestutils"
)

var _ = Describe("Helm Test", func() {

	Describe("gateway proxy extra annotations and crds", func() {
		labels := map[string]string{
			"gloo": translator.GatewayProxyName,
			"app":  "gloo",
		}
		selector := map[string]string{
			"gloo": translator.GatewayProxyName,
		}

		It("has a namespace", func() {
			rb := ResourceBuilder{
				Namespace: namespace,
				Name:      translator.GatewayProxyName,
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
			svc.Annotations = map[string]string{"test": "test"}
			testManifest.ExpectService(svc)
		})
	})
})
