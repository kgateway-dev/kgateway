package install

import "github.com/rotisserie/eris"

var (
	GlooAlreadyInstalled = func(namespace string) error {
		return eris.Errorf("Gloo has already been installed to namespace %s", namespace)
	}
	NoReleaseForCRDs        = eris.New("Could not find a release from which to pull CRDs")
	MultipleReleasesForCRDs = eris.New("Found multiple releases from which to pull CRDs")
)
