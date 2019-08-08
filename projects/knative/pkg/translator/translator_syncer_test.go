package translator

import (
	"context"
	"time"

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
	v1alpha12 "knative.dev/serving/pkg/apis/networking/v1alpha1"
	v1alpha13 "knative.dev/serving/pkg/client/clientset/versioned/typed/networking/v1alpha1"
)

var _ = Describe("TranslatorSyncer", func() {
	It("propagates successful proxy status to the ingresses it was created from", func() {
		proxyAddressExternal := "proxy-external-address"
		proxyAddressInternal := "proxy-internal-address"
		namespace := "write-namespace"
		proxyClient, _ := v1.NewProxyClient(&factory.MemoryResourceClientFactory{Cache: memory.NewInMemoryResourceCache()})
		ingress := &v1alpha1.Ingress{Ingress: knative.Ingress{ObjectMeta: v12.ObjectMeta{Generation: 1}}}
		knativeClient := &mockCiClient{ci: toKube(ingress)}

		syncer := NewSyncer(proxyAddressExternal, proxyAddressInternal, namespace, proxyClient, knativeClient, make(chan error)).(*translatorSyncer)
		proxy := &v1.Proxy{Metadata: core.Metadata{Name: "hi", Namespace: "howareyou"}}
		proxy, _ = proxyClient.Write(proxy, clients.WriteOpts{})

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

		ci, err := knativeClient.Get(ingress.Name, v12.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		Expect(ci.Status.IsReady()).To(BeTrue())
	})
})

func toKube(ci *v1alpha1.Ingress) *v1alpha12.Ingress {
	kubeCi := v1alpha12.Ingress(ci.Ingress)
	return &kubeCi
}

type mockCiClient struct{ ci *v1alpha12.Ingress }

func (c *mockCiClient) Ingresses(namespace string) v1alpha13.IngressInterface {
	return c
}

func (c *mockCiClient) UpdateStatus(ci *v1alpha12.Ingress) (*v1alpha12.Ingress, error) {
	c.ci.Status = ci.Status
	return ci, nil
}

func (*mockCiClient) Create(*v1alpha12.Ingress) (*v1alpha12.Ingress, error) {
	panic("implement me")
}

func (*mockCiClient) Update(*v1alpha12.Ingress) (*v1alpha12.Ingress, error) {
	panic("implement me")
}

func (*mockCiClient) Delete(name string, options *v12.DeleteOptions) error {
	panic("implement me")
}

func (*mockCiClient) DeleteCollection(options *v12.DeleteOptions, listOptions v12.ListOptions) error {
	panic("implement me")
}

func (c *mockCiClient) Get(name string, options v12.GetOptions) (*v1alpha12.Ingress, error) {
	return c.ci, nil
}

func (*mockCiClient) List(opts v12.ListOptions) (*v1alpha12.IngressList, error) {
	panic("implement me")
}

func (*mockCiClient) Watch(opts v12.ListOptions) (watch.Interface, error) {
	panic("implement me")
}

func (*mockCiClient) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha12.Ingress, err error) {
	panic("implement me")
}
