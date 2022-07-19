package aws_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	gloov1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/aws"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer"
	skcore "github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"

	. "github.com/solo-io/gloo/projects/gloo/pkg/syncer/aws"
)

var _ = Describe("AwsTranslatorSyncer", func() {

	var (
		ctx                context.Context
		cancel             context.CancelFunc
		translator         syncer.TranslatorSyncerExtension
		apiSnapshot        *gloov1snap.ApiSnapshot
		snapCache          *syncer.MockXdsCache
		settings           *gloov1.Settings
		resourceReports    reporter.ResourceReports
		unwrapAsApiGateway bool
		useWeightedDest    bool
		proxy              *gloov1.Proxy
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		var err error

		translator, err = NewTranslatorSyncerExtension(ctx, syncer.TranslatorSyncerExtensionParams{})
		Expect(err).NotTo(HaveOccurred())

		apiSnapshot = &gloov1snap.ApiSnapshot{}
		settings = &gloov1.Settings{}
		resourceReports = make(reporter.ResourceReports)
	})

	JustBeforeEach(func() {
		if useWeightedDest {
			proxy = getProxyWithMultiDestAwsDestinationSpec(unwrapAsApiGateway)
		} else {
			proxy = getProxyWithAwsDestinationSpec(unwrapAsApiGateway)
		}

		apiSnapshot = &gloov1snap.ApiSnapshot{
			Proxies: gloov1.ProxyList{proxy},
		}
	})

	AfterEach(func() {
		cancel()
	})

	Context("Single Destination", func() {
		BeforeEach(func() {
			useWeightedDest = false
		})
		Context("Listener contains virtualHost with enterprise aws settings enabled", func() {
			BeforeEach(func() {
				unwrapAsApiGateway = true
			})

			It("should error", func() {
				_, err := translator.Sync(ctx, apiSnapshot, settings, snapCache, resourceReports)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(ErrEnterpriseOnly))
			})
		})

		Context("Listener contains virtualHost with no enterprise aws settings enabled", func() {
			BeforeEach(func() {
				unwrapAsApiGateway = false
			})

			It("should error", func() {
				_, err := translator.Sync(ctx, apiSnapshot, settings, snapCache, resourceReports)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Context("Multi Destination", func() {
		BeforeEach(func() {
			useWeightedDest = true
		})
		Context("Listener contains virtualHost with enterprise aws settings enabled", func() {
			BeforeEach(func() {
				unwrapAsApiGateway = true
			})

			It("should error", func() {
				_, err := translator.Sync(ctx, apiSnapshot, settings, snapCache, resourceReports)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(ErrEnterpriseOnly))
			})
		})

		Context("Listener contains virtualHost with no enterprise aws settings enabled", func() {
			BeforeEach(func() {
				unwrapAsApiGateway = false
			})

			It("should error", func() {
				_, err := translator.Sync(ctx, apiSnapshot, settings, snapCache, resourceReports)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

})

func getBasicProxy(virtualHost *gloov1.VirtualHost) *gloov1.Proxy {
	return &gloov1.Proxy{
		Metadata: &skcore.Metadata{
			Name:      "proxy",
			Namespace: "gloo-system",
		},
		Listeners: []*gloov1.Listener{{
			Name: "listener-::-8443",
			ListenerType: &gloov1.Listener_HttpListener{
				HttpListener: &gloov1.HttpListener{
					VirtualHosts: []*gloov1.VirtualHost{virtualHost},
				},
			},
		}},
	}
}

func getProxyWithAwsDestinationSpec(unwrapAsApiGateway bool) *gloov1.Proxy {
	virtualHost := &gloov1.VirtualHost{
		Name:    "gloo-system.default",
		Domains: []string{"*"},
		Routes: []*gloov1.Route{{
			Action: &gloov1.Route_RouteAction{
				RouteAction: &gloov1.RouteAction{
					Destination: &gloov1.RouteAction_Single{
						Single: &gloov1.Destination{
							DestinationSpec: &gloov1.DestinationSpec{
								DestinationType: &gloov1.DestinationSpec_Aws{
									Aws: &aws.DestinationSpec{
										LogicalName:        "logical-name",
										UnwrapAsApiGateway: unwrapAsApiGateway,
									},
								},
							},
						},
					},
				},
			},
		}},
	}

	return getBasicProxy(virtualHost)
}

func getProxyWithMultiDestAwsDestinationSpec(unwrapAsApiGateway bool) *gloov1.Proxy {
	virtualHost := &gloov1.VirtualHost{
		Name:    "gloo-system.default",
		Domains: []string{"*"},
		Routes: []*gloov1.Route{{
			Action: &gloov1.Route_RouteAction{
				RouteAction: &gloov1.RouteAction{
					Destination: &gloov1.RouteAction_Multi{
						Multi: &gloov1.MultiDestination{
							Destinations: []*gloov1.WeightedDestination{{
								Weight: 1,
								Destination: &gloov1.Destination{
									DestinationSpec: &gloov1.DestinationSpec{
										DestinationType: &gloov1.DestinationSpec_Aws{
											Aws: &aws.DestinationSpec{
												LogicalName:        "logical-name",
												UnwrapAsApiGateway: unwrapAsApiGateway,
											},
										},
									},
								},
							}},
						},
					},
				},
			},
		}},
	}

	return getBasicProxy(virtualHost)
}
