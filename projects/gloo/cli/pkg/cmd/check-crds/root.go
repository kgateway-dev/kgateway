package check_crds

import (
	"bytes"
	"context"
	"regexp"
	"strings"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options/contextoptions"

	"github.com/ghodss/yaml"
	"github.com/rotisserie/eris"
	cliutil "github.com/solo-io/gloo/pkg/cliutil/install"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/install"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/istio"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/printers"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/spf13/cobra"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

var (
	printer printers.P
)

const (
	betaMessage   = "NOTE: this feature is still in beta and may return false positives, if this is suspected use the --show-yaml flag and inspect CRDs manually\n "
	helmChartRepo = "https://storage.googleapis.com/solo-public-helm/charts/"
)

func RootCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   constants.CHECK_CRD_COMMAND.Use,
		Short: constants.CHECK_CRD_COMMAND.Short,
		Long:  "usage: glooctl check-crds [-o FORMAT]",
		RunE: func(cmd *cobra.Command, args []string) error {
			printer = printers.P{OutputType: opts.Top.Output}
			printer.CheckResult = printer.NewCheckResult()
			err := CheckCRDS(opts)
			printer.AppendMessage(betaMessage)
			return err
		},
	}
	pflags := cmd.PersistentFlags()
	flagutils.AddVersionFlag(pflags, &opts.CheckCRD.Version)
	flagutils.AddLocalChartFlag(pflags, &opts.CheckCRD.LocalChart)
	flagutils.AddShowYamlFlag(pflags, &opts.CheckCRD.ShowYaml)
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}

func CheckCRDS(opts *options.Options) error {
	ctx, cancel := context.WithCancel(opts.Top.Ctx)
	defer cancel()

	version, err := getDeployedVersion(ctx, opts)
	if err != nil {
		return err
	}
	chartPath := helmChartRepo + "gloo-" + version + ".tgz"
	if opts.CheckCRD.LocalChart != "" {
		chartPath = opts.CheckCRD.LocalChart
	}
	expectedCRDs, err := getCRDsFromHelm(chartPath)
	if err != nil {
		return eris.Wrapf(err, "Error getting CRDs from %s", chartPath)
	}
	clusterCRDs, err := getCRDsInCluster(ctx)
	if err != nil {
		return eris.Wrapf(err, "Error getting CRDs in current cluster")
	}

	crdsInClusterMap := map[string]apiextv1.CustomResourceDefinition{}
	for _, crd := range clusterCRDs {
		crdsInClusterMap[crd.Name] = crd
	}

	diffs := []string{}
	for _, crd := range expectedCRDs {
		expectedCrdBytes, _ := yaml.Marshal(crd.Spec)
		if clusterCrd, ok := crdsInClusterMap[crd.Name]; !ok {
			diffs = append(diffs, crd.Name)
		} else {
			clusterCrdBytes, _ := yaml.Marshal(clusterCrd.Spec)
			if string(expectedCrdBytes) != string(clusterCrdBytes) {
				if opts.CheckCRD.ShowYaml {
					diffs = append(diffs, "Yaml for deployed "+clusterCrd.Name+" :", string(clusterCrdBytes))
					diffs = append(diffs, "Yaml for expected "+crd.Name+":", string(expectedCrdBytes))
				} else {
					diffs = append(diffs, crd.Name)
				}
			}
		}
	}
	if len(diffs) != 0 {
		crdsWithDiffs := strings.Join(diffs, "\n")
		errString := strings.Join([]string{"Diffs detected on the following CRDs:", crdsWithDiffs}, "\n\n")
		printer.AppendMessage(errString)
		return eris.New("One or more CRDs are out of date, see https://docs.solo.io/gloo-edge/latest/operations/upgrading/upgrade_steps/#step-3-apply-minor-version-specific-changes for more details")
	}
	printer.AppendMessage("All CRDs are up to date")
	return nil
}

func getDeployedVersion(ctx context.Context, opts *options.Options) (string, error) {
	deployedVersion, err := istio.GetGlooVersionWithoutV(ctx, opts.Metadata.GetNamespace())
	if err != nil {
		return "", eris.Wrapf(err, "Cannot get current version of gloo")
	}
	if opts.CheckCRD.Version != "" {
		deployedVersion = opts.CheckCRD.Version
	}
	return deployedVersion, nil
}

// preprocessCRD sets fields that would be set on the crd when deployed to a cluster but arent currently set
// crd.Spec.Names.Singular defaults to lowercased crd.Spec.Names.Kind if unset
// crd.Spec.Conversion.Strategy defaults to apiextv1.NoneConverter if unset
func preprocessCRD(crd *apiextv1.CustomResourceDefinition) {
	if crd.Spec.Names.Singular != "" {
		crd.Spec.Names.Singular = strings.ToLower(crd.Spec.Names.Kind)
	}
	crd.Spec.Names.Singular = ""
	if crd.Spec.Conversion == nil {
		crd.Spec.Conversion = &apiextv1.CustomResourceConversion{
			Strategy: apiextv1.NoneConverter,
		}
	}
	crd.Spec.Conversion = &apiextv1.CustomResourceConversion{}
}

// getCRDsInCluster gets all custom resources currently in the local cluster
func getCRDsInCluster(ctx context.Context) ([]apiextv1.CustomResourceDefinition, error) {
	crds := []apiextv1.CustomResourceDefinition{}

	kubectlArgs := []string{"get", "crd"}

	kubecontext := contextoptions.KubecontextFrom(ctx)
	if kubecontext != "" {
		kubectlArgs = append(kubectlArgs, "--context", kubecontext)
	}
	out, err := cliutil.KubectlOut(nil, kubectlArgs...)
	if err != nil {
		return nil, err
	}
	for _, crdName := range regexp.MustCompile(`(\S+)(.solo.io)`).FindAllString(string(out), -1) {
		crd := apiextv1.CustomResourceDefinition{}
		out, err := cliutil.KubectlOut(nil, "get", "crd", crdName, "-o", "yaml")
		if err != nil {
			return nil, err
		}
		err = yaml.Unmarshal(bytes.TrimSpace(out), &crd)
		if err != nil {
			return nil, eris.Wrapf(err, "Error unmarshalling clusters CRD:")
		}
		preprocessCRD(&crd)
		crds = append(crds, crd)
	}
	return crds, nil
}

// getCRDsForVersion gets all custom resources for a given helm chart at uri via helm client
func getCRDsFromHelm(uri string) ([]apiextv1.CustomResourceDefinition, error) {
	crds := []apiextv1.CustomResourceDefinition{}
	helmClient := install.DefaultHelmClient()
	chartObj, err := helmClient.DownloadChart(uri)
	if err != nil {
		return nil, err
	}
	for _, crdObject := range chartObj.CRDObjects() {
		crd := apiextv1.CustomResourceDefinition{}

		err = yaml.Unmarshal(bytes.TrimSpace(crdObject.File.Data), &crd)
		if err != nil {
			return nil, eris.Wrapf(err, "Error unmarshalling expected CRD:")
		}
		preprocessCRD(&crd)
		crds = append(crds, crd)
	}
	return crds, nil
}
