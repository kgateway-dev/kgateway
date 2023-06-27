package envoy

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"

	"github.com/solo-io/gloo/test/ginkgo/parallel"

	"github.com/solo-io/gloo/test/services"
	"go.uber.org/zap/zapcore"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"

	"github.com/onsi/ginkgo/v2"
	errors "github.com/rotisserie/eris"
	"github.com/solo-io/gloo/test/services/utils"
	"github.com/solo-io/gloo/test/testutils"
	"github.com/solo-io/gloo/test/testutils/version"
	"github.com/solo-io/skv2/codegen/util"
)

var _ Factory = new(factoryImpl)

const (
	envoyBinaryName = "envoy"
	ServiceName     = "gateway-proxy"
)

// Factory is a helper for running multiple envoy instances
type Factory interface {
	NewInstance() *Instance
	Clean()
}

// factoryImpl is the default implementation of the Factory interface
type factoryImpl struct {
	// defaultBootstrapTemplate is the default template used to generate the bootstrap config for Envoy
	// Individuals tests may supply their own
	defaultBootstrapTemplate *template.Template

	envoypath string
	tmpdir    string

	useDocker bool
	// The image that will be used to Run the Envoy instance in Docker
	// This can either be a previously released tag or the tag of a locally built image
	// See the Setup section of the ./test/e2e/README for details about building a local image
	dockerImage string

	instances []*Instance
}

// NewFactory returns a new envoy service Factory
func NewFactory() Factory {
	// if an envoy binary is explicitly specified, use it
	envoyPath := os.Getenv(testutils.EnvoyBinary)
	if envoyPath != "" {
		log.Printf("Using envoy from environment variable: %s", envoyPath)
		return NewLinuxFactory(bootstrapTemplate, envoyPath, "")
	}

	switch runtime.GOOS {
	case "darwin":
		log.Printf("Using docker to run envoy")

		image := fmt.Sprintf("quay.io/solo-io/gloo-envoy-wrapper:%s", mustGetEnvoyWrapperTag())
		return NewDockerFactory(bootstrapTemplate, image)

	case "linux":
		tmpdir, err := os.MkdirTemp(os.Getenv("HELPER_TMP"), "envoy")
		if err != nil {
			ginkgo.Fail(fmt.Sprintf("failed to create tmp dir: %v", err))
		}

		image := fmt.Sprintf("quay.io/solo-io/envoy-gloo:%s", mustGetEnvoyGlooTag())

		binaryPath, err := utils.GetBinary(utils.GetBinaryParams{
			Filename:    envoyBinaryName,
			DockerImage: image,
			DockerPath:  "/usr/local/bin/envoy",
			EnvKey:      testutils.EnvoyBinary, // this is inert here since we already check the env in this function, but leaving it for future compatibility
			TmpDir:      tmpdir,
		})
		if err != nil {
			ginkgo.Fail(fmt.Sprintf("failed to get binary: %v", err))
		}
		return NewLinuxFactory(bootstrapTemplate, binaryPath, tmpdir)

	default:
		ginkgo.Fail("Unsupported OS: " + runtime.GOOS)
	}
	return nil
}

// mustGetEnvoyGlooTag returns the tag of the envoy-gloo image which will be executed
// The tag is chosen using the following process:
//  1. If ENVOY_IMAGE_TAG is defined, use that tag
//  2. If not defined, use the ENVOY_GLOO_IMAGE tag defined in the Makefile
func mustGetEnvoyGlooTag() string {
	eit := os.Getenv(testutils.EnvoyImageTag)
	if eit != "" {
		return eit
	}

	makefile := filepath.Join(util.GetModuleRoot(), "Makefile")
	inFile, err := os.Open(makefile)
	if err != nil {
		ginkgo.Fail(errors.Wrapf(err, "failed to open Makefile").Error())
	}

	defer inFile.Close()

	const prefix = "ENVOY_GLOO_IMAGE ?= quay.io/solo-io/envoy-gloo:"

	scanner := bufio.NewScanner(inFile)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}

	ginkgo.Fail("Could not determine envoy-gloo tag. Find valid tag names here https://quay.io/repository/solo-io/envoy-gloo?tab=tags and update your make target or use the variable ENVOY_IMAGE_TAG to set it.")
	return ""
}

// mustGetEnvoyWrapperTag returns the tag of the gloo-envoy-wrapper image which will be executed
// The tag is chosen using the following process:
//  1. If ENVOY_IMAGE_TAG is defined, use that tag
//  2. If not defined, use the latest released tag of that image
func mustGetEnvoyWrapperTag() string {
	eit := os.Getenv(testutils.EnvoyImageTag)
	if eit != "" {
		return eit
	}

	latestPatchVersion, err := version.GetLastReleaseOfCurrentBranch()
	if err != nil {
		ginkgo.Fail(errors.Wrap(err, "Failed to extract the latest release of current minor").Error())
	}

	return strings.TrimPrefix(latestPatchVersion.String(), "v")
}

func NewDockerFactory(defaultBootstrapTemplate *template.Template, dockerImage string) Factory {
	return &factoryImpl{
		defaultBootstrapTemplate: defaultBootstrapTemplate,
		useDocker:                true,
		dockerImage:              dockerImage,
	}
}

func NewLinuxFactory(defaultBootstrapTemplate *template.Template, envoyPath, tmpDir string) Factory {
	return &factoryImpl{
		defaultBootstrapTemplate: defaultBootstrapTemplate,
		useDocker:                false,
		envoypath:                envoyPath,
		tmpdir:                   tmpDir,
	}
}

func (f *factoryImpl) Clean() {
	if f == nil {
		return
	}
	if f.tmpdir != "" {
		_ = os.RemoveAll(f.tmpdir)
	}
	instances := f.instances
	f.instances = nil
	for _, ei := range instances {
		ei.Clean()
	}
}

func (f *factoryImpl) NewInstance() *Instance {
	instance, err := f.newInstanceOrError()
	if err != nil {
		ginkgo.Fail(errors.Wrap(err, "failed to create envoy instance").Error())
	}
	return instance
}

func (f *factoryImpl) newInstanceOrError() (*Instance, error) {
	gloo := "127.0.0.1"

	if f.useDocker {
		var err error
		gloo, err = localAddr()
		if err != nil {
			return nil, err
		}
	}

	// Ensure that each Instance has a unique set of ports
	// This is done in an attempt to allow multiple envoy Instances to be executed in parallel
	// without port conflicts. This get's us a step closer to being able to run e2e tests in parallel.
	// I believe there are some lingering blockers to that succeeding, but this is a step in the right direction.
	advanceRequestPorts()

	ei := &Instance{
		defaultBootstrapTemplate: f.defaultBootstrapTemplate,
		envoypath:                f.envoypath,
		UseDocker:                f.useDocker,
		DockerImage:              f.dockerImage,
		DockerContainerName:      fmt.Sprintf("e2e_envoy-%d", parallel.GetParallelProcessCount()),
		GlooAddr:                 gloo,
		AccessLogPort:            NextAccessLogPort(),
		AccessLogAddr:            gloo,
		ApiVersion:               "V3",
		logLevel:                 getInstanceLogLevel(),
		RequestPorts: &RequestPorts{
			HttpPort:   defaults.HttpPort,
			HttpsPort:  defaults.HttpsPort,
			HybridPort: defaults.HybridPort,
			TcpPort:    defaults.TcpPort,
			AdminPort:  defaults.EnvoyAdminPort,
		},
	}

	log.Printf("Envoy Instance (%s) Ports: %+v", ei.DockerContainerName, ei.RequestPorts)
	f.instances = append(f.instances, ei)
	return ei, nil

}

func getInstanceLogLevel() string {
	logLevel := services.GetLogLevel(ServiceName)

	// Envoy log level options do not match Gloo's log level options, so we must convert
	// https://www.envoyproxy.io/docs/envoy/latest/start/quick-start/run-envoy#debugging-envoy
	// There are a few options which are available in Envoy, but not in Gloo ("trace", "critical", "off")
	// We opted not to support those options, to provide developers a consistent experience
	switch logLevel {
	case zapcore.DebugLevel, zapcore.InfoLevel, zapcore.WarnLevel, zapcore.ErrorLevel:
		return logLevel.String()
	}

	return zapcore.InfoLevel.String()
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
