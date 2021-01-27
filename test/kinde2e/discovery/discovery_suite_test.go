package discovery_test

import (
	"context"
	"os"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/solo-io/gloo/pkg/cliutil"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/k8s-utils/kubeutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	skhelpers "github.com/solo-io/solo-kit/test/helpers"
	"k8s.io/client-go/rest"

	"github.com/solo-io/go-utils/log"
)

var (
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	cfg    *rest.Config

	installNamespace     = "gloo-system"
	upstreamClient       gloov1.UpstreamClient
	virtualServiceClient gatewayv1.VirtualServiceClient
)

func TestDiscovery(t *testing.T) {
	if os.Getenv("KUBE2E_TESTS") != "discovery" {
		log.Warnf("This test is disabled. " +
			"To enable, set KUBE2E_TESTS to 'discovery' in your env.")
		return
	}
	skhelpers.RegisterCommonFailHandlers()
	skhelpers.SetupLog()
	_ = os.Remove(cliutil.GetLogsPath())
	skhelpers.RegisterPreFailHandler(helpers.KubeDumpOnFail(GinkgoWriter, installNamespace))
	RunSpecs(t, "Discovery Suite")
}

var _ = BeforeSuite(func() {
	ctx, cancel = context.WithCancel(context.Background())
	cfg, err = kubeutils.GetConfig("", "")
	Expect(err).NotTo(HaveOccurred())

	cache := kube.NewKubeCache(ctx)
	upstreamClientFactory := &factory.KubeResourceClientFactory{
		Crd:         gloov1.UpstreamCrd,
		Cfg:         cfg,
		SharedCache: cache,
	}
	virtualServiceClientFactory := &factory.KubeResourceClientFactory{
		Crd:         gatewayv1.VirtualServiceCrd,
		Cfg:         cfg,
		SharedCache: cache,
	}
	upstreamClient, err = gloov1.NewUpstreamClient(ctx, upstreamClientFactory)
	Expect(err).NotTo(HaveOccurred())
	virtualServiceClient, err = gatewayv1.NewVirtualServiceClient(ctx, virtualServiceClientFactory)
	Expect(err).NotTo(HaveOccurred())

	// Wait for Virtual Service to be accepted
	Eventually(func() bool {
		vs, err := virtualServiceClient.Read(defaults.GlooSystem, "default", clients.ReadOpts{})
		Expect(err).NotTo(HaveOccurred())
		return vs.Status.GetState() == core.Status_Accepted
	}, "15s", "0.5s").Should(BeTrue())
})

var _ = AfterSuite(func() {
	cancel()
})
