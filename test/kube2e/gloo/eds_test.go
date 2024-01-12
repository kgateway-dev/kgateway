package gloo_test

import (
	"context"
	"os/exec"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/gateway"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/kube2e"
	"github.com/solo-io/k8s-utils/testutils/helper"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

var _ = Describe("EDS", func() {
	var (
		testRunnerDestination *gloov1.Destination
		testRunnerVs          *gatewayv1.VirtualService

		glooResources *gloosnapshot.ApiSnapshot
	)

	BeforeEach(func() {
		// Create a VirtualService routing directly to the testrunner kubernetes service
		testRunnerDestination = &gloov1.Destination{
			DestinationType: &gloov1.Destination_Kube{
				Kube: &gloov1.KubernetesServiceDestination{
					Ref: &core.ResourceRef{
						Namespace: testHelper.InstallNamespace,
						Name:      helper.TestrunnerName,
					},
					Port: uint32(helper.TestRunnerPort),
				},
			},
		}
		testRunnerVs = helpers.NewVirtualServiceBuilder().
			WithName(helper.TestrunnerName).
			WithNamespace(testHelper.InstallNamespace).
			WithLabel(kube2e.UniqueTestResourceLabel, uuid.New().String()).
			WithDomain(helper.TestrunnerName).
			WithRoutePrefixMatcher(helper.TestrunnerName, "/").
			WithRouteActionToSingleDestination(helper.TestrunnerName, testRunnerDestination).
			Build()

		// The set of resources that these tests will generate
		glooResources = &gloosnapshot.ApiSnapshot{
			VirtualServices: gatewayv1.VirtualServiceList{
				// many tests route to the TestRunner Service so it makes sense to just
				// always create it
				// the other benefit is this ensures that all tests start with a valid Proxy CR
				testRunnerVs,
			},
		}
	})

	JustBeforeEach(func() {
		err := snapshotWriter.WriteSnapshot(glooResources, clients.WriteOpts{
			Ctx:               ctx,
			OverwriteExisting: false,
		})
		Expect(err).NotTo(HaveOccurred())
	})

	JustAfterEach(func() {
		err := snapshotWriter.DeleteSnapshot(glooResources, clients.DeleteOpts{
			Ctx:            ctx,
			IgnoreNotExist: true,
		})
		Expect(err).NotTo(HaveOccurred())
	})

	Context("Rest EDS", func() {
		BeforeEach(func() {
			kube2e.UpdateRestEdsSetting(ctx, true, testHelper.InstallNamespace)
		})

		It("should update Envoy config after modifying resources", func() {
			// confirm that the resources written during setup are in the Envoy config
			Eventually(func(g Gomega) {
				originalConfigDump := GetEnvoyCfgDump(testHelper)
				g.Expect(originalConfigDump).To(ContainSubstring("testrunner"))
			}, "10s", "1s").Should(Succeed())

			// Get envoy logs from gateway-proxy deployment
			logsCmd := exec.Command("kubectl", "logs", "-n", testHelper.InstallNamespace,
				"deployment/gateway-proxy")
			logsOut, err := logsCmd.Output()
			Expect(err).NotTo(HaveOccurred())

			// ensure that the logs do not contain any presence of the text:
			// Didn't find a registered config subscription factory implementation for name: 'envoy.config_subscription.rest'
			Expect(string(logsOut)).NotTo(ContainSubstring("Didn't find a registered config subscription factory implementation for name: 'envoy.config_subscription.rest'"))
		})
	})
})

func GetEnvoyCfgDump(testHelper *helper.SoloTestHelper) string {
	cfg, err := gateway.GetEnvoyAdminData(context.TODO(), "gateway-proxy", testHelper.InstallNamespace, "/config_dump", 5*time.Second)
	Expect(err).NotTo(HaveOccurred())
	return cfg
}

func GetEnvoyLogs(testHelper *helper.SoloTestHelper) string {
	cfg, err := gateway.GetEnvoyAdminData(context.TODO(), "gateway-proxy", testHelper.InstallNamespace, "/config_dump", 5*time.Second)
	Expect(err).NotTo(HaveOccurred())
	return cfg
}
