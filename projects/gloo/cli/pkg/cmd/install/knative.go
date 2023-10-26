package install

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options/contextoptions"

	"github.com/avast/retry-go"

	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/k8s-utils/kubeutils"
	"go.uber.org/zap"
	"helm.sh/helm/v3/pkg/chartutil"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/pkg/cliutil/install"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"sigs.k8s.io/yaml"
)

const (
	installedByUsAnnotationKey = "gloo.solo.io/glooctl_install_info"

	servingReleaseUrlTemplate    = "https://github.com/knative/serving/releases/download/v%v/serving.yaml"
	eventingReleaseUrlTemplate   = "https://github.com/knative/eventing/releases/download/v%v/release.yaml"
	monitoringReleaseUrlTemplate = "https://github.com/knative/serving/releases/download/v%v/monitoring.yaml"

	knativeIngressProviderLabel = "networking.knative.dev/ingress-provider"
	knativeIngressProviderIstio = "istio"

	yamlJoiner = "\n---\n"
)

// copied over from solo-kit to avoid a dependency on ginkgo
func kubectlOut(args ...string) (string, error) {
	cmd := exec.Command("kubectl", args...)
	cmd.Env = os.Environ()
	// disable DEBUG=1 from getting through to kube
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

func waitKnativeApiserviceReady() error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	for {
		stdout, err := kubectlOut("get", "apiservice", "-ojsonpath='{.items[*].status.conditions[*].status}'")
		if err != nil {
			contextutils.CliLogErrorw(ctx, "error getting apiserverice", "err", err)
		}
		if !strings.Contains(stdout, "False") {
			// knative apiservice is ready, we can attempt gloo installation now!
			break
		}
		if ctx.Err() != nil {
			return eris.Errorf("timed out waiting for knative apiservice to be ready: %v", ctx.Err())
		}
		time.Sleep(1 * time.Second)
	}
	return nil
}

func knativeCmd(opts *options.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:        "knative",
		Short:      "install Knative with Gloo on Kubernetes",
		Long:       "requires kubectl to be installed",
		Deprecated: "Knative with Gloo is deprecated in Gloo Edge 1.10 and will not be available in Gloo Edge 1.11",
		PreRun:     setVerboseMode(opts),
		RunE: func(cmd *cobra.Command, args []string) error {

			if opts.Install.Knative.InstallKnative {
				if !opts.Install.DryRun {
					installed, _, err := checkKnativeInstallation(opts.Top.Ctx)
					if err != nil {
						return eris.Wrapf(err, "checking for existing knative installation")
					}
					if installed {
						return eris.Errorf("knative-serving namespace found. please " +
							"uninstall the previous version of knative, or re-run this command with --install-knative=false")
					}
				}

				if err := installKnativeServing(opts); err != nil {
					return eris.Wrapf(err, "installing knative components failed. "+
						"options used: %#v", opts.Install.Knative)
				}
			}

			if !opts.Install.Knative.SkipGlooInstall {
				// wait for knative apiservice (autoscaler metrics) to be healthy before attempting gloo installation
				// if we try to install before it's ready, helm is unhappy because it can't get apiservice endpoints
				// we don't care about this if we're doing a dry run installation
				if !opts.Install.DryRun {
					if err := waitKnativeApiserviceReady(); err != nil {
						return err
					}
				}

				knativeValues, err := RenderKnativeValues(opts.Install.Knative.InstallKnativeVersion)
				if err != nil {
					return err
				}
				knativeOverrides, err := chartutil.ReadValues([]byte(knativeValues))
				if err != nil {
					return eris.Wrapf(err, "parsing override values for knative mode")
				}

				if err := NewInstaller(opts, DefaultHelmClient()).Install(&InstallerConfig{
					InstallCliArgs: &opts.Install,
					ExtraValues:    knativeOverrides,
					Verbose:        opts.Top.Verbose,
					Ctx:            opts.Top.Ctx,
				}); err != nil {
					return eris.Wrapf(err, "installing gloo edge in knative mode")
				}
			}
			return nil
		},
	}
	pflags := cmd.PersistentFlags()
	flagutils.AddGlooInstallFlags(cmd.Flags(), &opts.Install)
	flagutils.AddKnativeInstallFlags(pflags, &opts.Install.Knative)
	return cmd
}

func installKnativeServing(opts *options.Options) error {
	knativeOpts := opts.Install.Knative

	// store the opts as a label on the knative-serving namespace
	// we can use this to uninstall later on
	knativeOptsJson, err := json.Marshal(knativeOpts)
	if err != nil {
		return err
	}

	manifests, err := RenderKnativeManifests(knativeOpts)
	if err != nil {
		return err
	}
	if opts.Install.DryRun {
		fmt.Printf("%s", manifests)
		// For safety, print a YAML separator so multiple invocations of this function will produce valid output
		fmt.Printf(yamlJoiner)
		return nil
	}

	knativeCrdNames, knativeCrdManifests, err := getCrdManifests(manifests)
	if err != nil {
		return err
	}

	// install crds first
	fmt.Fprintln(os.Stderr, "installing Knative CRDs...")
	if err := install.KubectlApply([]byte(knativeCrdManifests)); err != nil {
		return eris.Wrapf(err, "installing knative crds with kubectl apply")
	}

	if err := waitForCrdsToBeRegistered(opts.Top.Ctx, knativeCrdNames); err != nil {
		return eris.Wrapf(err, "waiting for knative CRDs to be registered")
	}

	fmt.Fprintln(os.Stderr, "installing Knative...")

	if err := install.KubectlApply([]byte(manifests)); err != nil {
		// may need to retry the apply once in order to work around webhook race issue
		// https://github.com/knative/serving/issues/6353
		// https://knative.slack.com/archives/CA9RHBGJX/p1577458311043200
		if err2 := install.KubectlApply([]byte(manifests)); err2 != nil {
			return eris.Wrapf(err, "installing knative resources failed with retried kubectl apply: %v", err2)
		}
	}
	// label the knative-serving namespace as belonging to us
	if err := install.Kubectl(nil, "annotate", "namespace",
		"knative-serving", installedByUsAnnotationKey+"="+string(knativeOptsJson)); err != nil {
		return eris.Wrapf(err, "annotating installation namespace")
	}

	fmt.Fprintln(os.Stderr, "Knative successfully installed!")
	return nil
}

// if knative is present but was not installed by us, the return values will be true, nil, nil
func checkKnativeInstallation(ctx context.Context, kubeclient ...kubernetes.Interface) (bool, *options.Knative, error) {
	var kc kubernetes.Interface
	if len(kubeclient) > 0 {
		kc = kubeclient[0]
	} else {
		kubecontext := contextoptions.KubecontextFrom(ctx)
		kc = helpers.MustKubeClientWithKubecontext(kubecontext)
	}
	namespaces, err := kc.CoreV1().Namespaces().List(ctx, v1.ListOptions{})
	if err != nil {
		return false, nil, err
	}
	for _, ns := range namespaces.Items {
		if ns.Name == constants.KnativeServingNamespace {
			if ns.Annotations != nil && ns.Annotations[installedByUsAnnotationKey] != "" {
				installOpts := ns.Annotations[installedByUsAnnotationKey]
				var opts options.Knative
				if err := yaml.Unmarshal([]byte(installOpts), &opts); err != nil {
					return false, nil, eris.Wrapf(err, "parsing install opts "+
						"from knative-serving namespace annotation %v", installedByUsAnnotationKey)
				}
				return true, &opts, nil
			}
			return true, nil, nil
		}
	}
	return false, nil, nil
}

// used by e2e test
func RenderKnativeManifests(opts options.Knative) (string, error) {
	knativeVersion := opts.InstallKnativeVersion
	eventing := opts.InstallKnativeEventing
	monitoring := opts.InstallKnativeMonitoring

	servingReleaseUrl := fmt.Sprintf(servingReleaseUrlTemplate, knativeVersion)
	servingManifest, err := getManifestForInstallation(servingReleaseUrl)
	if err != nil {
		return "", err
	}
	outputManifests := []string{servingManifest}

	if eventing {
		eventingReleaseUrl := fmt.Sprintf(eventingReleaseUrlTemplate, opts.InstallKnativeEventingVersion)
		eventingManifest, err := getManifestForInstallation(eventingReleaseUrl)
		if err != nil {
			return "", err
		}
		outputManifests = append(outputManifests, eventingManifest)
	}

	if monitoring {
		monitoringReleaseUrl := fmt.Sprintf(monitoringReleaseUrlTemplate, knativeVersion)
		monitoringManifest, err := getManifestForInstallation(monitoringReleaseUrl)
		if err != nil {
			return "", err
		}
		outputManifests = append(outputManifests, monitoringManifest)
	}

	return strings.Join(outputManifests, yamlJoiner), nil
}

func getManifestForInstallation(url string) (string, error) {
	var (
		err      error
		response *http.Response
	)

	err = retry.Do(func() error {
		response, err = http.Get(url)
		if err != nil {
			return err
		}
		if response.StatusCode != 200 {
			return eris.Errorf("returned non-200 status code: %v %v", response.StatusCode, response.Status)
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	return removeIstioResources(string(raw))
}

func removeIstioResources(manifest string) (string, error) {
	var outputObjectsYaml []string

	// parse runtime.Objects from the input yaml
	objects, err := parseUnstructured(manifest)
	if err != nil {
		return "", err
	}

	for _, object := range objects {
		// objects parsed by UnstructuredJSONScheme can only be of
		// type *unstructured.Unstructured or *unstructured.UnstructuredList
		switch unstructuredObj := object.obj.(type) {
		case *unstructured.Unstructured:
			// append the object if it matches the provided labels
			if containsIstioLabels(unstructuredObj) {
				continue
			}
			outputObjectsYaml = append(outputObjectsYaml, object.yaml)
		case *unstructured.UnstructuredList:
			// filter the list items based on label
			var filteredItems []unstructured.Unstructured
			for _, obj := range unstructuredObj.Items {
				if containsIstioLabels(&obj) {
					continue
				}
				filteredItems = append(filteredItems, obj)
			}
			// only append the list if it still contains items after being filtered
			switch len(filteredItems) {
			case 0:
				// the whole list was filtered, omit it from the resultant yaml
				continue
			case len(unstructuredObj.Items):
				// nothing was filtered from the list, use the original yaml
				outputObjectsYaml = append(outputObjectsYaml, object.yaml)
			default:
				unstructuredObj.Items = filteredItems
				// list was partially filtered, we need to re-marshal it
				rawJson, err := runtime.Encode(unstructured.UnstructuredJSONScheme, unstructuredObj)
				if err != nil {
					return "", err
				}
				rawYaml, err := yaml.JSONToYAML(rawJson)
				if err != nil {
					return "", err
				}
				outputObjectsYaml = append(outputObjectsYaml, string(rawYaml))
			}
		default:
			contextutils.LoggerFrom(context.Background()).DPanic(fmt.Sprintf("unknown object type %T parsed from yaml: \n%v ", object.obj, object.yaml))
			return "", eris.Errorf("unknown object type %T parsed from yaml: \n%v ", object.obj, object.yaml)
		}
	}

	// re-join the objects into a single manifest
	return strings.Join(outputObjectsYaml, yamlJoiner), nil
}

func containsIstioLabels(obj *unstructured.Unstructured) bool {
	labels := obj.GetLabels()
	if labels == nil {
		return false
	}
	return labels[knativeIngressProviderLabel] == knativeIngressProviderIstio
}

var yamlSeparatorRegex = regexp.MustCompile("\n---")

// a tuple to represent a kubernetes object along with the original yaml snippet it was parsed from
type objectYamlTuple struct {
	obj  runtime.Object
	yaml string
}

func parseUnstructured(manifest string) ([]objectYamlTuple, error) {
	objectYamls := yamlSeparatorRegex.Split(manifest, -1)

	var resources []objectYamlTuple

	for _, objectYaml := range objectYamls {
		// empty yaml snippets, such as those which can be
		// generated by helm should be ignored
		// else they may be parsed into empty map[string]interface{} objects
		if isEmptyYamlSnippet(objectYaml) {
			continue
		}
		jsn, err := yaml.YAMLToJSON([]byte(objectYaml))
		if err != nil {
			return nil, err
		}
		runtimeObj, err := runtime.Decode(unstructured.UnstructuredJSONScheme, jsn)
		if err != nil {
			return nil, err
		}

		resources = append(resources, objectYamlTuple{obj: runtimeObj, yaml: objectYaml})
	}

	return resources, nil
}

var commentRegex = regexp.MustCompile("#.*")

func isEmptyYamlSnippet(objYaml string) bool {
	removeComments := commentRegex.ReplaceAllString(objYaml, "")
	removeNewlines := strings.Replace(removeComments, "\n", "", -1)
	removeDashes := strings.Replace(removeNewlines, "---", "", -1)
	removeSpaces := strings.Replace(removeDashes, " ", "", -1)
	removeNull := strings.Replace(removeSpaces, "null", "", -1)
	return removeNull == ""
}

func getCrdManifests(manifests string) ([]string, string, error) {
	// parse runtime.Objects from the input yaml
	objects, err := parseUnstructured(manifests)
	if err != nil {
		return nil, "", err
	}

	var crdNames, crdManifests []string

	for _, object := range objects {
		// objects parsed by UnstructuredJSONScheme can only be of
		// type *unstructured.Unstructured or *unstructured.UnstructuredList
		if unstructuredObj, ok := object.obj.(*unstructured.Unstructured); ok {
			if gvk := unstructuredObj.GroupVersionKind(); gvk.Kind == "CustomResourceDefinition" && gvk.Group == "apiextensions.k8s.io" {
				crdNames = append(crdNames, unstructuredObj.GetName())
				crdManifests = append(crdManifests, object.yaml)
			}
		}
	}

	// re-join the objects into a single manifest
	return crdNames, strings.Join(crdManifests, yamlJoiner), nil
}

func waitForCrdsToBeRegistered(ctx context.Context, crds []string) error {
	apiExts := helpers.MustApiExtsClient()
	logger := contextutils.LoggerFrom(ctx)
	for _, crdName := range crds {
		logger.Debugw("waiting for crd to be registered", zap.String("crd", crdName))
		if err := kubeutils.WaitForCrdActive(ctx, apiExts, crdName); err != nil {
			return eris.Wrapf(err, "waiting for crd %v to become registered", crdName)
		}
	}

	return nil
}
