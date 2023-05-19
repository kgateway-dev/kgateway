package extauth_test

import (
	"context"

	"github.com/hashicorp/go-multierror"
	"github.com/solo-io/go-utils/testutils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	extauth "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	gloov1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer"
	skcore "github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"

	. "github.com/solo-io/gloo/projects/gloo/pkg/syncer/extauth"
)

var _ = Describe("ExtauthTranslatorSyncer", func() {

	var (
		ctx         context.Context
		cancel      context.CancelFunc
		translator  syncer.TranslatorSyncerExtension
		apiSnapshot *gloov1snap.ApiSnapshot
		settings    *gloov1.Settings
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		translator = NewTranslatorSyncerExtension(ctx, syncer.TranslatorSyncerExtensionParams{})

		settings = &gloov1.Settings{}
		apiSnapshot = &gloov1snap.ApiSnapshot{}
	})

	AfterEach(func() {
		cancel()
	})

	ExpectSyncGeneratesEnterpriseOnlyError := func() {
		reports := make(reporter.ResourceReports)
		translator.Sync(ctx, apiSnapshot, settings, &syncer.MockXdsCache{}, reports)

		err := reports.ValidateStrict()
		multiErr, ok := err.(*multierror.Error)
		ExpectWithOffset(1, ok).To(BeTrue())
		ExpectWithOffset(1, multiErr.WrappedErrors()).To(ContainElement(testutils.HaveInErrorChain(ErrEnterpriseOnly)))
	}

	ExpectSyncDoesNotError := func() {
		reports := make(reporter.ResourceReports)
		translator.Sync(ctx, apiSnapshot, settings, &syncer.MockXdsCache{}, reports)

		err := reports.ValidateStrict()
		ExpectWithOffset(1, err).To(BeNil())
	}

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
				apiSnapshot = &gloov1snap.ApiSnapshot{
					Proxies:     gloov1.ProxyList{proxy},
					AuthConfigs: extauth.AuthConfigList{authConfig},
				}
			})

			It("should error", func() {
				ExpectSyncGeneratesEnterpriseOnlyError()
			})
		})

		When("defined on VirtualHost in HybridListener", func() {

			BeforeEach(func() {
				proxy := getProxyWithHybridListenerVirtualHostExtAuthExtension(extAuthExtension)
				apiSnapshot = &gloov1snap.ApiSnapshot{
					Proxies:     gloov1.ProxyList{proxy},
					AuthConfigs: extauth.AuthConfigList{authConfig},
				}
			})

			It("should error", func() {
				ExpectSyncGeneratesEnterpriseOnlyError()
			})
		})

		When("defined on Route", func() {

			BeforeEach(func() {
				proxy := getProxyWithRouteExtAuthExtension(extAuthExtension)
				apiSnapshot = &gloov1snap.ApiSnapshot{
					Proxies:     gloov1.ProxyList{proxy},
					AuthConfigs: extauth.AuthConfigList{authConfig},
				}
			})

			It("should error", func() {
				ExpectSyncGeneratesEnterpriseOnlyError()
			})
		})

		When("defined on WeightedDestination", func() {

			BeforeEach(func() {
				proxy := getProxyWithWeightedDestinationAuthExtension(extAuthExtension)
				apiSnapshot = &gloov1snap.ApiSnapshot{
					Proxies:     gloov1.ProxyList{proxy},
					AuthConfigs: extauth.AuthConfigList{authConfig},
				}
			})

			It("should error", func() {
				ExpectSyncGeneratesEnterpriseOnlyError()
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
				apiSnapshot = &gloov1snap.ApiSnapshot{
					Proxies: gloov1.ProxyList{proxy},
				}
			})

			It("should error", func() {
				ExpectSyncGeneratesEnterpriseOnlyError()
			})
		})

		When("defined on Route", func() {

			BeforeEach(func() {
				proxy := getProxyWithRouteExtAuthExtension(extAuthExtension)
				apiSnapshot = &gloov1snap.ApiSnapshot{
					Proxies: gloov1.ProxyList{proxy},
				}
			})

			It("should error", func() {
				ExpectSyncGeneratesEnterpriseOnlyError()
			})
		})

		When("defined on WeightedDestination", func() {

			BeforeEach(func() {
				proxy := getProxyWithWeightedDestinationAuthExtension(extAuthExtension)
				apiSnapshot = &gloov1snap.ApiSnapshot{
					Proxies: gloov1.ProxyList{proxy},
				}
			})

			It("should error", func() {
				ExpectSyncGeneratesEnterpriseOnlyError()
			})
		})

	})

	Context("Listener does not contain ExtAuthExtension.ConfigRef", func() {

		BeforeEach(func() {
			proxy := getProxyWithVirtualHostExtAuthExtension(nil)
			apiSnapshot = &gloov1snap.ApiSnapshot{
				Proxies: gloov1.ProxyList{proxy},
			}
		})

		It("should not error", func() {
			ExpectSyncDoesNotError()
		})

	})

	Context("Settings contain NamedExtauth", func() {

		BeforeEach(func() {
			settings.NamedExtauth = map[string]*extauth.Settings{
				"custom-auth-server": nil,
			}
		})

		It("should not error", func() {
			// Currently, we do not aggregate errors from Settings on resourceReports
			// We will only log this error
			ExpectSyncDoesNotError()
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
