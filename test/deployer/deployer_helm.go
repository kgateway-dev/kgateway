package deployer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwxv1a1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"
	"sigs.k8s.io/yaml"

	internaldeployer "github.com/kgateway-dev/kgateway/v2/internal/kgateway/deployer"
	pkgdeployer "github.com/kgateway-dev/kgateway/v2/pkg/deployer"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envutils"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

type HelmTestCase struct {
	Name   string
	Inputs *pkgdeployer.Inputs
	// InputFile is just the name of the manifest omitting the file extension suffix
	InputFile string
}

type DeployerTester struct {
	ControllerName    string
	AgwControllerName string
	ClassName         string
	AgwClassName      string
	WaypointClassName string
}

// ExtractCommonObjs will return a collection containing only objects necessary for collections.CommonCollections,
// so we don't add unknown objects to avoid logging from krttest package re: objects not consumed
func ExtractCommonObjs(t *testing.T, objs []client.Object) ([]client.Object, *gwv1.Gateway) {
	var commonObjs []client.Object
	var gtw *gwv1.Gateway
	for i := range objs {
		switch obj := objs[i].(type) {
		case *gwv1.Gateway:
			if gtw != nil {
				t.Log("found extraneous Gateway in input manifests", client.ObjectKeyFromObject(obj).String())
				t.Fail()
			}
			gtw = obj
			commonObjs = append(commonObjs, gtw)
		case *gwv1.GatewayClass:
			commonObjs = append(commonObjs, obj)
		case *gwxv1a1.XListenerSet:
			commonObjs = append(commonObjs, obj)
		}
	}
	return commonObjs, gtw
}

func (dt DeployerTester) RunHelmChartTest(
	t *testing.T,
	tt HelmTestCase,
	scheme *runtime.Scheme,
	dir string,
	extraParamsFunc func(cli client.Client, inputs *pkgdeployer.Inputs) []pkgdeployer.ExtraGatewayParameters,
) {
	filePath := filepath.Join(dir, "testdata/", tt.InputFile)
	inputFile := filePath + ".yaml"
	outputFile := filePath + "-out.yaml"

	objs, err := testutils.LoadFromFiles(inputFile, scheme, nil)
	assert.NoError(t, err, "error loading files from input file")

	commonObjs, gtw := ExtractCommonObjs(t, objs)
	if gtw == nil {
		t.Log("No Gateway found in test files, failing...")
		t.FailNow()
	}
	commonCols := NewCommonCols(t, commonObjs...)
	inputs := DefaultDeployerInputs(dt, commonCols)
	if tt.Inputs != nil {
		inputs = tt.Inputs
	}
	fakeClient := NewFakeClientWithObjsWithScheme(scheme, objs...)

	chart, err := internaldeployer.LoadGatewayChart()
	assert.NoError(t, err, "error loading chart")

	gwParams := internaldeployer.NewGatewayParameters(
		fakeClient,
		inputs,
	)
	if extraParamsFunc != nil {
		gwParams.WithExtraGatewayParameters(extraParamsFunc(fakeClient, inputs)...)
	}
	deployer := pkgdeployer.NewDeployer(
		dt.ControllerName,
		dt.AgwControllerName,
		dt.AgwClassName,
		fakeClient,
		chart,
		gwParams,
		internaldeployer.GatewayReleaseNameAndNamespace,
	)

	ctx := context.TODO()
	vals, err := gwParams.GetValues(ctx, gtw)
	assert.NoError(t, err, "error getting values for GwParams")

	got, err := deployer.RenderManifest(gtw.Namespace, gtw.Name, vals)
	assert.NoError(t, err, "error rendering helm manifest")

	if envutils.IsEnvTruthy("REFRESH_GOLDEN") {
		t.Log("REFRESH_GOLDEN is set, writing output file", outputFile)
		err = os.WriteFile(outputFile, got, 0o644) //nolint:gosec // G306: Golden test file can be readable
		assert.NoError(t, err, "error writing output file")
		t.FailNow()
	}
	data, err := os.ReadFile(outputFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			t.Log("outputFile does not exist, writing output file", outputFile)
			err = os.WriteFile(outputFile, got, 0o644) //nolint:gosec // G306: Golden test file can be readable
			assert.NoError(t, err, "error writing output file")
			t.FailNow()
		} else {
			t.Log("could not read outputFile", outputFile, err)
			t.FailNow()
		}
	}

	// Validate the output YAML
	validateYAML(t, outputFile, data)

	diff := cmp.Diff(data, got)
	assert.Empty(t, diff, diff, tt)
}

func DefaultDeployerInputs(dt DeployerTester, commonCols *collections.CommonCollections) *pkgdeployer.Inputs {
	return &pkgdeployer.Inputs{
		Dev:               false,
		CommonCollections: commonCols,
		ControlPlane: pkgdeployer.ControlPlaneInfo{
			XdsHost:    "xds.cluster.local",
			XdsPort:    9977,
			AgwXdsPort: 9978,
		},
		ImageInfo: &pkgdeployer.ImageInfo{
			Registry: "ghcr.io",
			Tag:      "v2.1.0-dev",
		},
		GatewayClassName:         dt.ClassName,
		WaypointGatewayClassName: dt.WaypointClassName,
		AgentgatewayClassName:    dt.AgwClassName,
	}
}

// validateYAML checks that the YAML file is valid:
// 1. No lines should end with ': ' (colon-space)
// 2. Each YAML document should be unmarshalable
func validateYAML(t *testing.T, filename string, data []byte) {
	t.Helper()

	// Check for lines ending with ': '
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		if strings.HasSuffix(line, ": ") {
			t.Errorf("helm chart produced invalid yaml: line %d in %s ends with ': ' (colon-space): %q", i+1, filename, line)
		}
	}

	// Split into YAML documents and unmarshal each one
	documents := strings.Split(string(data), "\n---\n")
	for i, doc := range documents {
		doc = strings.TrimSpace(doc)
		if doc == "" || doc == "---" {
			continue
		}
		var obj interface{}
		if err := yaml.Unmarshal([]byte(doc), &obj); err != nil {
			t.Errorf("helm chart produced invalid yaml: failed to unmarshal document %d in %s: %v", i+1, filename, err)
		}
	}
}
