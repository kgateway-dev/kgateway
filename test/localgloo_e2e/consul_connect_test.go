package localgloo_e2e_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/solo-io/gloo/pkg/log"
)

type ConsulService struct {
	Service Service `json:"service"`
}

type Service struct {
	Name    string  `json:"name"`
	Port    int     `json:"port"`
	Connect Connect `json:"connect"`
}

type Connect struct {
	Proxy Proxy `json:"proxy"`
}

type Proxy struct {
	ExecMode string   `json:"exec_mode"`
	Command  []string `json:"command"`
	Config   Config   `json:"config"`
}

type Config struct {
	Upstreams []Upstream `json:"upstreams"`
}

type Upstream struct {
	DestinationName string `json:"destination_name"`
	LocalBindPort   int    `json:"local_bind_port"`
}

type ProxyInfo struct {
	ProxyServiceID    string
	TargetServiceID   string
	TargetServiceName string
	ContentHash       string
	ExecMode          string
	Command           []string
	Config            ProxyConfig
}

type ProxyConfig struct {
	BindAddress         string     `json:"bind_address"`
	BindPort            uint       `json:"bind_port"`
	LocalServiceAddress string     `json:"local_service_address"`
	Upstreams           []Upstream `json:"upstreams"`
}

var _ = Describe("ConsulConnect", func() {
	var tmpdir string
	var consulConfigDir string
	var consulSession *gexec.Session
	var pathToGlooBridge string

	BeforeEach(func() {

		bridge := filepath.Join(os.Getenv("GOPATH"), "src", "github.com/solo-io/gloo-connect/cmd")
		_, err := os.Stat(bridge)
		if os.IsNotExist(err) {
			Skip("no bridge available skipping test")
		}

		pathToGlooBridge, err = gexec.Build("github.com/solo-io/gloo-connect/cmd")
		Expect(err).ShouldNot(HaveOccurred())

		envoypath := os.Getenv("ENVOY_PATH")
		Expect(envoypath).ToNot(BeEmpty())
		// generate the template

		tmpdir, err = ioutil.TempDir("", "")
		Expect(err).NotTo(HaveOccurred())

		bridgeConfigDir := filepath.Join(tmpdir, "bridge-config")
		err = os.Mkdir(bridgeConfigDir, 0755)
		Expect(err).NotTo(HaveOccurred())

		consulConfigDir = filepath.Join(tmpdir, "consul-config")
		err = os.Mkdir(consulConfigDir, 0755)
		Expect(err).NotTo(HaveOccurred())

		args := []string{
			pathToGlooBridge,
			"--gloo-address",
			"localhost",
			"--gloo-port",
			fmt.Sprintf("%v", xdsPort),
			"--conf-dir",
			bridgeConfigDir,
			"--envoy-path",
			envoypath,
			"--storage.type=file",
			"--storage.refreshrate=1s",
			"--file.config.dir=" + baseOpts.FileOptions.ConfigDir,
		}
		svc := ConsulService{
			Service: Service{
				Name: "web",
				Port: 9090,
				Connect: Connect{
					Proxy: Proxy{
						ExecMode: "daemon",
						Command:  args,
						Config: Config{
							Upstreams: []Upstream{
								{
									DestinationName: "consul",
									LocalBindPort:   1234,
								},
							},
						},
					},
				},
			},
		}

		data, err := json.Marshal(svc)
		Expect(err).NotTo(HaveOccurred())

		err = ioutil.WriteFile(filepath.Join(consulConfigDir, "service.json"), data, 0644)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		gexec.TerminateAndWait("5s")
		consulSession = nil
		gexec.CleanupBuildArtifacts()

		if tmpdir != "" {
			os.RemoveAll(tmpdir)
		}
	})

	runConsul := func() {
		consul := exec.Command("consul", "agent", "-dev", "--config-dir="+consulConfigDir)
		session, err := gexec.Start(consul, GinkgoWriter, GinkgoWriter)
		consulSession = session

		Expect(err).NotTo(HaveOccurred())
	}

	It("should start envoy", func() {
		log.Printf("test config dir: %v", baseOpts.FileOptions.ConfigDir)
		runConsul()
		time.Sleep(1 * time.Second)
		Expect(consulSession).ShouldNot(gexec.Exit())
		Eventually(consulSession.Out, "5s").Should(gbytes.Say("agent/proxy: starting proxy:"))

		// check that a port was opened where consul says it should have been opened (get the port from consul connect and check that it is open)
		resp, err := http.Get("http://127.0.0.1:8500/v1/agent/connect/proxy/web-proxy")
		Expect(err).NotTo(HaveOccurred())
		body, err := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		Expect(err).NotTo(HaveOccurred())

		var cfg ProxyInfo
		json.Unmarshal(body, &cfg)

		//runFakeXds(cfg.Config.BindAddress, cfg.Config.BindPort)

		time.Sleep(5 * time.Second)

		conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", cfg.Config.BindAddress, cfg.Config.BindPort))
		Expect(err).NotTo(HaveOccurred())

		// We are connected! good enough!
		conn.Close()
	})
})
