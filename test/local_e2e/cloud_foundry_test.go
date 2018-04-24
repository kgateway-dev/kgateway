package local_e2e

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"time"

	"code.cloudfoundry.org/copilot"
	"code.cloudfoundry.org/copilot/api"

	"code.cloudfoundry.org/copilot/config"
	"code.cloudfoundry.org/copilot/testhelpers"

	"github.com/gogo/protobuf/proto"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/plugins/cloudfoundry"
	"github.com/solo-io/gloo/pkg/protoutil"

	bbsmodels "code.cloudfoundry.org/bbs/models"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/onsi/gomega/ghttp"
)

const (
	Hostname    = "my.cf.host.com"
	HostAddress = "127.0.0.1"
)

var _ = Describe("Copilot", func() {
	var (
		copilotPath                    string
		session                        *gexec.Session
		istioClient                    copilot.IstioClient
		ccClient                       copilot.CloudControllerClient
		serverConfig                   *config.Config
		pilotClientTLSConfig           *tls.Config
		cloudControllerClientTLSConfig *tls.Config
		configFilePath                 string

		bbsServer    *ghttp.Server
		cleanupFuncs []func()

		listenAddrForPilot  string
		pilotClientTLSFiles testhelpers.ClientTLSFilePaths
		hostPort            uint32
		responses           <-chan *ReceivedRequest
	)

	BeforeEach(func() {
		// this is mostly copied from here:
		// https://github.com/cloudfoundry/copilot/blob/5cf2cbdd5277752bada9870d1a3026a65f224138/integration/integration_test.go

		if runtime.GOOS == "darwin" {
			Skip("skipping CF test on macOS")
		}
		copilotPath = os.Getenv("COPILOT_BINARY")
		if copilotPath == "" {
			Skip("must set COPILOT_BINARY to run CF tests. e.g.  COPILOT_BINARY=$GOPATH/src/code.cloudfoundry.org/copilot/copilot-server ginkgo ")
		}
		ctx, cancel := context.WithCancel(context.Background())
		cleanupFuncs = append(cleanupFuncs, cancel)

		hostPort, responses = RunTestServer(ctx)

		copilotCreds := testhelpers.GenerateMTLS()
		cleanupFuncs = append(cleanupFuncs, copilotCreds.CleanupTempFiles)

		listenAddrForPilot = fmt.Sprintf("127.0.0.1:%d", testhelpers.PickAPort())
		listenAddrForCloudController := fmt.Sprintf("127.0.0.1:%d", testhelpers.PickAPort())
		copilotTLSFiles := copilotCreds.CreateServerTLSFiles()

		bbsCreds := testhelpers.GenerateMTLS()
		cleanupFuncs = append(cleanupFuncs, copilotCreds.CleanupTempFiles)

		bbsTLSFiles := bbsCreds.CreateClientTLSFiles()

		// boot a fake BBS
		bbsServer = ghttp.NewUnstartedServer()
		bbsServer.HTTPTestServer.TLS = bbsCreds.ServerTLSConfig()

		bbsServer.RouteToHandler("POST", "/v1/cells/list.r1", func(w http.ResponseWriter, req *http.Request) {
			cellsResponse := bbsmodels.CellsResponse{}
			data, _ := proto.Marshal(&cellsResponse)
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
			w.Header().Set("Content-Type", "application/x-protobuf")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)
		})

		bbsServer.RouteToHandler("POST", "/v1/actual_lrp_groups/list", func(w http.ResponseWriter, req *http.Request) {
			actualLRPResponse := bbsmodels.ActualLRPGroupsResponse{
				ActualLrpGroups: []*bbsmodels.ActualLRPGroup{
					{
						Instance: &bbsmodels.ActualLRP{
							ActualLRPKey: bbsmodels.NewActualLRPKey("diego-process-guid-a", 1, "domain1"),
							State:        bbsmodels.ActualLRPStateRunning,
							ActualLRPNetInfo: bbsmodels.ActualLRPNetInfo{
								Address: HostAddress,
								Ports: []*bbsmodels.PortMapping{
									{ContainerPort: 8080, HostPort: hostPort},
								},
							},
						},
					},
				},
			}
			data, _ := proto.Marshal(&actualLRPResponse)
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
			w.Header().Set("Content-Type", "application/x-protobuf")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)
		})
		bbsServer.Start()
		cleanupFuncs = append(cleanupFuncs, bbsServer.Close)

		serverConfig = &config.Config{
			ListenAddressForPilot:           listenAddrForPilot,
			ListenAddressForCloudController: listenAddrForCloudController,
			PilotClientCAPath:               copilotTLSFiles.ClientCA,
			CloudControllerClientCAPath:     copilotTLSFiles.OtherClientCA,
			ServerCertPath:                  copilotTLSFiles.ServerCert,
			ServerKeyPath:                   copilotTLSFiles.ServerKey,
			BBS: &config.BBSConfig{
				ServerCACertPath: bbsTLSFiles.ServerCA,
				ClientCertPath:   bbsTLSFiles.ClientCert,
				ClientKeyPath:    bbsTLSFiles.ClientKey,
				Address:          bbsServer.URL(),
			},
		}

		configFilePath = testhelpers.TempFileName()
		cleanupFuncs = append(cleanupFuncs, func() { os.Remove(configFilePath) })

		Expect(serverConfig.Save(configFilePath)).To(Succeed())

		cmd := exec.Command(copilotPath, "-config", configFilePath)
		var err error
		session, err = gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred())
		Eventually(session.Out).Should(gbytes.Say(`started`))

		pilotClientTLSConfig = copilotCreds.ClientTLSConfig()
		pilotClientTLSFiles = copilotCreds.CreateClientTLSFiles()

		cloudControllerClientTLSConfig = copilotCreds.OtherClientTLSConfig()

		istioClient, err = copilot.NewIstioClient(serverConfig.ListenAddressForPilot, pilotClientTLSConfig)
		Expect(err).NotTo(HaveOccurred())
		ccClient, err = copilot.NewCloudControllerClient(serverConfig.ListenAddressForCloudController, cloudControllerClientTLSConfig)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		session.Interrupt()
		Eventually(session, "2s").Should(gexec.Exit())

		for i := len(cleanupFuncs) - 1; i >= 0; i-- {
			cleanupFuncs[i]()
		}
	})

	setup := func() {

		By("CC creates and maps a route")
		_, err := ccClient.UpsertRoute(context.Background(), &api.UpsertRouteRequest{
			Route: &api.Route{
				Guid: "route-guid-a",
				Host: Hostname,
			}})
		Expect(err).NotTo(HaveOccurred())
		_, err = ccClient.MapRoute(context.Background(), &api.MapRouteRequest{
			RouteMapping: &api.RouteMapping{
				RouteGuid:       "route-guid-a",
				CapiProcessGuid: "capi-process-guid-a",
			},
		})
		Expect(err).NotTo(HaveOccurred())
		_, err = ccClient.UpsertCapiDiegoProcessAssociation(context.Background(), &api.UpsertCapiDiegoProcessAssociationRequest{
			&api.CapiDiegoProcessAssociation{
				CapiProcessGuid: "capi-process-guid-a",
				DiegoProcessGuids: []string{
					"diego-process-guid-a",
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())

	}

	It("discovers copilot upstreams", func() {
		WaitForHealthy(istioClient, ccClient)
		setup()

		// run gloo instance with cf upstream
		err := envoyInstance.Run()
		Expect(err).NotTo(HaveOccurred())

		glooInstance.Args = []string{
			"--copilot.address=" + listenAddrForPilot,
			"--copilot.client-cert=" + pilotClientTLSFiles.ClientCert,
			"--copilot.client-key=" + pilotClientTLSFiles.ClientKey,
			"--copilot.server-ca=" + pilotClientTLSFiles.ServerCA,
		}

		err = glooInstance.Run()
		Expect(err).NotTo(HaveOccurred())

		envoyPort := glooInstance.EnvoyPort()

		us := NewCFUpstream()
		err = glooInstance.AddUpstream(us)
		Expect(err).NotTo(HaveOccurred())

		v := &v1.VirtualService{
			Name: "default",
			Routes: []*v1.Route{{
				Matcher: &v1.Route_RequestMatcher{
					RequestMatcher: &v1.RequestMatcher{
						Path: &v1.RequestMatcher_PathPrefix{PathPrefix: "/"},
					},
				},
				SingleDestination: &v1.Destination{
					DestinationType: &v1.Destination_Upstream{
						Upstream: &v1.UpstreamDestination{
							Name: us.Name,
						},
					},
				},
			}},
		}

		err = glooInstance.AddVhost(v)
		Expect(err).NotTo(HaveOccurred())

		body := []byte("solo.io test")

		Eventually(func() error {
			// send a request with a body
			var buf bytes.Buffer
			buf.Write(body)
			_, err := http.Post(fmt.Sprintf("http://%s:%d", "localhost", envoyPort), "application/octet-stream", &buf)
			return err
		}, 90, 1).Should(BeNil())

		expectedResponse := &ReceivedRequest{
			Method: "POST",
			Body:   body,
		}
		Eventually(responses).Should(Receive(Equal(expectedResponse)))
	})

})

func WaitForHealthy(istioClient copilot.IstioClient, ccClient copilot.CloudControllerClient) {
	By("waiting for the servers to become healthy")
	serverForPilotIsHealthy := func() error {
		ctx, cancelFunc := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancelFunc()
		_, err := istioClient.Health(ctx, new(api.HealthRequest))
		return err
	}
	Eventually(serverForPilotIsHealthy, 2*time.Second).Should(Succeed())

	serverForCloudControllerIsHealthy := func() error {
		ctx, cancelFunc := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancelFunc()
		_, err := ccClient.Health(ctx, new(api.HealthRequest))
		return err
	}
	Eventually(serverForCloudControllerIsHealthy, 2*time.Second).Should(Succeed())
}

func NewCFUpstream() *v1.Upstream {

	serviceSpec := cloudfoundry.UpstreamSpec{
		Hostname: Hostname,
	}
	v1Spec, err := protoutil.MarshalStruct(serviceSpec)
	if err != nil {
		panic(err)
	}
	u := &v1.Upstream{
		Name: "local", // TODO: randomize
		Type: cloudfoundry.UpstreamTypeCF,
		Spec: v1Spec,
	}

	return u
}
