//go:build e2e

package directresponse

import (
	"path/filepath"

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
)
