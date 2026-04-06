//go:build e2e

package directresponse

import (
	"path/filepath"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

var (
	setupManifest                  = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")
	basicDirectResponseManifests   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "basic-direct-response.yaml")
	basicDelegationManifests       = filepath.Join(fsutils.MustGetThisDir(), "testdata", "basic-delegation-direct-response.yaml")
	bodyFormatTextManifests        = filepath.Join(fsutils.MustGetThisDir(), "testdata", "body-format-text.yaml")
	bodyFormatJSONManifests        = filepath.Join(fsutils.MustGetThisDir(), "testdata", "body-format-json.yaml")
	bodyFormatContentTypeManifests = filepath.Join(fsutils.MustGetThisDir(), "testdata", "body-format-content-type.yaml")
	// TODO: Re-enable this test once the issue with conflicting filters is resolved or the expected behavior is clarified.
	// invalidDelegationConflictingFiltersManifests = filepath.Join(fsutils.MustGetThisDir(), "testdata", "invalid-delegation-conflicting-filters.yaml")
	// invalidMultipleRouteActionsManifests         = filepath.Join(fsutils.MustGetThisDir(), "testdata", "invalid-multiple-route-actions.yaml")

	httpbinMeta = metav1.ObjectMeta{
		Name:      "httpbin",
		Namespace: "httpbin",
	}
	httpbinDeployment = &appsv1.Deployment{ObjectMeta: httpbinMeta}
)
