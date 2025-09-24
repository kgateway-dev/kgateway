package agentgatewaysyncer

import (
	"testing"

	"github.com/agentgateway/agentgateway/go/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	agwir "github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/translator"
)

func TestBuildAgwFilters(t *testing.T) {
	testCases := []struct {
		name            string
		inputFilters    []gwv1.HTTPRouteFilter
		expectedFilters []*api.RouteFilter
		expectedError   bool
	}{
		{
			name: "Request header modifier filter",
			inputFilters: []gwv1.HTTPRouteFilter{
				{
					Type: gwv1.HTTPRouteFilterRequestHeaderModifier,
					RequestHeaderModifier: &gwv1.HTTPHeaderFilter{
						Set: []gwv1.HTTPHeader{
							{Name: "X-Custom-Header", Value: "custom-value"},
						},
						Add: []gwv1.HTTPHeader{
							{Name: "X-Added-Header", Value: "added-value"},
						},
						Remove: []string{"X-Remove-Header"},
					},
				},
			},
			expectedFilters: []*api.RouteFilter{
				{
					Kind: &api.RouteFilter_RequestHeaderModifier{
						RequestHeaderModifier: &api.HeaderModifier{
							Set: []*api.Header{
								{Name: "X-Custom-Header", Value: "custom-value"},
							},
							Add: []*api.Header{
								{Name: "X-Added-Header", Value: "added-value"},
							},
							Remove: []string{"X-Remove-Header"},
						},
					},
				},
			},
			expectedError: false,
		},
		{
			name: "Response header modifier filter",
			inputFilters: []gwv1.HTTPRouteFilter{
				{
					Type: gwv1.HTTPRouteFilterResponseHeaderModifier,
					ResponseHeaderModifier: &gwv1.HTTPHeaderFilter{
						Set: []gwv1.HTTPHeader{
							{Name: "X-Response-Header", Value: "response-value"},
						},
					},
				},
			},
			expectedFilters: []*api.RouteFilter{
				{
					Kind: &api.RouteFilter_ResponseHeaderModifier{
						ResponseHeaderModifier: &api.HeaderModifier{
							Set: []*api.Header{
								{Name: "X-Response-Header", Value: "response-value"},
							},
						},
					},
				},
			},
			expectedError: false,
		},
		{
			name: "Request redirect filter",
			inputFilters: []gwv1.HTTPRouteFilter{
				{
					Type: gwv1.HTTPRouteFilterRequestRedirect,
					RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
						Scheme:     ptr.To("https"),
						Hostname:   ptr.To(gwv1.PreciseHostname("secure.example.com")),
						StatusCode: ptr.To(301),
					},
				},
			},
			expectedFilters: []*api.RouteFilter{
				{
					Kind: &api.RouteFilter_RequestRedirect{
						RequestRedirect: &api.RequestRedirect{
							Scheme: "https",
							Host:   "secure.example.com",
							Status: 301,
						},
					},
				},
			},
			expectedError: false,
		},
		{
			name: "URL rewrite filter",
			inputFilters: []gwv1.HTTPRouteFilter{
				{
					Type: gwv1.HTTPRouteFilterURLRewrite,
					URLRewrite: &gwv1.HTTPURLRewriteFilter{
						Path: &gwv1.HTTPPathModifier{
							Type:               gwv1.PrefixMatchHTTPPathModifier,
							ReplacePrefixMatch: ptr.To("/new-prefix"),
						},
					},
				},
			},
			expectedFilters: []*api.RouteFilter{
				{
					Kind: &api.RouteFilter_UrlRewrite{
						UrlRewrite: &api.UrlRewrite{
							Path: &api.UrlRewrite_Prefix{
								Prefix: "/new-prefix",
							},
						},
					},
				},
			},
			expectedError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := translator.RouteContext{
				RouteContextInputs: translator.RouteContextInputs{
					Grants:       translator.ReferenceGrants{},
					RouteParents: translator.RouteParents{},
				},
			}

			result, err := translator.BuildAgwFilters(ctx, "default", tc.inputFilters)

			if tc.expectedError {
				assert.NotNil(t, err)
				return
			}

			assert.Nil(t, err)
			require.Equal(t, len(tc.expectedFilters), len(result))

			for i, expectedFilter := range tc.expectedFilters {
				actualFilter := result[i]

				// Compare filter types
				switch expectedFilter.Kind.(type) {
				case *api.RouteFilter_RequestHeaderModifier:
					assert.IsType(t, &api.RouteFilter_RequestHeaderModifier{}, actualFilter.Kind)
				case *api.RouteFilter_ResponseHeaderModifier:
					assert.IsType(t, &api.RouteFilter_ResponseHeaderModifier{}, actualFilter.Kind)
				case *api.RouteFilter_RequestRedirect:
					assert.IsType(t, &api.RouteFilter_RequestRedirect{}, actualFilter.Kind)
				case *api.RouteFilter_UrlRewrite:
					assert.IsType(t, &api.RouteFilter_UrlRewrite{}, actualFilter.Kind)
				}
			}
		})
	}
}

func TestGetProtocolAndTLSConfig(t *testing.T) {
	testCases := []struct {
		name          string
		gateway       translator.GatewayListener
		expectedProto api.Protocol
		expectedTLS   *api.TLSConfig
		expectedOk    bool
	}{
		{
			name: "HTTP protocol",
			gateway: translator.GatewayListener{
				ParentInfo: translator.ParentInfo{
					Protocol: gwv1.HTTPProtocolType,
				},
				TLSInfo: nil,
			},
			expectedProto: api.Protocol_HTTP,
			expectedTLS:   nil,
			expectedOk:    true,
		},
		{
			name: "HTTPS protocol with TLS",
			gateway: translator.GatewayListener{
				ParentInfo: translator.ParentInfo{
					Protocol: gwv1.HTTPSProtocolType,
				},
				TLSInfo: &translator.TLSInfo{
					Cert: []byte("cert-data"),
					Key:  []byte("key-data"),
				},
			},
			expectedProto: api.Protocol_HTTPS,
			expectedTLS: &api.TLSConfig{
				Cert:       []byte("cert-data"),
				PrivateKey: []byte("key-data"),
			},
			expectedOk: true,
		},
		{
			name: "HTTPS protocol without TLS (should fail)",
			gateway: translator.GatewayListener{
				ParentInfo: translator.ParentInfo{
					Protocol: gwv1.HTTPSProtocolType,
				},
				TLSInfo: nil,
			},
			expectedProto: api.Protocol_HTTPS,
			expectedTLS:   nil,
			expectedOk:    false,
		},
		{
			name: "TCP protocol",
			gateway: translator.GatewayListener{
				ParentInfo: translator.ParentInfo{
					Protocol: gwv1.TCPProtocolType,
				},
				TLSInfo: nil,
			},
			expectedProto: api.Protocol_TCP,
			expectedTLS:   nil,
			expectedOk:    true,
		},
		{
			name: "TLS protocol with TLS",
			gateway: translator.GatewayListener{
				ParentInfo: translator.ParentInfo{
					Protocol: gwv1.TLSProtocolType,
				},
				TLSInfo: &translator.TLSInfo{
					Cert: []byte("tls-cert"),
					Key:  []byte("tls-key"),
				},
			},
			expectedProto: api.Protocol_TLS,
			expectedTLS: &api.TLSConfig{
				Cert:       []byte("tls-cert"),
				PrivateKey: []byte("tls-key"),
			},
			expectedOk: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			syncer := &Syncer{}

			proto, tlsConfig, ok := syncer.getProtocolAndTLSConfig(tc.gateway)

			assert.Equal(t, tc.expectedOk, ok)
			if tc.expectedOk {
				assert.Equal(t, tc.expectedProto, proto)
				if tc.expectedTLS != nil {
					require.NotNil(t, tlsConfig)
					assert.Equal(t, tc.expectedTLS.Cert, tlsConfig.Cert)
					assert.Equal(t, tc.expectedTLS.PrivateKey, tlsConfig.PrivateKey)
				} else {
					assert.Nil(t, tlsConfig)
				}
			}
		})
	}
}

func TestAgwResourcesForGatewayEquals(t *testing.T) {
	testCases := []struct {
		name      string
		resource1 agwir.AgwResourcesForGateway
		resource2 agwir.AgwResourcesForGateway
		expected  bool
	}{
		{
			name: "Equal bind resources",
			resource1: agwir.AgwResourcesForGateway{
				Resources: []*api.Resource{{
					Kind: &api.Resource_Bind{
						Bind: &api.Bind{
							Key:  "test-key",
							Port: 8080,
						},
					},
				}},
				Gateway: types.NamespacedName{Name: "test", Namespace: "default"},
			},
			resource2: agwir.AgwResourcesForGateway{
				Resources: []*api.Resource{{
					Kind: &api.Resource_Bind{
						Bind: &api.Bind{
							Key:  "test-key",
							Port: 8080,
						},
					},
				}},
				Gateway: types.NamespacedName{Name: "test", Namespace: "default"},
			},
			expected: true,
		},
		{
			name: "Different gateway",
			resource1: agwir.AgwResourcesForGateway{
				Resources: []*api.Resource{{
					Kind: &api.Resource_Bind{
						Bind: &api.Bind{
							Key:  "test-key",
							Port: 8080,
						},
					},
				}},
				Gateway: types.NamespacedName{Name: "test", Namespace: "default"},
			},
			resource2: agwir.AgwResourcesForGateway{
				Resources: []*api.Resource{{
					Kind: &api.Resource_Bind{
						Bind: &api.Bind{
							Key:  "test-key",
							Port: 8080,
						},
					},
				}},
				Gateway: types.NamespacedName{Name: "other", Namespace: "default"},
			},
			expected: false,
		},
		{
			name: "Different resource port",
			resource1: agwir.AgwResourcesForGateway{
				Resources: []*api.Resource{{
					Kind: &api.Resource_Bind{
						Bind: &api.Bind{
							Key:  "test-key",
							Port: 8080,
						},
					},
				}},
				Gateway: types.NamespacedName{Name: "test", Namespace: "default"},
			},
			resource2: agwir.AgwResourcesForGateway{
				Resources: []*api.Resource{{
					Kind: &api.Resource_Bind{
						Bind: &api.Bind{
							Key:  "test-key",
							Port: 9090,
						},
					},
				}},
				Gateway: types.NamespacedName{Name: "test", Namespace: "default"},
			},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := proto.Equal(tc.resource1.Resources[0], tc.resource2.Resources[0]) && tc.resource1.Gateway == tc.resource2.Gateway
			assert.Equal(t, tc.expected, result)
		})
	}
}
