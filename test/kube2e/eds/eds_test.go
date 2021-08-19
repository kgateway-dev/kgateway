package eds_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"regexp"

	"github.com/solo-io/gloo/test/kube2e"

	kubeutils "github.com/solo-io/k8s-utils/testutils/kube"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"k8s.io/client-go/rest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	utils "github.com/solo-io/k8s-utils/kubeutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

var _ = Describe("endpoint discovery (EDS) works", func() {

	var (
		ctx    context.Context
		cancel context.CancelFunc
		err    error
		cfg    *rest.Config

		upstreamClient       gloov1.UpstreamClient
		virtualServiceClient gatewayv1.VirtualServiceClient
	)

	const (
		configDumpPath = "http://localhost:19000/config_dump"
		clustersPath   = "http://localhost:19000/clusters"
		kubeCtx        = ""
	)

	var (
		gatewayProxyPodName string
		prevConfigDumpLen   int

		findPetstoreClusterEndpoints = func() int {
			clusters := kubeutils.CurlWithEphemeralPod(ctx, ioutil.Discard, kubeCtx, defaults.GlooSystem, gatewayProxyPodName, clustersPath)
			petstoreClusterEndpoints := regexp.MustCompile("\ndefault-petstore-8080_gloo-system::[0-9.]+:8080::")
			matches := petstoreClusterEndpoints.FindAllStringIndex(clusters, -1)
			fmt.Println(fmt.Sprintf("Number of cluster stats for petstore (i.e., checking for endpoints) on clusters page: %d", len(matches)))
			return len(matches)
		}
		findConfigDumpHttp2Count = func() int {
			configDump := kubeutils.CurlWithEphemeralPod(ctx, ioutil.Discard, kubeCtx, defaults.GlooSystem, gatewayProxyPodName, configDumpPath, "-s")
			http2Configs := regexp.MustCompile("http2_protocol_options")
			matches := http2Configs.FindAllStringIndex(configDump, -1)
			fmt.Println(fmt.Sprintf("Number of http2_protocol_options (i.e., clusters) on config dump page: %d", len(matches)))
			return len(matches)
		}

		upstreamChangesPickedUp = func() bool {
			currConfigDumpLen := findConfigDumpHttp2Count()
			if prevConfigDumpLen != currConfigDumpLen {
				prevConfigDumpLen = currConfigDumpLen
				fmt.Println("Upstream changes picked up! (cluster exists)")
				return true
			}
			return false
		}

		checkClusterEndpoints = func() {
			Eventually(func() bool {
				if upstreamChangesPickedUp() {
					numEndpoints := findPetstoreClusterEndpoints()
					By("check that endpoints were discovered")
					Expect(numEndpoints).Should(BeNumerically(">", 0), "petstore endpoints should exist")
					fmt.Println("Endpoints exist for cluster!")
					return true
				}
				return false
			}, "45s", "1s").Should(BeTrue(), "upstream changes were never picked up!")
		}
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		cfg, err = utils.GetConfig("", "")
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
			return vs.GetStatus().GetState() == core.Status_Accepted
		}, "15s", "0.5s").Should(BeTrue())

		// Find gateway-proxy pod name
		gatewayProxyPodName = kubeutils.FindPodNameByLabel(cfg, ctx, defaults.GlooSystem, "gloo=gateway-proxy")

		// Disable discovery so that we can modify upstreams without interruption
		kubeutils.DisableContainer(ctx, GinkgoWriter, kubeCtx, defaults.GlooSystem, "discovery", "discovery")
	})

	AfterEach(func() {
		kubeutils.EnableContainer(ctx, GinkgoWriter, kubeCtx, defaults.GlooSystem, "discovery")
		cancel()
	})

	endpointsDontLagTest := func() {
		// Initialize a way to track the envoy config dump in order to tell when it has changed, and when the
		// new upstream changes have been picked up.
		Eventually(func() int {
			prevConfigDumpLen = findConfigDumpHttp2Count()
			return prevConfigDumpLen
		}, "30s", "1s").ShouldNot(Equal(0), "cluster count should be nonzero")

		// We should consistently be able to modify upstreams
		Consistently(func() error {
			// Modify the upstream
			us, err := upstreamClient.Read(defaults.GlooSystem, "default-petstore-8080", clients.ReadOpts{Ctx: ctx})
			Expect(err).NotTo(HaveOccurred())
			us.UseHttp2 = &wrappers.BoolValue{Value: !us.UseHttp2.GetValue()}
			_, err = upstreamClient.Write(us, clients.WriteOpts{Ctx: ctx, OverwriteExisting: true})
			Expect(err).NotTo(HaveOccurred())

			// Check that the changed was picked up and the new config has the correct endpoints
			checkClusterEndpoints()

			return nil
		}, "3m", "5s").Should(BeNil()) // 3 min to be safe, usually repros in ~40s when running locally without REST EDS
	}

	Context("rest EDS", func() {

		BeforeEach(func() {
			kube2e.UpdateRestEdsSetting(ctx, true, defaults.GlooSystem)
		})

		It("can modify upstreams repeatedly, and endpoints don't lag via EDS", endpointsDontLagTest)
	})

	Context("gRPC", func() {

		BeforeEach(func() {
			kube2e.UpdateRestEdsSetting(ctx, false, defaults.GlooSystem)
		})

		It("can modify upstreams repeatedly, and endpoints don't lag via EDS", func() {
			failures := InterceptGomegaFailures(endpointsDontLagTest)
			Expect(failures).To(BeEmpty())
		})
	})

})
