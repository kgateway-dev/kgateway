package ratelimit_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/ratelimit"
	gloov1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer"
	"github.com/solo-io/solo-apis/pkg/api/ratelimit.solo.io/v1alpha1"
	skcore "github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"

	. "github.com/solo-io/gloo/projects/gloo/pkg/syncer/ratelimit"
)

var _ = Describe("RatelimitTranslatorSyncer", func() {

	var (
		ctx         context.Context
		cancel      context.CancelFunc
		proxy       *gloov1.Proxy
		params      syncer.TranslatorSyncerExtensionParams
		translator  syncer.TranslatorSyncerExtension
		apiSnapshot *gloov1snap.ApiSnapshot
		snapCache   *syncer.MockXdsCache
		settings    *gloov1.Settings
	)

	Context("config with enterprise ratelimit feature is set on listener", func() {

		BeforeEach(func() {
			var err error
			ctx, cancel = context.WithCancel(context.Background())

			translator, err = NewTranslatorSyncerExtension(ctx, params)
			Expect(err).NotTo(HaveOccurred())
		})

		JustBeforeEach(func() {
			settings = &gloov1.Settings{}

			apiSnapshot = &gloov1snap.ApiSnapshot{
				Proxies: []*gloov1.Proxy{proxy},
			}
		})

		AfterEach(func() {
			cancel()
		})

		Context("config ratelimitBasic", func() {

			BeforeEach(func() {
				config := &ratelimit.IngressRateLimit{
					AuthorizedLimits: nil,
					AnonymousLimits:  nil,
				}

				proxy = &gloov1.Proxy{
					Metadata: &skcore.Metadata{
						Name:      "proxy",
						Namespace: "gloo-system",
					},
					Listeners: []*gloov1.Listener{{
						Name: "listener-::-8080",
						ListenerType: &gloov1.Listener_HttpListener{
							HttpListener: &gloov1.HttpListener{
								VirtualHosts: []*gloov1.VirtualHost{
									{
										Name: "gloo-system.default",
										Options: &gloov1.VirtualHostOptions{
											RatelimitBasic: config,
										},
									},
								},
							},
						},
					}},
				}
			})

			It("should error when enterprise ratelimitBasic config is set", func() {
				_, err := translator.Sync(ctx, apiSnapshot, settings, snapCache, make(reporter.ResourceReports))
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("The Gloo Advanced Rate limit API feature 'ratelimitBasic' is enterprise-only, please upgrade or use the Envoy rate-limit API instead"))
			})
		})

		Context("config ratelimitBasic HybridListener", func() {

			BeforeEach(func() {
				config := &ratelimit.IngressRateLimit{
					AuthorizedLimits: nil,
					AnonymousLimits:  nil,
				}

				proxy = &gloov1.Proxy{
					Metadata: &skcore.Metadata{
						Name:      "proxy",
						Namespace: "gloo-system",
					},
					Listeners: []*gloov1.Listener{{
						Name: "listener-::-8080",
						ListenerType: &gloov1.Listener_HybridListener{
							HybridListener: &gloov1.HybridListener{
								MatchedListeners: []*gloov1.MatchedListener{
									{
										ListenerType: &gloov1.MatchedListener_HttpListener{
											HttpListener: &gloov1.HttpListener{
												VirtualHosts: []*gloov1.VirtualHost{
													&gloov1.VirtualHost{
														Name: "gloo-system.default",
														Options: &gloov1.VirtualHostOptions{
															RatelimitBasic: config,
														},
													},
												},
											},
										},
									},
								},
							},
						},
					}},
				}
			})

			It("should error when enterprise ratelimitBasic config is set", func() {
				_, err := translator.Sync(ctx, apiSnapshot, settings, snapCache, make(reporter.ResourceReports))
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("The Gloo Advanced Rate limit API feature 'ratelimitBasic' is enterprise-only, please upgrade or use the Envoy rate-limit API instead"))
			})
		})

		Context("config RateLimitConfig", func() {

			BeforeEach(func() {
				config := &ratelimit.RateLimitConfigRef{
					Name:      "foo",
					Namespace: "gloo-system",
				}

				route := &gloov1.Route{
					Options: &gloov1.RouteOptions{
						RateLimitConfigType: &gloov1.RouteOptions_RateLimitConfigs{
							RateLimitConfigs: &ratelimit.RateLimitConfigRefs{
								Refs: []*ratelimit.RateLimitConfigRef{
									config,
								},
							},
						},
					},
				}

				proxy = &gloov1.Proxy{
					Metadata: &skcore.Metadata{
						Name:      "proxy",
						Namespace: "gloo-system",
					},
					Listeners: []*gloov1.Listener{{
						Name: "listener-::-8080",
						ListenerType: &gloov1.Listener_HttpListener{
							HttpListener: &gloov1.HttpListener{
								VirtualHosts: []*gloov1.VirtualHost{
									{
										Routes: []*gloov1.Route{route},
									},
								},
							},
						},
					}},
				}
			})

			It("should error when enterprise RateLimitConfig config is set", func() {
				_, err := translator.Sync(ctx, apiSnapshot, settings, snapCache, make(reporter.ResourceReports))
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("The Gloo Advanced Rate limit API feature 'RateLimitConfig' is enterprise-only, please upgrade or use the Envoy rate-limit API instead"))
			})
		})

		Context("config setActions", func() {

			BeforeEach(func() {
				proxy = &gloov1.Proxy{
					Metadata: &skcore.Metadata{
						Name:      "proxy",
						Namespace: "gloo-system",
					},
					Listeners: []*gloov1.Listener{{
						Name: "listener-::-8080",
						ListenerType: &gloov1.Listener_HttpListener{
							HttpListener: &gloov1.HttpListener{
								VirtualHosts: []*gloov1.VirtualHost{
									{
										Name: "gloo-system.default",
										Options: &gloov1.VirtualHostOptions{
											RateLimitConfigType: &gloov1.VirtualHostOptions_Ratelimit{
												Ratelimit: &ratelimit.RateLimitVhostExtension{
													RateLimits: []*v1alpha1.RateLimitActions{
														{
															SetActions: []*v1alpha1.Action{},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					}},
				}
			})

			It("should error when enterprise setActions config is set", func() {
				_, err := translator.Sync(ctx, apiSnapshot, settings, snapCache, make(reporter.ResourceReports))
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("The Gloo Advanced Rate limit API feature 'setActions' is enterprise-only, please upgrade or use the Envoy rate-limit API instead"))
			})
		})

		Context("Staged Rate Limiting", func() {

			When("RateLimitEarlyConfigs is set on VirtualHost", func() {

				BeforeEach(func() {
					virtualHost := &gloov1.VirtualHost{
						Name: "gloo-system.default",
						Options: &gloov1.VirtualHostOptions{
							RateLimitEarlyConfigType: &gloov1.VirtualHostOptions_RateLimitEarlyConfigs{
								RateLimitEarlyConfigs: &ratelimit.RateLimitConfigRefs{
									Refs: []*ratelimit.RateLimitConfigRef{{
										Name:      "foo",
										Namespace: "gloo-system",
									}},
								},
							},
						},
					}

					proxy = &gloov1.Proxy{
						Metadata: &skcore.Metadata{
							Name:      "proxy",
							Namespace: "gloo-system",
						},
						Listeners: []*gloov1.Listener{{
							Name: "listener-::-8080",
							ListenerType: &gloov1.Listener_HttpListener{
								HttpListener: &gloov1.HttpListener{
									VirtualHosts: []*gloov1.VirtualHost{
										virtualHost,
									},
								},
							},
						}},
					}
				})

				It("errors", func() {
					_, err := translator.Sync(ctx, apiSnapshot, settings, snapCache, make(reporter.ResourceReports))
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError("The Gloo Advanced Rate limit API feature 'RateLimitEarly' is enterprise-only, please upgrade or use the Envoy rate-limit API instead"))
				})
			})

			When("RateLimitEarly is set on VirtualHost", func() {

				BeforeEach(func() {
					virtualHost := &gloov1.VirtualHost{
						Name: "gloo-system.default",
						Options: &gloov1.VirtualHostOptions{
							RateLimitEarlyConfigType: &gloov1.VirtualHostOptions_RatelimitEarly{
								RatelimitEarly: &ratelimit.RateLimitVhostExtension{
									RateLimits: []*v1alpha1.RateLimitActions{
										{
											SetActions: []*v1alpha1.Action{},
										},
									},
								},
							},
						},
					}

					proxy = &gloov1.Proxy{
						Metadata: &skcore.Metadata{
							Name:      "proxy",
							Namespace: "gloo-system",
						},
						Listeners: []*gloov1.Listener{{
							Name: "listener-::-8080",
							ListenerType: &gloov1.Listener_HttpListener{
								HttpListener: &gloov1.HttpListener{
									VirtualHosts: []*gloov1.VirtualHost{
										virtualHost,
									},
								},
							},
						}},
					}
				})

				It("errors", func() {
					_, err := translator.Sync(ctx, apiSnapshot, settings, snapCache, make(reporter.ResourceReports))
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError("The Gloo Advanced Rate limit API feature 'RateLimitEarly' is enterprise-only, please upgrade or use the Envoy rate-limit API instead"))
				})
			})

			When("RateLimitEarlyConfigs is set on Route", func() {

				BeforeEach(func() {
					route := &gloov1.Route{
						Options: &gloov1.RouteOptions{
							RateLimitEarlyConfigType: &gloov1.RouteOptions_RateLimitEarlyConfigs{
								RateLimitEarlyConfigs: &ratelimit.RateLimitConfigRefs{
									Refs: []*ratelimit.RateLimitConfigRef{{
										Name:      "foo",
										Namespace: "gloo-system",
									}},
								},
							},
						},
					}

					proxy = &gloov1.Proxy{
						Metadata: &skcore.Metadata{
							Name:      "proxy",
							Namespace: "gloo-system",
						},
						Listeners: []*gloov1.Listener{{
							Name: "listener-::-8080",
							ListenerType: &gloov1.Listener_HttpListener{
								HttpListener: &gloov1.HttpListener{
									VirtualHosts: []*gloov1.VirtualHost{
										{
											Routes: []*gloov1.Route{route},
										},
									},
								},
							},
						}},
					}
				})

				It("errors", func() {
					_, err := translator.Sync(ctx, apiSnapshot, settings, snapCache, make(reporter.ResourceReports))
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError("The Gloo Advanced Rate limit API feature 'RateLimitEarly' is enterprise-only, please upgrade or use the Envoy rate-limit API instead"))
				})
			})

			When("RateLimitEarly is set on Route", func() {

				BeforeEach(func() {
					route := &gloov1.Route{
						Options: &gloov1.RouteOptions{
							RateLimitEarlyConfigType: &gloov1.RouteOptions_RatelimitEarly{
								RatelimitEarly: &ratelimit.RateLimitRouteExtension{
									RateLimits: []*v1alpha1.RateLimitActions{
										{
											SetActions: []*v1alpha1.Action{},
										},
									},
								},
							},
						},
					}

					proxy = &gloov1.Proxy{
						Metadata: &skcore.Metadata{
							Name:      "proxy",
							Namespace: "gloo-system",
						},
						Listeners: []*gloov1.Listener{{
							Name: "listener-::-8080",
							ListenerType: &gloov1.Listener_HttpListener{
								HttpListener: &gloov1.HttpListener{
									VirtualHosts: []*gloov1.VirtualHost{
										{
											Routes: []*gloov1.Route{route},
										},
									},
								},
							},
						}},
					}
				})

				It("errors", func() {
					_, err := translator.Sync(ctx, apiSnapshot, settings, snapCache, make(reporter.ResourceReports))
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError("The Gloo Advanced Rate limit API feature 'RateLimitEarly' is enterprise-only, please upgrade or use the Envoy rate-limit API instead"))
				})
			})

		})

	})
})
