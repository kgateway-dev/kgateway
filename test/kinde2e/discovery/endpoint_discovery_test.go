package discovery_test

import (
	"fmt"
	"io/ioutil"
	"regexp"

	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/k8s-utils/testutils/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

var _ = Describe("Endpoint discovery works", func() {

	var (
		gatewayProxyPodName string
		configDumpPath      = "http://localhost:19000/config_dump"
		clustersPath        = "http://localhost:19000/clusters"
		kubeCtx             string
		prevConfigDumpLen   int

		findPetstoreClusterEndpoints = func() int {
			clusters := kube.CurlWithEphemeralPod(ctx, ioutil.Discard, kubeCtx, defaults.GlooSystem, gatewayProxyPodName, clustersPath)
			petstoreClusterEndpoints := regexp.MustCompile("\ndefault-petstore-8080_gloo-system::[0-9.]+:8080::")
			matches := petstoreClusterEndpoints.FindAllStringIndex(clusters, -1)
			fmt.Println(len(matches))
			return len(matches)
		}
		findConfigDumpHttp2Count = func() int {
			configDump := kube.CurlWithEphemeralPod(ctx, ioutil.Discard, kubeCtx, defaults.GlooSystem, gatewayProxyPodName, configDumpPath, "-s")
			http2Configs := regexp.MustCompile("http2_protocol_options")
			matches := http2Configs.FindAllStringIndex(configDump, -1)
			return len(matches)
		}

		upstreamChangesPickedUp = func() bool {
			currConfigDumpLen := findConfigDumpHttp2Count()
			if prevConfigDumpLen != currConfigDumpLen {
				prevConfigDumpLen = currConfigDumpLen
				return true
			}
			return false
		}

		checkClusterEndpoints = func() {
			Eventually(func() bool {
				if upstreamChangesPickedUp() {
					By("check that endpoints were discovered")
					Expect(findPetstoreClusterEndpoints()).NotTo(Equal(0))
					return true
				}
				return false
			}, "2m", "1s").Should(BeTrue())
		}
	)

	BeforeEach(func() {
		// Find gateway-proxy pod name
		gatewayProxyPodName = kube.FindPodNameByLabel(cfg, ctx, defaults.GlooSystem, "gloo=gateway-proxy")

		// Disable discovery so that we can modify upstreams without interruption
		kube.DisableContainer(ctx, GinkgoWriter, kubeCtx, defaults.GlooSystem, "discovery", "discovery")
	})

	AfterEach(func() {
		kube.EnableContainer(ctx, GinkgoWriter, kubeCtx, defaults.GlooSystem, "discovery")
	})

	It("can modify upstreams repeatedly", func() {
		// Initialize a way to track the envoy config dump in order to tell when it has changed, and when the
		// new upstream changes have been picked up.
		Eventually(func() int {
			prevConfigDumpLen = findConfigDumpHttp2Count()
			return prevConfigDumpLen
		}, "30s", "1s").ShouldNot(Equal(0))

		// We should consistently be able to modify upstreams
		Consistently(func() error {
			// Modify the upstream
			us, err := upstreamClient.Read(defaults.GlooSystem, "default-petstore-8080", clients.ReadOpts{Ctx: ctx})
			us.UseHttp2 = &wrappers.BoolValue{Value: !us.UseHttp2.GetValue()}
			_, err = upstreamClient.Write(us, clients.WriteOpts{Ctx: ctx, OverwriteExisting: true})
			Expect(err).NotTo(HaveOccurred())

			// Check that the changed was picked up and the new config has the correct endpoints
			checkClusterEndpoints()

			return nil
		}, "5m", "5s").Should(BeNil())
	})
})
