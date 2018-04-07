package e2e

import (
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/solo-io/gloo/test/helpers"
	kubev1 "k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var _ = Describe("Kubernetes Ingress Controller", func() {
	const host1 = "host1.example.com"
	const host2 = "host2.example.com"
	const helloService = "helloservice"
	const helloService2 = "helloservice-2"
	const servicePort = 8080
	const path1a = "/helloservicea"
	const path1b = "/helloserviceb"
	const path2a = "/helloservice2a"
	const path2b = "/helloservice2b"
	Context("creating a kubernetes ingress with default backend", func() {
		var ingress *v1beta1.Ingress
		BeforeEach(func() {
			ingress = &v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ig-default-backend",
					Namespace: namespace,
				},
				Spec: v1beta1.IngressSpec{
					Backend: &v1beta1.IngressBackend{
						ServiceName: helloService,
						ServicePort: intstr.IntOrString{IntVal: servicePort},
					},
				},
			}
			_, err := kube.ExtensionsV1beta1().Ingresses(namespace).Create(ingress)
			Must(err)
		})
		AfterEach(func() {
			kube.ExtensionsV1beta1().Ingresses(namespace).Delete(ingress.Name, nil)
		})
		It("should configure envoy with a 200 OK route (backed by helloservice)", func() {
			curlEventuallyShouldRespond(curlOpts{path: "/"}, "< HTTP/1.1 200", time.Minute*5)
		})
		It("should update the ingress with the lb status of the service", func() {
			envoySvc, err := kube.CoreV1().Services(namespace).Get("test-ingress", metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			// just to test, set the envoySvcLB status to soemthing
			envoySvc.Status.LoadBalancer.Ingress = append(envoySvc.Status.LoadBalancer.Ingress, kubev1.LoadBalancerIngress{
				IP: "10.10.10.10",
			})
			envoySvc, err = kube.CoreV1().Services(namespace).Update(envoySvc)
			Expect(err).NotTo(HaveOccurred())
			expectedLbStatus := envoySvc.Status.LoadBalancer.Ingress
			Eventually(func() ([]kubev1.LoadBalancerIngress, error) {
				updated, err := kube.ExtensionsV1beta1().Ingresses(namespace).Get(ingress.Name, metav1.GetOptions{})
				if err != nil {
					return nil, err
				}
				return updated.Status.LoadBalancer.Ingress, nil
			}, "1m","5s").Should(Equal(expectedLbStatus))
		})
	})
	Context("creating a kubernetes ingress with multiple rules", func() {
		var ingress *v1beta1.Ingress
		BeforeEach(func() {
			ingress = &v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ig-default-backend",
					Namespace: namespace,
				},
				Spec: v1beta1.IngressSpec{
					Rules: []v1beta1.IngressRule{
						{
							Host: host1,
							IngressRuleValue: v1beta1.IngressRuleValue{
								HTTP: &v1beta1.HTTPIngressRuleValue{
									Paths: []v1beta1.HTTPIngressPath{
										{
											Path: path1a,
											Backend: v1beta1.IngressBackend{
												ServiceName: helloService,
												ServicePort: intstr.FromInt(servicePort),
											},
										},
										{
											Path: path1b,
											Backend: v1beta1.IngressBackend{
												ServiceName: helloService,
												ServicePort: intstr.FromInt(servicePort),
											},
										},
									},
								},
							},
						},
						{
							Host: host2,
							IngressRuleValue: v1beta1.IngressRuleValue{
								HTTP: &v1beta1.HTTPIngressRuleValue{
									Paths: []v1beta1.HTTPIngressPath{
										{
											Path: path2a,
											Backend: v1beta1.IngressBackend{
												ServiceName: helloService2,
												ServicePort: intstr.FromInt(servicePort),
											},
										},
										{
											Path: path2b,
											Backend: v1beta1.IngressBackend{
												ServiceName: helloService2,
												ServicePort: intstr.FromInt(servicePort),
											},
										},
									},
								},
							},
						},
					},
				},
			}
			_, err := kube.ExtensionsV1beta1().Ingresses(namespace).Create(ingress)
			Must(err)
		})
		AfterEach(func() {
			kube.ExtensionsV1beta1().Ingresses(namespace).Delete(ingress.Name, nil)
		})
		It("should configure envoy with two routes for host1", func() {
			curlEventuallyShouldRespond(curlOpts{path: path1a, host: host1}, "expected-reply-1", time.Minute*5)
			curlEventuallyShouldRespond(curlOpts{path: path1b, host: host1}, "expected-reply-1", time.Minute*5)
		})
		It("should configure envoy with two routes for host2", func() {
			curlEventuallyShouldRespond(curlOpts{path: path2a, host: host2}, "expected-reply-2", time.Minute*5)
			curlEventuallyShouldRespond(curlOpts{path: path2b, host: host2}, "expected-reply-2", time.Minute*5)
		})
	})
	//TODO: Context("creating a kubernetes ingress with tls config", func() {
})
