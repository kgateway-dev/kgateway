package localhelpers

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"io/ioutil"

	"time"

	"github.com/onsi/ginkgo"
)

const defualtNomadDockerImage = "djenriquez/nomad"

type NomadFactory struct {
	nomadpath string
	tmpdir    string
}

func NewNomadFactory() (*NomadFactory, error) {
	nomadpath := os.Getenv("NOMAD_BINARY")

	if nomadpath != "" {
		return &NomadFactory{
			nomadpath: nomadpath,
		}, nil
	}

	// try to grab one form docker...
	tmpdir, err := ioutil.TempDir(os.Getenv("HELPER_TMP"), "nomad")
	if err != nil {
		return nil, err
	}

	bash := fmt.Sprintf(`
set -ex
CID=$(docker run -d  %s /bin/sh -c exit)

# just print the image sha for repoducibility
echo "Using Nomad Image:"
docker inspect %s -f "{{.RepoDigests}}"

docker cp $CID:/bin/nomad .
docker rm -f $CID
    `, defualtNomadDockerImage, defualtNomadDockerImage)
	scriptfile := filepath.Join(tmpdir, "getnomad.sh")

	ioutil.WriteFile(scriptfile, []byte(bash), 0755)

	cmd := exec.Command("bash", scriptfile)
	cmd.Dir = tmpdir
	cmd.Stdout = ginkgo.GinkgoWriter
	cmd.Stderr = ginkgo.GinkgoWriter
	if err := cmd.Run(); err != nil {
		return nil, err
	}

	return &NomadFactory{
		nomadpath: filepath.Join(tmpdir, "nomad"),
		tmpdir:    tmpdir,
	}, nil
}

func (ef *NomadFactory) Clean() error {
	if ef == nil {
		return nil
	}
	if ef.tmpdir != "" {
		os.RemoveAll(ef.tmpdir)

	}
	return nil
}

type NomadInstance struct {
	nomadpath string
	tmpdir    string
	cmd       *exec.Cmd
}

func (ef *NomadFactory) NewNomadInstance() (*NomadInstance, error) {
	// try to grab one form docker...
	tmpdir, err := ioutil.TempDir(os.Getenv("HELPER_TMP"), "nomad")
	if err != nil {
		return nil, err
	}

	return &NomadInstance{
		nomadpath: ef.nomadpath,
		tmpdir:    tmpdir,
	}, nil

}

func (i *NomadInstance) Run() error {
	return i.RunWithPort()
}

func (i *NomadInstance) RunWithPort() error {
	cmd := exec.Command(i.nomadpath, "-dev",
		"--vault-enabled=true",
		"--vault-address=http://127.0.0.1:8200",
		"--vault-token=root",
		"-network-interface=docker0",
	)
	cmd.Dir = i.tmpdir
	cmd.Stdout = ginkgo.GinkgoWriter
	cmd.Stderr = ginkgo.GinkgoWriter
	err := cmd.Start()
	if err != nil {
		return err
	}
	time.Sleep(time.Millisecond * 1500)
	i.cmd = cmd
	return nil
}

func (i *NomadInstance) Binary() string {
	return i.nomadpath
}

func (i *NomadInstance) Clean() error {
	if i.cmd != nil {
		i.cmd.Process.Kill()
		i.cmd.Wait()
	}
	if i.tmpdir != "" {
		os.RemoveAll(i.tmpdir)
	}
	return nil
}
