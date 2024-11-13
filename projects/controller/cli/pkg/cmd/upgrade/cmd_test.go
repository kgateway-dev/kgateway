package upgrade

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Cmd", func() {

	var (
		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		ctx = context.WithValue(context.Background(), "githubURL", ts.URL+"/")
		ctx, cancel = context.WithCancel(ctx)
	})

	AfterEach(func() {
		cancel()
	})

	DescribeTable("release types",
		func(releaseTag, expectedRelease, expectedErrString string) {
			rel, err := getReleaseWithAsset(ctx, ts.Client(), releaseTag, glooctlBinaryName)
			if err != nil {
				Expect(err.Error()).To(ContainSubstring(expectedErrString))
				Expect(expectedErrString).ShouldNot(BeEmpty())
				Expect(expectedRelease).To(BeEmpty())
			} else {
				Expect(*rel.Name).To(Equal(expectedRelease))
			}

		},
		Entry("experimental gets largest semver", "experimental", "v1.11.0-beta11", ""),
		Entry("latest gets latest stable", "latest", "v1.10.18", ""),
		Entry("v1.10.x gets latest stable", "v1.10.x", "v1.10.18", ""),
		Entry("v1.9.x gets in minor", "v1.9.x", "v1.9.7", ""),
		Entry("v1.2.x is not found", "v1.2.x", "", errorNotFoundString),
		Entry("v2.2.x is not found", "v2.2.x", "", errorNotFoundString),
	)

	It("DOES NOT handle new major versions", func() {
		ctx = context.WithValue(ctx, "githubURL", ts.URL+"/newMajor/")

		// relExp, _ := getReleaseWithAsset(ctx, ts.Client(), "experimental", glooctlBinaryName)
		// Expect(*relExp.Name).To(Equal("v2.0.0-beta1"))
		relLatest, _ := getReleaseWithAsset(ctx, ts.Client(), "latest", glooctlBinaryName)
		Expect(*relLatest.Name).To(Equal("v1.12.5"))
	})
})
