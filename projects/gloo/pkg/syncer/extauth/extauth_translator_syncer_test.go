package extauth_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	extauth "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer"
	skcore "github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"

	. "github.com/solo-io/gloo/projects/gloo/pkg/syncer/extauth"
)

var _ = Describe("ExtauthTranslatorSyncer", func() {

	var (
		ctx             context.Context
		cancel          context.CancelFunc
		translator      syncer.TranslatorSyncerExtension
		apiSnapshot     *gloov1.ApiSnapshot
		snapCache       *syncer.MockXdsCache
		settings        *gloov1.Settings
		resourceReports reporter.ResourceReports
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		var err error

		translator, err = NewTranslatorSyncerExtension(ctx, syncer.TranslatorSyncerExtensionParams{})
		Expect(err).NotTo(HaveOccurred())

		apiSnapshot = &gloov1.ApiSnapshot{}
		settings = &gloov1.Settings{}
		resourceReports = make(reporter.ResourceReports)
	})

	AfterEach(func() {
		cancel()
	})

	Context("Listener contains ExtAuthExtension.ConfigRef", func() {

		var (
			authConfig       *extauth.AuthConfig
			extAuthExtension *extauth.ExtAuthExtension
		)

		BeforeEach(func() {
			authConfig = &extauth.AuthConfig{
				Metadata: &skcore.Metadata{
					Name:      "auth",
					Namespace: defaults.GlooSystem,
				},
				Configs: []*extauth.AuthConfig_Config{{
					AuthConfig: &extauth.AuthConfig_Config_Oauth{},
				}},
			}

			extAuthExtension = &extauth.ExtAuthExtension{
				Spec: &extauth.ExtAuthExtension_ConfigRef{
					ConfigRef: authConfig.Metadata.Ref(),
				},
			}
		})

		When("defined on VirtualHost", func() {

			BeforeEach(func() {
				proxy := getProxyWithVirtualHostExtAuthExtension(extAuthExtension)
				apiSnapshot = &gloov1.ApiSnapshot{
					Proxies:     gloov1.ProxyList{proxy},
					AuthConfigs: extauth.AuthConfigList{authConfig},
				}
			})

			It("should error", func() {
				_, err := translator.Sync(ctx, apiSnapshot, settings, snapCache, resourceReports)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(ErrEnterpriseOnly))
			})
		})

		When("defined on VirtualHost in HybridListener", func() {

			BeforeEach(func() {
				proxy := getProxyWithHybridListenerVirtualHostExtAuthExtension(extAuthExtension)
				apiSnapshot = &gloov1.ApiSnapshot{
					Proxies:     gloov1.ProxyList{proxy},
					AuthConfigs: extauth.AuthConfigList{authConfig},
				}
			})

			It("should error", func() {
				_, err := translator.Sync(ctx, apiSnapshot, settings, snapCache, resourceReports)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(ErrEnterpriseOnly))
			})
		})

		When("defined on Route", func() {

			BeforeEach(func() {
				proxy := getProxyWithRouteExtAuthExtension(extAuthExtension)
				apiSnapshot = &gloov1.ApiSnapshot{
					Proxies:     gloov1.ProxyList{proxy},
					AuthConfigs: extauth.AuthConfigList{authConfig},
				}
			})

			It("should error", func() {
				_, err := translator.Sync(ctx, apiSnapshot, settings, snapCache, resourceReports)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(ErrEnterpriseOnly))
			})
		})

		When("defined on WeightedDestination", func() {

			BeforeEach(func() {
				proxy := getProxyWithWeightedDestinationAuthExtension(extAuthExtension)
				apiSnapshot = &gloov1.ApiSnapshot{
					Proxies:     gloov1.ProxyList{proxy},
					AuthConfigs: extauth.AuthConfigList{authConfig},
				}
			})

			It("should error", func() {
				_, err := translator.Sync(ctx, apiSnapshot, settings, snapCache, resourceReports)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(ErrEnterpriseOnly))
			})
		})

	})

	Context("Listener contains ExtAuthExtension.CustomAuth.Name", func() {

		var (
			extAuthExtension *extauth.ExtAuthExtension
		)

		BeforeEach(func() {
			extAuthExtension = &extauth.ExtAuthExtension{
				Spec: &extauth.ExtAuthExtension_CustomAuth{
					CustomAuth: &extauth.CustomAuth{
						Name: "custom-auth-name",
					},
				},
			}
		})

		When("defined on VirtualHost", func() {

			BeforeEach(func() {
				proxy := getProxyWithVirtualHostExtAuthExtension(extAuthExtension)
				apiSnapshot = &gloov1.ApiSnapshot{
					Proxies: gloov1.ProxyList{proxy},
				}
			})

			It("should error", func() {
				_, err := translator.Sync(ctx, apiSnapshot, settings, snapCache, resourceReports)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(ErrEnterpriseOnly))
			})
		})

		When("defined on Route", func() {

			BeforeEach(func() {
				proxy := getProxyWithRouteExtAuthExtension(extAuthExtension)
				apiSnapshot = &gloov1.ApiSnapshot{
					Proxies: gloov1.ProxyList{proxy},
				}
			})

			It("should error", func() {
				_, err := translator.Sync(ctx, apiSnapshot, settings, snapCache, resourceReports)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(ErrEnterpriseOnly))
			})
		})

		When("defined on WeightedDestination", func() {

			BeforeEach(func() {
				proxy := getProxyWithWeightedDestinationAuthExtension(extAuthExtension)
				apiSnapshot = &gloov1.ApiSnapshot{
					Proxies: gloov1.ProxyList{proxy},
				}
			})

			It("should error", func() {
				_, err := translator.Sync(ctx, apiSnapshot, settings, snapCache, resourceReports)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(ErrEnterpriseOnly))
			})
		})

	})

	Context("Listener does not contain ExtAuthExtension.ConfigRef", func() {

		BeforeEach(func() {
			proxy := getProxyWithVirtualHostExtAuthExtension(nil)
			apiSnapshot = &gloov1.ApiSnapshot{
				Proxies: gloov1.ProxyList{proxy},
			}
		})

		It("should not error", func() {
			_, err := translator.Sync(ctx, apiSnapshot, settings, snapCache, resourceReports)
			Expect(err).NotTo(HaveOccurred())
		})

	})

	Context("Settings contain NamedExtauth", func() {

		BeforeEach(func() {
			settings.NamedExtauth = map[string]*extauth.Settings{
				"custom-auth-server": nil,
			}
		})

		It("should error", func() {
			_, err := translator.Sync(ctx, apiSnapshot, settings, snapCache, resourceReports)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(ErrEnterpriseOnly))
		})

	})

})

func getProxyWithVirtualHostExtAuthExtension(extension *extauth.ExtAuthExtension) *gloov1.Proxy {
	virtualHost := &gloov1.VirtualHost{
		Name: "gloo-system.default",
		Options: &gloov1.VirtualHostOptions{
			Extauth: extension,
		},
	}

	return getBasicProxy(virtualHost)
}

func getProxyWithHybridListenerVirtualHostExtAuthExtension(extension *extauth.ExtAuthExtension) *gloov1.Proxy {
	virtualHost := &gloov1.VirtualHost{
		Name: "gloo-system.default",
		Options: &gloov1.VirtualHostOptions{
			Extauth: extension,
		},
	}

	return getBasicHybridListenerProxy(virtualHost)
}

func getProxyWithRouteExtAuthExtension(extension *extauth.ExtAuthExtension) *gloov1.Proxy {
	virtualHost := &gloov1.VirtualHost{
		Name: "gloo-system.default",
		Routes: []*gloov1.Route{{
			Name: "route",
			Options: &gloov1.RouteOptions{
				Extauth: extension,
			},
		}},
	}

	return getBasicProxy(virtualHost)
}

func getProxyWithWeightedDestinationAuthExtension(extension *extauth.ExtAuthExtension) *gloov1.Proxy {
	virtualHost := &gloov1.VirtualHost{
		Name: "gloo-system.default",
		Routes: []*gloov1.Route{{
			Name: "route",
			Action: &gloov1.Route_RouteAction{
				RouteAction: &gloov1.RouteAction{
					Destination: &gloov1.RouteAction_Multi{
						Multi: &gloov1.MultiDestination{
							Destinations: []*gloov1.WeightedDestination{{
								Options: &gloov1.WeightedDestinationOptions{
									Extauth: extension,
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

func getBasicHybridListenerProxy(virtualHost *gloov1.VirtualHost) *gloov1.Proxy {
	return &gloov1.Proxy{
		Metadata: &skcore.Metadata{
			Name:      "proxy",
			Namespace: "gloo-system",
		},
		Listeners: []*gloov1.Listener{{
			Name: "listener-::-8443",
			ListenerType: &gloov1.Listener_HybridListener{
				HybridListener: &gloov1.HybridListener{
					MatchedListeners: []*gloov1.MatchedListener{
						{
							ListenerType: &gloov1.MatchedListener_HttpListener{
								HttpListener: &gloov1.HttpListener{
									VirtualHosts: []*gloov1.VirtualHost{virtualHost},
								},
							},
						},
					},
				},
			},
		}},
	}
}
