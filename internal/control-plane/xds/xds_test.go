package xds_test

import (
	"time"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoylistener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	envoyhttpconnectionmanager "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	"github.com/envoyproxy/go-control-plane/pkg/cache"
	"github.com/envoyproxy/go-control-plane/pkg/util"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	bootstrap "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v2"
	. "github.com/solo-io/gloo/internal/control-plane/xds"
	"github.com/solo-io/gloo/pkg/log"
	. "github.com/solo-io/gloo/test/helpers"
	"google.golang.org/grpc"
)

var _ = Describe("Xds", func() {
	var (
		srv *grpc.Server

		routeConfigName = "xds-test-route-config"
		listenerName    = "xds-test-listener"
	)
	BeforeEach(func() {
		cache, grpcSrv, err := RunXDS(8081)
		Must(err)
		srv = grpcSrv

		snapshot, err := createSnapshot(routeConfigName, listenerName)
		if err != nil {
			log.Fatalf(err.Error())
		}
		cache.SetSnapshot(NodeKey, snapshot)
	})
	Describe("RunXDS Server", func() {
		It("successfully bootstraps the envoy proxy", func() {
			Eventually(envoyInstance.Logs, time.Second*30).Should(ContainSubstring("lds: add/update listener '" + listenerName))
		})
	})
})

func createSnapshot(routeConfigName, listenerName string) (cache.Snapshot, error) {
	var (
		endpoints []cache.Resource
		clusters  []cache.Resource
		routes    []cache.Resource
	)
	adsSource := envoycore.ConfigSource{
		ConfigSourceSpecifier: &envoycore.ConfigSource_Ads{
			Ads: &envoycore.AggregatedConfigSource{},
		},
	}
	manager := &envoyhttpconnectionmanager.HttpConnectionManager{
		CodecType:  envoyhttpconnectionmanager.AUTO,
		StatPrefix: "http",
		RouteSpecifier: &envoyhttpconnectionmanager.HttpConnectionManager_Rds{
			Rds: &envoyhttpconnectionmanager.Rds{
				ConfigSource:    adsSource,
				RouteConfigName: routeConfigName,
			},
		},
	}
	pbst, err := util.MessageToStruct(manager)
	if err != nil {
		panic(err)
	}

	listener := &v2.Listener{
		Name: listenerName,
		Address: envoycore.Address{
			Address: &envoycore.Address_SocketAddress{
				SocketAddress: &envoycore.SocketAddress{
					Protocol: envoycore.TCP,
					Address:  "0.0.0.0",
					PortSpecifier: &envoycore.SocketAddress_PortValue{
						PortValue: 1234,
					},
				},
			},
		},
		FilterChains: []envoylistener.FilterChain{{
			Filters: []envoylistener.Filter{{
				Name:   "envoy.http_connection_manager",
				Config: pbst,
			}},
		}},
	}

	listeners := []cache.Resource{
		listener,
	}
	return cache.NewSnapshot("foo", endpoints, clusters, routes, listeners), nil
}

// MakeBootstrap creates a bootstrap envoy configuration
func makeBootstrap(ads bool, controlPort, adminPort uint32) *bootstrap.Bootstrap {
	source := &envoycore.ApiConfigSource{
		ApiType:      envoycore.ApiConfigSource_GRPC,
		ClusterNames: []string{"xds_cluster"},
	}

	var dynamic *bootstrap.Bootstrap_DynamicResources
	if ads {
		dynamic = &bootstrap.Bootstrap_DynamicResources{
			LdsConfig: &envoycore.ConfigSource{
				ConfigSourceSpecifier: &envoycore.ConfigSource_Ads{Ads: &envoycore.AggregatedConfigSource{}},
			},
			CdsConfig: &envoycore.ConfigSource{
				ConfigSourceSpecifier: &envoycore.ConfigSource_Ads{Ads: &envoycore.AggregatedConfigSource{}},
			},
			AdsConfig: source,
		}
	} else {
		dynamic = &bootstrap.Bootstrap_DynamicResources{
			LdsConfig: &envoycore.ConfigSource{
				ConfigSourceSpecifier: &envoycore.ConfigSource_ApiConfigSource{ApiConfigSource: source},
			},
			CdsConfig: &envoycore.ConfigSource{
				ConfigSourceSpecifier: &envoycore.ConfigSource_ApiConfigSource{ApiConfigSource: source},
			},
		}
	}

	return &bootstrap.Bootstrap{
		Node: &envoycore.Node{
			Id:      "test-id",
			Cluster: "test-cluster",
		},
		Admin: bootstrap.Admin{
			AccessLogPath: "/dev/null",
			Address: envoycore.Address{
				Address: &envoycore.Address_SocketAddress{
					SocketAddress: &envoycore.SocketAddress{
						Address: "127.0.0.1",
						PortSpecifier: &envoycore.SocketAddress_PortValue{
							PortValue: adminPort,
						},
					},
				},
			},
		},
		StaticResources: &bootstrap.Bootstrap_StaticResources{
			Clusters: []v2.Cluster{{
				Name:           "xds_cluster",
				ConnectTimeout: 5 * time.Second,
				Type:           v2.Cluster_STATIC,
				Hosts: []*envoycore.Address{{
					Address: &envoycore.Address_SocketAddress{
						SocketAddress: &envoycore.SocketAddress{
							Address: "127.0.0.1",
							PortSpecifier: &envoycore.SocketAddress_PortValue{
								PortValue: controlPort,
							},
						},
					},
				}},
				LbPolicy:             v2.Cluster_ROUND_ROBIN,
				Http2ProtocolOptions: &envoycore.Http2ProtocolOptions{},
			}},
		},
		DynamicResources: dynamic,
	}
}
