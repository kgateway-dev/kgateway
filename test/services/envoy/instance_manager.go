package envoy

import (
	"net"
	"os"
	"sync/atomic"
	"text/template"

	"github.com/onsi/gomega"
	errors "github.com/rotisserie/eris"
	"github.com/solo-io/gloo/test/ginkgo/parallel"
)

var adminPort = uint32(20000)

// InstanceManager is a helper for managing multiple Envoy instances
type InstanceManager struct {
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

func NewDockerInstanceManager(defaultBootstrapTemplate *template.Template, dockerImage string) *InstanceManager {
	return &InstanceManager{
		defaultBootstrapTemplate: defaultBootstrapTemplate,
		useDocker:                true,
		dockerImage:              dockerImage,
	}
}

func NewLinuxInstanceManager(defaultBootstrapTemplate *template.Template, envoyPath, tmpDir string) *InstanceManager {
	return &InstanceManager{
		defaultBootstrapTemplate: defaultBootstrapTemplate,
		useDocker:                false,
		envoypath:                envoyPath,
		tmpdir:                   tmpDir,
	}
}

func (m *InstanceManager) EnvoyPath() string {
	return m.envoypath
}

func (m *InstanceManager) Clean() error {
	if m == nil {
		return nil
	}
	if m.tmpdir != "" {
		os.RemoveAll(m.tmpdir)
	}
	instances := m.instances
	m.instances = nil
	for _, ei := range instances {
		ei.Clean()
	}
	return nil
}

func (m *InstanceManager) MustEnvoyInstance() *Instance {
	envoyInstance, err := m.NewEnvoyInstance()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	return envoyInstance
}

func (m *InstanceManager) NewEnvoyInstance() (*Instance, error) {
	gloo := "127.0.0.1"

	if m.useDocker {
		var err error
		gloo, err = localAddr()
		if err != nil {
			return nil, err
		}
	}

	ei := &Instance{
		defaultBootstrapTemplate: m.defaultBootstrapTemplate,
		envoypath:                m.envoypath,
		UseDocker:                m.useDocker,
		DockerImage:              m.dockerImage,
		GlooAddr:                 gloo,
		AccessLogAddr:            gloo,
		AdminPort:                atomic.AddUint32(&adminPort, 1) + uint32(parallel.GetPortOffset()),
		ApiVersion:               "V3",
	}
	m.instances = append(m.instances, ei)
	return ei, nil

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
