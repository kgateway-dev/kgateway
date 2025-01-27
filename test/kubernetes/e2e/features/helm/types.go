package helm

import (
	"path/filepath"

	"github.com/solo-io/gloo/test/kubernetes/e2e/tests/base"
	"github.com/solo-io/skv2/codegen/util"
)

var (
	productionRecommendationsSetup = filepath.Join(util.MustGetThisDir(), "testdata/manifests", "production-recommendations.yaml")
	configMapChangeSetup           = filepath.Join(util.MustGetThisDir(), "testdata/manifests", "config-map-change.yaml")

	helmTestCases = map[string]*base.TestCase{
		"TestProductionRecommendations": {
			SimpleTestCase: base.SimpleTestCase{
				UpgradeValues: productionRecommendationsSetup,
			},
		},
	}

	enterpriseCRDCategory = "solo-io"
	CommonCRDCategory     = "gloo-gateway"

	enterpriseCRDs = []string{
		"authconfigs.enterprise.gloo.solo.io",
		"ratelimitconfigs.ratelimit.solo.io",
		"graphqlapis.graphql.gloo.solo.io",
	}
)
