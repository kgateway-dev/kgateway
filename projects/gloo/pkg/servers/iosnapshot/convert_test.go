package iosnapshot

import (
	. "github.com/onsi/ginkgo/v2"
	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/test/gomega/assertions"
)

var _ = Describe("Convert", func() {

	It("will fail if the ApiSnapshot has a new top level field", func() {
		// This test checks whether the ApiSnapshot has a new top-level field.
		// If the number of fields changes, it should be used as an indication that the
		// `snapshotToKubeResources` function might need to be updated to support the new field.
		assertions.ExpectNumFields(v1snap.ApiSnapshot{}, 16)
	})
})
