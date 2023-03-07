package ratelimit_test

import (
	"context"
	"fmt"

	"github.com/hashicorp/go-multierror"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rotisserie/eris"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/ratelimit"
	gloov1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer"
	"github.com/solo-io/go-utils/testutils"
	"github.com/solo-io/solo-apis/pkg/api/ratelimit.solo.io/v1alpha1"
	skcore "github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"

	. "github.com/solo-io/gloo/projects/gloo/pkg/syncer/ratelimit"
)

var _ = Describe("RatelimitTranslatorSyncer", func() {

	var (
		ctx        context.Context
		cancel     context.CancelFunc
		proxy      *gloov1.Proxy
		translator syncer.TranslatorSyncerExtension
	)

	Context("config with enterprise ratelimit feature is set on listener", func() {

		BeforeEach(func() {
			ctx, cancel = context.WithCancel(context.Background())
			translator = NewTranslatorSyncerExtension(ctx, syncer.TranslatorSyncerExtensionParams{})
		})

		AfterEach(func() {
			cancel()
		})

		ExpectSyncGeneratesEnterpriseOnlyError := func(errorName string) {
			apiSnapshot := &gloov1snap.ApiSnapshot{
				Proxies: []*gloov1.Proxy{proxy},
			}
			reports := make(reporter.ResourceReports)
			translator.Sync(ctx, apiSnapshot, &gloov1.Settings{}, &syncer.MockXdsCache{}, reports)

			// validate the reports contain appropriate error
			expectedErrorMessage := fmt.Sprintf("The Gloo Advanced Rate limit API feature '%s' is enterprise-only, please upgrade or use the Envoy rate-limit API instead", errorName)
			err := reports.ValidateStrict()
			multiErr, ok := err.(*multierror.Error)
			ExpectWithOffset(1, ok).To(BeTrue())
			ExpectWithOffset(1, multiErr.WrappedErrors()).To(ContainElement(testutils.HaveInErrorChain(eris.New(expectedErrorMessage))))
		}

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
				ExpectSyncGeneratesEnterpriseOnlyError("ratelimitBasic")
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
				ExpectSyncGeneratesEnterpriseOnlyError("ratelimitBasic")
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
				ExpectSyncGeneratesEnterpriseOnlyError("RateLimitConfig")
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
				ExpectSyncGeneratesEnterpriseOnlyError("setActions")
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
					ExpectSyncGeneratesEnterpriseOnlyError("RateLimitEarly")
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
					ExpectSyncGeneratesEnterpriseOnlyError("RateLimitEarly")
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
					ExpectSyncGeneratesEnterpriseOnlyError("RateLimitEarly")
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
					ExpectSyncGeneratesEnterpriseOnlyError("RateLimitEarly")
				})
			})

			When("RateLimitRegularConfigs is set on VirtualHost", func() {

				BeforeEach(func() {
					virtualHost := &gloov1.VirtualHost{
						Name: "gloo-system.default",
						Options: &gloov1.VirtualHostOptions{
							RateLimitRegularConfigType: &gloov1.VirtualHostOptions_RateLimitRegularConfigs{
								RateLimitRegularConfigs: &ratelimit.RateLimitConfigRefs{
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
					ExpectSyncGeneratesEnterpriseOnlyError("RateLimitRegular")
				})
			})

			When("RateLimitRegular is set on VirtualHost", func() {

				BeforeEach(func() {
					virtualHost := &gloov1.VirtualHost{
						Name: "gloo-system.default",
						Options: &gloov1.VirtualHostOptions{
							RateLimitRegularConfigType: &gloov1.VirtualHostOptions_RatelimitRegular{
								RatelimitRegular: &ratelimit.RateLimitVhostExtension{
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
					ExpectSyncGeneratesEnterpriseOnlyError("RateLimitRegular")
				})
			})

			When("RateLimitRegularConfigs is set on Route", func() {

				BeforeEach(func() {
					route := &gloov1.Route{
						Options: &gloov1.RouteOptions{
							RateLimitRegularConfigType: &gloov1.RouteOptions_RateLimitRegularConfigs{
								RateLimitRegularConfigs: &ratelimit.RateLimitConfigRefs{
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
					ExpectSyncGeneratesEnterpriseOnlyError("RateLimitRegular")
				})
			})

			When("RateLimitRegular is set on Route", func() {

				BeforeEach(func() {
					route := &gloov1.Route{
						Options: &gloov1.RouteOptions{
							RateLimitRegularConfigType: &gloov1.RouteOptions_RatelimitRegular{
								RatelimitRegular: &ratelimit.RateLimitRouteExtension{
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
					ExpectSyncGeneratesEnterpriseOnlyError("RateLimitRegular")
				})
			})

		})

	})
})
