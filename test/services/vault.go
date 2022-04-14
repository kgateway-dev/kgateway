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

	"regexp"
	"strings"

	"github.com/onsi/ginkgo"
	errors "github.com/rotisserie/eris"
)

const defaultVaultDockerImage = "vault:1.1.3"
const TestPathPrefix = "test-org"

type VaultFactory struct {
	vaultPath  string
	pathprefix string
	tmpdir     string
}

type VaultFactoryConfig struct {
	PathPrefix string
}

func NewVaultFactory(config *VaultFactoryConfig) (*VaultFactory, error) {
	path := os.Getenv("VAULT_BINARY")

	if path != "" {
		return &VaultFactory{
			vaultPath:  path,
			pathprefix: config.PathPrefix,
		}, nil
	}

	vaultPath, err := exec.LookPath("vault")
	if err == nil {
		log.Printf("Using vault from PATH: %s", vaultPath)
		return &VaultFactory{
			vaultPath:  vaultPath,
			pathprefix: config.PathPrefix,
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
		vaultPath:  filepath.Join(tmpdir, "vault"),
		pathprefix: config.PathPrefix,
		tmpdir:     tmpdir,
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
	vaultpath  string
	tmpdir     string
	pathprefix string
	cmd        *exec.Cmd
	token      string
	session    *gexec.Session
}

func (ef *VaultFactory) NewVaultInstance() (*VaultInstance, error) {
	// try to grab one form docker...
	tmpdir, err := ioutil.TempDir(os.Getenv("HELPER_TMP"), "vault")
	if err != nil {
		return nil, err
	}

	return &VaultInstance{
		vaultpath:  ef.vaultPath,
		pathprefix: ef.pathprefix,
		tmpdir:     tmpdir,
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

	out := string(session.Out.Contents())

	tokenSlice := regexp.MustCompile("Root Token: ([\\-[:word:]]+)").FindAllString(out, 1)
	if len(tokenSlice) < 1 {
		return errors.Errorf("%s did not contain root token", out)
	}

	i.token = strings.TrimPrefix(tokenSlice[0], "Root Token: ")

	// We'll need to create a new (secrets engine) path if we're testing the non-default "secret" path
	if i.pathprefix != "" && i.pathprefix != "secret" {
		return i.SetupCustomPathPrefixOnServer()
	}

	return nil
}

func (i *VaultInstance) SetupCustomPathPrefixOnServer() error {
	enableCmd := exec.Command(i.vaultpath,
		"secrets",
		"enable",
		"-address=http://127.0.0.1:8200",
		fmt.Sprintf("-path=%s", i.pathprefix),
		"kv")

	enableCmd.Env = append(enableCmd.Env, fmt.Sprintf("VAULT_TOKEN=%s", i.Token()))

	enableCmdOut, err := enableCmd.CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "enabling kv storage failed: %s", enableCmdOut)
	}
	return nil
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
	cmd.Env = os.Environ()
	// disable DEBUG=1 from getting through to nomad
	for i, pair := range cmd.Env {
		if strings.HasPrefix(pair, "DEBUG") {
			cmd.Env = append(cmd.Env[:i], cmd.Env[i+1:]...)
			break
		}
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("%s (%v)", out, err)
	}
	return string(out), err
}
