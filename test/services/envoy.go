package services

import (
	"bufio"
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"text/template"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"

	"bytes"
	"io"

	"github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/config"
	. "github.com/onsi/gomega"
	errors "github.com/rotisserie/eris"

	"github.com/solo-io/go-utils/log"
)

const (
	containerName = "e2e_envoy"
)

var adminPort = uint32(20000)
var bindPort = uint32(10080)

func NextBindPort() uint32 {
	return AdvanceBindPort(&bindPort)
}

func AdvanceBindPort(p *uint32) uint32 {
	return atomic.AddUint32(p, 1) + uint32(config.GinkgoConfig.ParallelNode*1000)
}

func (ei *EnvoyInstance) buildBootstrap() string {
	var b bytes.Buffer
	if err := parsedTemplate.Execute(&b, ei); err != nil {
		panic(err)
	}
	return b.String()
}

const envoyConfigTemplate = `
node:
 cluster: ingress
 id: {{.ID}}
 metadata:
  role: {{.Role}}
{{if .MetricsAddr}}
stats_sinks:
  - name: envoy.stat_sinks.metrics_service
    typed_config:
      "@type": type.googleapis.com/envoy.config.metrics.v3.MetricsServiceConfig
      grpc_service:
        envoy_grpc: {cluster_name: metrics_cluster}
{{end}}

static_resources:
  clusters:
  - name: xds_cluster
    connect_timeout: 5.000s
    load_assignment:
      cluster_name: xds_cluster
      endpoints:
        - lb_endpoints:
            - endpoint:
                address:
                  socket_address:
                    address: {{.GlooAddr}}
                    port_value: {{.Port}}
    http2_protocol_options: {}
    type: STATIC
{{if .RatelimitAddr}}
  - name: ratelimit_cluster
    connect_timeout: 5.000s
    load_assignment:
      cluster_name: ratelimit_cluster
      endpoints:
        - lb_endpoints:
            - endpoint:
                address:
                  socket_address:
                    address: {{.RatelimitAddr}}
                    port_value: {{.RatelimitPort}}
    http2_protocol_options: {}
    type: STATIC
{{end}}
{{if .AccessLogAddr}}
  - name: access_log_cluster
    connect_timeout: 5.000s
    load_assignment:
      cluster_name: access_log_cluster
      endpoints:
        - lb_endpoints:
            - endpoint:
                address:
                  socket_address:
                    address: {{.AccessLogAddr}}
                    port_value: {{.AccessLogPort}}
    http2_protocol_options: {}
    type: STATIC
{{end}}
  - name: aws_sts_cluster
    connect_timeout: 5.000s
    type: LOGICAL_DNS
    lb_policy: ROUND_ROBIN
    load_assignment:
      cluster_name: aws_sts_cluster
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                port_value: 443
                address: sts.amazonaws.com
    transport_socket:
      name: envoy.transport_sockets.tls
      typed_config:
        "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.UpstreamTlsContext
        sni: sts.amazonaws.com
{{if .MetricsAddr}}
  - name: metrics_cluster
    connect_timeout: 5.000s
    load_assignment:
      cluster_name: metrics_cluster
      endpoints:
        - lb_endpoints:
            - endpoint:
                address:
                  socket_address:
                    address: {{.MetricsAddr}}
                    port_value: {{.MetricsPort}}
    http2_protocol_options: {}
    type: STATIC
{{end}}

dynamic_resources:
  ads_config:
    api_type: GRPC
    grpc_services:
    - envoy_grpc: {cluster_name: xds_cluster}
  cds_config:
    ads: {}
  lds_config:
    ads: {}

admin:
  access_log_path: /dev/null
  address:
    socket_address:
      address: 0.0.0.0
      port_value: {{.AdminPort}}

`

var parsedTemplate = template.Must(template.New("bootstrap").Parse(envoyConfigTemplate))

type EnvoyFactory struct {
	envoypath string
	tmpdir    string
	useDocker bool
	instances []*EnvoyInstance
}

func getEnvoyImageTag() string {
	eit := os.Getenv("ENVOY_IMAGE_TAG")
	if eit != "" {
		return eit
	}

	// get project base
	gomod, err := exec.Command("go", "env", "GOMOD").CombinedOutput()
	Expect(err).NotTo(HaveOccurred())
	gomodfile := strings.TrimSpace(string(gomod))
	projectbase, _ := filepath.Split(gomodfile)

	makefile := filepath.Join(projectbase, "Makefile")
	inFile, err := os.Open(makefile)
	Expect(err).NotTo(HaveOccurred())

	defer inFile.Close()

	const prefix = "ENVOY_GLOO_IMAGE ?= quay.io/solo-io/envoy-gloo:"

	scanner := bufio.NewScanner(inFile)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}

func NewEnvoyFactory() (*EnvoyFactory, error) {
	// if an envoy binary is explicitly specified
	// use it
	envoyPath := os.Getenv("ENVOY_BINARY")
	if envoyPath != "" {
		log.Printf("Using envoy from environment variable: %s", envoyPath)
		return &EnvoyFactory{
			envoypath: envoyPath,
		}, nil
	}

	// maybe it is in the path?!
	// only try to use local path if FETCH_ENVOY_BINARY is not set;
	// there are two options:
	// - you are using local envoy binary you just built and want to test (don't set the variable)
	// - you want to use the envoy version gloo is shipped with (set the variable)
	if os.Getenv("FETCH_ENVOY_BINARY") != "" {
		envoyPath, err := exec.LookPath("envoy")
		if err == nil {
			log.Printf("Using envoy from PATH: %s", envoyPath)
			return &EnvoyFactory{
				envoypath: envoyPath,
			}, nil
		}
	}

	switch runtime.GOOS {
	case "darwin":
		log.Printf("Using docker to run envoy")

		return &EnvoyFactory{useDocker: true}, nil
	case "linux":
		// try to grab one form docker...
		tmpdir, err := ioutil.TempDir(os.Getenv("HELPER_TMP"), "envoy")
		if err != nil {
			return nil, err
		}

		envoyImageTag := getEnvoyImageTag()
		if envoyImageTag == "" {
			panic("Must set the ENVOY_IMAGE_TAG env var. Find valid tag names here https://quay.io/repository/solo-io/gloo-envoy-wrapper?tab=tags")
		}
		log.Printf("Using envoy docker image tag: %s", envoyImageTag)

		bash := fmt.Sprintf(`
set -ex
CID=$(docker run -d  quay.io/solo-io/envoy-gloo:%s /bin/bash -c exit)

# just print the image sha for repoducibility
echo "Using Envoy Image:"
docker inspect quay.io/solo-io/envoy-gloo:%s -f "{{.RepoDigests}}"

docker cp $CID:/usr/local/bin/envoy .
docker rm $CID
    `, envoyImageTag, envoyImageTag)
		scriptfile := filepath.Join(tmpdir, "getenvoy.sh")

		ioutil.WriteFile(scriptfile, []byte(bash), 0755)

		cmd := exec.Command("bash", scriptfile)
		cmd.Dir = tmpdir
		cmd.Stdout = ginkgo.GinkgoWriter
		cmd.Stderr = ginkgo.GinkgoWriter
		if err := cmd.Run(); err != nil {
			return nil, err
		}

		return &EnvoyFactory{
			envoypath: filepath.Join(tmpdir, "envoy"),
			tmpdir:    tmpdir,
		}, nil

	default:
		return nil, errors.New("Unsupported OS: " + runtime.GOOS)
	}
}

func (ef *EnvoyFactory) EnvoyPath() string {
	return ef.envoypath
}

func (ef *EnvoyFactory) Clean() error {
	if ef == nil {
		return nil
	}
	if ef.tmpdir != "" {
		os.RemoveAll(ef.tmpdir)
	}
	instances := ef.instances
	ef.instances = nil
	for _, ei := range instances {
		ei.Clean()
	}
	return nil
}

type EnvoyInstance struct {
	MetricsAddr   string
	MetricsPort   uint32
	AccessLogAddr string
	AccessLogPort uint32
	RatelimitAddr string
	RatelimitPort uint32
	ID            string
	Role          string
	envoypath     string
	envoycfg      string
	logs          *bytes.Buffer
	cmd           *exec.Cmd
	UseDocker     bool
	GlooAddr      string // address for gloo and services
	Port          uint32
	AdminPort     uint32
	// Path to access logs for binary run
	AccessLogs string

	DockerOptions
}

// Extra options for running in docker
type DockerOptions struct {
	// Extra volume arguments
	Volumes []string
	// Extra env arguments.
	// see https://docs.docker.com/engine/reference/run/#env-environment-variables for more info
	Env []string
}

func (ef *EnvoyFactory) MustEnvoyInstance() *EnvoyInstance {
	envoyInstance, err := ef.NewEnvoyInstance()
	Expect(err).NotTo(HaveOccurred())
	return envoyInstance
}

func (ef *EnvoyFactory) NewEnvoyInstance() (*EnvoyInstance, error) {

	gloo := "127.0.0.1"

	if ef.useDocker {
		var err error
		gloo, err = localAddr()
		if err != nil {
			return nil, err
		}
	}

	ei := &EnvoyInstance{
		envoypath:     ef.envoypath,
		UseDocker:     ef.useDocker,
		GlooAddr:      gloo,
		AccessLogAddr: gloo,
		MetricsAddr:   gloo,
		AdminPort:     atomic.AddUint32(&adminPort, 1) + uint32(config.GinkgoConfig.ParallelNode*1000),
	}
	ef.instances = append(ef.instances, ei)
	return ei, nil

}

func (ei *EnvoyInstance) RunWithId(id string) error {
	ei.ID = id
	ei.Role = "default~proxy"

	// TODO: refactor this function to include a context.
	return ei.runWithPort(context.TODO(), 8081)
}

func (ei *EnvoyInstance) Run(port int) error {
	ei.Role = "default~proxy"

	// TODO: refactor this function to include a context.
	return ei.runWithPort(context.TODO(), uint32(port))
}
func (ei *EnvoyInstance) RunWithRole(role string, port int) error {
	ei.Role = role
	// TODO: refactor this function to include a context.
	return ei.runWithPort(context.TODO(), uint32(port))
}

type EnvoyInstanceConfig interface {
	Role() string
	Port() uint32
	Context() context.Context
}

func (ei *EnvoyInstance) RunWith(eic EnvoyInstanceConfig) error {
	ei.Role = eic.Role()
	return ei.runWithPort(eic.Context(), eic.Port())
}

func (ei *EnvoyInstance) runWithPort(ctx context.Context, port uint32) error {
	go func() {
		<-ctx.Done()
		ei.Clean()
	}()
	if ei.ID == "" {
		ei.ID = "ingress~for-testing"
	}
	ei.Port = port

	ei.envoycfg = ei.buildBootstrap()
	if ei.UseDocker {
		err := ei.runContainer(ctx)
		if err != nil {
			return err
		}
		return nil
	}

	args := []string{"--config-yaml", ei.envoycfg, "--disable-hot-restart", "--log-level", "debug"}

	// run directly
	cmd := exec.CommandContext(ctx, ei.envoypath, args...)

	buf := &bytes.Buffer{}
	ei.logs = buf
	w := io.MultiWriter(ginkgo.GinkgoWriter, buf)
	cmd.Stdout = w
	cmd.Stderr = w

	runner := Runner{Sourcepath: ei.envoypath, ComponentName: "ENVOY"}
	cmd, err := runner.run(cmd)
	if err != nil {
		return err
	}
	ei.cmd = cmd
	return nil
}

func (ei *EnvoyInstance) Binary() string {
	return ei.envoypath
}

func (ei *EnvoyInstance) LocalAddr() string {
	return ei.GlooAddr
}

func (ei *EnvoyInstance) SetPanicThreshold() error {
	_, err := http.Post(fmt.Sprintf("http://localhost:%d/runtime_modify?upstream.healthy_panic_threshold=%d", ei.AdminPort, 0), "", nil)
	return err
}

func (ei *EnvoyInstance) Clean() error {
	if ei == nil {
		return nil
	}
	http.Post(fmt.Sprintf("http://localhost:%d/quitquitquit", ei.AdminPort), "", nil)
	if ei.cmd != nil {
		ei.cmd.Process.Kill()
		ei.cmd.Wait()
	}

	if ei.UseDocker {
		if err := stopContainer(); err != nil {
			return err
		}
	}
	return nil
}

func (ei *EnvoyInstance) runContainer(ctx context.Context) error {
	envoyImageTag := getEnvoyImageTag()
	if envoyImageTag == "" {
		return errors.New("Must set the ENVOY_IMAGE_TAG env var. Find valid tag names here https://quay.io/repository/solo-io/gloo-envoy-wrapper?tab=tags")
	}

	image := "quay.io/solo-io/gloo-envoy-wrapper:" + envoyImageTag
	args := []string{"run", "--rm", "--name", containerName,
		"-p", fmt.Sprintf("%d:%d", defaults.HttpPort, defaults.HttpPort),
		"-p", fmt.Sprintf("%d:%d", defaults.HttpsPort, defaults.HttpsPort),
		"-p", fmt.Sprintf("%d:%d", ei.AdminPort, ei.AdminPort),
	}

	for _, volume := range ei.DockerOptions.Volumes {
		args = append(args, "-v", volume)
	}

	for _, env := range ei.DockerOptions.Env {
		args = append(args, "-e", env)
	}

	args = append(args,
		"--entrypoint=envoy",
		image,
		"--disable-hot-restart", "--log-level", "debug",
		"--config-yaml", ei.envoycfg,
	)

	fmt.Fprintln(ginkgo.GinkgoWriter, args)
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = ginkgo.GinkgoWriter
	cmd.Stderr = ginkgo.GinkgoWriter
	if err := cmd.Start(); err != nil {
		return errors.Wrap(err, "Unable to start envoy container")
	}

	return nil
}

func stopContainer() error {
	cmd := exec.Command("docker", "stop", containerName)
	cmd.Stdout = ginkgo.GinkgoWriter
	cmd.Stderr = ginkgo.GinkgoWriter
	err := cmd.Run()
	if err != nil {
		return errors.Wrap(err, "Error stopping container "+containerName)
	}
	return nil
}

func localAddr() (string, error) {
	ip := os.Getenv("GLOO_IP")
	if ip != "" {
		return ip, nil
	}
	// go over network interfaces
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, i := range ifaces {
		if (i.Flags&net.FlagUp == 0) ||
			(i.Flags&net.FlagLoopback != 0) ||
			(i.Flags&net.FlagPointToPoint != 0) {
			continue
		}
		addrs, err := i.Addrs()
		if err != nil {
			return "", err
		}
		for _, addr := range addrs {
			switch v := addr.(type) {
			case *net.IPNet:
				if v.IP.To4() != nil {
					return v.IP.String(), nil
				}
			case *net.IPAddr:
				if v.IP.To4() != nil {
					return v.IP.String(), nil
				}
			}
		}
	}
	return "", errors.New("unable to find Gloo IP")
}

func (ei *EnvoyInstance) Logs() (string, error) {
	if ei.UseDocker {
		logsArgs := []string{"logs", containerName}
		cmd := exec.Command("docker", logsArgs...)
		byt, err := cmd.CombinedOutput()
		if err != nil {
			return "", errors.Wrap(err, "Unable to fetch logs from envoy container")
		}
		return string(byt), nil
	}

	return ei.logs.String(), nil
}
