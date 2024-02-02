package selectionutils_test

import (
	"context"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	errors "github.com/rotisserie/eris"
	mock_listers "github.com/solo-io/gloo/pkg/listers/mocks"
	"github.com/solo-io/gloo/pkg/utils/selectionutils"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	mock_gateway "github.com/solo-io/gloo/projects/gateway/pkg/mocks/mock_v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	sk_errors "github.com/solo-io/solo-kit/pkg/errors"
	"github.com/solo-io/solo-kit/test/matchers"
)

var (
	mockCtrl     *gomock.Controller
	vsClient     *mock_gateway.MockVirtualServiceClient
	nsLister     *mock_listers.MockNamespaceLister
	selector     selectionutils.VirtualServiceSelector
	podNamespace = "pod-ns"
	otherNs      = "ns"
	metadata     = &core.Metadata{
		Namespace: otherNs,
		Name:      "name",
	}
	ref     = metadata.Ref()
	testErr = errors.Errorf("test-err")
)

var _ = Describe("SelectorTest", func() {
	getDefault := func(namespace, name string) *gatewayv1.VirtualService {
		return &gatewayv1.VirtualService{
			Metadata: &core.Metadata{
				Namespace: namespace,
				Name:      name,
			},
			VirtualHost: &gatewayv1.VirtualHost{
				Domains: []string{"*"},
			},
		}
	}

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		vsClient = mock_gateway.NewMockVirtualServiceClient(mockCtrl)
		nsLister = mock_listers.NewMockNamespaceLister(mockCtrl)
		selector = selectionutils.NewVirtualServiceSelector(vsClient, nsLister, podNamespace)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Describe("VirtualServiceSelector", func() {
		getVirtualService := func(meta *core.Metadata, domain string) *gatewayv1.VirtualService {
			return &gatewayv1.VirtualService{
				Metadata:    meta,
				VirtualHost: &gatewayv1.VirtualHost{Domains: []string{domain}},
			}
		}

		Context("ref is not nil or empty", func() {
			It("returns an existing virtual service if one is found for the given ref", func() {
				expected := getVirtualService(metadata, "")

				vsClient.EXPECT().
					Read(ref.Namespace, ref.Name, clients.ReadOpts{Ctx: context.Background()}).
					Return(expected, nil)

				actual, err := selector.SelectOrBuildVirtualService(context.Background(), ref)
				Expect(err).NotTo(HaveOccurred())
				Expect(actual).To(matchers.MatchProto(expected))
			})

			It("creates a new default vs with the provided name and namespace if not found", func() {
				expected := getDefault(ref.GetNamespace(), ref.GetName())

				vsClient.EXPECT().
					Read(ref.GetNamespace(), ref.GetName(), clients.ReadOpts{Ctx: context.Background()}).
					Return(nil, sk_errors.NewNotExistErr(ref.GetNamespace(), ref.GetName(), testErr))

				actual, err := selector.SelectOrBuildVirtualService(context.Background(), ref)
				Expect(err).NotTo(HaveOccurred())
				Expect(actual).To(matchers.MatchProto(expected))
			})

			It("creates a new default vs with the provided name and default namespace", func() {
				nameRef := &core.ResourceRef{Name: "just-name"}
				expected := getDefault(podNamespace, nameRef.Name)

				actual, err := selector.SelectOrBuildVirtualService(context.Background(), nameRef)
				Expect(err).NotTo(HaveOccurred())
				Expect(actual).To(matchers.MatchProto(expected))
			})

			It("errors when the client errors on read", func() {
				vsClient.EXPECT().
					Read(ref.Namespace, ref.Name, clients.ReadOpts{Ctx: context.Background()}).
					Return(nil, testErr)

				_, err := selector.SelectOrBuildVirtualService(context.Background(), ref)
				Expect(err).To(HaveOccurred())
				Expect(err).To(Equal(testErr))
			})
		})

		Context("ref is nil", func() {
			It("returns the first virtual service with domain * if one exists", func() {
				expected := getVirtualService(metadata, "*")
				list := []*gatewayv1.VirtualService{
					getVirtualService(&core.Metadata{Namespace: otherNs}, ""),
					expected,
				}

				nsLister.EXPECT().
					List(context.Background()).
					Return([]string{podNamespace, otherNs}, nil)
				vsClient.EXPECT().
					List(podNamespace, clients.ListOpts{Ctx: context.Background()}).
					Return(nil, nil)
				vsClient.EXPECT().
					List(otherNs, clients.ListOpts{Ctx: context.Background()}).
					Return(list, nil)

				actual, err := selector.SelectOrBuildVirtualService(context.Background(), nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(actual).To(matchers.MatchProto(expected))
			})

			It("creates a new default vs when no vs with domain * is found", func() {
				expected := getDefault(podNamespace, "default")

				nsLister.EXPECT().
					List(context.Background()).
					Return([]string{podNamespace}, nil)
				vsClient.EXPECT().
					List(podNamespace, clients.ListOpts{Ctx: context.Background()}).
					Return(nil, nil)

				actual, err := selector.SelectOrBuildVirtualService(context.Background(), nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(actual).To(matchers.MatchProto(expected))
			})

			It("errors when the namespaceLister errors on list", func() {
				nsLister.EXPECT().
					List(context.Background()).
					Return(nil, testErr)

				_, err := selector.SelectOrBuildVirtualService(context.Background(), nil)
				Expect(err).To(HaveOccurred())
				Expect(err).To(Equal(testErr))
			})

			It("errors when the client errors on list", func() {
				nsLister.EXPECT().
					List(context.Background()).
					Return([]string{otherNs}, nil)
				vsClient.EXPECT().
					List(otherNs, clients.ListOpts{Ctx: context.Background()}).
					Return(nil, testErr)

				_, err := selector.SelectOrBuildVirtualService(context.Background(), nil)
				Expect(err).To(HaveOccurred())
				Expect(err).To(Equal(testErr))
			})
		})
	})
})
