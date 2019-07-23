package conversion_test

import (
	"context"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewayv2 "github.com/solo-io/gloo/projects/gateway/pkg/api/v2"
	"github.com/solo-io/gloo/projects/gateway/pkg/conversion"
	"github.com/solo-io/gloo/projects/gateway/pkg/mocks/mock_conversion"
	"github.com/solo-io/gloo/projects/gateway/pkg/mocks/mock_v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/mocks/mock_v2"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var (
	resourceConverter conversion.ResourceConverter
	mockCtrl          *gomock.Controller
	v1GatewayClient   *mock_v1.MockGatewayClient
	v2GatewayClient   *mock_v2.MockGatewayClient
	gatewayConverter  *mock_conversion.MockGatewayConverter
	namespace         = "test-ns"
	testErr           = errors.Errorf("test-err")
)

var _ = Describe("ResourceConverter", func() {
	Describe("ConvertAll", func() {

		getV1Gateway := func(name string) *gatewayv1.Gateway {
			return &gatewayv1.Gateway{Metadata: core.Metadata{Namespace: namespace, Name: name}}
		}

		getV2Gateway := func(name string) *gatewayv2.Gateway {
			return &gatewayv2.Gateway{Metadata: core.Metadata{Namespace: namespace, Name: name}}
		}

		BeforeEach(func() {
			mockCtrl = gomock.NewController(GinkgoT())
			v1GatewayClient = mock_v1.NewMockGatewayClient(mockCtrl)
			v2GatewayClient = mock_v2.NewMockGatewayClient(mockCtrl)
			gatewayConverter = mock_conversion.NewMockGatewayConverter(mockCtrl)
			resourceConverter = conversion.NewResourceConverter(context.TODO(), namespace, v1GatewayClient, v2GatewayClient, gatewayConverter)
		})

		AfterEach(func() {
			mockCtrl.Finish()
		})

		It("works", func() {
			fooV1 := getV1Gateway("foo")
			barV1 := getV1Gateway("bar")
			fooV2 := getV2Gateway("foo")
			barV2 := getV2Gateway("bar")
			v1Gateways := []*gatewayv1.Gateway{fooV1, barV1}

			v1GatewayClient.EXPECT().
				List(namespace, clients.ListOpts{Ctx: context.TODO()}).
				Return(v1Gateways, nil)
			gatewayConverter.EXPECT().
				FromV1ToV2(fooV1).
				Return(fooV2)
			gatewayConverter.EXPECT().
				FromV1ToV2(barV1).
				Return(barV2)
			v2GatewayClient.EXPECT().
				Write(fooV2, clients.WriteOpts{Ctx: context.TODO(), OverwriteExisting: true}).
				Return(fooV2, nil)
			v2GatewayClient.EXPECT().
				Write(barV2, clients.WriteOpts{Ctx: context.TODO(), OverwriteExisting: true}).
				Return(barV2, nil)

			err := resourceConverter.ConvertAll()
			Expect(err).NotTo(HaveOccurred())
		})

		It("errors if gatewayv1 gateway client errors on list", func() {
			v1GatewayClient.EXPECT().
				List(namespace, clients.ListOpts{Ctx: context.TODO()}).
				Return(nil, testErr)

			err := resourceConverter.ConvertAll()
			Expect(err).To(HaveOccurred())
			expectedErr := conversion.FailedToListGatewayResourcesError(err, "gatewayv1", namespace)
			Expect(expectedErr.Error()).To(ContainSubstring(err.Error()))
		})

		It("errors if v2 gateway client errors on write", func() {
			fooV1 := getV1Gateway("foo")
			fooV2 := getV2Gateway("foo")
			v1Gateways := []*gatewayv1.Gateway{fooV1}

			v1GatewayClient.EXPECT().
				List(namespace, clients.ListOpts{Ctx: context.TODO()}).
				Return(v1Gateways, nil)
			gatewayConverter.EXPECT().
				FromV1ToV2(fooV1).
				Return(fooV2)
			v2GatewayClient.EXPECT().
				Write(fooV2, clients.WriteOpts{Ctx: context.TODO(), OverwriteExisting: true}).
				Return(nil, testErr)

			err := resourceConverter.ConvertAll()
			Expect(err).To(HaveOccurred())
			expectedErr := conversion.FailedToWriteGatewayError(err, "v2", namespace, "foo")
			Expect(expectedErr.Error()).To(ContainSubstring(err.Error()))
		})
	})
})
