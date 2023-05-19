package extauth_test

import (
	"time"

	"github.com/solo-io/gloo/projects/gloo/pkg/syncer/extauth"

	envoycore "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	envoymatcher "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	envoytype "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	extauthv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/extauth"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	. "github.com/solo-io/go-utils/testutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/utils/prototime"
	"github.com/solo-io/solo-kit/test/matchers"
)

var _ = Describe("ExtAuthzConfigGenerator", func() {

	var (
		extAuthzConfigGenerator ExtAuthzConfigGenerator
		defaultSettings         *extauthv1.Settings
	)

	Context("GenerateListenerExtAuthzConfig", func() {

		Context("MultiConfigGenerator (multiple ext_authz filters)", func() {

			It("Returns ErrEnterpriseOnly error", func() {
				extAuthzConfigGenerator = &MultiConfigGenerator{}
				_, err := extAuthzConfigGenerator.GenerateListenerExtAuthzConfig(nil, nil)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(extauth.ErrEnterpriseOnly))
			})

		})

		Context("DefaultConfigGenerator (single ext_authz filter)", func() {

			JustBeforeEach(func() {
				extAuthzConfigGenerator = NewDefaultConfigGenerator(defaultSettings)
			})

			When("no default settings are provided", func() {

				BeforeEach(func() {
					defaultSettings = nil
				})

				It("does not return any filter", func() {
					filters, err := extAuthzConfigGenerator.GenerateListenerExtAuthzConfig(nil, nil)
					Expect(err).NotTo(HaveOccurred())
					Expect(filters).To(HaveLen(0))
				})
			})

			When("default settings do not contain ext auth server ref", func() {

				BeforeEach(func() {
					defaultSettings = &extauthv1.Settings{}
				})

				It("returns an error", func() {
					_, err := extAuthzConfigGenerator.GenerateListenerExtAuthzConfig(nil, nil)
					Expect(err).To(HaveOccurred())
					Expect(err).To(Equal(NoServerRefErr))
				})
			})

			When("settings contain incorrect ext auth server ref", func() {

				var invalidUs *core.ResourceRef

				BeforeEach(func() {
					invalidUs = &core.ResourceRef{
						Name:      "non",
						Namespace: "existent",
					}
					defaultSettings = &extauthv1.Settings{
						ExtauthzServerRef: invalidUs,
					}
				})

				It("returns an error", func() {
					_, err := extAuthzConfigGenerator.GenerateListenerExtAuthzConfig(nil, nil)
					Expect(err).To(HaveOccurred())
					Expect(err).To(HaveInErrorChain(ServerNotFound(invalidUs)))
				})
			})

			Context("listener settings override global settings", func() {
				var (
					extauthSettings *extauthv1.Settings
					initParams      plugins.InitParams
					params          plugins.Params
					extauthPlugin   plugins.HttpFilterPlugin
					listener        *gloov1.HttpListener
					ref             core.ResourceRef
				)

				BeforeEach(func() {
					extauthPlugin = NewPlugin()
					ref = core.ResourceRef{
						Name:      "test",
						Namespace: "test",
					}
					params.Snapshot = &v1snap.ApiSnapshot{
						Upstreams: []*gloov1.Upstream{
							{
								Metadata: &core.Metadata{
									Name:      "extauth-upstream",
									Namespace: "ns",
								},
							},
						},
					}
					params.Snapshot.Upstreams = []*gloov1.Upstream{
						{
							Metadata: &core.Metadata{
								Name:      "test",
								Namespace: "test",
							},
						},
					}
					extauthSettings = &extauthv1.Settings{
						ExtauthzServerRef: &ref,
						FailureModeAllow:  true,
					}
					initParams.Settings = &gloov1.Settings{}
					extauthPlugin.Init(initParams)
					listener = &gloov1.HttpListener{
						Options: &gloov1.HttpListenerOptions{
							Extauth: extauthSettings,
						},
					}
				})

				It("should get extauth settings first from the listener, then from the global settings", func() {
					filters, err := extauthPlugin.HttpFilters(params, listener)
					Expect(err).NotTo(HaveOccurred(), "Should be able to build extauth filters")
					Expect(filters).To(HaveLen(1), "Should only have created one custom filter")
					// Should take config from http listener
					Expect(filters[0].Stage.Weight).To(Equal(0))
					Expect(filters[0].Stage.RelativeTo).To(Equal(plugins.AuthNStage))
					Expect(filters[0].HttpFilter.Name).To(Equal(wellknown.HTTPExternalAuthorization))
				})
			})

			Context("settings point to a valid ext auth server", func() {

				var (
					upstream       *gloov1.Upstream
					expectedConfig *envoyauth.ExtAuthz
				)

				BeforeEach(func() {
					upstream = &gloov1.Upstream{
						Metadata: &core.Metadata{
							Name:      "extauth",
							Namespace: "default",
						},
						UpstreamType: &gloov1.Upstream_Static{
							Static: &static.UpstreamSpec{
								Hosts: []*static.Host{{
									Addr: "test",
									Port: 1234,
								}},
							},
						},
					}
				})

				When("minimal settings are provided", func() {

					BeforeEach(func() {
						usRef := upstream.Metadata.Ref()

						defaultSettings = &extauthv1.Settings{
							ExtauthzServerRef: usRef,
						}

						expectedConfig = &envoyauth.ExtAuthz{
							TransportApiVersion:       envoycore.ApiVersion_V3,
							MetadataContextNamespaces: []string{JWTFilterName},
							Services: &envoyauth.ExtAuthz_GrpcService{
								GrpcService: &envoycore.GrpcService{
									Timeout: DefaultTimeout,
									TargetSpecifier: &envoycore.GrpcService_EnvoyGrpc_{
										EnvoyGrpc: &envoycore.GrpcService_EnvoyGrpc{
											ClusterName: translator.UpstreamToClusterName(usRef),
										},
									},
								},
							},
						}
					})

					It("uses the expected defaults", func() {
						filters, err := extAuthzConfigGenerator.GenerateListenerExtAuthzConfig(nil, gloov1.UpstreamList{upstream})
						Expect(err).NotTo(HaveOccurred())
						Expect(filters).To(HaveLen(1))

						actualFilterConfig := filters[0]
						Expect(actualFilterConfig).To(matchers.MatchProto(expectedConfig))
					})
				})

				When("transport protocol version is set to V3 in settings", func() {

					BeforeEach(func() {
						defaultSettings = &extauthv1.Settings{
							ExtauthzServerRef: upstream.Metadata.Ref(),
						}
					})

					It("sets TransportApiVersion to V3 on the ext auth filter", func() {
						filters, err := extAuthzConfigGenerator.GenerateListenerExtAuthzConfig(nil, gloov1.UpstreamList{upstream})
						Expect(err).NotTo(HaveOccurred())
						Expect(filters).To(HaveLen(1))

						actualFilterConfig := filters[0]
						Expect(actualFilterConfig.GetTransportApiVersion()).To(Equal(envoycore.ApiVersion_V3))
					})
				})

				When("complete settings are provided", func() {

					BeforeEach(func() {
						usRef := upstream.Metadata.Ref()

						customTimeout := prototime.DurationToProto(500 * time.Millisecond)

						defaultSettings = &extauthv1.Settings{
							ExtauthzServerRef: usRef,
							RequestTimeout:    customTimeout,
							FailureModeAllow:  true,
							RequestBody: &extauthv1.BufferSettings{
								AllowPartialMessage: true,
								MaxRequestBytes:     54,
								PackAsBytes:         true,
							},
							ClearRouteCache: true,
							StatusOnError:   400,
						}

						expectedConfig = &envoyauth.ExtAuthz{
							TransportApiVersion:       envoycore.ApiVersion_V3,
							MetadataContextNamespaces: []string{JWTFilterName},
							Services: &envoyauth.ExtAuthz_GrpcService{
								GrpcService: &envoycore.GrpcService{
									Timeout: customTimeout,
									TargetSpecifier: &envoycore.GrpcService_EnvoyGrpc_{
										EnvoyGrpc: &envoycore.GrpcService_EnvoyGrpc{
											ClusterName: translator.UpstreamToClusterName(usRef),
										},
									},
								},
							},
							FailureModeAllow: true,
							WithRequestBody: &envoyauth.BufferSettings{
								AllowPartialMessage: true,
								MaxRequestBytes:     54,
								PackAsBytes:         true,
							},
							ClearRouteCache: true,
							StatusOnError:   &envoytype.HttpStatus{Code: envoytype.StatusCode_BadRequest},
						}
					})

					It("generates the expected configuration", func() {
						filters, err := extAuthzConfigGenerator.GenerateListenerExtAuthzConfig(nil, gloov1.UpstreamList{upstream})
						Expect(err).NotTo(HaveOccurred())
						Expect(filters).To(HaveLen(1))

						actualFilterConfig := filters[0]
						Expect(actualFilterConfig).To(matchers.MatchProto(expectedConfig))
					})
				})

				When("invalid settings are provided", func() {

					BeforeEach(func() {
						usRef := upstream.Metadata.Ref()

						defaultSettings = &extauthv1.Settings{
							ExtauthzServerRef: usRef,
							// This is the only thing that can go wrong in the BuildSingleHttpFilter function
							StatusOnError: 999,
						}
					})

					It("returns an error", func() {
						_, err := extAuthzConfigGenerator.GenerateListenerExtAuthzConfig(nil, gloov1.UpstreamList{upstream})
						Expect(err).To(HaveOccurred())
						Expect(err).To(HaveInErrorChain(InvalidStatusOnErrorErr(999)))
					})
				})

				When("an HTTP service is configured", func() {

					BeforeEach(func() {
						usRef := upstream.Metadata.Ref()
						defaultSettings = &extauthv1.Settings{
							ExtauthzServerRef: usRef,
							ServiceType: &extauthv1.Settings_HttpService{
								HttpService: &extauthv1.HttpService{
									PathPrefix: "/foo",
									Request: &extauthv1.HttpService_Request{
										AllowedHeaders:      []string{"allowed-header"},
										AllowedHeadersRegex: []string{"allowed-header-regex*"},
										HeadersToAdd:        map[string]string{"header": "add"},
									},
									Response: &extauthv1.HttpService_Response{
										AllowedClientHeaders:           []string{"allowed-client-header"},
										AllowedUpstreamHeaders:         []string{"allowed-upstream-header"},
										AllowedUpstreamHeadersToAppend: []string{"allowed-upstream-header-to-append"},
									},
								},
							},
						}

						expectedConfig = &envoyauth.ExtAuthz{
							TransportApiVersion:       envoycore.ApiVersion_V3,
							MetadataContextNamespaces: []string{JWTFilterName},
							Services: &envoyauth.ExtAuthz_HttpService{
								HttpService: &envoyauth.HttpService{
									AuthorizationRequest: &envoyauth.AuthorizationRequest{
										AllowedHeaders: &envoymatcher.ListStringMatcher{
											Patterns: []*envoymatcher.StringMatcher{{
												MatchPattern: &envoymatcher.StringMatcher_Exact{Exact: "allowed-header"},
											}, {
												MatchPattern: &envoymatcher.StringMatcher_SafeRegex{SafeRegex: &envoymatcher.RegexMatcher{
													Regex: "allowed-header-regex*",
													EngineType: &envoymatcher.RegexMatcher_GoogleRe2{
														GoogleRe2: &envoymatcher.RegexMatcher_GoogleRE2{},
													},
												}},
											}},
										},
										HeadersToAdd: []*envoycore.HeaderValue{{
											Key:   "header",
											Value: "add",
										}},
									},
									AuthorizationResponse: &envoyauth.AuthorizationResponse{
										AllowedClientHeaders: &envoymatcher.ListStringMatcher{
											Patterns: []*envoymatcher.StringMatcher{{
												MatchPattern: &envoymatcher.StringMatcher_Exact{Exact: "allowed-client-header"},
											}},
										},
										AllowedUpstreamHeaders: &envoymatcher.ListStringMatcher{
											Patterns: []*envoymatcher.StringMatcher{{
												MatchPattern: &envoymatcher.StringMatcher_Exact{Exact: "allowed-upstream-header"},
											}},
										},
										AllowedUpstreamHeadersToAppend: &envoymatcher.ListStringMatcher{
											Patterns: []*envoymatcher.StringMatcher{{
												MatchPattern: &envoymatcher.StringMatcher_Exact{Exact: "allowed-upstream-header-to-append"},
											}},
										},
									},
									PathPrefix: "/foo",
									ServerUri: &envoycore.HttpUri{
										Timeout: DefaultTimeout,
										Uri:     HttpServerUri,
										HttpUpstreamType: &envoycore.HttpUri_Cluster{
											Cluster: translator.UpstreamToClusterName(usRef),
										},
									},
								},
							},
						}
					})

					It("uses the expected defaults", func() {
						filters, err := extAuthzConfigGenerator.GenerateListenerExtAuthzConfig(nil, gloov1.UpstreamList{upstream})
						Expect(err).NotTo(HaveOccurred())
						Expect(filters).To(HaveLen(1))

						actualFilterConfig := filters[0]
						Expect(actualFilterConfig).To(matchers.MatchProto(expectedConfig))
					})
				})
				When("an GRPC service is configured", func() {

					BeforeEach(func() {
						usRef := upstream.Metadata.Ref()
						authority := "something.com"
						defaultSettings = &extauthv1.Settings{
							ExtauthzServerRef: usRef,
							ServiceType: &extauthv1.Settings_GrpcService{
								GrpcService: &extauthv1.GrpcService{
									Authority: authority,
								},
							},
						}

						expectedConfig = &envoyauth.ExtAuthz{
							TransportApiVersion:       envoycore.ApiVersion_V3,
							MetadataContextNamespaces: []string{JWTFilterName},
							Services: &envoyauth.ExtAuthz_GrpcService{
								GrpcService: &envoycore.GrpcService{
									Timeout: DefaultTimeout,
									TargetSpecifier: &envoycore.GrpcService_EnvoyGrpc_{
										EnvoyGrpc: &envoycore.GrpcService_EnvoyGrpc{
											ClusterName: translator.UpstreamToClusterName(usRef),
											Authority:   authority,
										},
									},
								},
							},
						}
					})

					It("uses the expected defaults", func() {
						filters, err := extAuthzConfigGenerator.GenerateListenerExtAuthzConfig(nil, gloov1.UpstreamList{upstream})
						Expect(err).NotTo(HaveOccurred())
						Expect(filters).To(HaveLen(1))

						actualFilterConfig := filters[0]
						Expect(actualFilterConfig).To(matchers.MatchProto(expectedConfig))
					})
				})
			})

		})

	})

	// TODO (sam-heilbron) - Test other methods of ExtAuthzConfigGenerator. They were not tested previously

})
