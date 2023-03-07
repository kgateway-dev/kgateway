package services

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/log"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega/gexec"
)

const defaultVaultDockerImage = "vault:1.12.2"
const defaultAddress = "127.0.0.1:8200"
const DefaultVaultToken = "root"
const defaultAWSArn = "arn:aws:iam::802411188784:user/gloo-edge-e2e-user"

type VaultFactory struct {
	vaultPath string
	tmpdir    string
	useTls    bool
}

type VaultFactoryConfig struct {
	PathPrefix string
	UseTls     bool
}

func NewVaultFactory() (*VaultFactory, error) {
	return NewVaultFactoryForConfig(&VaultFactoryConfig{})
}

func NewVaultFactoryForConfig(cfg *VaultFactoryConfig) (*VaultFactory, error) {
	if cfg == nil {
		cfg = &VaultFactoryConfig{}
	}
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
	tmpdir, err := os.MkdirTemp(os.Getenv("HELPER_TMP"), "vault")
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

	os.WriteFile(scriptfile, []byte(bash), 0755)

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
		useTls:    cfg.UseTls,
	}, nil
}

func (vf *VaultFactory) Clean() error {
	if vf == nil {
		return nil
	}
	if vf.tmpdir != "" {
		os.RemoveAll(vf.tmpdir)

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
	useTls    bool
	customCfg string
}

func (vf *VaultFactory) NewVaultInstance() (*VaultInstance, error) {
	// try to get an executable from docker...
	tmpdir, err := os.MkdirTemp(os.Getenv("HELPER_TMP"), "vault")
	if err != nil {
		return nil, err
	}

	return &VaultInstance{
		vaultpath: vf.vaultPath,
		tmpdir:    tmpdir,
		useTls:    vf.useTls,
	}, nil

}

func (i *VaultInstance) Run() error {
	return i.RunWithAddress(defaultAddress)
}

func (i *VaultInstance) RunWithAddress(address string) error {
	i.token = DefaultVaultToken
	i.address = address
	devCmd := "-dev"
	if i.useTls {
		devCmd = "-dev-tls"
	}

	cmd := exec.Command(i.vaultpath,
		"server",
		// https://www.vaultproject.io/docs/concepts/dev-server
		devCmd,
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
	i.address = address

	return nil
}

func (i *VaultInstance) Token() string {
	return i.token
}

func (i *VaultInstance) Address() string {
	scheme := "http"
	if i.useTls {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s", scheme, i.address)
}

func (i *VaultInstance) EnableSecretEngine(secretEngine string) error {
	_, err := i.Exec("secrets", "enable", "-version=2", fmt.Sprintf("-path=%s", secretEngine), "kv")
	return err
}

func (i *VaultInstance) EnableAWSAuthMethod(settings *v1.Settings_VaultSecrets) error {
	// Enable the AWS auth method
	_, err := i.Exec("auth", "enable", "aws")
	if err != nil {
		return err
	}

	// Add our admin policy
	tmpFileName := filepath.Join(i.tmpdir, "policy.json")
	err = os.WriteFile(tmpFileName, []byte(`{"path":{"*":{"capabilities":["create","read","update","delete","list","patch","sudo"]}}}`), 0666)
	if err != nil {
		return err
	}
	_, err = i.Exec("policy", "write", "admin", tmpFileName)
	if err != nil {
		return err
	}

	// Configure the AWS auth method with the creds provided
	_, err = i.Exec("write", "auth/aws/config/client", fmt.Sprintf("secret_key=%s", settings.GetAws().GetSecretAccessKey()), fmt.Sprintf("access_key=%s", settings.GetAws().GetAccessKeyId()))
	if err != nil {
		return err
	}

	// Configure the Vault role to align with the provided AWS role
	_, err = i.Exec("write", "auth/aws/role/vault-role", "auth_type=iam", fmt.Sprintf("bound_iam_principal_arn=%s", defaultAWSArn), "policies=admin")
	if err != nil {
		return err
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
