package gloo_test

import (
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/test/kube2e"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/test/helpers"

	"github.com/solo-io/gloo/test/kube2e/helper"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

// This tests the gloo/test/helpers/snapshot_writer.go functionality in a Kubernetes cluster

var _ = Describe("SnapshotWriter Test", func() {

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

	Context("DeleteSnapshot", func() {

		It("continues when deleting a resource that does not exist", func() {
			// We add a resource that does not exist to our API Snapshot
			// In the JustAfterEach, we should attempt to delete the resource, and even though
			// it does not exist, we should continue the delete operation and succeed
			glooResources.Upstreams = append(glooResources.Upstreams, &gloov1.Upstream{
				Metadata: &core.Metadata{
					Name:      "resource-does-not-exist",
					Namespace: namespace,
				},
			})
		})
	})

})
