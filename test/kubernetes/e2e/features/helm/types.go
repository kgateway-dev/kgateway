//go:build ignore

package helm

import (
	"path/filepath"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/tests/base"
)

var (
	productionRecommendationsSetup = filepath.Join(fsutils.MustGetThisDir(), "testdata/manifests", "production-recommendations.yaml")
	configMapChangeSetup           = filepath.Join(fsutils.MustGetThisDir(), "testdata/manifests", "config-map-change.yaml")

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
