package helm

import (
	"path/filepath"

	"github.com/solo-io/gloo/test/kubernetes/e2e/tests/base"
	"github.com/solo-io/skv2/codegen/util"
)

var (
	productionRecommendationsSetup = filepath.Join(util.MustGetThisDir(), "testdata/manifests", "production-recommendations.yaml")
	protoDescriptorBinSetup        = filepath.Join(util.MustGetThisDir(), "testdata/manifests", "proto-descriptor-bin.yaml")
	configMapChangeSetup           = filepath.Join(util.MustGetThisDir(), "testdata/manifests", "config-map-change.yaml")

	helmTestCases = map[string]*base.TestCase{
		"TestProductionRecommendations": {
			SimpleTestCase: base.SimpleTestCase{
				UpgradeValues: productionRecommendationsSetup,
			},
		},
		"TestProtoDescriptorBin": {
			SimpleTestCase: base.SimpleTestCase{
				UpgradeValues: protoDescriptorBinSetup,
			},
		},
	}
)
