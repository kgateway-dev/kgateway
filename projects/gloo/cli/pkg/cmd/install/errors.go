package install

import "github.com/solo-io/go-utils/errors"

var (
	NoReleaseForCRDs        = errors.New("Could not find a release from which to pull CRDs")
	MultipleReleasesForCRDs = errors.New("Found multiple releases from which to pull CRDs")
)
