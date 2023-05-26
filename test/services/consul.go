package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"

	"github.com/hashicorp/consul/api"

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
	tmpdir, err := os.MkdirTemp(os.Getenv("HELPER_TMP"), "consul")
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

	err = os.WriteFile(scriptFile, []byte(bash), 0755)
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

func (cf *ConsulFactory) MustConsulInstance() *ConsulInstance {
	instance, err := cf.NewConsulInstance()
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return instance
}

func (cf *ConsulFactory) NewConsulInstance() (*ConsulInstance, error) {
	// try to grab one form docker...
	tmpdir, err := os.MkdirTemp(os.Getenv("HELPER_TMP"), "consul")
	if err != nil {
		return nil, err
	}

	cfgDir := filepath.Join(tmpdir, "config")
	err = os.Mkdir(cfgDir, 0755)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(cf.consulPath, "agent", "-dev", "--client=0.0.0.0", "-config-dir", cfgDir,
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

	client *api.Client
}

func (i *ConsulInstance) AddConfig(svcId, content string) error {
	fileName := filepath.Join(i.cfgDir, svcId+".json")
	return os.WriteFile(fileName, []byte(content), 0644)
}

func (i *ConsulInstance) AddConfigFromStruct(svcId string, cfg interface{}) error {
	content, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return i.AddConfig(svcId, string(content))
}

func (i *ConsulInstance) ReloadConfig() error {
	err := i.cmd.Process.Signal(syscall.SIGHUP)
	if err != nil {
		return err
	}
	return nil
}

func (i *ConsulInstance) Silence() {
	i.cmd.Stdout = nil
	i.cmd.Stderr = nil
}

// Run starts the ConsulInstance
// When the provided context is Done, the ConsulInstance is cleaned up
func (i *ConsulInstance) Run(ctx context.Context) error {
	go func() {
		// Ensure the ConsulInstance is cleaned up when the Run context is completed
		<-ctx.Done()
		i.Clean()
	}()

	var err error
	i.session, err = gexec.Start(i.cmd, GinkgoWriter, GinkgoWriter)

	if err != nil {
		return err
	}
	EventuallyWithOffset(1, i.session.Out, "5s").Should(gbytes.Say("New leader elected"))
	return nil
}

func (i *ConsulInstance) withClient() *ConsulInstance {
	client, err := api.NewClient(api.DefaultConfig())
	Expect(err).NotTo(HaveOccurred())

	i.client = client
	return i
}

func (i *ConsulInstance) Binary() string {
	return i.consulPath
}

// Clean stops the ConsulInstance
func (i *ConsulInstance) Clean() {
	if i == nil {
		return
	}
	if i.session != nil {
		i.session.Kill()
	}
	if i.cmd != nil && i.cmd.Process != nil {
		i.cmd.Process.Kill()
	}
	if i.tmpdir != "" {
		_ = os.RemoveAll(i.tmpdir)
	}
}

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

	i.registeredServices[svcId] = svcDef

	err := i.AddConfigFromStruct(svcId, svcDef)
	if err != nil {
		return err
	}

	return i.ReloadConfig()
}

// RegisterLiveService While it may be tempting to just reload all config using `consul reload` or marshalling new json and
// sending SIGHUP to the process (per https://www.consul.io/commands/reload), it is preferable to live update
// using the consul APIs as this is a more realistic flow and doesn't fire our watches too actively (which can
// both make debugging hard and hide bugs)
func (i *ConsulInstance) RegisterLiveService(svcName, svcId, address string, tags []string, port uint32) error {
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
	fileName := filepath.Join(i.cfgDir, svcId+".json")
	err = os.Remove(fileName) // ensure we upsert the config update
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	err = os.WriteFile(fileName, content, 0644)
	if err != nil {
		return err
	}
	cmd := exec.Command("curl", "--request", "PUT", "--data", fmt.Sprintf("@%s", fileName), "0.0.0.0:8500/v1/agent/service/register")
	cmd.Dir = i.tmpdir
	cmd.Stdout = GinkgoWriter
	cmd.Stderr = GinkgoWriter
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

// Put wraps the Consul KV Put API call
func (i *ConsulInstance) Put(key string, value []byte) error {
	kvp := &api.KVPair{
		Key:   key,
		Value: value,
	}
	_, err := i.Client().KV().Put(kvp, &api.WriteOptions{})

	return err

}

// Client returns the Consul API client, constructing one if necessary
func (i *ConsulInstance) Client() *api.Client {
	if i.client == nil {
		i = i.withClient()
	}
	return i.client
}
