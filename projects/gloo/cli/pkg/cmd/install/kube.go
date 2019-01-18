package install

import (
	"bytes"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/solo-io/gloo/pkg/version"

	"github.com/pkg/errors"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/solo-kit/pkg/utils/kubeutils"
	"k8s.io/api/core/v1"
	kubeerrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/spf13/cobra"
)

// TODO: support configuring install namespace
// requires changing a few places in the yaml as well
const (
	InstallNamespace    = "gloo-system"
	imagePullSecretName = "solo-io-docker-secret"
)

//go:generate sh -c "2gobytes -p install -a glooManifestBytes -i ${GOPATH}/src/github.com/solo-io/gloo/install/gloo.yaml | sed 's@// date.*@@g' > gloo.yaml.go"
//go:generate sh -c "2gobytes -p install -a glooKnativeManifestBytes -i ${GOPATH}/src/github.com/solo-io/gloo/install/gloo-knative.yaml | sed 's@// date.*@@g' > gloo-knative.yaml.go"

func KubeCmd(opts *options.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "kube",
		Short: "install Gloo on kubernetes",
		Long:  "requires kubectl to be installed",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := createImagePullSecretIfNeeded(opts.Install); err != nil {
				return errors.Wrapf(err, "creating image pull secret")
			}

			imageVersion := opts.Install.Version
			if imageVersion == "" {
				imageVersion = version.Version
			}

			return applyManifest(glooManifestBytes, imageVersion)
		},
	}
	pflags := cmd.PersistentFlags()
	flagutils.AddInstallFlags(pflags, &opts.Install)
	return cmd
}

func applyManifest(manifest []byte, imageVersion string) error {
	kubectl := exec.Command("kubectl", "apply", "-f", "-")
	updatedManifest := UpdateBytesWithVersion(manifest, imageVersion)
	kubectl.Stdin = bytes.NewBuffer(updatedManifest)
	kubectl.Stdout = os.Stdout
	kubectl.Stderr = os.Stderr
	return kubectl.Run()
}

func UpdateBytesWithVersion(manifestBytes []byte, version string) []byte {
	if version == "undefined" {
		return manifestBytes
	}
	manifest := string(manifestBytes)
	regexString := `image: soloio/\S+:(.+)`
	regex := regexp.MustCompile(regexString)
	matches := regex.FindStringSubmatch(manifest)
	if len(matches) < 2 {
		return manifestBytes
	}
	oldVersion := matches[1]
	updatedManifest := strings.Replace(manifest, oldVersion, version, -1)
	return []byte(updatedManifest)
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

	kubectl := exec.Command("kubectl", "create", "secret", "docker-registry", "-n", InstallNamespace,
		"--docker-email", install.DockerAuth.Email,
		"--docker-username", install.DockerAuth.Username,
		"--docker-password", install.DockerAuth.Password,
		"--docker-server", install.DockerAuth.Server,
		imagePullSecretName,
	)
	kubectl.Stdout = os.Stdout
	kubectl.Stderr = os.Stderr
	return kubectl.Run()
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
	installNamespace := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: InstallNamespace,
		},
	}
	if _, err := kube.CoreV1().Namespaces().Create(installNamespace); err != nil && !kubeerrs.IsAlreadyExists(err) {
		return err
	}
	return nil
}
