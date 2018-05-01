package xds_test

import (
	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoylistener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	envoyhttpconnectionmanager "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/envoyproxy/go-control-plane/pkg/cache"
	"github.com/envoyproxy/go-control-plane/pkg/util"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"google.golang.org/grpc"
	"time"
	. "github.com/solo-io/gloo/internal/control-plane/xds"
	"net/http"
	"io/ioutil"
)

var _ = Describe("Xds", func() {
	var (
		srv *grpc.Server

		routeConfigName = "xds-test-route-config"
		listenerName    = "xds-test-listener"
		nodeGroup       = "valid-group"
		badNodeSnapshot = BadNodeSnapshot("0.0.0.0", 1234)
	)
	var _ = BeforeEach(func() {
		var err error
		envoyInstance, err = envoyFactory.NewEnvoyInstance()
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if envoyInstance != nil {
			envoyInstance.Clean()
		}
	})

	Describe("RunXDS Server", func() {
		Context("with invalid node name", func() {
			BeforeEach(func() {
				err := envoyInstance.Run()
				Expect(err).NotTo(HaveOccurred())
				_, grpcSrv, err := RunXDS(8081, badNodeSnapshot)
				Expect(err).NotTo(HaveOccurred())
				srv = grpcSrv
			})
			AfterEach(func() {
				srv.Stop()
			})
			It("successfully bootstraps the envoy proxy", func() {
				Eventually(envoyInstance.Logs, time.Second*30).Should(ContainSubstring("lds: add/update listener 'listener-for-invalid-envoy'"))
			})
			It("has a 500 route with body 'Invalid Envoy Bootstrap Configuration. Please refer to Gloo documentation https://gloo.solo.io/'", func() {
				Eventually(func() error {
					_, err := http.Get("http://" + envoyInstance.LocalAddr() + ":1234/")
					return err
				}).Should(Not(HaveOccurred()))
				res, err := http.Get("http://" + envoyInstance.LocalAddr() + ":1234/")
				Expect(err).NotTo(HaveOccurred())
				Expect(res.StatusCode).To(Equal(500))
				Expect(res.Body).NotTo(BeNil())
				b, err := ioutil.ReadAll(res.Body)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(b)).To(Equal("Invalid Envoy Bootstrap Configuration. Please refer to Gloo documentation https://gloo.solo.io/"))
			})
		})
		Context("with valid node name", func() {
			BeforeEach(func() {
				err := envoyInstance.RunWithId(nodeGroup + "~12345")
				Expect(err).NotTo(HaveOccurred())
				cache, grpcSrv, err := RunXDS(8081, badNodeSnapshot)
				Expect(err).NotTo(HaveOccurred())
				srv = grpcSrv
				snapshot, err := createSnapshot(routeConfigName, listenerName)
				Expect(err).NotTo(HaveOccurred())
				cache.SetSnapshot(nodeGroup, snapshot)
			})
			AfterEach(func() {
				srv.Stop()
			})
			It("successfully bootstraps the envoy proxy", func() {
				Eventually(envoyInstance.Logs, time.Second*30).Should(ContainSubstring("lds: add/update listener '" + listenerName))
			})
			It("works with the test route", func() {
				Eventually(func() error {
					_, err := http.Get("http://" + envoyInstance.LocalAddr() + ":1234/")
					return err
				}).Should(Not(HaveOccurred()))
				res, err := http.Get("http://" + envoyInstance.LocalAddr() + ":1234/")
				Expect(err).NotTo(HaveOccurred())
				Expect(res.StatusCode).To(Equal(200))
				Expect(res.Body).NotTo(BeNil())
				b, err := ioutil.ReadAll(res.Body)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(b)).To(Equal("the route worked. yay."))
			})
		})
	})
})

func createSnapshot(routeConfigName, listenerName string) (cache.Snapshot, error) {
	var (
		endpoints []cache.Resource
		clusters  []cache.Resource
	)
	routes := []cache.Resource{
		&envoyapi.RouteConfiguration{
			Name: routeConfigName,
			VirtualHosts: []envoyroute.VirtualHost{
				{
					Name:    "test-vhost",
					Domains: []string{"*"},
					Routes: []envoyroute.Route{
						{
							Match: envoyroute.RouteMatch{
								PathSpecifier: &envoyroute.RouteMatch_Prefix{
									Prefix: "/",
								},
							},
							Action: &envoyroute.Route_DirectResponse{
								DirectResponse: &envoyroute.DirectResponseAction{
									Status: 200,
									Body: &envoycore.DataSource{
										Specifier: &envoycore.DataSource_InlineString{
											InlineString: "the route worked. yay.",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
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
		HttpFilters: []*envoyhttpconnectionmanager.HttpFilter{
			{
				Name: "envoy.router",
			},
		},
	}
	pbst, err := util.MessageToStruct(manager)
	if err != nil {
		panic(err)
	}

	listener := &envoyapi.Listener{
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
			Filters: []envoylistener.Filter{
				{
					Name:   "envoy.http_connection_manager",
					Config: pbst,
				},
			},
		}},
	}

	listeners := []cache.Resource{
		listener,
	}
	return cache.NewSnapshot("1", endpoints, clusters, routes, listeners), nil
}
