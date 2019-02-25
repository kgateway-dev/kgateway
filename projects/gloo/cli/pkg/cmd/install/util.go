package install

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/go-utils/kubeutils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/helm/pkg/manifest"
	"sigs.k8s.io/yaml"

	"github.com/solo-io/gloo/pkg/cliutil/install"
	"github.com/solo-io/go-utils/errors"
	"k8s.io/helm/pkg/chartutil"
	"k8s.io/helm/pkg/renderutil"

	"github.com/solo-io/gloo/pkg/version"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
)

// Entry point for all three GLoo installation commands
func installGloo(opts *options.Options, valueFileName string) error {

	// Get Gloo release version
	version, err := getGlooVersion(opts)
	if err != nil {
		return err
	}

	// Get location of Gloo helm chart
	helmChartArchiveUri := fmt.Sprintf(constants.GlooHelmRepoTemplate, version)
	if helmChartOverride := opts.Install.HelmChartOverride; helmChartOverride != "" {
		helmChartArchiveUri = helmChartOverride
	}

	if err := installFromUri(helmChartArchiveUri, opts, valueFileName); err != nil {
		return errors.Wrapf(err, "installing Gloo from helm chart")
	}
	return nil
}

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

func installFromUri(helmArchiveUri string, opts *options.Options, valuesFileName string) error {

	if path.Ext(helmArchiveUri) != ".tgz" && !strings.HasSuffix(helmArchiveUri, ".tar.gz") {
		return errors.Errorf("unsupported file extension in manifest URI: %s", path.Ext(helmArchiveUri))
	}

	chart, err := install.GetHelmArchive(helmArchiveUri)
	if err != nil {
		return errors.Wrapf(err, "retrieving gloo helm chart archive")
	}

	values, err := install.GetValueFile(chart, valuesFileName)

	crdChart, err := install.GetCrdChart(chart)
	if err != nil {
		return errors.Wrapf(err, "retrieving crd sub-chart")
	}

	// These are the .Release.* variables used during rendering
	renderOpts := renderutil.Options{
		ReleaseOptions: chartutil.ReleaseOptions{
			Namespace: opts.Install.Namespace,
			Name:      "gloo", // TODO: make configurable
		},
	}

	// FILTER FUNCTION 1: Exclude knative install if necessary
	filterKnativeResources, err := GetKnativeResourceFilterFunction()
	if err != nil {
		return err
	}

	// FILTER FUNCTION 2: Just collect the names of the CRD that need to be registered
	var crdNames []string
	extractCrdNames := func(input []manifest.Manifest) ([]manifest.Manifest, error) {
		for _, man := range input {
			for _, doc := range strings.Split(man.Content, "---") {

				// We need to define this ourselves, because if we unmarshal into `apiextensions.CustomResourceDefinition`
				// we don't get the TypeMeta (in the yaml they are nested under `metadata`, but the k8s struct has
				// them as top level fields...)
				var crd struct{ Metadata v1.ObjectMeta }
				if err := yaml.Unmarshal([]byte(doc), &crd); err != nil {
					return nil, errors.Wrapf(err, "parsing CRD: %s", doc)
				}
				if crdName := crd.Metadata.Name; crdName != "" {
					crdNames = append(crdNames, crdName)
				}
			}
		}
		return input, nil
	}

	// Render and install CRD manifests
	crdManifestBytes, err := install.RenderChart(crdChart, values, renderOpts, filterKnativeResources, extractCrdNames, install.ExcludeEmptyManifests)
	if err != nil {
		return errors.Wrapf(err, "rendering crd sub-chart")
	}
	if err := installManifest(crdManifestBytes, opts.Install.DryRun); err != nil {
		return errors.Wrapf(err, "installing crd manifest")
	}

	// Only run if this is not a dry run
	if !opts.Install.DryRun {
		if err := waitForCrdsToBeRegistered(crdNames, time.Second*5, time.Millisecond*500); err != nil {
			return errors.Wrapf(err, "waiting for crds to be registered")
		}

	}

	// Render and install Gloo manifest
	manifestBytes, err := install.RenderChart(chart, values, renderOpts, filterKnativeResources, install.ExcludeEmptyManifests)
	if err != nil {
		return err
	}
	return installManifest(manifestBytes, opts.Install.DryRun)
}

func installManifest(manifest []byte, isDryRun bool) error {
	if isDryRun {
		fmt.Printf("%s", manifest)
		ioutil.WriteFile("gloo-install-"+time.Now().String()+".yaml", manifest, 0777)
		return nil
	}
	if err := kubectlApply(manifest); err != nil {
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

// If this is a knative deployment, we have to check whether knative itself is already installed in the cluster.
// If knative is already installed and we don't own it, don't install/upgrade it (It's okay to update the installation if we own it).

// If this is not a knative deployment, skipKnativeInstall might still evaluate to true, but in that case Helm will
// filter out all the knative resources during template rendering.
func GetKnativeResourceFilterFunction() (install.ManifestFilterFunc, error) {
	installed, ours, err := knativeInstalled()
	if err != nil {
		return nil, err
	}
	skipKnativeInstall := installed && !ours
	return func(input []manifest.Manifest) ([]manifest.Manifest, error) {
		var output []manifest.Manifest
		for _, man := range input {
			if strings.Contains(man.Name, "knative") && skipKnativeInstall {
				continue
			}
			output = append(output, man)
		}
		return output, nil
	}, nil

}

func knativeInstalled() (isInstalled bool, isOurs bool, err error) {
	restCfg, err := kubeutils.GetConfig("", "")
	if err != nil {
		return false, false, err
	}
	kube, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return false, false, err
	}
	namespaces, err := kube.CoreV1().Namespaces().List(metav1.ListOptions{})
	if err != nil {
		return false, false, err
	}
	for _, ns := range namespaces.Items {
		if ns.Name == constants.KnativeServingNamespace {
			ours := ns.Labels != nil && ns.Labels["app"] == "gloo"
			return true, ours, nil
		}
	}
	return false, false, nil
}

// register knative crds first
func waitForCrdsToBeRegistered(crds []string, timeout, interval time.Duration) error {
	if len(crds) == 0 {
		return nil
	}

	// TODO: think about improving
	// Just pick the last crd in the list an wait for it to be created. It is reasonable to assume that by the time we
	// get to applying the manifest the other ones will be ready as well.
	crdName := crds[len(crds)-1]

	elapsed := time.Duration(0)
	for {
		select {
		case <-time.After(interval):
			if err := kubectl(nil, "get", crdName); err == nil {
				return nil
			}
			elapsed += interval
			if elapsed > timeout {
				return errors.Errorf("failed to confirm knative crd registration after %v", timeout)
			}
		}
	}
}
