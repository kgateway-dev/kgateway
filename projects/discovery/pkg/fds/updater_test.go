package fds_test

import (
	"context"
	"fmt"
	"net/url"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/solo-io/gloo/projects/discovery/pkg/fds"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"

	plugins "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options"
	kubernetes_plugins_gloo_solo_io "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/kubernetes"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	core_solo_io "github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

type testUpstreamWriterClient struct{}

func (t *testUpstreamWriterClient) Write(resource *v1.Upstream, opts clients.WriteOpts) (*v1.Upstream, error) {
	return resource, nil
}

func (t *testUpstreamWriterClient) Read(namespace, name string, opts clients.ReadOpts) (*v1.Upstream, error) {
	return nil, fmt.Errorf("test - no upstream")
}

type functionsCalled struct {
	isUpstreamFunctional bool
	detectUpstreamType   bool
	detectFunctions      bool
}

type testDiscovery struct {
	isUpstreamFunctionalResult bool
	serviceSpec                *plugins.ServiceSpec
	detectUpstreamTypeError    error
	detectFunctionsError       error
	mutate                     UpstreamMutator

	functionsCalled atomic.Value
}

func (t *testDiscovery) getFunctionsCalled() functionsCalled {
	return t.functionsCalled.Load().(functionsCalled)
}

func (t *testDiscovery) setFunctionsCalled(f functionsCalled) {
	t.functionsCalled.Store(f)
}

func (t *testDiscovery) NewFunctionDiscovery(u *v1.Upstream) UpstreamFunctionDiscovery {
	return t
}

func (t *testDiscovery) IsFunctional() bool {
	fc := t.getFunctionsCalled()
	fc.isUpstreamFunctional = true
	t.setFunctionsCalled(fc)
	return t.isUpstreamFunctionalResult
}
func (t *testDiscovery) DetectType(ctx context.Context, url *url.URL) (*plugins.ServiceSpec, error) {
	fc := t.getFunctionsCalled()
	fc.detectUpstreamType = true
	t.setFunctionsCalled(fc)
	return t.serviceSpec, t.detectUpstreamTypeError
}

func (t *testDiscovery) DetectFunctions(ctx context.Context, url *url.URL, dependencies func() Dependencies, out func(UpstreamMutator) error) error {
	fc := t.getFunctionsCalled()
	fc.detectFunctions = true
	t.setFunctionsCalled(fc)
	if t.mutate != nil {
		out(t.mutate)
	}

	return t.detectFunctionsError
}

type fakeResolver struct {
	resolveUrl   *url.URL
	resolveError error
}

func (t *fakeResolver) Resolve(u *v1.Upstream) (*url.URL, error) {
	return t.resolveUrl, t.resolveError
}

var _ = Describe("Updater", func() {

	var (
		ctx                  context.Context
		cancel               context.CancelFunc
		resolver             *fakeResolver
		testDisc             *testDiscovery
		updater              *Updater
		up                   *v1.Upstream
		upstreamWriterClient *testUpstreamWriterClient
	)

	BeforeEach(func() {
		upstreamWriterClient = &testUpstreamWriterClient{}
		ctx, cancel = context.WithCancel(context.Background())
		u, err := url.Parse("http://solo.io")
		Expect(err).NotTo(HaveOccurred())
		resolver = &fakeResolver{
			resolveUrl: u,
		}
		testDisc = &testDiscovery{}
		testDisc.functionsCalled.Store(functionsCalled{})
		updater = NewUpdater(ctx, resolver, upstreamWriterClient, 0, []FunctionDiscoveryFactory{testDisc})
		up = &v1.Upstream{
			Metadata: core_solo_io.Metadata{
				Namespace: "ns",
				Name:      "up",
			},
			UpstreamType: &v1.Upstream_Kube{
				Kube: &kubernetes_plugins_gloo_solo_io.UpstreamSpec{},
			},
		}
	})

	AfterEach(func() {
		cancel()
	})

	It("should detect functions when upstream type is known", func() {
		testDisc.isUpstreamFunctionalResult = true
		updater.UpstreamAdded(up)
		time.Sleep(time.Second / 10)
		fc := testDisc.getFunctionsCalled()
		Expect(fc.isUpstreamFunctional).To(BeTrue())
		Expect(fc.detectUpstreamType).To(BeFalse())
		Expect(fc.detectFunctions).To(BeTrue())
	})

	It("should detect functions when upstream type is known", func() {
		testDisc.isUpstreamFunctionalResult = false
		testDisc.serviceSpec = &plugins.ServiceSpec{}
		updater.UpstreamAdded(up)
		time.Sleep(time.Second / 10)
		fc := testDisc.getFunctionsCalled()
		Expect(fc.isUpstreamFunctional).To(BeTrue())
		Expect(fc.detectUpstreamType).To(BeTrue())
		Expect(fc.detectFunctions).To(BeTrue())
	})

})
