package translator

import (
	"context"
	"time"

	knativev1 "github.com/solo-io/gloo/projects/knative/pkg/api/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/knative/api/external/knative"
	v1alpha1 "github.com/solo-io/gloo/projects/knative/pkg/api/external/knative"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	knativev1alpha1 "knative.dev/serving/pkg/apis/networking/v1alpha1"
	v1alpha13 "knative.dev/serving/pkg/client/clientset/versioned/typed/networking/v1alpha1"
)

var _ = Describe("TranslatorSyncer", func() {
	var (
		proxyAddressExternal = "proxy-external-address"
		proxyAddressInternal = "proxy-internal-address"
		namespace            = "write-namespace"
		proxyClient          v1.ProxyClient
		knativeClient        v1alpha13.IngressesGetter
		ingress              *v1alpha1.Ingress
		proxy                *v1.Proxy
	)
	BeforeEach(func() {
		proxyClient, _ = v1.NewProxyClient(&factory.MemoryResourceClientFactory{Cache: memory.NewInMemoryResourceCache()})
		ingress = &v1alpha1.Ingress{Ingress: knative.Ingress{ObjectMeta: v12.ObjectMeta{Generation: 1},
			Spec: knativev1alpha1.IngressSpec{
				Rules: []knativev1alpha1.IngressRule{{
					Hosts: []string{"*"},
					HTTP: &knativev1alpha1.HTTPIngressRuleValue{
						Paths: []knativev1alpha1.HTTPIngressPath{
							{
								Path: "/hay",
								Splits: []knativev1alpha1.IngressBackendSplit{
									{
										IngressBackend: knativev1alpha1.IngressBackend{
											ServiceName:      "a",
											ServiceNamespace: "b",
											ServicePort: intstr.IntOrString{
												Type:   intstr.Int,
												IntVal: 1234,
											},
										},
									},
								},
							},
						}},
				},
				}},
		}}
		knativeClient = &mockCiClient{ci: toKube(ingress)}
		proxy = &v1.Proxy{Metadata: core.Metadata{Name: "hi", Namespace: "howareyou"}}
		proxy, _ = proxyClient.Write(proxy, clients.WriteOpts{})
	})
	It("only processes annotated proxies when requireIngressClass is set to true successful proxy status to the ingresses it was created from", func() {
		syncer := NewSyncer(proxyAddressExternal, proxyAddressInternal, namespace, proxyClient, knativeClient, make(chan error), true).(*translatorSyncer)

		// expect ingress without class to be ignored
		err := syncer.Sync(context.TODO(), &knativev1.TranslatorSnapshot{
			Ingresses: []*v1alpha1.Ingress{ingress},
		})
		Expect(err).NotTo(HaveOccurred())

		// expect the ingress to be ignored
		// we should have no listeners
		proxies, err := proxyClient.List(namespace, clients.ListOpts{})
		Expect(err).NotTo(HaveOccurred())
		Expect(proxies).To(HaveLen(2))
		Expect(proxies[0].Listeners).To(HaveLen(0))

		ingress.Annotations = map[string]string{
			ingressClassAnnotation: glooIngressClass,
		}

		err = syncer.Sync(context.TODO(), &knativev1.TranslatorSnapshot{
			Ingresses: []*v1alpha1.Ingress{ingress},
		})
		Expect(err).NotTo(HaveOccurred())

		// expect a proxy to be created
		proxies, err = proxyClient.List(namespace, clients.ListOpts{})
		Expect(err).NotTo(HaveOccurred())
		Expect(proxies).To(HaveLen(2))
		Expect(proxies[0].Listeners).To(HaveLen(1))
		Expect(proxies[0].Listeners[0].GetHttpListener()).NotTo(BeNil())
		Expect(proxies[0].Listeners[0].GetHttpListener().VirtualHosts).To(HaveLen(1))
	})

	It("propagates successful proxy status to the ingresses it was created from", func() {
		// requireIngressClass = true
		syncer := NewSyncer(proxyAddressExternal, proxyAddressInternal, namespace, proxyClient, knativeClient, make(chan error), false).(*translatorSyncer)

		go func() {
			defer GinkgoRecover()
			// update status after a 1s sleep
			time.Sleep(time.Second / 5)
			proxy.Status.State = core.Status_Accepted
			_, err := proxyClient.Write(proxy, clients.WriteOpts{OverwriteExisting: true})
			Expect(err).NotTo(HaveOccurred())
		}()

		err := syncer.propagateProxyStatus(context.TODO(), proxy, v1alpha1.IngressList{ingress})
		Expect(err).NotTo(HaveOccurred())

		ci, err := knativeClient.Ingresses(ingress.Namespace).Get(ingress.Name, v12.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		Expect(ci.Status.IsReady()).To(BeTrue())
	})
})

func toKube(ci *v1alpha1.Ingress) *knativev1alpha1.Ingress {
	kubeCi := knativev1alpha1.Ingress(ci.Ingress)
	return &kubeCi
}

type mockCiClient struct{ ci *knativev1alpha1.Ingress }

func (c *mockCiClient) Ingresses(namespace string) v1alpha13.IngressInterface {
	return c
}

func (c *mockCiClient) UpdateStatus(ci *knativev1alpha1.Ingress) (*knativev1alpha1.Ingress, error) {
	c.ci.Status = ci.Status
	return ci, nil
}

func (*mockCiClient) Create(*knativev1alpha1.Ingress) (*knativev1alpha1.Ingress, error) {
	panic("implement me")
}

func (*mockCiClient) Update(*knativev1alpha1.Ingress) (*knativev1alpha1.Ingress, error) {
	panic("implement me")
}

func (*mockCiClient) Delete(name string, options *v12.DeleteOptions) error {
	panic("implement me")
}

func (*mockCiClient) DeleteCollection(options *v12.DeleteOptions, listOptions v12.ListOptions) error {
	panic("implement me")
}

func (c *mockCiClient) Get(name string, options v12.GetOptions) (*knativev1alpha1.Ingress, error) {
	return c.ci, nil
}

func (*mockCiClient) List(opts v12.ListOptions) (*knativev1alpha1.IngressList, error) {
	panic("implement me")
}

func (*mockCiClient) Watch(opts v12.ListOptions) (watch.Interface, error) {
	panic("implement me")
}

func (*mockCiClient) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *knativev1alpha1.Ingress, err error) {
	panic("implement me")
}
