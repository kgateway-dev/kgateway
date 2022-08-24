package consul_test

import (
	"context"

	"github.com/golang/mock/gomock"
	"github.com/hashicorp/consul/api"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/solo-io/gloo/projects/gloo/pkg/upstreams/consul"
	mock_consul "github.com/solo-io/gloo/projects/gloo/pkg/upstreams/consul/mocks"
)

var _ = Describe("InternalConsulClient", func() {

	var (
		cancel     context.CancelFunc
		ctrl       *gomock.Controller
		mockClient *mock_consul.MockInternalConsulClient
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(T)
		mockClient = mock_consul.NewMockInternalConsulClient(ctrl)
	})

	AfterEach(func() {
		if cancel != nil {
			cancel()
		}
		ctrl.Finish()
	})

	Describe("Services operation", func() {
		Context("When Filtering By Tags", func() {
			var (
				client InternalConsulClient
			)
			BeforeEach(func() {
				dc := []string{"dc1"}
				serviceTagsAllowlist := []string{"tag-1", "tag-4"}
				services := map[string][]string{
					"svc-1": {"tag-1", "tag-2"}, //filtered in with tag-1
					"svc-2": {"tag-2"},          //filtered out
					"svc-3": {"tag-1", "tag-4"}, //filtered in with both tags
					"svc-4": {"tag-4", "tag-5"}, //filtered in with tag-4
					"svc-5": {},                 //filtered out
				}

				apiQueryMeta := &api.QueryMeta{}
				client, _ = NewFilteredConsulClient(mockClient, dc, serviceTagsAllowlist)
				mockClient.EXPECT().Services(gomock.Any()).Return(services, apiQueryMeta, nil)
			})

			It("returns the filtered upstreams", func() {
				services, _, err := client.Services(&api.QueryOptions{RequireConsistent: false, AllowStale: false, UseCache: true})
				Expect(err).NotTo(HaveOccurred())

				Expect(services).To(HaveLen(3))

				expectedServices := map[string][]string{
					"svc-1": {"tag-1", "tag-2"}, //filtered in with tag-1
					"svc-3": {"tag-1", "tag-4"}, //filtered in with both tags
					"svc-4": {"tag-4", "tag-5"}, //filtered in with tag-4
				}
				Expect(services).To(Equal(expectedServices))
			})
		})
	})

})
