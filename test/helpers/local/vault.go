package localhelpers

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"io/ioutil"

	"time"

	"bytes"
	"io"
	"regexp"
	"strings"

	"github.com/onsi/ginkgo"
	"github.com/pkg/errors"
)

const defualtVaultDockerImage = "vault"

type VaultFactory struct {
	vaultpath string
	tmpdir    string
}

func NewVaultFactory() (*VaultFactory, error) {
	envoypath := os.Getenv("VAULT_BINARY")

	if envoypath != "" {
		return &VaultFactory{
			vaultpath: envoypath,
		}, nil
	}

	// try to grab one form docker...
	tmpdir, err := ioutil.TempDir(os.Getenv("HELPER_TMP"), "vault")
	if err != nil {
		return nil, err
	}

	bash := fmt.Sprintf(`
set -ex
CID=$(docker run -d  %s /bin/sh -c exit)

# just print the image sha for repoducibility
echo "Using Vault Image:"
docker inspect %s -f "{{.RepoDigests}}"

docker cp $CID:/bin/vault .
docker rm -f $CID
    `, defualtVaultDockerImage, defualtVaultDockerImage)
	scriptfile := filepath.Join(tmpdir, "getvault.sh")

	ioutil.WriteFile(scriptfile, []byte(bash), 0755)

	cmd := exec.Command("bash", scriptfile)
	cmd.Dir = tmpdir
	cmd.Stdout = ginkgo.GinkgoWriter
	cmd.Stderr = ginkgo.GinkgoWriter
	if err := cmd.Run(); err != nil {
		return nil, err
	}

	return &VaultFactory{
		vaultpath: filepath.Join(tmpdir, "vault"),
		tmpdir:    tmpdir,
	}, nil
}

func (ef *VaultFactory) Clean() error {
	if ef == nil {
		return nil
	}
	if ef.tmpdir != "" {
		os.RemoveAll(ef.tmpdir)

	}
	return nil
}

type VaultInstance struct {
	vaultpath string
	tmpdir    string
	cmd       *exec.Cmd
	token     string
}

func (ef *VaultFactory) NewVaultInstance() (*VaultInstance, error) {
	// try to grab one form docker...
	tmpdir, err := ioutil.TempDir(os.Getenv("HELPER_TMP"), "vault")
	if err != nil {
		return nil, err
	}

	return &VaultInstance{
		vaultpath: ef.vaultpath,
		tmpdir:    tmpdir,
	}, nil

}

func (i *VaultInstance) Run() error {
	return i.RunWithPort()
}

func (i *VaultInstance) Token() string {
	return i.token
}

func (i *VaultInstance) RunWithPort() error {
	cmd := exec.Command(i.vaultpath,
		"server",
		"-dev",
		"-dev-root-token-id=root",
		"-dev-listen-address=0.0.0.0:8200",
	)
	buf := &bytes.Buffer{}
	w := io.MultiWriter(ginkgo.GinkgoWriter, buf)
	cmd.Dir = i.tmpdir
	cmd.Stdout = w
	cmd.Stderr = w
	err := cmd.Start()
	if err != nil {
		return err
	}
	time.Sleep(time.Millisecond * 1500)
	i.cmd = cmd

	tokenSlice := regexp.MustCompile("Root Token: ([\\-[:word:]]+)").FindAllString(buf.String(), 1)
	if len(tokenSlice) < 1 {
		return errors.Errorf("%s did not contain root token", buf.String())
	}

	i.token = strings.TrimPrefix(tokenSlice[0], "Root Token: ")

	return nil
}

func (i *VaultInstance) Binary() string {
	return i.vaultpath
}

func (i *VaultInstance) Clean() error {
	if i.cmd != nil {
		i.cmd.Process.Kill()
		i.cmd.Wait()
	}
	if i.tmpdir != "" {
		os.RemoveAll(i.tmpdir)
	}
	return nil
}
