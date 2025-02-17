//go:build ignore

package gloo_test

import (
	"os/exec"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	gatewayv1 "github.com/kgateway-dev/kgateway/v2/internal/gateway/pkg/api/v1"
	gloov1 "github.com/kgateway-dev/kgateway/v2/internal/gloo/pkg/api/v1"
	"github.com/kgateway-dev/kgateway/v2/internal/gloo/pkg/api/v1/gloosnapshot"
	"github.com/kgateway-dev/kgateway/v2/test/helpers"
	"github.com/kgateway-dev/kgateway/v2/test/kube2e"
	"github.com/kgateway-dev/kgateway/v2/test/kube2e/helper"

	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

var _ = Describe("EDS", func() {
	var (
		testServerDestination *gloov1.Destination
		testServerVs          *gatewayv1.VirtualService

		glooResources *gloosnapshot.ApiSnapshot
	)

	BeforeEach(func() {
		// Create a VirtualService routing directly to the testServer kubernetes service
		testServerDestination = &gloov1.Destination{
			DestinationType: &gloov1.Destination_Kube{
				Kube: &gloov1.KubernetesServiceDestination{
					Ref: &core.ResourceRef{
						Namespace: testHelper.InstallNamespace,
						Name:      helper.TestServerName,
					},
					Port: uint32(helper.TestServerPort),
				},
			},
		}
		testServerVs = helpers.NewVirtualServiceBuilder().
			WithName(helper.TestServerName).
			WithNamespace(testHelper.InstallNamespace).
			WithLabel(kube2e.UniqueTestResourceLabel, uuid.New().String()).
			WithDomain(helper.TestServerName).
			WithRoutePrefixMatcher(helper.TestServerName, "/").
			WithRouteActionToSingleDestination(helper.TestServerName, testServerDestination).
			Build()

		// The set of resources that these tests will generate
		glooResources = &gloosnapshot.ApiSnapshot{
			VirtualServices: gatewayv1.VirtualServiceList{
				// many tests route to the TestServer Service so it makes sense to just
				// always create it
				// the other benefit is this ensures that all tests start with a valid Proxy CR
				testServerVs,
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

	Context("Rest EDS", Ordered, func() {
		BeforeAll(func() {
			// enable REST EDS
			kube2e.UpdateRestEdsSetting(ctx, true, testHelper.InstallNamespace)
		})

		AfterAll(func() {
			// reset REST EDS to default
			kube2e.UpdateRestEdsSetting(ctx, false, testHelper.InstallNamespace)
		})

		// This test is inspired by the issue here: https://github.com/kgateway-dev/kgateway/issues/8968
		// There were some versions of Gloo Edge 1.15.x which depended on versions of envoy-gloo
		// which did not have REST config subscription enabled, and so gateway-proxy logs would
		// contain warnings about not finding a registered config subscription factory implementation
		// for REST EDS. This test validates that we have not regressed to that state.
		It("should not warn when REST EDS is configured", func() {
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
	})
})
