package gloomtls_test

import (
	"time"

	"github.com/google/uuid"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/test/helpers"

	"github.com/solo-io/gloo/test/kube2e"

	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/test/kube2e/helper"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

var _ = Describe("Kube2e: mTLS", func() {

	var (
		glooResources *gloosnapshot.ApiSnapshot
	)

	BeforeEach(func() {
		// Create a VirtualService routing directly to the testserver kubernetes service
		testServerDestination := &gloov1.Destination{
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
		testServerVs := helpers.NewVirtualServiceBuilder().
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

	It("can route request to upstream", func() {
		testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
			Protocol:          "http",
			Path:              "/",
			Method:            "GET",
			Host:              helper.TestServerName,
			Service:           defaults.GatewayProxyName,
			Port:              gatewayPort,
			ConnectionTimeout: 1,
		}, kube2e.TestServerHttpResponse(), 0, time.Second*60, time.Second*1)
	})

})
