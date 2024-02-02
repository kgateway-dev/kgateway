package consul_test

import (
	"context"

	"github.com/golang/mock/gomock"
	"github.com/hashicorp/consul/api"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/solo-io/gloo/projects/gloo/pkg/upstreams/consul"
	mock_consul "github.com/solo-io/gloo/projects/gloo/pkg/upstreams/consul/mocks"
)

var _ = Describe("ClientWrapper", func() {

	var (
		cancel     context.CancelFunc
		ctrl       *gomock.Controller
		mockClient *mock_consul.MockClientWrapper
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(T)
		mockClient = mock_consul.NewMockClientWrapper(ctrl)
	})

	AfterEach(func() {
		if cancel != nil {
			cancel()
		}
		ctrl.Finish()
	})

	Describe("When calling consul.Catalog.Services operation", func() {

		var (
			services map[string][]string
		)

		BeforeEach(func() {
			services = map[string][]string{
				"svc-1": {"tag-1", "tag-2"}, //filtered in with tag-1
				"svc-2": {"tag-2"},          //filtered out
				"svc-3": {"tag-1", "tag-4"}, //filtered in with both tags
				"svc-4": {"tag-4", "tag-5"}, //filtered in with tag-4
				"svc-5": {},                 //filtered out
			}
		})

		Context("When Filtering response By Tags", func() {
			var (
				client ClientWrapper
			)
			BeforeEach(func() {
				dc := []string{"dc1"}
				serviceTagsAllowlist := []string{"tag-1", "tag-4"}
				apiQueryMeta := &api.QueryMeta{}
				client, _ = NewFilteredConsulClient(mockClient, dc, serviceTagsAllowlist)
				mockClient.EXPECT().Services(gomock.Any()).Return(services, apiQueryMeta, nil)
			})

			It("returns the filtered services", func() {
				responseServices, _, err := client.Services(&api.QueryOptions{RequireConsistent: false, AllowStale: false, UseCache: true})
				Expect(err).NotTo(HaveOccurred())

				Expect(responseServices).To(HaveLen(3))

				expectedServices := map[string][]string{
					"svc-1": {"tag-1", "tag-2"}, //filtered in with tag-1
					"svc-3": {"tag-1", "tag-4"}, //filtered in with both tags
					"svc-4": {"tag-4", "tag-5"}, //filtered in with tag-4
				}
				Expect(responseServices).To(Equal(expectedServices))
			})
		})
		Context("When not Filtering response By Tags", func() {
			var (
				client ClientWrapper
			)
			BeforeEach(func() {
				dc := []string{"dc1"}
				apiQueryMeta := &api.QueryMeta{}
				client, _ = NewFilteredConsulClient(mockClient, dc, []string{})
				mockClient.EXPECT().Services(gomock.Any()).Return(services, apiQueryMeta, nil)
			})

			It("returns all services", func() {
				responseServices, _, err := client.Services(&api.QueryOptions{RequireConsistent: false, AllowStale: false, UseCache: true})
				Expect(err).NotTo(HaveOccurred())

				Expect(responseServices).To(HaveLen(5))
				Expect(responseServices).To(Equal(services))
			})
		})
	})

})
