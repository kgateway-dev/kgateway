package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	errors "github.com/rotisserie/eris"

	"github.com/onsi/gomega"
	"github.com/solo-io/gloo/test/testutils"
	"github.com/solo-io/solo-kit/pkg/utils/protoutils"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/log"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega/gexec"
)

const (
	DefaultHost = "127.0.0.1"
	DefaultPort = 8200

	DefaultVaultToken       = "root"
	defaultVaultDockerImage = "vault:1.12.2"
)

type VaultFactory struct {
	vaultPath string
	tmpdir    string
	useTls    bool
}

// NewVaultFactory returns a VaultFactory
// TODO (sam-heilbron):
// TODO This factory supports a number of mechanisms to Run vault (binary path as env var, lookup vault, docker)
// TODO We should just decide what pattern(s) we want to support and simplify this service to match
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
	tmpdir, err := os.MkdirTemp(os.Getenv("HELPER_TMP"), "vault")
	if err != nil {
		return nil, err
	}

	bash := fmt.Sprintf(`
set -ex
CID=$(docker Run -d  %s /bin/sh -c exit)

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
	hostname  string
	port      uint32
	useTls    bool
	customCfg string
}

func (vf *VaultFactory) MustVaultInstance() *VaultInstance {
	vaultInstance, err := vf.NewVaultInstance()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	return vaultInstance
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
		useTls:    false, // this is not used currently but we know we will need to support it soon
		token:     DefaultVaultToken,
		hostname:  DefaultHost,
		port:      DefaultPort,
	}, nil
}

func (i *VaultInstance) Run(ctx context.Context) error {
	go func() {
		// Ensure the VaultInstance is cleaned up when the Run context is completed
		<-ctx.Done()
		i.Clean()
	}()

	devCmd := "-dev"
	if i.useTls {
		devCmd = "-dev-tls"
	}

	cmd := exec.Command(i.vaultpath,
		"server",
		// https://www.vaultproject.io/docs/concepts/dev-server
		devCmd,
		fmt.Sprintf("-dev-root-token-id=%s", i.token),
		fmt.Sprintf("-dev-listen-address=%s", i.Host()),
	)
	cmd.Dir = i.tmpdir
	cmd.Stdout = ginkgo.GinkgoWriter
	cmd.Stderr = ginkgo.GinkgoWriter
	session, err := gexec.Start(cmd, ginkgo.GinkgoWriter, ginkgo.GinkgoWriter)
	if err != nil {
		return err
	}

	i.cmd = cmd
	i.session = session

	return i.waitForVaultToBeRunning()
}

func (i *VaultInstance) waitForVaultToBeRunning() error {
	pingInterval := time.Tick(time.Millisecond * 100)
	pingDuration := time.Second * 5
	pingEndpoint := fmt.Sprintf("%s:%d", i.hostname, i.port)

	ctx, cancel := context.WithTimeout(context.Background(), pingDuration)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return errors.Errorf("timed out waiting for vault on %s", pingEndpoint)

		case <-pingInterval:
			conn, _ := net.Dial("tcp", pingEndpoint)
			if conn != nil {
				conn.Close()
				return nil
			}
			continue
		}
	}
}

func (i *VaultInstance) Token() string {
	return i.token
}

func (i *VaultInstance) Address() string {
	scheme := "http"
	if i.useTls {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s", scheme, i.Host())
}

func (i *VaultInstance) Host() string {
	return fmt.Sprintf("%s:%d", i.hostname, i.port)
}

func (i *VaultInstance) EnableSecretEngine(secretEngine string) error {
	_, err := i.Exec("secrets", "enable", "-version=2", fmt.Sprintf("-path=%s", secretEngine), "kv")
	return err
}

func (i *VaultInstance) EnableAWSAuthMethod(settings *v1.Settings_VaultSecrets, awsAuthRole string) error {
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
	_, err = i.Exec("write", "auth/aws/role/vault-role", "auth_type=iam", fmt.Sprintf("bound_iam_principal_arn=%s", awsAuthRole), "policies=admin")
	if err != nil {
		return err
	}

	return nil
}

// WriteSecret persists a Secret in Vault
func (i *VaultInstance) WriteSecret(secret *v1.Secret) error {
	requestBuilder := testutils.DefaultRequestBuilder().
		WithPostBody(i.getVaultSecretPayload(secret)).
		WithPath(i.getVaultSecretPath(secret)).
		WithHostname(i.hostname).
		WithPort(i.port).
		WithHeader("X-Vault-Token", i.token)

	_, err := testutils.DefaultHttpClient.Do(requestBuilder.Build())
	return err
}

// getVaultSecretPayload converts a Gloo secret into a string representing the data that will be pushed to Vault
// Mirrors the functionality in: https://github.com/solo-io/solo-kit/blob/9b38e31e4ba879b94dd5ebd925471d0c8f363565/pkg/api/v1/clients/vault/resource_client.go#L47
func (i *VaultInstance) getVaultSecretPayload(secret *v1.Secret) string {
	values := make(map[string]interface{})
	data, err := protoutils.MarshalMap(secret)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "can marshal secret into map")
	values["data"] = data

	vaultSecretBytes, err := json.Marshal(values)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "can unmarshal map into bytes")

	return string(vaultSecretBytes)
}

// getVaultSecretPath returns the path where a Gloo secret will be persisted in Vault
// Mirrors the functionality in: https://github.com/solo-io/solo-kit/blob/9b38e31e4ba879b94dd5ebd925471d0c8f363565/pkg/api/v1/clients/vault/resource_client.go#L335
func (i *VaultInstance) getVaultSecretPath(secret *v1.Secret) string {
	return fmt.Sprintf("v1/secret/data/gloo/gloo.solo.io/v1/Secret/%s/%s",
		secret.GetMetadata().GetNamespace(), secret.GetMetadata().GetName())
}

func (i *VaultInstance) Binary() string {
	return i.vaultpath
}

func (i *VaultInstance) Clean() {
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
