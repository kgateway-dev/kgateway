package discovery_test

import (
	"context"
	"time"

	"github.com/solo-io/gloo/pkg/utils/statusutils"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	discmocks "github.com/solo-io/gloo/projects/gloo/pkg/discovery/mocks"
	. "github.com/solo-io/gloo/test/gomega"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	. "github.com/solo-io/gloo/projects/gloo/pkg/discovery"
)

var _ = Describe("Discovery", func() {

	var (
		ctx    context.Context
		cancel context.CancelFunc

		statusClient resources.StatusClient
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		statusClient = statusutils.GetStatusClientForNamespace("default")
	})

	AfterEach(func() {
		cancel()
	})

	It("preserves cached EDS results across calls to StartEDS", func() {
		// in this test we will run 2 plugins in EDS.
		// we will write endpoints for 2 plugins
		// then we will restart eds, and send another update
		// we want to see that we didnt lose the endpoints
		// for the eds plugin that didn't get updated after the restart
		ns := ""
		ctl := gomock.NewController(GinkgoT())
		endpointClient, _ := v1.NewEndpointClient(ctx, &factory.MemoryResourceClientFactory{
			Cache: memory.NewInMemoryResourceCache(),
		})

		wo := clients.WatchOpts{
			Ctx: context.TODO(),
		}

		makeEdsPlugin := func() (DiscoveryPlugin, chan v1.EndpointList) {
			discoveryPlugin := discmocks.NewMockDiscoveryPlugin(ctl)
			edsChan := make(chan v1.EndpointList, 1)
			discoveryPlugin.EXPECT().WatchEndpoints(ns, nil, wo).Return(edsChan, nil, nil).Times(2) // 1 time for each StartEds call
			return discoveryPlugin, edsChan
		}

		redEdsPlugin, redChan := makeEdsPlugin()
		blueEdsPlugin, blueChan := makeEdsPlugin()

		eds := NewEndpointDiscovery(nil, ns,
			endpointClient,
			statusClient,
			[]DiscoveryPlugin{
				redEdsPlugin,
				blueEdsPlugin,
			},
		)

		// start eds once
		// we don't care about the upstreams here,
		// we just want to verify endpoints are preserved across calls to StartEds
		_, err := eds.StartEds(nil, wo)
		Expect(err).NotTo(HaveOccurred())

		ep := func(suffix string) *v1.Endpoint {
			return &v1.Endpoint{
				Metadata: &core.Metadata{
					Name: "endpoint-" + suffix,
				},
			}
		}

		// preserve the blue ep
		blueEp := ep("blue-1")

		// emit some endpoints, expect they are propagated (sanity check)
		select {
		case redChan <- v1.EndpointList{ep("red-1")}:
		default:
			Fail("expected redChan to be empty")
		}
		select {
		case blueChan <- v1.EndpointList{blueEp}:
		default:
			Fail("expected blueChan to be empty")
		}

		// expect the eds to make it to us
		listEps := func() ([]string, error) {
			list, err := endpointClient.List(ns, clients.ListOpts{})
			if err != nil {
				return nil, err
			}
			var epNames []string
			list.Each(func(element *v1.Endpoint) {
				epNames = append(epNames, element.Metadata.Name)
			})
			return epNames, nil
		}

		Eventually(listEps, time.Minute*2).Should(HaveLen(2))

		// start eds again
		// verify that discovery never removes endpoints from the client
		_, err = eds.StartEds(nil, wo)
		Expect(err).NotTo(HaveOccurred())

		// update eds by sending another eds list

		select {
		case redChan <- v1.EndpointList{ep("red-2"), ep("red-3")}: // change to 2 red endpoints
		default:
			Fail("expected redChan to be empty")
		}

		// expect 1 blue always
		Consistently(listEps, DefaultConsistentlyDuration, DefaultConsistentlyPollingInterval).Should(ContainElement("endpoint-blue-1"))
	})
})
