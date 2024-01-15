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

	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
)

var _ = FDescribe("EDS", func() {
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
			// enable REST EDS
			kube2e.UpdateRestEdsSetting(ctx, true, testHelper.InstallNamespace)
		})

		AfterEach(func() {
			// reset REST EDS to default
			kube2e.UpdateRestEdsSetting(ctx, false, testHelper.InstallNamespace)
		})

		// This test is inspired by the issue here: https://github.com/solo-io/gloo/issues/8968
		// There were some versions of Gloo Edge 1.15.x which depended on versions of envoy-gloo
		// which did not have REST config subscription enabled, and so gateway-proxy logs would
		// contain warnings about not finding a registered config subscription factory implementation
		// for REST EDS. This test validates that we have not regressed to that state.
		FIt("should not warn when REST EDS is configured", func() {
			Consistently(func(g Gomega) {
				// Get envoy logs from gateway-proxy deployment
				logsCmd := exec.Command("kubectl", "logs", "-n", testHelper.InstallNamespace,
					"deployment/gateway-proxy")
				logsOut, err := logsCmd.Output()
				g.Expect(err).NotTo(HaveOccurred())

				// ensure that the logs do not contain any presence of the text:
				// Didn't find a registered config subscription factory implementation for name: 'envoy.config_subscription.rest'
				g.Expect(string(logsOut)).NotTo(ContainSubstring("Didn't find a registered config subscription factory implementation for name: 'envoy.config_subscription.rest'"))
			}, "10s", "1s").Should(Succeed())
		})

		FIt("should not reject config updates when REST EDS is configured", func() {
			Eventually(func(g Gomega) {
				// validate that the envoy config dump contains the testrunner service
				cfgDump := GetEnvoyCfgDump(testHelper)
				g.Expect(cfgDump).To(ContainSubstring("testrunner"))
			}, "10s", "1s").Should(Succeed())

			err := helpers.PatchResource(
				ctx,
				testRunnerVs.GetMetadata().Ref(),
				func(resource resources.Resource) resources.Resource {
					// Give the resource a new domain
					// This is essentially a no-op, but it will be reflected
					// in the envoy config dump
					resource.(*gatewayv1.VirtualService).VirtualHost.Domains = append(resource.(*gatewayv1.VirtualService).VirtualHost.Domains, "new-domain")

					return resource
				},
				resourceClientset.VirtualServiceClient().BaseClient())
			Expect(err).NotTo(HaveOccurred())

			Eventually(func(g Gomega) {
				// validate that the envoy config dump contains the new domain
				cfgDump := GetEnvoyCfgDump(testHelper)
				g.Expect(cfgDump).To(ContainSubstring("new-domain"))
			}, "10s", "1s").Should(Succeed())

		})
	})
})

func GetEnvoyCfgDump(testHelper *helper.SoloTestHelper) string {
	cfg, err := gateway.GetEnvoyAdminData(context.TODO(), "gateway-proxy", testHelper.InstallNamespace, "/config_dump", 5*time.Second)
	Expect(err).NotTo(HaveOccurred())
	return cfg
}
