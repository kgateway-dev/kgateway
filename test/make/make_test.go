package make_test

import (
	. "github.com/onsi/ginkgo/v2"
)

// Environment Variables which control the value of makefile vars
const (
	PublishContext = "PUBLISH_CONTEXT"
)

// Makefile vars
const (
	HelmBucket          = "HELM_BUCKET"
	QuayExpirationLabel = "QUAY_EXPIRATION_LABEL"
)

// NOTE: These tests are mostly to check that the makefile ifeq directive logic is working as expected.
// It's preferred that if Makefile logic gets anymore complicated, to direct the logic to go code rather than
// increase the complexity of the makefile.
var _ = Describe("Make", func() {
	Context("PUBLISH_CONTEXT", func() {
		It("Correctly sets variables when PUBLISH_CONTEXT is unset", func() {
			ExpectMakeVarsWithEnvVars([]*EnvVar{
				{PublishContext, ""},
			}, []*MakeVar{
				{HelmBucket, "gs://solo-public-tagged-helm"},
				{QuayExpirationLabel, "--label quay.expires-after=3w"},
			})
		})

		It("Correctly sets variables when PUBLISH_CONTEXT is RELEASE", func() {
			ExpectMakeVarsWithEnvVars([]*EnvVar{
				{PublishContext, "RELEASE"},
			}, []*MakeVar{
				{HelmBucket, "gs://solo-public-helm"},
				{QuayExpirationLabel, ""},
			})
		})

		It("Correctly sets variables when PUBLISH_CONTEXT is PULL_REQUEST", func() {
			ExpectMakeVarsWithEnvVars([]*EnvVar{
				{PublishContext, "PULL_REQUEST"},
			}, []*MakeVar{
				{HelmBucket, "gs://solo-public-tagged-helm"},
				{QuayExpirationLabel, "--label quay.expires-after=3w"},
			})
		})
	})
})
