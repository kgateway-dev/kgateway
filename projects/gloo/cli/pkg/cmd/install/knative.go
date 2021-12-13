package install

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/solo-io/go-utils/versionutils"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/k8s-utils/kubeutils"
	"github.com/solo-io/solo-kit/test/setup"
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

	knativeIngressProviderLabel = "networking.knative.dev/ingress-provider"
	knativeIngressProviderIstio = "istio"

	yamlJoiner = "\n---\n"
)

func waitKnativeApiserviceReady() error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	for {
		stdout, err := setup.KubectlOut("get", "apiservice", "-ojsonpath='{.items[*].status.conditions[*].status}'")
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
		Use:    "knative",
		Short:  "install Knative with Gloo on Kubernetes",
		Long:   "requires kubectl to be installed",
		PreRun: setVerboseMode(opts),
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

				if err := NewInstaller(DefaultHelmClient()).Install(&InstallerConfig{
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
	_, _ = fmt.Fprintf(os.Stderr, "Installing knative components %#v...\n", knativeOpts)

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

	// TODO (sam-heilbron) - In later versions of Knative, the CRD manifests are defined separately.
	// We could improve this logic to get CRD and CR manifests separately
	knativeCrdNames, knativeCrdManifests, err := getCrdManifests(manifests)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Knative CRD Names %v \n", knativeCrdNames)

	// install crds first
	fmt.Fprintln(os.Stderr, "installing Knative CRDs...")
	if err := install.KubectlApply([]byte(knativeCrdManifests), "-v"); err != nil {
		return eris.Wrapf(err, "installing knative crds with kubectl apply")
	}

	if err := waitForCrdsToBeRegistered(opts.Top.Ctx, knativeCrdNames); err != nil {
		return eris.Wrapf(err, "waiting for knative CRDs to be registered")
	}

	fmt.Fprintln(os.Stderr, "installing Knative...")

	if err := install.KubectlApply([]byte(manifests), "-v"); err != nil {
		fmt.Fprintln(os.Stderr, "Kubectl apply failed. retrying...")
		// may need to retry the apply once in order to work around webhook race issue
		// https://github.com/knative/serving/issues/6353
		// https://knative.slack.com/archives/CA9RHBGJX/p1577458311043200
		if err2 := install.KubectlApply([]byte(manifests), "-v"); err2 != nil {
			return eris.Wrapf(err, "installing knative resources failed with retried kubectl apply: %v", err2)
		}
	}
	fmt.Fprintln(os.Stderr, "labelling knative-serving namespace...")

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
		kc = helpers.MustKubeClient()
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

// We rely on the Knative manifests, persisted in the Github repository to install appropriate resources
// In later Knative versions, the naming and location of these manifests has changed.
// ManifestPathSource was added to abstract these details from the glooctl installer
// It allows us to validate the logic of each of the path sources, and update them independently
// without modifying the install code
type ManifestPathSource interface {
	GetPaths() []string
}

var _ ManifestPathSource = new(EmptyManifestSource)
var _ ManifestPathSource = new(ServingManifestSource)
var _ ManifestPathSource = new(EventingManifestSource)

type ServingManifestSource struct {
	version           *versionutils.Version
	installMonitoring bool
}

const servingTemplate = "https://github.com/knative/serving/releases/download/v%v/serving.yaml"
const servingCoreTemplate = "https://github.com/knative/serving/releases/download/v%v/serving-core.yaml"
const monitoringTemplate = "https://github.com/knative/serving/releases/download/v%v/monitoring.yaml"

func (s *ServingManifestSource) GetPaths() []string {
	// v0.15.1 shipped serving.yaml for the last time: https://github.com/knative/serving/releases/tag/v0.15.1
	// In later versions, this is split into separate artifacts
	servingAndMonitoringVersion := versionutils.Version{
		Major:        0,
		Minor:        15,
		Patch:        1,
	}
	if !s.version.MustIsGreaterThan(servingAndMonitoringVersion) {
		return s.getPre16Paths()
	}

	// v0.19.0 removed the monitoring bundle: https://github.com/knative/serving/releases/tag/v0.19.0
	servingNoMonitoringVersion :=  versionutils.Version{
		Major:        0,
		Minor:        19,
		Patch:        0,
	}
	if !s.version.MustIsGreaterThan(servingNoMonitoringVersion) {
		return s.getPre19Paths()
	}

	return s.getLatestPaths()
}

func (s *ServingManifestSource) getPre16Paths() []string {
	paths := []string{
		fmt.Sprintf(servingTemplate, s.version),
	}

	if s.installMonitoring {
		paths = append(paths, fmt.Sprintf(monitoringTemplate, s.version))
	}
	return paths
}

func (s *ServingManifestSource) getPre19Paths() []string {
	paths := []string{fmt.Sprintf(servingCoreTemplate, s.version)}

	if s.installMonitoring {
		paths = append(paths, fmt.Sprintf(monitoringTemplate, s.version))
	}

	return paths
}

func (s *ServingManifestSource) getLatestPaths() []string {
	return []string{fmt.Sprintf(servingCoreTemplate, s.version)}
}

type EventingManifestSource struct {
	version *versionutils.Version
}

func (e *EventingManifestSource) GetPaths() []string {
	template := "https://github.com/knative/eventing/releases/download/v%v/release.yaml"

	// In 0.12.0, the knative/eventing components bundle was renamed
	// https://github.com/knative/eventing/releases/tag/v0.12.0
	renamedManifestVersion := versionutils.Version{
		Major: 0,
		Minor: 12,
		Patch: 0,
	}
	if e.version.MustIsGreaterThan(renamedManifestVersion) {
		template = "https://github.com/knative/eventing/releases/download/v%v/eventing.yaml"
	}

	return []string{fmt.Sprintf(template, e.version)}
}

type EmptyManifestSource struct {
}

func (m *EmptyManifestSource) GetPaths() []string {
	return []string{}
}

func RenderKnativeManifests(opts options.Knative) (string, error) {
	// choose a path source for the manifest URLs based on the serving and eventing knative versions
	servingManifestPathSource, eventingManifestPathSource, err := getKnativeManifestPathSources(opts)
	if err != nil {
		return "", err
	}

	// aggregate all manifest URLs
	var manifestPaths = append(
		servingManifestPathSource.GetPaths(),
		eventingManifestPathSource.GetPaths()...)

	// aggregate all manifests
	var knativeManifests []string
	for _, manifestPath := range manifestPaths {
		manifest, err := getManifestForInstallation(manifestPath)
		if err != nil {
			return "", err
		}

		knativeManifests = append(knativeManifests, manifest)
	}

	return strings.Join(knativeManifests, yamlJoiner), nil
}

func getKnativeManifestPathSources(opts options.Knative) (ManifestPathSource, ManifestPathSource, error) {
	var servingManifestSource, eventingManifestSource ManifestPathSource

	servingVersion, err := versionutils.ParseVersion(opts.InstallKnativeVersion)
	if err != nil {
		return nil, nil, err
	}

	servingManifestSource = &ServingManifestSource{
		version: servingVersion,
		// In later versions of Knative, monitoring is removed
		// I opted to keep the flag in glooctl and just make it a no-op
		installMonitoring: opts.InstallKnativeMonitoring,
	}

	eventingManifestSource = &EmptyManifestSource{}
	if opts.InstallKnativeEventing {
		eventingVersion, err := versionutils.ParseVersion(opts.InstallKnativeEventingVersion)
		if err != nil {
			return nil, nil, err
		}

		eventingManifestSource = &EventingManifestSource{
			version: eventingVersion,
		}
	}
	return servingManifestSource, eventingManifestSource, nil
}

func getManifestForInstallation(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != 200 {
		return "", eris.Errorf("returned non-200 status code: %v %v", resp.StatusCode, resp.Status)
	}
	raw, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

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
			panic(fmt.Sprintf("unknown object type %T parsed from yaml: \n%v ", object.obj, object.yaml))
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
