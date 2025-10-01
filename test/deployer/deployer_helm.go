package deployer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwxv1a1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"

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

	ctx := context.TODO()
	// contains objects necessary for commonCollections, don't add extra stuff here
	// to avoid logging from krttest package re: objects not consumed
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
	if gtw == nil {
		t.Log("No Gateway found in test files, failing...")
		t.FailNow()
	}
	commonCols := NewCommonCols(t, commonObjs...)
	inputs := dt.defaultDeployerInputs(commonCols)
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

	diff := cmp.Diff(data, got)
	assert.Empty(t, diff, diff)
}

func (dt DeployerTester) defaultDeployerInputs(commonCols *collections.CommonCollections) *pkgdeployer.Inputs {
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
