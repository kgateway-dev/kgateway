package helm_settings

import (
	"github.com/solo-io/gloo/test/kubernetes/e2e/tests/base"
)

var (
	testCases = map[string]*base.TestCase{
		"TestProductionRecommendations": {
			SimpleTestCase: base.SimpleTestCase{
				UpgradeValues: productionRecommendationsSetup,
			},
		},
	}
)
