package services

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"

	static_plugin_gloo "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/faultinjection"
)

// for each service we will create an envoy config and run it with
// env vars describing the location of the upstreams
// and run envoy along with them.

type Service struct {
	Name    string
	Process string

	// these will be filled by Start
	Port     uint32
	MeshPort uint32
}

var envoyPort = 8300

type QuoteUnquoteMesh struct {
	clients       TestClients
	portfor       func(i, j int) uint32
	proxies       []*gloov1.Proxy
	upstreams     []gloov1.Upstream
	meshupstreams []gloov1.Upstream
	Envoys        []*EnvoyInstance
}

func proxyForService(i int) string {
	return fmt.Sprintf("proxy-%d", i)
}

func (m *QuoteUnquoteMesh) getSelfListener(svcIndex int) *gloov1.Listener {
	return &gloov1.Listener{
		Name:        "listener-self",
		BindAddress: "127.0.0.1",
		BindPort:    m.portfor(svcIndex, svcIndex),
		ListenerType: &gloov1.Listener_HttpListener{
			HttpListener: &gloov1.HttpListener{
				VirtualHosts: []*gloov1.VirtualHost{{
					Name:    "virt-self",
					Domains: []string{"*"},
					Routes: []*gloov1.Route{{
						Action: &gloov1.Route_RouteAction{
							RouteAction: &gloov1.RouteAction{
								Destination: &gloov1.RouteAction_Single{
									Single: &gloov1.Destination{
										DestinationType: &gloov1.Destination_Upstream{
											Upstream: m.upstreams[svcIndex].GetMetadata().Ref(),
										},
									},
								},
							},
						},
					}},
				}},
			},
		},
	}
}

func (m *QuoteUnquoteMesh) AddFault(svcIndex int, percent float32) {
	// the first listener is the self listener
	l := m.getSelfListener(svcIndex)

	route := l.GetListenerType().(*gloov1.Listener_HttpListener).HttpListener.GetVirtualHosts()[0].GetRoutes()[0]
	route.Options = &gloov1.RouteOptions{
		Faults: &faultinjection.RouteFaults{
			Abort: &faultinjection.RouteAbort{
				HttpStatus: http.StatusServiceUnavailable,
				Percentage: percent,
			},
		},
	}

	m.UpdateSelfListener(svcIndex, l)
}

func (m *QuoteUnquoteMesh) RemoveFault(svcIndex int) {
	// the first listener is the self listener
	l := m.getSelfListener(svcIndex)
	m.UpdateSelfListener(svcIndex, l)
}

func (m *QuoteUnquoteMesh) UpdateSelfListener(svcIndex int, l *gloov1.Listener) {

	var ropts clients.ReadOpts
	proxy := m.proxies[svcIndex]

	existingproxy, err := m.clients.ProxyClient.Read(proxy.GetMetadata().GetNamespace(), proxy.GetMetadata().GetName(), ropts)
	Expect(err).NotTo(HaveOccurred())

	existingproxy.GetListeners()[0] = l
	var opts clients.WriteOpts
	opts.OverwriteExisting = true
	_, err = m.clients.ProxyClient.Write(existingproxy, opts)
	Expect(err).NotTo(HaveOccurred())
}

func (m *QuoteUnquoteMesh) Start(ef *EnvoyFactory, testClients TestClients, services []*Service) {
	m.clients = testClients

	numServices := len(services)

	m.portfor = func(i, j int) uint32 {
		return uint32(envoyPort + i*numServices + j)
	}

	envoyPort += numServices * numServices

	// fill in ports
	for i, svc := range services {
		svc.Port = uint32(7100 + i)
		svc.MeshPort = m.portfor(i, i)
	}

	for i, s := range services {

		u := gloov1.Upstream{
			Metadata: &core.Metadata{
				Name:      fmt.Sprintf("local-%d", i),
				Namespace: "default",
			},
			UpstreamType: &gloov1.Upstream_Static{
				Static: &static_plugin_gloo.UpstreamSpec{
					Hosts: []*static_plugin_gloo.Host{{
						Addr: "localhost",
						Port: s.Port,
					}},
				},
			},
		}
		m.upstreams = append(m.upstreams, u)
	}

	for i := range services {

		u := gloov1.Upstream{
			Metadata: &core.Metadata{
				Name:      fmt.Sprintf("mesh-local-%d", i),
				Namespace: "default",
			},
			UpstreamType: &gloov1.Upstream_Static{
				Static: &static_plugin_gloo.UpstreamSpec{
					Hosts: []*static_plugin_gloo.Host{{
						Addr: "localhost",
						Port: m.portfor(i, i),
					}},
				},
			},
		}
		m.meshupstreams = append(m.meshupstreams, u)
	}

	for i := range services {
		// proxy object for service i
		proxy := &gloov1.Proxy{
			Metadata: &core.Metadata{
				Name:      proxyForService(i),
				Namespace: "default",
			},
			Listeners: []*gloov1.Listener{m.getSelfListener(i)},
		}
		for j := range services {
			if i == j {
				continue
			}

			proxy.Listeners = append(proxy.GetListeners(), &gloov1.Listener{
				Name:        fmt.Sprintf("listener-%d-to-%d", i, j),
				BindAddress: "127.0.0.1",
				BindPort:    m.portfor(i, j),
				ListenerType: &gloov1.Listener_HttpListener{
					HttpListener: &gloov1.HttpListener{
						VirtualHosts: []*gloov1.VirtualHost{{
							Name:    fmt.Sprintf("virt-%d-to-%d", i, j),
							Domains: []string{"*"},
							Routes: []*gloov1.Route{{
								Action: &gloov1.Route_RouteAction{
									RouteAction: &gloov1.RouteAction{
										Destination: &gloov1.RouteAction_Single{
											Single: &gloov1.Destination{
												DestinationType: &gloov1.Destination_Upstream{
													Upstream: m.meshupstreams[j].GetMetadata().Ref(),
												},
											},
										},
									},
								},
							}},
						}},
					},
				},
			})

		}
		m.proxies = append(m.proxies, proxy)
	}

	var opts clients.WriteOpts

	for _, u := range m.upstreams {
		_, err := m.clients.UpstreamClient.Write(&u, opts)
		Expect(err).NotTo(HaveOccurred())

	}
	for _, u := range m.meshupstreams {
		_, err := m.clients.UpstreamClient.Write(&u, opts)
		Expect(err).NotTo(HaveOccurred())

	}
	for _, p := range m.proxies {
		_, err := m.clients.ProxyClient.Write(p, opts)
		Expect(err).NotTo(HaveOccurred())

	}

	// now start all the envoys

	for _, p := range m.proxies {
		ei, err := ef.NewEnvoyInstance()
		Expect(err).NotTo(HaveOccurred())

		ei.RunWithRole(p.GetMetadata().GetNamespace()+"~"+p.GetMetadata().GetName(), m.clients.GlooPort)
		m.Envoys = append(m.Envoys, ei)
	}

	// now start all the services!
	var cmds []*exec.Cmd
	for i, svc := range services {
		var envvars []string
		envvars = append(envvars, "SELF="+fmt.Sprintf("%d", i))
		envvars = append(envvars, "TOTAL="+fmt.Sprintf("%d", numServices))
		envvars = append(envvars, "PORT="+fmt.Sprintf("%d", svc.Port))
		for j := 0; j < numServices; j++ {
			if i == j {
				// help services error if they have a bug.
				envvars = append(envvars, fmt.Sprintf("SVC%d=0", j))
			} else {
				envvars = append(envvars, fmt.Sprintf("SVC%d=%d", j, m.portfor(i, j)))
			}
		}
		cmd := exec.Command(svc.Process)
		cmd.Env = append(os.Environ(), envvars...)

		_, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred())

		cmds = append(cmds, cmd)
	}
	// wait for services to die so we exit
	// for _, cmd := range cmds {
	// 	cmd.Wait()
	// }
}
