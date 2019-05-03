package services

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"io/ioutil"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"

	"github.com/solo-io/go-utils/log"
)

const defaultConsulDockerImage = "consul@sha256:6ffe55dcc1000126a6e874b298fe1f1b87f556fb344781af60681932e408ec6a"

type ConsulFactory struct {
	consulpath string
	tmpdir     string
}

func NewConsulFactory() (*ConsulFactory, error) {
	consulpath := os.Getenv("CONSUL_BINARY")

	if consulpath != "" {
		return &ConsulFactory{
			consulpath: consulpath,
		}, nil
	}

	consulpath, err := exec.LookPath("consul")
	if err == nil {
		log.Printf("Using consul from PATH: %s", consulpath)
		return &ConsulFactory{
			consulpath: consulpath,
		}, nil
	}

	// try to grab one form docker...
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
    `, defaultConsulDockerImage, defaultConsulDockerImage)
	scriptfile := filepath.Join(tmpdir, "getconsul.sh")

	ioutil.WriteFile(scriptfile, []byte(bash), 0755)

	cmd := exec.Command("bash", scriptfile)
	cmd.Dir = tmpdir
	cmd.Stdout = GinkgoWriter
	cmd.Stderr = GinkgoWriter
	if err := cmd.Run(); err != nil {
		return nil, err
	}

	return &ConsulFactory{
		consulpath: filepath.Join(tmpdir, "consul"),
		tmpdir:     tmpdir,
	}, nil
}

func (ef *ConsulFactory) Clean() error {
	if ef == nil {
		return nil
	}
	if ef.tmpdir != "" {
		os.RemoveAll(ef.tmpdir)

	}
	return nil
}

type ConsulInstance struct {
	consulpath string
	tmpdir     string
	cfgdir     string
	cmd        *exec.Cmd

	session *gexec.Session
}

func (ef *ConsulFactory) NewConsulInstance() (*ConsulInstance, error) {
	// try to grab one form docker...
	tmpdir, err := ioutil.TempDir(os.Getenv("HELPER_TMP"), "consul")
	if err != nil {
		return nil, err
	}

	cfgdir := filepath.Join(tmpdir, "config")
	os.Mkdir(cfgdir, 0755)

	cmd := exec.Command(ef.consulpath, "agent", "-dev", "--client=0.0.0.0", "-config-dir", cfgdir)
	cmd.Dir = ef.tmpdir
	cmd.Stdout = GinkgoWriter
	cmd.Stderr = GinkgoWriter
	return &ConsulInstance{
		consulpath: ef.consulpath,
		tmpdir:     tmpdir,
		cfgdir:     cfgdir,
		cmd:        cmd,
	}, nil

}

func (i *ConsulInstance) AddConfig(name, content string) {
	fname := filepath.Join(i.cfgdir, name)
	ioutil.WriteFile(fname, []byte(content), 0644)
	i.ReloadConfig()
}

func (i *ConsulInstance) AddConfigFromStruct(name string, cfg interface{}) {
	content, err := json.Marshal(cfg)
	Expect(err).NotTo(HaveOccurred())
	i.AddConfig(name, string(content))
}

func (i *ConsulInstance) ReloadConfig() {
	i.cmd.Process.Signal(syscall.SIGHUP)
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
	Eventually(i.session.Out, "5s").Should(gbytes.Say("New leader elected"))
	return nil
}

func (i *ConsulInstance) Binary() string {
	return i.consulpath
}

func (i *ConsulInstance) Clean() error {
	if i.session != nil {
		i.session.Kill()
	}
	if i.tmpdir != "" {
		os.RemoveAll(i.tmpdir)
	}
	return nil
}
