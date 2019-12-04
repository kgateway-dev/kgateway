package translator

import (
	"context"
	"time"

	alpha1 "knative.dev/serving/pkg/client/clientset/versioned/typed/networking/v1alpha1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/clusteringress/api/external/knative"
	v1alpha1 "github.com/solo-io/gloo/projects/clusteringress/pkg/api/external/knative"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	v1alpha12 "knative.dev/serving/pkg/apis/networking/v1alpha1"
)

var _ = Describe("TranslatorSyncer", func() {
	It("propagates successful proxy status to the clusteringresses it was created from", func() {
		proxyAddress := "proxy-address"
		namespace := "write-namespace"
		proxyClient, _ := v1.NewProxyClient(&factory.MemoryResourceClientFactory{Cache: memory.NewInMemoryResourceCache()})
		clusterIngress := &v1alpha1.ClusterIngress{ClusterIngress: knative.ClusterIngress{
			ObjectMeta: v12.ObjectMeta{Generation: 1},
		}}
		knativeClient := &mockIngressesGetter{
			ciClient: &mockCiClient{ci: toKube(clusterIngress)}}

		syncer := NewSyncer(proxyAddress, namespace, proxyClient, knativeClient, make(chan error)).(*translatorSyncer)
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

		err := syncer.propagateProxyStatus(context.TODO(), proxy, v1alpha1.ClusterIngressList{clusterIngress})
		Expect(err).NotTo(HaveOccurred())

		var ci *v1alpha12.Ingress
		ci, err = knativeClient.ciClient.Get(clusterIngress.Name, v12.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		Expect(ci.Status.IsReady()).To(BeTrue())
	})
})

type mockIngressesGetter struct{ ciClient alpha1.IngressInterface }

func (m *mockIngressesGetter) Ingresses(namespace string) alpha1.IngressInterface {
	return m.ciClient
}

func toKube(ci *v1alpha1.ClusterIngress) *v1alpha12.Ingress {
	kubeCi := v1alpha12.Ingress(ci.ClusterIngress)
	return &kubeCi
}

type mockCiClient struct{ ci *v1alpha12.Ingress }

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
