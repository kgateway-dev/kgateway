package services

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/onsi/gomega/gexec"
	"github.com/solo-io/go-utils/log"

	"io/ioutil"

	"time"

	"strings"

	"github.com/onsi/ginkgo"
)

const defaultVaultDockerImage = "vault:1.1.3"
const defaultAddress = "127.0.0.1:8200"

type VaultFactory struct {
	vaultPath string
	tmpdir    string
}

type VaultFactoryConfig struct {
	PathPrefix string
}

func NewVaultFactory() (*VaultFactory, error) {
	path := os.Getenv("VAULT_BINARY")

	if path != "" {
		return &VaultFactory{
			vaultPath: path,
		}, nil
	}

	vaultPath, err := exec.LookPath("vault")
	if err == nil {
		log.Printf("Using vault from PATH: %s", vaultPath)
		return &VaultFactory{
			vaultPath: vaultPath,
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
    `, defaultVaultDockerImage, defaultVaultDockerImage)
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
		vaultPath: filepath.Join(tmpdir, "vault"),
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
	session   *gexec.Session
	token     string
	address   string
}

func (ef *VaultFactory) NewVaultInstance() (*VaultInstance, error) {
	// try to grab one form docker...
	tmpdir, err := ioutil.TempDir(os.Getenv("HELPER_TMP"), "vault")
	if err != nil {
		return nil, err
	}

	return &VaultInstance{
		vaultpath: ef.vaultPath,
		tmpdir:    tmpdir,
	}, nil

}

func (i *VaultInstance) Run() error {
	return i.RunWithAddress(defaultAddress)
}

func (i *VaultInstance) RunWithAddress(address string) error {
	i.token = "root"
	i.address = address

	cmd := exec.Command(i.vaultpath,
		"server",
		// https://www.vaultproject.io/docs/concepts/dev-server
		"-dev",
		fmt.Sprintf("-dev-root-token-id=%s", i.token),
		fmt.Sprintf("-dev-listen-address=%s", i.address),
	)
	cmd.Dir = i.tmpdir
	cmd.Stdout = ginkgo.GinkgoWriter
	cmd.Stderr = ginkgo.GinkgoWriter
	session, err := gexec.Start(cmd, ginkgo.GinkgoWriter, ginkgo.GinkgoWriter)
	if err != nil {
		return err
	}
	time.Sleep(time.Millisecond * 1500)
	i.cmd = cmd
	i.session = session

	//out := string(session.Out.Contents())

	//tokenSlice := regexp.MustCompile("Root Token: ([\\-[:word:]]+)").FindAllString(out, 1)
	//if len(tokenSlice) < 1 {
	///	return errors.Errorf("%s did not contain root token", out)
	//}

	i.address = address
	//i.token = strings.TrimPrefix(tokenSlice[0], "Root Token: ")

	return nil
}

func (i *VaultInstance) Token() string {
	return i.token
}

func (i *VaultInstance) Address() string {
	return fmt.Sprintf("http://%s", i.address)
}

func (i *VaultInstance) EnableSecretEngine(secretEngine string) error {
	_, err := i.Exec("secrets", "enable", "-version=2", fmt.Sprintf("-path=%s", secretEngine), "kv")
	return err
}

func (i *VaultInstance) Binary() string {
	return i.vaultpath
}

func (i *VaultInstance) Clean() error {
	if i.session != nil {
		i.session.Kill()
	}
	if i.cmd != nil && i.cmd.Process != nil {
		i.cmd.Process.Kill()
	}
	if i.tmpdir != "" {
		return os.RemoveAll(i.tmpdir)
	}
	return nil
}

func (i *VaultInstance) Exec(args ...string) (string, error) {
	cmd := exec.Command(i.vaultpath, args...)
	cmd.Dir = i.tmpdir
	cmd.Env = os.Environ()
	// disable DEBUG=1 from getting through to nomad
	for e, pair := range cmd.Env {
		if strings.HasPrefix(pair, "DEBUG") {
			cmd.Env = append(cmd.Env[:e], cmd.Env[e+1:]...)
			break
		}
	}
	cmd.Env = append(
		cmd.Env,
		fmt.Sprintf("VAULT_TOKEN=%s", i.Token()),
		fmt.Sprintf("VAULT_ADDR=%s", i.Address()))

	out, err := cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("%s (%v)", out, err)
	}
	return string(out), err
}
