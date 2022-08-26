package services

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"io/ioutil"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"

	"github.com/solo-io/go-utils/log"
)

const consulDockerImage = "consul:1.5.2"

type ConsulFactory struct {
	consulPath string
	tmpdir     string
}

type serviceDef struct {
	Service *consulService `json:"service"`
}

type consulService struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Port    uint32   `json:"port"`
	Tags    []string `json:"tags"`
	Address string   `json:"address"`
}

func NewConsulFactory() (*ConsulFactory, error) {
	consulPath := os.Getenv("CONSUL_BINARY")

	if consulPath != "" {
		return &ConsulFactory{
			consulPath: consulPath,
		}, nil
	}

	consulPath, err := exec.LookPath("consul")
	if err == nil {
		log.Printf("Using consul from PATH: %s", consulPath)
		return &ConsulFactory{
			consulPath: consulPath,
		}, nil
	}

	// try to grab one from docker...
	tmpdir, err := ioutil.TempDir(os.Getenv("HELPER_TMP"), "consul")
	if err != nil {
		return nil, err
	}

	bash := fmt.Sprintf(`
set -ex
CID=$(docker run -d  %s /bin/sh -c exit)

# just print the image sha for repoducibility
echo "Using Consul Image:"
docker inspect %s -f "{{.RepoDigests}}"

docker cp $CID:/bin/consul .
docker rm -f $CID
    `, consulDockerImage, consulDockerImage)
	scriptFile := filepath.Join(tmpdir, "get_consul.sh")

	err = ioutil.WriteFile(scriptFile, []byte(bash), 0755)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command("bash", scriptFile)
	cmd.Dir = tmpdir
	cmd.Stdout = GinkgoWriter
	cmd.Stderr = GinkgoWriter
	if err := cmd.Run(); err != nil {
		return nil, err
	}

	return &ConsulFactory{
		consulPath: filepath.Join(tmpdir, "consul"),
		tmpdir:     tmpdir,
	}, nil
}

func (cf *ConsulFactory) Clean() error {
	if cf == nil {
		return nil
	}
	if cf.tmpdir != "" {
		_ = os.RemoveAll(cf.tmpdir)

	}
	return nil
}

func (cf *ConsulFactory) NewConsulInstance() (*ConsulInstance, error) {
	// try to grab one from docker...
	tmpdir, err := ioutil.TempDir(os.Getenv("HELPER_TMP"), "consul")
	if err != nil {
		return nil, err
	}

	cfgDir := filepath.Join(tmpdir, "config")
	err = os.Mkdir(cfgDir, 0755)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(cf.consulPath, "agent", "-dev", "--client=0.0.0.0",
		"-node", "consul-dev")
	cmd.Dir = cf.tmpdir
	cmd.Stdout = GinkgoWriter
	cmd.Stderr = GinkgoWriter
	return &ConsulInstance{
		consulPath:         cf.consulPath,
		tmpdir:             tmpdir,
		cfgDir:             cfgDir,
		cmd:                cmd,
		registeredServices: map[string]*serviceDef{},
	}, nil
}

type ConsulInstance struct {
	consulPath string
	tmpdir     string
	cfgDir     string
	cmd        *exec.Cmd

	session *gexec.Session

	registeredServices map[string]*serviceDef
}

func (i *ConsulInstance) AddConfig(svcId, content string) (string, error) {
	fileName := filepath.Join(i.cfgDir, svcId+".json")
	return fileName, ioutil.WriteFile(fileName, []byte(content), 0644)
}

func (i *ConsulInstance) Silence() {
	i.cmd.Stdout = nil
	i.cmd.Stderr = nil
}

func (i *ConsulInstance) Run() error {
	var err error
	i.session, err = gexec.Start(i.cmd, GinkgoWriter, GinkgoWriter)

	if err != nil {
		return err
	}
	EventuallyWithOffset(2, i.session.Out, "5s").Should(gbytes.Say("New leader elected"))
	return nil
}

func (i *ConsulInstance) Binary() string {
	return i.consulPath
}

func (i *ConsulInstance) Clean() error {
	cmd := exec.Command("consul", "leave") /// gracefully leave so tests can run consecutively without issues
	cmd.Dir = i.tmpdir
	cmd.Stdout = GinkgoWriter
	cmd.Stderr = GinkgoWriter
	return cmd.Run()
}

// While it may be tempting to just reload all config using `consul reload` or marshalling new json and
// sending SIGHUP to the process (per https://www.consul.io/commands/reload), it is preferable to live update
// using the consul APIs as this is a more realistic flow and doesn't fire our watches too actively (which can
// both make debugging hard and hide bugs)
func (i *ConsulInstance) RegisterService(svcName, svcId, address string, tags []string, port uint32) error {
	svcDef := &serviceDef{
		Service: &consulService{
			ID:      svcId,
			Name:    svcName,
			Address: address,
			Tags:    tags,
			Port:    port,
		},
	}
	content, err := json.Marshal(svcDef.Service)
	if err != nil {
		return err
	}
	postData := string(content)
	fileName, err := i.AddConfig(svcId, postData)
	if err != nil {
		return err
	}
	cmd := exec.Command("curl", "--request", "PUT", "--data", fmt.Sprintf("@%s", fileName), "localhost:8500/v1/agent/service/register")
	cmd.Dir = i.tmpdir
	cmd.Stdout = GinkgoWriter
	cmd.Stderr = GinkgoWriter
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}
