package httproute

import (
	. "github.com/onsi/ginkgo/v2"
)

const funcName = "my-function"

var _ = Describe("GatewayHttpRouteTranslator", func() {
	/* TODO: move to upstream unit tests
	type makeDestinationSpecTestCase struct {
		upstream     *gloov1.Upstream
		filters      []gwv1.HTTPRouteFilter
		expectedSpec *v1.DestinationSpec
		expectedErr  error
	}

	validFuncRefs := []gwv1.HTTPRouteFilter{{
		Type: gwv1.HTTPRouteFilterExtensionRef,
		ExtensionRef: &gwv1.LocalObjectReference{
			Group: parameters.ParameterGroup,
			Kind:  parameters.ParameterKind,
			Name:  funcName,
		},
	}}

	invalidFuncRefs := []gwv1.HTTPRouteFilter{{
		Type: gwv1.HTTPRouteFilterExtensionRef,
		ExtensionRef: &gwv1.LocalObjectReference{
			Group: "acme.io",
			Kind:  "Example",
			Name:  funcName,
		},
	}}
	azureUpstream := &gloov1.Upstream{Spec: v1.Upstream{
		UpstreamType: &v1.Upstream_Azure{},
	}}

	awsUpstream := &gloov1.Upstream{Spec: v1.Upstream{
		UpstreamType: &v1.Upstream_Aws{},
	}}

	staticUpstream := &gloov1.Upstream{Spec: v1.Upstream{
		UpstreamType: &v1.Upstream_Static{},
	}}

	DescribeTable("makeDestinationSpec", func(tc makeDestinationSpecTestCase) {
		spec, err := makeDestinationSpec(tc.upstream, tc.filters)
		if tc.expectedErr != nil {
			Expect(err).To(Equal(tc.expectedErr))
		} else {
			Expect(err).NotTo(HaveOccurred())
		}
		if tc.expectedSpec == nil {
			Expect(spec).To(BeNil())
		} else {
			Expect(spec).To(Equal(tc.expectedSpec))
		}
	},
		Entry("aws no parameter filter", makeDestinationSpecTestCase{
			upstream:     awsUpstream,
			filters:      invalidFuncRefs,
			expectedSpec: nil,
			expectedErr:  awsMissingFuncRefError,
		}),
		Entry("azure no parameter filter", makeDestinationSpecTestCase{
			upstream:     azureUpstream,
			filters:      invalidFuncRefs,
			expectedSpec: nil,
			expectedErr:  azureMissingFuncRefError,
		}),
		Entry("non function upstream with  parameter filter", makeDestinationSpecTestCase{
			upstream:     staticUpstream,
			filters:      validFuncRefs,
			expectedSpec: nil,
			expectedErr:  nonFunctionUpstreamWithParameterError,
		}),
		Entry("aws with parameter filter", makeDestinationSpecTestCase{
			upstream: awsUpstream,
			filters:  validFuncRefs,
			expectedSpec: &v1.DestinationSpec{
				DestinationType: &v1.DestinationSpec_Aws{
					Aws: &aws.DestinationSpec{
						LogicalName: funcName,
					},
				},
			},
			expectedErr: nil,
		}),
		Entry("azure with parameter filter", makeDestinationSpecTestCase{
			upstream: azureUpstream,
			filters:  validFuncRefs,
			expectedSpec: &v1.DestinationSpec{
				DestinationType: &v1.DestinationSpec_Azure{
					Azure: &azure.DestinationSpec{
						FunctionName: funcName,
					},
				},
			},
			expectedErr: nil,
		}),
		Entry("non function upstream with  no parameter filter", makeDestinationSpecTestCase{
			upstream:     staticUpstream,
			filters:      invalidFuncRefs,
			expectedSpec: nil,
			expectedErr:  nil,
		}),
	)
	*/
})
