package e2e_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"

	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/test/helpers"
	"github.com/solo-io/solo-kit/test/setup"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/test/v1helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	kubev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	kubecore "k8s.io/kubernetes/pkg/apis/core"
	"k8s.io/kubernetes/pkg/apis/core/validation"
)

var _ = Describe("Happypath", func() {

	var (
		ctx           context.Context
		cancel        context.CancelFunc
		testClients   services.TestClients
		envoyInstance *services.EnvoyInstance
		tu            *v1helpers.TestUpstream
		envoyPort     uint32
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		var err error
		envoyInstance, err = envoyFactory.NewEnvoyInstance()
		Expect(err).NotTo(HaveOccurred())

		tu = v1helpers.NewTestHttpUpstream(ctx, envoyInstance.LocalAddr())
		envoyPort = 8080
	})

	AfterEach(func() {
		if envoyInstance != nil {
			envoyInstance.Clean()
		}
		cancel()
	})

	TestUpstremReachable := func() {

		body := []byte("solo.io test")

		EventuallyWithOffset(1, func() error {
			// send a request with a body
			var buf bytes.Buffer
			buf.Write(body)

			res, err := http.Post(fmt.Sprintf("http://%s:%d/1", "localhost", envoyPort), "application/octet-stream", &buf)
			if err != nil {
				return err
			}
			if res.StatusCode != http.StatusOK {
				return errors.New(fmt.Sprintf("%v is not OK", res.StatusCode))
			}
			return nil
		}, "10s", ".5s").Should(BeNil())

		EventuallyWithOffset(1, tu.C).Should(Receive(PointTo(MatchFields(IgnoreExtras, Fields{
			"Method": Equal("POST"),
			"Body":   Equal(body),
		}))))
	}

	Describe("in memory", func() {

		BeforeEach(func() {
			testClients = services.RunGateway(ctx, true)
			err := envoyInstance.Run(testClients.GlooPort)
			Expect(err).NotTo(HaveOccurred())

		})
		It("should not crash", func() {

			var opts clients.WriteOpts
			up := tu.Upstream
			_, err := testClients.UpstreamClient.Write(up, opts)
			Expect(err).NotTo(HaveOccurred())

			proxycli := testClients.ProxyClient
			proxy := getTrivialProxyForUpstream("default", envoyPort, up.Metadata.Ref())
			_, err = proxycli.Write(proxy, opts)
			Expect(err).NotTo(HaveOccurred())

			TestUpstremReachable()

		})
	})
	Describe("kubernetes happy path", func() {
		BeforeEach(func() {
			if os.Getenv("RUN_KUBE_TESTS") != "1" {
				Skip("This test creates kubernetes resources and is disabled by default. To enable, set RUN_KUBE_TESTS=1 in your env.")
			}
		})

		var (
			namespace  string
			cfg        *rest.Config
			kubeClient kubernetes.Interface
			svc        *kubev1.Service
		)

		BeforeEach(func() {
			namespace = "gloo-e2e-" + helpers.RandString(8)
			err := setup.SetupKubeForTest(namespace)
			Expect(err).NotTo(HaveOccurred())
			kubeconfigPath := filepath.Join(os.Getenv("HOME"), ".kube", "config")
			cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
			Expect(err).NotTo(HaveOccurred())
			kubeClient, err = kubernetes.NewForConfig(cfg)
			Expect(err).NotTo(HaveOccurred())
			svc, err = kubeClient.CoreV1().Services(namespace).Create(&kubev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      "headlessservice",
				},
				Spec: kubev1.ServiceSpec{
					Ports: []kubev1.ServicePort{
						{
							Name: "foo",
							Port: int32(tu.Port),
						},
					},
				},
			})
			Expect(err).NotTo(HaveOccurred())
			_, err = kubeClient.CoreV1().Endpoints(namespace).Create(&kubev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      svc.Name,
				},
				Subsets: []kubev1.EndpointSubset{{
					Addresses: []kubev1.EndpointAddress{{
						IP:       getIpThatsNotLocalhost(),
						Hostname: "localhost",
					}},
					Ports: []kubev1.EndpointPort{{
						Port: int32(tu.Port),
					}},
				}},
			})
			Expect(err).NotTo(HaveOccurred())

			testClients = services.RunGatewayWithNamespaceAndKubeClient(ctx, true, namespace, kubeClient)
			role := namespace + "~proxy"
			err = envoyInstance.RunWithRole(role, testClients.GlooPort)
			Expect(err).NotTo(HaveOccurred())

		})
		AfterEach(func() {
			setup.TeardownKube(namespace)
		})

		It("should not crash", func() {

			getupstream := func() (*gloov1.Upstream, error) {
				l, err := testClients.UpstreamClient.List(namespace, clients.ListOpts{})
				if err != nil {
					return nil, err
				}
				for _, u := range l {
					if u.Status.State == core.Status_Pending {
						continue
					}
					if strings.Contains(u.Metadata.Name, svc.Name) {
						Expect(u.Status.State).To(Equal(core.Status_Accepted))
						return u, nil
					}
				}
				return nil, nil
			}

			Eventually(getupstream, "15s", "0.5s").ShouldNot(BeNil())

			up, err := getupstream()
			Expect(err).NotTo(HaveOccurred())

			proxycli := testClients.ProxyClient
			proxy := getTrivialProxyForUpstream(namespace, envoyPort, up.Metadata.Ref())
			var opts clients.WriteOpts
			_, err = proxycli.Write(proxy, opts)
			Expect(err).NotTo(HaveOccurred())

			TestUpstremReachable()

		})

	})
})

func getTrivialProxyForUpstream(ns string, bindport uint32, upstream core.ResourceRef) *gloov1.Proxy {
	return &gloov1.Proxy{
		Metadata: core.Metadata{
			Name:      "proxy",
			Namespace: ns,
		},
		Listeners: []*gloov1.Listener{{
			Name:        "listener",
			BindAddress: "127.0.0.1",
			BindPort:    bindport,
			ListenerType: &gloov1.Listener_HttpListener{
				HttpListener: &gloov1.HttpListener{
					VirtualHosts: []*gloov1.VirtualHost{{
						Name:    "virt1",
						Domains: []string{"*"},
						Routes: []*gloov1.Route{{
							Matcher: &gloov1.Matcher{
								PathSpecifier: &gloov1.Matcher_Prefix{
									Prefix: "/",
								},
							},
							Action: &gloov1.Route_RouteAction{
								RouteAction: &gloov1.RouteAction{
									Destination: &gloov1.RouteAction_Single{
										Single: &gloov1.Destination{
											Upstream: upstream,
										},
									},
								},
							},
						}},
					}},
				},
			},
		}},
	}

}

func getIpThatsNotLocalhost() string {
	// kubernetes endpoints doesn't like localhost, so we just give it some other local address
	// from: k8s.io/kubernetes/pkg/apis/core/validation/validation.go
	/*
		func validateNonSpecialIP(ipAddress string, fldPath *field.Path) field.ErrorList {
		        // We disallow some IPs as endpoints or external-ips.  Specifically,
		        // unspecified and loopback addresses are nonsensical and link-local
		        // addresses tend to be used for node-centric purposes (e.g. metadata
		        // service).
	*/
	ifaces, err := net.Interfaces()
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			default:
				continue
			}

			// make sure that kubernetes like this endpoint:
			endpoints := &kubecore.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "validate",
					Name:      "validate",
				},
				Subsets: []kubecore.EndpointSubset{{
					Addresses: []kubecore.EndpointAddress{{
						IP:       ip.String(),
						Hostname: "localhost",
					}},
					Ports: []kubecore.EndpointPort{{
						Port:     int32(5555),
						Protocol: kubecore.ProtocolTCP,
					}},
				}},
			}

			errs := validation.ValidateEndpoints(endpoints)
			if len(errs) != 0 {
				continue
			}

			return ip.String()
		}
	}
	Fail("no ip address available", 1)
	return ""
}
