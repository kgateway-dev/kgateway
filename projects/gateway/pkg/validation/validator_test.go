package validation

import (
	"context"
	"fmt"

	"github.com/solo-io/go-utils/errors"

	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	v2 "github.com/solo-io/gloo/projects/gateway/pkg/api/v2"
	"github.com/solo-io/gloo/projects/gateway/pkg/translator"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	validationutils "github.com/solo-io/gloo/projects/gloo/pkg/utils/validation"
	"github.com/solo-io/gloo/test/samples"
	"google.golang.org/grpc"
)

var _ = Describe("Validator", func() {
	var (
		t  translator.Translator
		vc *mockValidationClient
		ns string
		v  *validator
	)
	BeforeEach(func() {
		t = translator.NewDefaultTranslator()
		vc = &mockValidationClient{}
		ns = "my-namespace"
		v = NewValidator(NewValidatorConfig(t, vc, ns, false, false))
	})
	It("returns error before sync called", func() {
		_, err := v.ValidateVirtualService(nil, nil)
		Expect(err).To(MatchError("validation is yet not available. Waiting for first snapshot"))
		err = v.Sync(nil, &v2.ApiSnapshot{})
		Expect(err).NotTo(HaveOccurred())
	})

	Context("validating a route table", func() {
		Context("proxy validation accepted", func() {
			It("accepts the rt", func() {
				vc.validateProxy = acceptProxy
				us := samples.SimpleUpstream()
				snap := samples.GatewaySnapshotWithDelegates(us.Metadata.Ref(), ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				proxyReports, err := v.ValidateRouteTable(context.TODO(), snap.RouteTables[0])
				Expect(err).NotTo(HaveOccurred())
				Expect(proxyReports).To(HaveLen(0))
			})
		})

		Context("proxy validation returns error", func() {
			It("rejects the rt", func() {
				vc.validateProxy = failProxy
				us := samples.SimpleUpstream()
				snap := samples.GatewaySnapshotWithDelegates(us.Metadata.Ref(), ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				// change something to change the hash
				snap.RouteTables[0].Metadata.Labels = map[string]string{"change": "my mind"}

				proxyReports, err := v.ValidateRouteTable(context.TODO(), snap.RouteTables[0])
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to validate Proxy with Gloo validation server"))
				Expect(proxyReports).To(HaveLen(1))
			})
		})

		Context("proxy validation fails (bad connection)", func() {
			Context("ignoreProxyValidation=false", func() {
				It("rejects the rt", func() {
					vc.validateProxy = communicationErr
					us := samples.SimpleUpstream()
					snap := samples.GatewaySnapshotWithDelegates(us.Metadata.Ref(), ns)
					err := v.Sync(context.TODO(), snap)
					Expect(err).NotTo(HaveOccurred())

					// change something to change the hash
					snap.RouteTables[0].Metadata.Labels = map[string]string{"change": "my mind"}

					proxyReports, err := v.ValidateRouteTable(context.TODO(), snap.RouteTables[0])
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("failed to communicate with Gloo Proxy validation server"))
					Expect(proxyReports).To(BeNil())
				})
			})
			Context("ignoreProxyValidation=true", func() {
				It("accepts the rt", func() {
					vc.validateProxy = communicationErr
					v = NewValidator(NewValidatorConfig(t, vc, ns, true, false))
					us := samples.SimpleUpstream()
					snap := samples.GatewaySnapshotWithDelegates(us.Metadata.Ref(), ns)
					err := v.Sync(context.TODO(), snap)
					Expect(err).NotTo(HaveOccurred())
					proxyReports, err := v.ValidateRouteTable(context.TODO(), snap.RouteTables[0])
					Expect(err).NotTo(HaveOccurred())
					Expect(proxyReports).To(HaveLen(0))
				})
			})
			Context("allowBrokenLinks=true", func() {
				BeforeEach(func() {
					v = NewValidator(NewValidatorConfig(t, vc, ns, true, true))
				})
				It("accepts a vs with missing route table ref", func() {
					vc.validateProxy = communicationErr
					err := v.Sync(context.TODO(), &v2.ApiSnapshot{})
					Expect(err).NotTo(HaveOccurred())
					vs, _ := samples.LinkedRouteTablesWithVirtualService("vs", "ns")
					proxyReports, err := v.ValidateVirtualService(context.TODO(), vs)
					Expect(err).NotTo(HaveOccurred())
					Expect(proxyReports).To(HaveLen(0))
				})
				It("accepts a rt with missing route table ref", func() {
					vc.validateProxy = communicationErr
					err := v.Sync(context.TODO(), &v2.ApiSnapshot{})
					Expect(err).NotTo(HaveOccurred())
					_, rts := samples.LinkedRouteTablesWithVirtualService("vs", "ns")
					proxyReports, err := v.ValidateRouteTable(context.TODO(), rts[1])
					Expect(err).NotTo(HaveOccurred())
					Expect(proxyReports).To(HaveLen(0))
				})
				It("accepts delete leaf route table", func() {
					vc.validateProxy = communicationErr
					us := samples.SimpleUpstream()
					snap := samples.GatewaySnapshotWithDelegates(us.Metadata.Ref(), ns)
					err := v.Sync(context.TODO(), snap)
					Expect(err).NotTo(HaveOccurred())

					ref := snap.RouteTables[len(snap.RouteTables)-1].Metadata.Ref()

					err = v.ValidateDeleteRouteTable(context.TODO(), ref)
					Expect(err).NotTo(HaveOccurred())
				})
			})
		})

		Context("route table rejected", func() {
			It("rejects the rt", func() {
				badRoute := &gatewayv1.Route{
					Action: &gatewayv1.Route_DelegateAction{
						DelegateAction: nil,
					},
				}

				// validate proxy should never be called
				vc.validateProxy = nil
				us := samples.SimpleUpstream()
				snap := samples.GatewaySnapshotWithDelegates(us.Metadata.Ref(), ns)
				rt := snap.RouteTables[0].DeepCopyObject().(*gatewayv1.RouteTable)
				rt.Routes = append(rt.Routes, badRoute)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				proxyReports, err := v.ValidateRouteTable(context.TODO(), rt)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("could not render proxy"))
				Expect(proxyReports).To(HaveLen(0))
			})
		})
	})

	Context("delete a route table", func() {
		Context("has parents", func() {
			It("rejects deletion", func() {
				vc.validateProxy = acceptProxy
				us := samples.SimpleUpstream()
				snap := samples.GatewaySnapshotWithDelegateChain(us.Metadata.Ref(), ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				err = v.ValidateDeleteRouteTable(context.TODO(), snap.RouteTables[1].Metadata.Ref())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Deletion blocked because active Routes delegate to this Route Table. " +
					"Remove delegate actions to this route table from the virtual services: [] and the route tables: [{node-0 my-namespace}], then try again"))
			})
		})
		Context("has no parents", func() {
			It("deletes safely", func() {
				vc.validateProxy = acceptProxy
				us := samples.SimpleUpstream()
				snap := samples.GatewaySnapshotWithDelegateChain(us.Metadata.Ref(), ns)
				// break the parent chain
				snap.RouteTables[1].Routes = nil
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				ref := snap.RouteTables[2].Metadata.Ref()
				err = v.ValidateDeleteRouteTable(context.TODO(), ref)
				Expect(err).NotTo(HaveOccurred())

				// ensure route table was removed from validator internal snapshot
				_, err = v.latestSnapshot.RouteTables.Find(ref.Strings())
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Context("validating a virtual service", func() {

		Context("proxy validation returns error", func() {
			It("rejects the vs", func() {
				vc.validateProxy = failProxy
				us := samples.SimpleUpstream()
				snap := samples.SimpleGatewaySnapshot(us.Metadata.Ref(), ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				// change something to change the hash
				snap.VirtualServices[0].Metadata.Labels = map[string]string{"change": "my mind"}

				proxyReports, err := v.ValidateVirtualService(context.TODO(), snap.VirtualServices[0])
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to validate Proxy with Gloo validation server"))
				Expect(proxyReports).To(HaveLen(1))

			})
		})
		Context("proxy validation accepted", func() {
			It("accepts the vs", func() {
				vc.validateProxy = acceptProxy
				us := samples.SimpleUpstream()
				snap := samples.SimpleGatewaySnapshot(us.Metadata.Ref(), ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				proxyReports, err := v.ValidateVirtualService(context.TODO(), snap.VirtualServices[0])
				Expect(err).NotTo(HaveOccurred())
				Expect(proxyReports).To(HaveLen(0))
			})
		})
		Context("no gateways for virtualservice", func() {
			It("accepts the vs", func() {
				vc.validateProxy = failProxy
				us := samples.SimpleUpstream()
				snap := samples.SimpleGatewaySnapshot(us.Metadata.Ref(), ns)
				snap.Gateways.Each(func(element *v2.Gateway) {
					http, ok := element.GatewayType.(*v2.Gateway_HttpGateway)
					if !ok {
						return
					}
					http.HttpGateway.VirtualServiceSelector = map[string]string{"nobody": "hastheselabels"}

				})
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				proxyReports, err := v.ValidateVirtualService(context.TODO(), snap.VirtualServices[0])
				Expect(err).NotTo(HaveOccurred())
				Expect(proxyReports).To(HaveLen(0))
			})
		})
		Context("virtual service rejected", func() {
			It("rejects the vs", func() {
				badRoute := &gatewayv1.Route{
					Action: &gatewayv1.Route_DelegateAction{
						DelegateAction: nil,
					},
				}

				// validate proxy should never be called
				vc.validateProxy = nil
				us := samples.SimpleUpstream()
				snap := samples.SimpleGatewaySnapshot(us.Metadata.Ref(), ns)
				vs := snap.VirtualServices[0].DeepCopyObject().(*gatewayv1.VirtualService)
				vs.VirtualHost.Routes = append(vs.VirtualHost.Routes, badRoute)

				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				proxyReports, err := v.ValidateVirtualService(context.TODO(), vs)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("could not render proxy"))
				Expect(proxyReports).To(HaveLen(0))
			})
		})
	})

	Context("delete a virtual service", func() {
		Context("has parent gateways", func() {
			It("rejects deletion", func() {
				vc.validateProxy = acceptProxy
				us := samples.SimpleUpstream()
				snap := samples.SimpleGatewaySnapshot(us.Metadata.Ref(), ns)
				ref := snap.VirtualServices[0].Metadata.Ref()
				snap.Gateways.Each(func(element *v2.Gateway) {
					http, ok := element.GatewayType.(*v2.Gateway_HttpGateway)
					if !ok {
						return
					}
					http.HttpGateway.VirtualServices = []core.ResourceRef{ref}
				})
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				err = v.ValidateDeleteVirtualService(context.TODO(), ref)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("Deletion blocked because active Gateways reference this Virtual Service. "+
					"Remove refs to this virtual service from the gateways: [{%s my-namespace} {%s-ssl my-namespace}], "+
					"then try again", defaults.GatewayProxyName, defaults.GatewayProxyName)))
			})
		})
		Context("has no parent gateways", func() {
			It("deletes safely", func() {
				vc.validateProxy = acceptProxy
				us := samples.SimpleUpstream()
				snap := samples.SimpleGatewaySnapshot(us.Metadata.Ref(), ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				ref := snap.VirtualServices[0].Metadata.Ref()
				err = v.ValidateDeleteVirtualService(context.TODO(), ref)
				Expect(err).NotTo(HaveOccurred())

				// ensure vs was removed from validator internal snapshot
				_, err = v.latestSnapshot.VirtualServices.Find(ref.Strings())
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Context("validating a gateway", func() {

		Context("proxy validation returns error", func() {
			It("rejects the gw", func() {
				vc.validateProxy = failProxy
				us := samples.SimpleUpstream()
				snap := samples.SimpleGatewaySnapshot(us.Metadata.Ref(), ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				// change something to change the hash
				snap.Gateways[0].Metadata.Labels = map[string]string{"change": "my mind"}

				proxyReports, err := v.ValidateGateway(context.TODO(), snap.Gateways[0])
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to validate Proxy with Gloo validation server"))
				Expect(proxyReports).To(HaveLen(1))
			})
		})
		Context("proxy validation accepted", func() {
			It("accepts the gw", func() {
				vc.validateProxy = acceptProxy
				us := samples.SimpleUpstream()
				snap := samples.SimpleGatewaySnapshot(us.Metadata.Ref(), ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				proxyReports, err := v.ValidateGateway(context.TODO(), snap.Gateways[0])
				Expect(err).NotTo(HaveOccurred())
				Expect(proxyReports).To(HaveLen(0))
			})
		})
		Context("gw rejected", func() {
			It("rejects the gw", func() {
				badRef := core.ResourceRef{}

				// validate proxy should never be called
				vc.validateProxy = nil
				us := samples.SimpleUpstream()
				snap := samples.SimpleGatewaySnapshot(us.Metadata.Ref(), ns)
				gw := snap.Gateways[0].DeepCopyObject().(*v2.Gateway)

				gw.GatewayType.(*v2.Gateway_HttpGateway).HttpGateway.VirtualServices = append(gw.GatewayType.(*v2.Gateway_HttpGateway).HttpGateway.VirtualServices, badRef)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				proxyReports, err := v.ValidateGateway(context.TODO(), gw)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("could not render proxy"))
				Expect(proxyReports).To(HaveLen(0))
			})
		})
	})

})

type mockValidationClient struct {
	validateProxy func(ctx context.Context, in *validation.ProxyValidationServiceRequest, opts ...grpc.CallOption) (*validation.ProxyValidationServiceResponse, error)
}

func (c *mockValidationClient) ValidateProxy(ctx context.Context, in *validation.ProxyValidationServiceRequest, opts ...grpc.CallOption) (*validation.ProxyValidationServiceResponse, error) {
	if c.validateProxy == nil {
		Fail("validateProxy was called unexpectedly")
	}
	return c.validateProxy(ctx, in, opts...)
}

func acceptProxy(ctx context.Context, in *validation.ProxyValidationServiceRequest, opts ...grpc.CallOption) (*validation.ProxyValidationServiceResponse, error) {
	return &validation.ProxyValidationServiceResponse{ProxyReport: validationutils.MakeReport(in.Proxy)}, nil
}

func failProxy(ctx context.Context, in *validation.ProxyValidationServiceRequest, opts ...grpc.CallOption) (*validation.ProxyValidationServiceResponse, error) {
	rpt := validationutils.MakeReport(in.Proxy)
	validationutils.AppendListenerError(rpt.ListenerReports[0], validation.ListenerReport_Error_SSLConfigError, "you should try harder next time")
	return &validation.ProxyValidationServiceResponse{ProxyReport: rpt}, nil
}

func communicationErr(ctx context.Context, in *validation.ProxyValidationServiceRequest, opts ...grpc.CallOption) (*validation.ProxyValidationServiceResponse, error) {
	return nil, errors.Errorf("communication no good")
}
