package install

import (
	"bytes"
	"fmt"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/version"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/go-utils/kubeutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"io"
	"io/ioutil"
	kubev1 "k8s.io/api/core/v1"
	kubeerrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"net/http"
	"os"
	"os/exec"
)

const (
	installNamespace    = defaults.GlooSystem
	imagePullSecretName = "solo-io-docker-secret"
)

func preInstall(opts *options.Options) error {
	if err := createImagePullSecretIfNeeded(opts.Install); err != nil {
		return errors.Wrapf(err, "creating image pull secret")
	}
	if err := registerSettingsCrd(); err != nil {
		return errors.Wrapf(err, "registering settings crd")
	}
	return nil
}

func installFromUrl(opts *options.Options, manifestUrlTemplate string) error {
	releaseVersion := version.Version
	// override release version
	if opts.Install.ReleaseVersion != "" {
		releaseVersion = opts.Install.ReleaseVersion
	}
	manifestBytes, err := readReleaseManifest(releaseVersion, manifestUrlTemplate)
	if err != nil {
		return errors.Wrapf(err, "reading gloo ingress manifest")
	}
	if opts.Install.DryRun {
		fmt.Printf("%s", manifestBytes)
		return nil
	}
	if err := kubectlApply(manifestBytes); err != nil {
		return errors.Wrapf(err, "running kubectl apply on manifest")
	}
	return nil
}

func kubectlApply(manifest []byte) error {
	return kubectl(bytes.NewBuffer(manifest), "apply", "-f", "-")
}

func kubectl(stdin io.Reader, args ...string) error {
	kubectl := exec.Command("kubectl", args...)
	if stdin != nil {
		kubectl.Stdin = stdin
	}
	kubectl.Stdout = os.Stdout
	kubectl.Stderr = os.Stderr
	return kubectl.Run()
}

func registerSettingsCrd() error {
	cfg, err := kubeutils.GetConfig("", os.Getenv("KUBECONFIG"))
	if err != nil {
		return err
	}

	settingsClient, err := v1.NewSettingsClient(&factory.KubeResourceClientFactory{
		Crd:         v1.SettingsCrd,
		Cfg:         cfg,
		SharedCache: kube.NewKubeCache(),
	})

	return settingsClient.Register()
}

func createImagePullSecretIfNeeded(install options.Install) error {
	if err := createNamespaceIfNotExist(); err != nil {
		return errors.Wrapf(err, "creating installation namespace")
	}
	dockerSecretDesired := install.DockerAuth.Username != "" ||
		install.DockerAuth.Password != "" ||
		install.DockerAuth.Email != ""

	if !dockerSecretDesired {
		return nil
	}

	validOpts := install.DockerAuth.Username != "" &&
		install.DockerAuth.Password != "" &&
		install.DockerAuth.Email != "" &&
		install.DockerAuth.Server != ""

	if !validOpts {
		return errors.Errorf("must provide one of each flag for docker authentication: \n" +
			"--docker-email \n" +
			"--docker-username \n" +
			"--docker-password \n")
	}

	if install.DryRun {
		return nil
	}

	return kubectl(nil, "create", "secret", "docker-registry", "-n", installNamespace,
		"--docker-email", install.DockerAuth.Email,
		"--docker-username", install.DockerAuth.Username,
		"--docker-password", install.DockerAuth.Password,
		"--docker-server", install.DockerAuth.Server,
		imagePullSecretName,
	)
}

func createNamespaceIfNotExist() error {
	restCfg, err := kubeutils.GetConfig("", "")
	if err != nil {
		return err
	}
	kube, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return err
	}
	installNamespace := &kubev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: installNamespace,
		},
	}
	if _, err := kube.CoreV1().Namespaces().Create(installNamespace); err != nil && !kubeerrs.IsAlreadyExists(err) {
		return err
	}
	return nil
}

func readFile(url string) ([]byte, error) {
	// Get the data
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("http GET returned status %d", resp.StatusCode)
	}

	// Write the body to file
	return ioutil.ReadAll(resp.Body)
}

func readReleaseManifest(releaseVersion, urlTemplate string) ([]byte, error) {
	if releaseVersion == version.UndefinedVersion || releaseVersion == version.DevVersion {
		return nil, errors.Errorf("You must provide a file containing the knative manifest when running an unreleased version of glooctl.")
	}
	url := fmt.Sprintf(urlTemplate, releaseVersion)
	bytes, err := readFile(url)
	if err != nil {
		return nil, errors.Wrapf(err, "Error reading manifest for gloo version %s at url %s", releaseVersion, url)
	}
	return bytes, nil
}
