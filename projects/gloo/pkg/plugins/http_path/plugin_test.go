package http_path

import (
	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	wrapperspb "github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1static "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("http_path plugin", func() {
	var (
		p               *plugin
		params          plugins.Params
		upstream        *v1.Upstream
		upstreamSpec    *v1static.UpstreamSpec
		out             *envoy_config_cluster_v3.Cluster
		baseHealthCheck *envoy_config_core_v3.HealthCheck_HttpHealthCheck
	)

	BeforeEach(func() {
			p = NewPlugin()
			out = new(envoy_config_cluster_v3.Cluster)
			baseHealthCheck = &envoy_config_core_v3.HealthCheck_HttpHealthCheck{
				Host:                   "foo",
				Path:                   "/health",
				CodecClientType:        envoy_type_v3.CodecClientType_HTTP2,
				RequestHeadersToRemove: []string{"test"},
				RequestHeadersToAdd: []*envoy_config_core_v3.HeaderValueOption{
					&envoy_config_core_v3.HeaderValueOption{
						Header: &envoy_config_core_v3.HeaderValue{Key: "key", Value: "value"},
						Append: &wrapperspb.BoolValue{Value: true},
					},
				},
			}
			out.HealthChecks = []*envoy_config_core_v3.HealthCheck{
				{
					HealthChecker: &envoy_config_core_v3.HealthCheck_HttpHealthCheck_{

						HttpHealthCheck: baseHealthCheck,
					},
				},
			}

			p.Init(plugins.InitParams{})
			upstreamSpec = &v1static.UpstreamSpec{
				Hosts: []*v1static.Host{{
					Addr: "localhost",
					Port: 1234,
					HealthCheckConfig: &v1static.Host_HealthCheckConfig{
						Path: "/foo",
					},
				}},
			}
			upstream = &v1.Upstream{
				Metadata: &core.Metadata{
					Name:      "extauth-server",
					Namespace: "default",
				},
				UpstreamType: &v1.Upstream_Static{
					Static: upstreamSpec,
				},
			}


	})

	It("should not process upstream if http_path config is nil", func() {
		err := p.ProcessUpstream(plugins.Params{}, &v1.Upstream{}, nil)
		Expect(err).NotTo(HaveOccurred())
	})


	It("will err if http_path is configured on process upstream", func() {
		err := p.ProcessUpstream(params, upstream, out)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal(errEnterpriseOnly))
	})

})

