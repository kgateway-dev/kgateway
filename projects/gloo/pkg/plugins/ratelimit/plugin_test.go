package ratelimit_test

import (
	"time"

	"github.com/envoyproxy/go-control-plane/pkg/conversion"
	"github.com/solo-io/gloo/pkg/utils/gogoutils"
	extauthv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/extauth"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/ratelimit"

	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoyratelimit "github.com/envoyproxy/go-control-plane/envoy/config/filter/http/rate_limit/v2"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	rlconfig "github.com/envoyproxy/go-control-plane/envoy/config/ratelimit/v2"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	ratelimitpb "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/ratelimit"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

var _ = Describe("Plugin", func() {
	var (
		rlSettings *ratelimitpb.Settings
		initParams plugins.InitParams
		params     plugins.Params
		rlPlugin   *Plugin
		ref        core.ResourceRef
	)

	BeforeEach(func() {
		rlPlugin = NewPlugin()
		ref = core.ResourceRef{
			Name:      "test",
			Namespace: "test",
		}

		rlSettings = &ratelimitpb.Settings{
			RatelimitServerRef: &ref,
		}
		initParams = plugins.InitParams{
			Settings: &gloov1.Settings{},
		}
		params.Snapshot = &gloov1.ApiSnapshot{}
	})

	JustBeforeEach(func() {
		initParams.Settings = &gloov1.Settings{RatelimitServer: rlSettings}
		rlPlugin.Init(initParams)
	})
	It("should fave fail mode deny off by default", func() {

		filters, err := rlPlugin.HttpFilters(params, nil)
		Expect(err).NotTo(HaveOccurred())

		Expect(filters).To(HaveLen(1))
		for _, f := range filters {
			cfg := getConfig(f.HttpFilter)
			Expect(cfg.FailureModeDeny).To(BeFalse())
		}

		hundredms := DefaultTimeout
		expectedConfig := &envoyratelimit.RateLimit{
			Domain:          "custom",
			FailureModeDeny: false,
			Stage:           1,
			Timeout:         gogoutils.DurationStdToProto(&hundredms),
			RequestType:     "both",
			RateLimitService: &rlconfig.RateLimitServiceConfig{
				GrpcService: &envoycore.GrpcService{TargetSpecifier: &envoycore.GrpcService_EnvoyGrpc_{
					EnvoyGrpc: &envoycore.GrpcService_EnvoyGrpc{
						ClusterName: translator.UpstreamToClusterName(ref),
					},
				}},
			},
		}

		cfg := getConfig(filters[0].HttpFilter)
		Expect(cfg).To(BeEquivalentTo(expectedConfig))
	})

	It("default timeout is 100ms", func() {
		filters, err := rlPlugin.HttpFilters(params, nil)
		Expect(err).NotTo(HaveOccurred())
		timeout := DefaultTimeout
		Expect(filters).To(HaveLen(1))
		for _, f := range filters {
			cfg := getConfig(f.HttpFilter)
			Expect(*cfg.Timeout).To(Equal(timeout))
		}
	})

	Context("rate limit ordering", func() {
		JustBeforeEach(func() {
			timeout := time.Second
			params.Snapshot.Upstreams = []*gloov1.Upstream{
				{
					Metadata: core.Metadata{
						Name:      "extauth-upstream",
						Namespace: "ns",
					},
				},
			}
			rlSettings.RateLimitBeforeAuth = true
			initParams.Settings = &gloov1.Settings{
				RatelimitServer: rlSettings,
				Extauth: &extauthv1.Settings{
					ExtauthzServerRef: &core.ResourceRef{
						Name:      "extauth-upstream",
						Namespace: "ns",
					},
					RequestTimeout: &timeout,
				},
			}
			rlPlugin.Init(initParams)
		})

		It("should be ordered before ext auth", func() {
			filters, err := rlPlugin.HttpFilters(params, nil)
			Expect(err).NotTo(HaveOccurred(), "Should be able to build rate limit filters")
			Expect(filters).To(HaveLen(1), "Should only have created one custom filter")

			customStagedFilter := filters[0]
			extAuthPlugin := extauth.NewCustomAuthPlugin()
			err = extAuthPlugin.Init(initParams)
			Expect(err).NotTo(HaveOccurred(), "Should be able to initialize the ext auth plugin")
			extAuthFilters, err := extAuthPlugin.HttpFilters(params, nil)
			Expect(err).NotTo(HaveOccurred(), "Should be able to build the ext auth filters")
			Expect(extAuthFilters).NotTo(BeEmpty(), "Should have actually created more than zero ext auth filters")

			for _, extAuthFilter := range extAuthFilters {
				Expect(plugins.FilterStageComparison(extAuthFilter.Stage, customStagedFilter.Stage)).To(Equal(1), "Ext auth filters should occur after rate limiting")
			}
		})
	})

	Context("fail mode deny", func() {

		BeforeEach(func() {
			rlSettings.DenyOnFail = true
		})

		It("should fave fail mode deny on", func() {
			filters, err := rlPlugin.HttpFilters(params, nil)
			Expect(err).NotTo(HaveOccurred())

			Expect(filters).To(HaveLen(1))
			for _, f := range filters {
				cfg := getConfig(f.HttpFilter)
				Expect(cfg.FailureModeDeny).To(BeTrue())
			}
		})
	})

	Context("timeout", func() {

		BeforeEach(func() {
			s := time.Second
			rlSettings.RequestTimeout = &s
		})

		It("should custom timeout set", func() {
			filters, err := rlPlugin.HttpFilters(params, nil)
			Expect(err).NotTo(HaveOccurred())

			Expect(filters).To(HaveLen(1))
			for _, f := range filters {
				cfg := getConfig(f.HttpFilter)
				Expect(*cfg.Timeout).To(Equal(time.Second))
			}
		})
	})

})

func getConfig(f *envoyhttp.HttpFilter) *envoyratelimit.RateLimit {
	cfg := f.GetConfig()
	rcfg := new(envoyratelimit.RateLimit)
	err := conversion.StructToMessage(cfg, rcfg)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return rcfg
}
