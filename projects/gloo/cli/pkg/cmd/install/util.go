package install

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"

	"github.com/solo-io/gloo/pkg/cliutil"
	"github.com/solo-io/gloo/pkg/cliutil/install"
	"github.com/solo-io/go-utils/errors"
	"k8s.io/helm/pkg/chartutil"
	"k8s.io/helm/pkg/renderutil"

	"github.com/solo-io/gloo/pkg/version"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/kubeutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	kubev1 "k8s.io/api/core/v1"
	kubeerrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Returns
func getGlooVersion(opts *options.Options) (string, error) {
	if !version.IsReleaseVersion() {
		if opts.Install.ReleaseVersion == "" {
			return "", errors.Errorf("you must provide a file or a release version " +
				"containing the manifest when running an unreleased version of glooctl.")
		}
		return opts.Install.ReleaseVersion, nil
	} else {
		return version.Version, nil
	}
}

func installFromUri(manifestUri string, opts *options.Options, valuesFileName string) error {

	// Pre-install step writes to k8s. Run only if this is not a dry run.
	if !opts.Install.DryRun {
		if err := preInstall(opts.Install.Namespace); err != nil {
			return errors.Wrapf(err, "pre-install failed")
		}
	}

	var manifestBytes []byte

	switch path.Ext(manifestUri) {
	case ".json", ".yaml", ".yml":
		var err error
		manifestBytes, err = getFileManifestBytes(manifestUri)
		if err != nil {
			return err
		}
	case ".tgz":
		var err error
		renderOpts := renderutil.Options{
			ReleaseOptions: chartutil.ReleaseOptions{
				Namespace: opts.Install.Namespace,
				Name:      "gloo",
			},
		}

		manifestBytes, err = install.GetHelmManifest(manifestUri, valuesFileName, renderOpts, install.ExcludeEmptyManifests)
		if err != nil {
			return err
		}
	default:
		return errors.Errorf("unsupported file extension in manifest URI: %s", path.Ext(manifestUri))
	}

	return installManifest(manifestBytes, opts)
}

func preInstall(namespace string) error {
	if err := registerSettingsCrd(); err != nil {
		return errors.Wrapf(err, "registering settings crd")
	}
	if err := createNamespaceIfNotExist(namespace); err != nil {
		return errors.Wrapf(err, "attempting to create new namespace")
	}
	return nil
}

func installManifest(manifest []byte, opts *options.Options) error {
	if opts.Install.DryRun {
		fmt.Printf("%s", manifest)
		return nil
	}
	if err := kubectlApply(manifest, opts.Install.Namespace); err != nil {
		return errors.Wrapf(err, "running kubectl apply on manifest")
	}
	return nil
}

func kubectlApply(manifest []byte, namespace string) error {
	return kubectl(bytes.NewBuffer(manifest), "apply", "-n", namespace, "-f", "-")
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
		SharedCache: kube.NewKubeCache(context.TODO()),
	})

	return settingsClient.Register()
}

func createNamespaceIfNotExist(namespace string) error {
	restCfg, err := kubeutils.GetConfig("", "")
	if err != nil {
		return err
	}
	kubeClient, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return err
	}
	installNamespace := &kubev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
	if _, err := kubeClient.CoreV1().Namespaces().Create(installNamespace); err != nil && !kubeerrs.IsAlreadyExists(err) {
		return err
	}
	return nil
}

func getFileManifestBytes(uri string) ([]byte, error) {
	manifestFile, err := cliutil.GetResource(uri)
	if err != nil {
		return nil, errors.Wrapf(err, "getting manifest file %v", uri)
	}
	//noinspection GoUnhandledErrorResult
	defer manifestFile.Close()
	manifestBytes, err := ioutil.ReadAll(manifestFile)
	if err != nil {
		return nil, errors.Wrapf(err, "reading manifest file %v", uri)
	}
	return manifestBytes, nil
}
